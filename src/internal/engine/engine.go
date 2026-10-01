package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Darida/openrouterclient/src/internal/catalog"
	"github.com/Darida/openrouterclient/src/internal/chat"
	"github.com/Darida/openrouterclient/src/internal/cost"
	"github.com/Darida/openrouterclient/src/internal/hedge"
	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/internal/quality"
	"github.com/Darida/openrouterclient/src/internal/replyfile"
	"github.com/Darida/openrouterclient/src/internal/review"
	"github.com/Darida/openrouterclient/src/internal/schema"
	"github.com/Darida/openrouterclient/src/model"
)

const nearestExclusionShown = 3

// Generation and review outcomes are kept apart in history, so a model's
// record as a reviewer weighs only half when picking a generator.
const (
	generatorTagSuffix = "-generate"
	reviewerTagSuffix  = "-review"
)

type Engine struct {
	settings Settings
	apiKey   string
	// Every history entry's tag starts with it.
	tag string
	// How long an attempt may stay pending before the next one starts, and
	// how long a success may take before it counts against its model.
	timeout time.Duration
	http    *http.Client
	catalog *catalog.Catalog
	history history.History
	replies replyfile.Saver
	logger  *slog.Logger
	// Stragglers still settling after their race was won.
	background sync.WaitGroup
	// What went wrong while recording stragglers, returned by Close.
	backgroundMu  sync.Mutex
	backgroundErr error
}

func New(settings Settings, apiKey, tag string, timeout time.Duration, store history.History, replies replyfile.Saver, logger *slog.Logger) *Engine {
	httpClient := &http.Client{}
	return &Engine{
		settings: settings,
		apiKey:   apiKey,
		tag:      tag,
		timeout:  timeout,
		http:     httpClient,
		catalog:  &catalog.Catalog{URL: settings.CatalogURL, HTTP: httpClient, Replies: replies},
		history:  store,
		replies:  replies,
		logger:   logger,
	}
}

// GenerateReviewed generates, reviews, and corrects until a result meets
// TargetQuality or the request's corrections run out. Every correction
// resends the original prompt, the latest reply, and its review violations.
// Like every public Engine call, its errors are ctx.Err(),
// *model.AttemptsExhaustedError, and *model.UnexpectedError.
func (e *Engine) GenerateReviewed(ctx context.Context, req model.GenerateReviewedRequest) (model.ReviewedText, error) {
	text, exhausted, err := e.generateReviewed(ctx, req)
	return text, publicError(ctx, exhausted, err)
}

// Generate returns the first schema-valid output. With no review to fail it,
// it is recorded as high.
func (e *Engine) Generate(ctx context.Context, req model.GenerateRequest) (model.GeneratedText, error) {
	text, exhausted, err := e.generate(ctx, req)
	return text, publicError(ctx, exhausted, err)
}

// Review rates caller-supplied content against the criteria. There is no
// generation in history for its verdict to rate, so only the reviewer's own
// outcome is recorded.
func (e *Engine) Review(ctx context.Context, req model.ReviewRequest) (model.Review, error) {
	rev, exhausted, err := e.review(ctx, req)
	return rev, publicError(ctx, exhausted, err)
}

func (e *Engine) Rate(ctx context.Context, generationID string, q model.Quality, reason string) error {
	return e.history.RecordManual(generationID, q, reason, time.Now())
}

// CandidateModels lists the models a first Generate attempt for req would
// pick from, applying every filter Generate does, without sending a chat request.
func (e *Engine) CandidateModels(req model.GenerateRequest) ([]string, error) {
	if err := validateGenerate(req); err != nil {
		return nil, err
	}
	return e.candidateModels(req.Models, e.tag+generatorTagSuffix, promptTokens([]chat.Message{chat.UserMessage(req.Prompt)}, req.OutputSchema), maxOutputTokens(req.MaxOutputTokens))
}

// Close blocks until every straggler from an already-won race is settled and
// recorded, and returns what went wrong while recording them.
func (e *Engine) Close() error {
	e.background.Wait()
	e.backgroundMu.Lock()
	defer e.backgroundMu.Unlock()
	return e.backgroundErr
}

