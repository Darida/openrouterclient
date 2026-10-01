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

type Engine struct {
	settings Settings
	apiKey   string
	http     *http.Client
	catalog  *catalog.Catalog
	history  history.History
	replies  replyfile.Saver
	logger   *slog.Logger
	// Stragglers still settling after their race was won.
	background sync.WaitGroup
	// What went wrong while recording stragglers, returned by Close.
	backgroundMu  sync.Mutex
	backgroundErr error
}

func New(settings Settings, apiKey string, store history.History, replies replyfile.Saver, logger *slog.Logger) *Engine {
	httpClient := &http.Client{}
	return &Engine{
		settings: settings,
		apiKey:   apiKey,
		http:     httpClient,
		catalog:  &catalog.Catalog{URL: settings.CatalogURL, HTTP: httpClient, Replies: replies},
		history:  store,
		replies:  replies,
		logger:   logger,
	}
}

// GenerateText generates, reviews, and corrects until a result meets
// TargetQuality or the request's corrections run out. Every correction
// resends the original prompt, the latest reply, and its review violations.
// Its errors are ctx.Err(), *model.AttemptsExhaustedError, and
// *model.UnexpectedError.
func (e *Engine) GenerateText(ctx context.Context, req model.TextGenerationRequirements) (model.GeneratedText, error) {
	text, err := e.generateText(ctx, req)
	return text, publicError(ctx, err)
}

func (e *Engine) generateText(ctx context.Context, req model.TextGenerationRequirements) (model.GeneratedText, error) {
	if err := validateRequirements(req); err != nil {
		return model.GeneratedText{}, err
	}
	outputValidator, err := schema.Compile(req.OutputSchema.Name, req.OutputSchema.Schema)
	if err != nil {
		return model.GeneratedText{}, err
	}
	validate := func(content json.RawMessage) (error, error) { return outputValidator.Validate(content), nil }
	if req.OutputValidationRules == "" {
		return e.generateUnreviewed(ctx, req, validate)
	}
	var failures []model.FailedAttempt
	messages := []chat.Message{chat.UserMessage(req.Prompt)}

	maxRounds := 1 + req.MaxCorrections
	for round := 1; round <= maxRounds; round++ {
		result, roundFailures, ok, err := e.runRound(ctx, req, validate, messages, round, maxRounds)
		failures = append(failures, roundFailures...)
		if err != nil {
			return model.GeneratedText{}, err
		}
		if err := ctx.Err(); err != nil {
			return model.GeneratedText{}, err
		}
		if !ok {
			return model.GeneratedText{}, &model.AttemptsExhaustedError{Attempts: failures}
		}
		below, err := quality.Below(result.quality, req.TargetQuality)
		if err != nil {
			return model.GeneratedText{}, err
		}
		if !below {
			return result.generatedText(), nil
		}
		rejection, err := e.rejection(result)
		if err != nil {
			return model.GeneratedText{}, err
		}
		failures = append(failures, rejection)
		messages = []chat.Message{chat.UserMessage(req.Prompt), chat.AssistantMessage(string(result.gen.content)), chat.UserMessage(review.CorrectionPrompt(result.verdict.Violations))}
	}
	return model.GeneratedText{}, &model.AttemptsExhaustedError{Attempts: failures}
}