func (e *Engine) generateReviewed(ctx context.Context, req model.GenerateReviewedRequest) (model.ReviewedText, *model.AttemptsExhaustedError, error) {
	if err := validateGenerateReviewed(req); err != nil {
		return model.ReviewedText{}, nil, err
	}
	validate, err := outputValidator(req.OutputSchema)
	if err != nil {
		return model.ReviewedText{}, nil, err
	}
	var failures []model.FailedAttempt
	messages := []chat.Message{chat.UserMessage(req.Prompt)}

	maxRounds := 1 + req.MaxCorrections
	for round := 1; round <= maxRounds; round++ {
		result, roundFailures, ok, err := e.runRound(ctx, req, validate, messages, round, maxRounds)
		failures = append(failures, roundFailures...)
		if err != nil {
			return model.ReviewedText{}, nil, err
		}
		if err := ctx.Err(); err != nil {
			return model.ReviewedText{}, nil, err
		}
		if !ok {
			return model.ReviewedText{}, &model.AttemptsExhaustedError{Attempts: failures}, nil
		}
		below, err := quality.Below(result.quality, req.TargetQuality)
		if err != nil {
			return model.ReviewedText{}, nil, err
		}
		if !below {
			return result.reviewedText(), nil, nil
		}
		rejection, err := e.rejection(result)
		if err != nil {
			return model.ReviewedText{}, nil, err
		}
		failures = append(failures, rejection)
		messages = []chat.Message{chat.UserMessage(req.Prompt), chat.AssistantMessage(string(result.gen.content)), chat.UserMessage(review.CorrectionPrompt(result.verdict.Violations))}
	}
	return model.ReviewedText{}, &model.AttemptsExhaustedError{Attempts: failures}, nil
}

func (e *Engine) generate(ctx context.Context, req model.GenerateRequest) (model.GeneratedText, *model.AttemptsExhaustedError, error) {
	if err := validateGenerate(req); err != nil {
		return model.GeneratedText{}, nil, err
	}
	validate, err := outputValidator(req.OutputSchema)
	if err != nil {
		return model.GeneratedText{}, nil, err
	}
	e.logger.Info("openrouter: generating without review")
	label := e.generatorLabel(req.TargetQuality)
	gen, failures, ok, err := e.hedge(ctx, race{label: label, models: req.Models, messages: []chat.Message{chat.UserMessage(req.Prompt)}, schema: req.OutputSchema, validate: validate, maxTokens: maxOutputTokens(req.MaxOutputTokens)})
	if err != nil {
		return model.GeneratedText{}, nil, err
	}
	if err := ctx.Err(); err != nil {
		return model.GeneratedText{}, nil, err
	}
	if !ok {
		return model.GeneratedText{}, &model.AttemptsExhaustedError{Attempts: failures}, nil
	}
	if err := e.append(entryFields(gen, label, model.QualityHigh, "no review; assumed high")); err != nil {
		return model.GeneratedText{}, nil, err
	}
	return gen.generatedText(), nil, nil
}

func (e *Engine) review(ctx context.Context, req model.ReviewRequest) (model.Review, *model.AttemptsExhaustedError, error) {
	if err := validateReview(req); err != nil {
		return model.Review{}, nil, err
	}
	e.logger.Info("openrouter: reviewing supplied content")
	rev, failures, ok, err := e.hedge(ctx, e.reviewRace(req.Task, req.Content, req.Criteria, req.Models, req.MaxOutputTokens, ""))
	if err != nil {
		return model.Review{}, nil, err
	}
	if err := ctx.Err(); err != nil {
		return model.Review{}, nil, err
	}
	if !ok {
		return model.Review{}, &model.AttemptsExhaustedError{Attempts: failures}, nil
	}
	verdict, err := review.Parse(rev.content)
	if err != nil {
		return model.Review{}, nil, err
	}
	rated := quality.FromBadScore(verdict.TotalBadScore, req.Criteria.ToleranceThreshold)
	e.logger.Info("openrouter: reviewed", "reviewer", rev.model, "quality", rated, "violations", len(verdict.Violations), "badScore", verdict.TotalBadScore, "threshold", req.Criteria.ToleranceThreshold)
	return model.Review{Verdict: verdict, Quality: rated, Model: rev.model, GenerationID: rev.generationID}, nil, nil
}

// publicError passes exhausted and ctx's own error through and marks any
// other err unexpected.
func publicError(ctx context.Context, exhausted *model.AttemptsExhaustedError, err error) error {
	switch {
	case err != nil && err == ctx.Err():
		return err
	case err != nil:
		return &model.UnexpectedError{Err: err}
	case exhausted != nil:
		return exhausted
	}
	return nil
}

func outputValidator(outputSchema model.JSONSchema) (validator, error) {
	compiled, err := schema.Compile(outputSchema.Name, outputSchema.Schema)
	if err != nil {
		return nil, err
	}
	return func(content json.RawMessage) (error, error) { return compiled.Validate(content), nil }, nil
}