// publicError passes ctx's own error and AttemptsExhaustedError through and
// marks anything else unexpected.
func publicError(ctx context.Context, err error) error {
	var exhausted *model.AttemptsExhaustedError
	if err == nil || errors.As(err, &exhausted) || err == ctx.Err() {
		return err
	}
	return &model.UnexpectedError{Err: err}
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

// generateUnreviewed returns the first schema-valid output. With no rules to
// fail, it is recorded as high.
func (e *Engine) generateUnreviewed(ctx context.Context, req model.TextGenerationRequirements, validate validator) (model.GeneratedText, error) {
	e.logger.Info("openrouter: generating without review")
	gen, failures, ok, err := e.hedge(ctx, req, generatorLabel(req), []chat.Message{chat.UserMessage(req.Prompt)}, req.OutputSchema, validate)
	if err != nil {
		return model.GeneratedText{}, err
	}
	if err := ctx.Err(); err != nil {
		return model.GeneratedText{}, err
	}
	if !ok {
		return model.GeneratedText{}, &model.AttemptsExhaustedError{Attempts: failures}
	}
	if err := e.recordGeneration(gen, req, model.QualityHigh, "no validation rules; assumed high"); err != nil {
		return model.GeneratedText{}, err
	}
	return model.GeneratedText{Content: gen.content, Model: gen.model, GenerationID: gen.generationID}, nil
}

func (e *Engine) Rate(ctx context.Context, generationID string, q model.Quality, reason string) error {
	return e.history.RecordManual(generationID, q, reason, time.Now())
}

// CandidateModels lists the models a first generation attempt for req would
// pick from, applying every filter GenerateText does, without sending a chat request.
func (e *Engine) CandidateModels(req model.TextGenerationRequirements) ([]string, error) {
	if err := validateRequirements(req); err != nil {
		return nil, err
	}
	return e.candidateModels(req.ModelTier, req.Tag, req.ExcludedModels, promptTokens([]chat.Message{chat.UserMessage(req.Prompt)}, req.OutputSchema), maxOutputTokens(req))
}

// runRound generates from messages and reviews the winner. It returns false
// when either the generation or the review had no winner.
func (e *Engine) runRound(ctx context.Context, req model.TextGenerationRequirements, validate validator, messages []chat.Message, round, maxRounds int) (reviewedRound, []model.FailedAttempt, bool, error) {
	e.logger.Info("openrouter: generating", "round", round, "maxRounds", maxRounds)
	gen, failures, ok, err := e.hedge(ctx, req, generatorLabel(req), messages, req.OutputSchema, validate)
	if err != nil || !ok {
		return reviewedRound{}, failures, false, err
	}
	result, reviewFailures, ok, err := e.reviewGeneration(ctx, req, gen, round)
	return result, append(failures, reviewFailures...), ok, err
}

// reviewGeneration rates gen against the rules and records it with that
// rating, or with none if the review had no winner.
func (e *Engine) reviewGeneration(ctx context.Context, req model.TextGenerationRequirements, gen attempt, round int) (reviewedRound, []model.FailedAttempt, bool, error) {
	messages := review.Messages(req.Prompt, gen.content, req.OutputValidationRules)
	rev, failures, ok, err := e.hedge(ctx, req, raceLabel{role: history.RoleReviewer, tag: req.Tag, timeout: req.Timeout, reviewed: gen.generationID}, messages, review.Schema(), review.Validate)
	if err != nil {
		return reviewedRound{}, failures, false, err
	}
	if !ok {
		return reviewedRound{}, failures, false, e.recordGeneration(gen, req, "", "never reviewed")
	}

	verdict, err := review.Parse(rev.content)
	if err != nil {
		return reviewedRound{}, failures, false, err
	}
	rated := quality.FromBadScore(verdict.TotalBadScore, req.ReviewToleranceThreshold)
	if err := e.recordGeneration(gen, req, rated, review.FormatViolations(verdict.Violations)); err != nil {
		return reviewedRound{}, failures, false, err
	}
	e.logger.Info("openrouter: reviewed", "round", round, "model", gen.model, "reviewer", rev.model, "quality", rated, "target", req.TargetQuality, "violations", len(verdict.Violations), "badScore", verdict.TotalBadScore, "threshold", req.ReviewToleranceThreshold)
	return reviewedRound{gen: gen, rev: rev, verdict: verdict, quality: rated}, failures, true, nil
}

// Close blocks until every straggler from an already-won race is settled and
// recorded, and returns what went wrong while recording them.
func (e *Engine) Close() error {
	e.background.Wait()
	e.backgroundMu.Lock()
	defer e.backgroundMu.Unlock()
	return e.backgroundErr
}

// hedge returns as soon as an attempt wins, recording the rest in the
// background; failures are returned only when nothing won. Each attempt asks
// a model picked at random from the candidates. The generator's winner is
// left unrecorded, because its quality comes from the review. A fatal
// attempt aborts the race and comes back as err.
func (e *Engine) hedge(ctx context.Context, req model.TextGenerationRequirements, label raceLabel, messages []chat.Message, schema model.JSONSchema, validate validator) (attempt, []model.FailedAttempt, bool, error) {
	maxTokens := maxOutputTokens(req)
	candidates, err := e.candidateModels(req.ModelTier, label.tag, req.ExcludedModels, promptTokens(messages, schema), maxTokens)
	if err != nil {
		return attempt{}, nil, false, err
	}
	race := hedge.Run(ctx, e.timing(req), func(attemptCtx context.Context, num int) (attempt, hedge.Verdict) {
		modelID := candidates[rand.IntN(len(candidates))]
		payload, err := chat.BuildPayload(messages, schema, modelID, maxTokens)
		if err != nil {
			return fatal(modelID, "", err), hedge.Aborted
		}
		return e.runAttempt(attemptCtx, modelID, payload, validate)
	})
	winner, ok := race.Winner()
	if !ok {
		failures, err := e.recordAttempts(race.Settled(), label)
		return attempt{}, failures, false, err
	}
	e.background.Add(1)
	go func() {
		defer e.background.Done()
		if _, err := e.recordAttempts(race.Settled(), label); err != nil {
			e.logger.Error("openrouter: recording a won race's stragglers failed", "err", err)
			e.backgroundMu.Lock()
			defer e.backgroundMu.Unlock()
			e.backgroundErr = errors.Join(e.backgroundErr, err)
		}
	}()
	return winner, nil, true, nil
}

func (e *Engine) timing(req model.TextGenerationRequirements) hedge.Timing {
	return hedge.Timing{MaxAttempts: e.settings.MaxAttempts, Stagger: req.Timeout, AttemptTimeout: req.Timeout + e.settings.GraceAfterTimeout}
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

func (e *Engine) recordGeneration(gen attempt, req model.TextGenerationRequirements, rated model.Quality, reason string) error {
	return e.append(entryFields(gen, generatorLabel(req), rated, reason))
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

// candidateModels drops models whose context can't hold the request, then
// excluded models, then for the paid tier keeps only the cheapest by
// estimated cost. It errors when nothing is left to ask.
func (e *Engine) candidateModels(tier model.ModelTier, tag string, callerExcluded []string, promptTokens, maxTokens int) ([]string, error) {
	available, err := e.catalog.Candidates(tier)
	if err != nil {
		return nil, err
	}
	if len(available) == 0 {
		return nil, fmt.Errorf("engine: OpenRouter's catalog lists no %s structured-output models", tier)
	}
	if err := validateExcludedModels(callerExcluded, available, tier); err != nil {
		return nil, err
	}
	historyExcluded, err := e.excludedModels(tag)
	if err != nil {
		return nil, err
	}
	excluded := slices.Concat(historyExcluded, callerExcluded)
	requestTokens := promptTokens + maxTokens
	var fitting []catalog.Model
	for _, m := range available {
		if requestTokens <= m.ContextTokens {
			fitting = append(fitting, m)
		}
	}
	if len(fitting) == 0 {
		return nil, fmt.Errorf("engine: no %s structured-output model has context for ~%d prompt + %d output tokens", tier, promptTokens, maxTokens)
	}
	var models []catalog.Model
	for _, m := range fitting {
		if !slices.Contains(excluded, m.ID) {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("engine: every %s structured-output model with enough context is excluded: %v", tier, excluded)
	}
	if tier == model.ModelTierPaid {
		pool, ceilingUSD, err := cost.CheapestPool(models, promptTokens, maxTokens)
		if err != nil {
			return nil, err
		}
		e.logger.Info("openrouter: paid pool", "size", len(pool), "of", len(models), "maxEstimateUSD", ceilingUSD)
		models = pool
	}
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids, nil
}

// promptTokens counts the schema too, since providers fold it into the prompt.
func promptTokens(messages []chat.Message, schema model.JSONSchema) int {
	tokens := cost.EstimateTokens(string(schema.Schema))
	for _, m := range messages {
		tokens += cost.EstimateTokens(m.Content)
	}
	return tokens
}

func maxOutputTokens(req model.TextGenerationRequirements) int {
	if req.MaxOutputTokens == 0 {
		return model.DefaultMaxOutputTokens
	}
	return req.MaxOutputTokens
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

func validateRequirements(req model.TextGenerationRequirements) error {
	if req.Prompt == "" || req.OutputSchema.Name == "" || len(req.OutputSchema.Schema) == 0 || req.Tag == "" {
		return fmt.Errorf("engine: Prompt, OutputSchema.Name, OutputSchema.Schema, and Tag are all required: %+v", req)
	}
	if !quality.IsRating(req.TargetQuality) {
		return fmt.Errorf("engine: TargetQuality must be high, medium, or low, got %q", req.TargetQuality)
	}
	if req.OutputValidationRules != "" && req.ReviewToleranceThreshold < 1 {
		return fmt.Errorf("engine: ReviewToleranceThreshold must be at least 1 when OutputValidationRules is set, got %d", req.ReviewToleranceThreshold)
	}
	if req.OutputValidationRules == "" && req.ReviewToleranceThreshold != 0 {
		return fmt.Errorf("engine: ReviewToleranceThreshold must be 0 when OutputValidationRules is empty, got %d", req.ReviewToleranceThreshold)
	}
	if req.MaxCorrections < 0 {
		return fmt.Errorf("engine: MaxCorrections must not be negative, got %d", req.MaxCorrections)
	}
	if req.OutputValidationRules == "" && req.MaxCorrections != 0 {
		return fmt.Errorf("engine: MaxCorrections must be 0 when OutputValidationRules is empty, got %d", req.MaxCorrections)
	}
	if req.ModelTier != model.ModelTierFree && req.ModelTier != model.ModelTierPaid {
		return fmt.Errorf("engine: ModelTier must be free or paid, got %q", req.ModelTier)
	}
	if req.Timeout <= 0 {
		return fmt.Errorf("engine: Timeout must be positive, got %v", req.Timeout)
	}
	if req.MaxOutputTokens < 0 {
		return fmt.Errorf("engine: MaxOutputTokens must not be negative, got %d", req.MaxOutputTokens)
	}
	return nil
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

func generatorLabel(req model.TextGenerationRequirements) raceLabel {
	return raceLabel{role: history.RoleGenerator, target: req.TargetQuality, tag: req.Tag, timeout: req.Timeout}
}

func validateExcludedModels(excluded []string, available []catalog.Model, tier model.ModelTier) error {
	for _, id := range excluded {
		if !slices.ContainsFunc(available, func(m catalog.Model) bool { return m.ID == id }) {
			return fmt.Errorf("engine: ExcludedModels names %q, which is not a %s structured-output model in OpenRouter's catalog", id, tier)
		}
	}
	return nil
}