func (e *Engine) rejection(result reviewedRound) (model.FailedAttempt, error) {
	output, err := e.replies.Describe(result.gen.generationID, result.gen.generationID, result.gen.content)
	if err != nil {
		return model.FailedAttempt{}, err
	}
	verdict, err := e.replies.Describe(result.rev.generationID, result.rev.generationID, result.rev.content)
	if err != nil {
		return model.FailedAttempt{}, err
	}
	return result.rejection(output, verdict), nil
}

// runRound generates from messages and reviews the winner. It returns false
// when either the generation or the review had no winner.
func (e *Engine) runRound(ctx context.Context, req model.GenerateReviewedRequest, validate validator, messages []chat.Message, round, maxRounds int) (reviewedRound, []model.FailedAttempt, bool, error) {
	e.logger.Info("openrouter: generating", "round", round, "maxRounds", maxRounds)
	gen, failures, ok, err := e.hedge(ctx, race{label: e.generatorLabel(req.TargetQuality), models: req.GenerationModels, messages: messages, schema: req.OutputSchema, validate: validate, maxTokens: maxOutputTokens(req.MaxOutputTokens)})
	if err != nil || !ok {
		return reviewedRound{}, failures, false, err
	}
	result, reviewFailures, ok, err := e.reviewGeneration(ctx, req, gen, round)
	return result, append(failures, reviewFailures...), ok, err
}

// reviewGeneration rates gen against the criteria and records it with that
// rating, or with none if the review had no winner.
func (e *Engine) reviewGeneration(ctx context.Context, req model.GenerateReviewedRequest, gen attempt, round int) (reviewedRound, []model.FailedAttempt, bool, error) {
	label := e.generatorLabel(req.TargetQuality)
	rev, failures, ok, err := e.hedge(ctx, e.reviewRace(req.Prompt, gen.content, req.Criteria, req.ReviewModels, req.MaxOutputTokens, gen.generationID))
	if err != nil {
		return reviewedRound{}, failures, false, err
	}
	if !ok {
		return reviewedRound{}, failures, false, e.append(entryFields(gen, label, "", "never reviewed"))
	}

	verdict, err := review.Parse(rev.content)
	if err != nil {
		return reviewedRound{}, failures, false, err
	}
	rated := quality.FromBadScore(verdict.TotalBadScore, req.Criteria.ToleranceThreshold)
	if err := e.append(entryFields(gen, label, rated, review.FormatViolations(verdict.Violations))); err != nil {
		return reviewedRound{}, failures, false, err
	}
	e.logger.Info("openrouter: reviewed", "round", round, "model", gen.model, "reviewer", rev.model, "quality", rated, "target", req.TargetQuality, "violations", len(verdict.Violations), "badScore", verdict.TotalBadScore, "threshold", req.Criteria.ToleranceThreshold)
	return reviewedRound{gen: gen, rev: rev, verdict: verdict, quality: rated}, failures, true, nil
}

// reviewRace asks for a verdict on content; reviewed is the generation that
// verdict rates in history, or empty when the content is the caller's own.
func (e *Engine) reviewRace(task string, content json.RawMessage, criteria model.ReviewCriteria, models model.ModelSelection, maxTokens int, reviewed string) race {
	return race{
		label:     raceLabel{role: history.RoleReviewer, tag: e.tag + reviewerTagSuffix, timeout: e.timeout, reviewed: reviewed},
		models:    models,
		messages:  review.Messages(task, content, criteria.Rules),
		schema:    review.Schema(),
		validate:  review.Validate,
		maxTokens: maxOutputTokens(maxTokens),
	}
}

func (e *Engine) generatorLabel(target model.Quality) raceLabel {
	return raceLabel{role: history.RoleGenerator, target: target, tag: e.tag + generatorTagSuffix, timeout: e.timeout}
}

// hedge returns as soon as an attempt wins, recording the rest in the
// background; failures are returned only when nothing won. Each attempt asks
// a model picked at random from the candidates. The generator's winner is
// left unrecorded, because the caller decides its quality. A fatal attempt
// aborts the race and comes back as err.
func (e *Engine) hedge(ctx context.Context, r race) (attempt, []model.FailedAttempt, bool, error) {
	candidates, err := e.candidateModels(r.models, r.label.tag, promptTokens(r.messages, r.schema), r.maxTokens)
	if err != nil {
		return attempt{}, nil, false, err
	}
	timing := hedge.Timing{MaxAttempts: e.settings.MaxAttempts, Stagger: e.timeout, AttemptTimeout: e.timeout + e.settings.GraceAfterTimeout}
	run := hedge.Run(ctx, timing, func(attemptCtx context.Context, num int) (attempt, hedge.Verdict) {
		modelID := candidates[rand.IntN(len(candidates))]
		payload, err := chat.BuildPayload(r.messages, r.schema, modelID, r.maxTokens)
		if err != nil {
			return fatal(modelID, "", err), hedge.Aborted
		}
		return e.runAttempt(attemptCtx, modelID, payload, r.validate)
	})
	winner, ok := run.Winner()
	if !ok {
		failures, err := e.recordAttempts(run.Settled(), r.label)
		return attempt{}, failures, false, err
	}
	e.background.Add(1)
	go func() {
		defer e.background.Done()
		if _, err := e.recordAttempts(run.Settled(), r.label); err != nil {
			e.logger.Error("openrouter: recording a won race's stragglers failed", "err", err)
			e.backgroundMu.Lock()
			defer e.backgroundMu.Unlock()
			e.backgroundErr = errors.Join(e.backgroundErr, err)
		}
	}()
	return winner, nil, true, nil
}

// recordAttempts returns the first fatal attempt's error, after recording
// every other settled attempt.
func (e *Engine) recordAttempts(outcome hedge.Outcome[attempt], label raceLabel) ([]model.FailedAttempt, error) {
	var failures []model.FailedAttempt
	var fatalErr error
	for i, a := range outcome.Results {
		switch {
		case a.fatal != nil:
			if fatalErr == nil {
				fatalErr = a.fatal
			}
		case a.canceled:
		default:
			failure, err := e.recordAttempt(a, label, outcome, i)
			if err != nil {
				return nil, errors.Join(fatalErr, err)
			}
			if failure != nil {
				failures = append(failures, *failure)
			}
		}
	}
	return failures, fatalErr
}

// recordAttempt returns a's failure, or nil when it succeeded.
func (e *Engine) recordAttempt(a attempt, label raceLabel, outcome hedge.Outcome[attempt], i int) (*model.FailedAttempt, error) {
	role := label.role
	if a.outcome != history.OutcomeSuccess {
		e.logger.Warn("openrouter: attempt", "role", role, "n", i+1, "model", a.model, "outcome", a.outcome, "latency", a.latency.Round(time.Millisecond), "resends", a.resends, "reason", a.reason)
		public, err := publicOutcome(a.outcome)
		if err != nil {
			return nil, err
		}
		if isRecordableFailure(a, outcome.HasWinner()) {
			if err := e.append(entryFields(a, label, model.QualityUnusable, a.reason)); err != nil {
				return nil, err
			}
		}
		return &model.FailedAttempt{Outcome: public, Model: a.model, GenerationID: a.generationID, Quality: model.QualityUnusable, Reason: rolePrefix(role) + a.reason}, nil
	}
	e.logger.Info("openrouter: attempt", "role", role, "n", i+1, "model", a.model, "outcome", a.outcome, "latency", a.latency.Round(time.Millisecond), "resends", a.resends)
	switch {
	case role == history.RoleReviewer:
		return nil, e.append(reviewerFields(a, label, outcome.IsWinner(i)))
	case !outcome.IsWinner(i):
		return nil, e.append(entryFields(a, label, "", "succeeded after another attempt won"))
	}
	return nil, nil
}

// A refusal counts against its model only when another attempt at the same
// request won, proving the request itself was servable.
func isRecordableFailure(a attempt, raceWon bool) bool {
	if a.outcome == history.OutcomeRefused {
		return raceWon
	}
	return a.generationID != ""
}

func (e *Engine) append(fields history.EntryFields) error {
	entry, err := history.NewEntry(fields)
	if err != nil {
		return err
	}
	return e.history.Append(entry)
}

// Only the winner's verdict rated the generation, so only it links there.
func reviewerFields(a attempt, label raceLabel, won bool) history.EntryFields {
	fields := entryFields(a, label, "", "")
	if won {
		fields.ReviewedGenerationID = label.reviewed
	}
	return fields
}

func entryFields(a attempt, label raceLabel, q model.Quality, reason string) history.EntryFields {
	return history.EntryFields{
		Timestamp:      time.Now().UTC(),
		Model:          a.model,
		GenerationID:   a.generationID,
		Role:           label.role,
		Source:         history.SourceAuto,
		Outcome:        a.outcome,
		Quality:        q,
		TargetQuality:  label.target,
		LatencySeconds: a.latency.Seconds(),
		TimeoutSeconds: label.timeout.Seconds(),
		Reason:         reason,
		Tag:            label.tag,
	}
}

// candidateModels takes the selection's models, drops those whose context
// can't hold the request, then those history excludes, then keeps only the
// cheapest by estimated cost. It errors when nothing is left to ask.
func (e *Engine) candidateModels(sel model.ModelSelection, tag string, promptTokens, maxTokens int) ([]string, error) {
	available, err := e.selectionPool(sel)
	if err != nil {
		return nil, err
	}
	if len(available) == 0 {
		return nil, fmt.Errorf("engine: OpenRouter's catalog lists no structured-output models for selection %+v", sel)
	}
	requestTokens := promptTokens + maxTokens
	var fitting []catalog.Model
	for _, m := range available {
		if requestTokens <= m.ContextTokens {
			fitting = append(fitting, m)
		}
	}
	if len(fitting) == 0 {
		return nil, fmt.Errorf("engine: no structured-output model of selection %+v has context for ~%d prompt + %d output tokens", sel, promptTokens, maxTokens)
	}
	excluded, err := e.excludedModels(tag)
	if err != nil {
		return nil, err
	}
	var models []catalog.Model
	for _, m := range fitting {
		if !slices.Contains(excluded, m.ID) {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("engine: history excludes every structured-output model of selection %+v with enough context: %v", sel, excluded)
	}
	pool, ceilingUSD, err := cost.CheapestPool(models, promptTokens, maxTokens)
	if err != nil {
		return nil, err
	}
	e.logger.Info("openrouter: cost pool", "size", len(pool), "of", len(models), "maxEstimateUSD", ceilingUSD)
	models = pool
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids, nil
}

// selectionPool expects a selection that passed validateSelection.
func (e *Engine) selectionPool(sel model.ModelSelection) ([]catalog.Model, error) {
	if sel.Tier != "" {
		return e.catalog.Candidates(sel.Tier)
	}
	all, err := e.catalog.AllCandidates()
	if err != nil {
		return nil, err
	}
	allowing := len(sel.Allowed) > 0
	listed, field := sel.Allowed, "Allowed"
	if !allowing {
		listed, field = sel.Denied, "Denied"
	}
	for _, id := range listed {
		if !slices.ContainsFunc(all, func(m catalog.Model) bool { return m.ID == id }) {
			return nil, fmt.Errorf("engine: ModelSelection.%s names %q, which is not a structured-output model in OpenRouter's catalog", field, id)
		}
	}
	var pool []catalog.Model
	for _, m := range all {
		if slices.Contains(listed, m.ID) == allowing {
			pool = append(pool, m)
		}
	}
	return pool, nil
}

// promptTokens counts the schema too, since providers fold it into the prompt.
func promptTokens(messages []chat.Message, schema model.JSONSchema) int {
	tokens := cost.EstimateTokens(string(schema.Schema))
	for _, m := range messages {
		tokens += cost.EstimateTokens(m.Content)
	}
	return tokens
}

func maxOutputTokens(requested int) int {
	if requested == 0 {
		return model.DefaultMaxOutputTokens
	}
	return requested
}

func (e *Engine) excludedModels(tag string) ([]string, error) {
	exclusions, err := e.history.Exclusions(time.Now(), tag)
	if err != nil {
		return nil, err
	}
	if len(exclusions.Excluded) > 0 {
		e.logger.Info("openrouter: excluded models", "tag", tag, "count", len(exclusions.Excluded), "models", strings.Join(exclusions.Excluded, ", "))
	}
	if len(exclusions.BelowCap) > 0 {
		nearest := exclusions.BelowCap[:min(nearestExclusionShown, len(exclusions.BelowCap))]
		e.logger.Info("openrouter: models nearest exclusion", "tag", tag, "total", len(exclusions.BelowCap), "top", formatFailureCounts(nearest))
	}
	return exclusions.Excluded, nil
}

func formatFailureCounts(counts []history.FailureCounts) string {
	parts := make([]string, len(counts))
	for i, c := range counts {
		parts[i] = fmt.Sprintf("%s (today=%g week=%g month=%g lifetime=%g)", c.Model, c.Today, c.Week, c.Month, c.Lifetime)
	}
	return strings.Join(parts, ", ")
}

func publicOutcome(o history.Outcome) (model.AttemptOutcome, error) {
	switch o {
	case history.OutcomeFailed:
		return model.OutcomeFailed, nil
	case history.OutcomeTimeout:
		return model.OutcomeTimeout, nil
	case history.OutcomeInvalidOutput:
		return model.OutcomeInvalidOutput, nil
	case history.OutcomeRefused:
		return model.OutcomeRefused, nil
	}
	return "", fmt.Errorf("engine: no public outcome for %q", o)
}

func rolePrefix(role history.Role) string {
	if role == history.RoleReviewer {
		return "review: "
	}
	return ""
}
