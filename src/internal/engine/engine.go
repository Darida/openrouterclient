package engine

import (
	"context"
	"encoding/json"
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
	"github.com/Darida/openrouterclient/src/internal/review"
	"github.com/Darida/openrouterclient/src/internal/schema"
	"github.com/Darida/openrouterclient/src/model"
)

type Engine struct {
	settings Settings
	apiKey   string
	http     *http.Client
	catalog  *catalog.Catalog
	history  *history.Store
	logger   *slog.Logger
	// Stragglers still settling after their race was won.
	background sync.WaitGroup
}

func New(settings Settings, apiKey string, store *history.Store, logger *slog.Logger) *Engine {
	httpClient := &http.Client{}
	return &Engine{
		settings: settings,
		apiKey:   apiKey,
		http:     httpClient,
		catalog:  &catalog.Catalog{URL: settings.CatalogURL, HTTP: httpClient},
		history:  store,
		logger:   logger,
	}
}

// GenerateText generates, reviews, and corrects until a result meets
// TargetQuality or the request's corrections run out. Every correction
// resends the original prompt, the latest reply, and its review notes.
func (e *Engine) GenerateText(ctx context.Context, req model.TextGenerationRequirements) (model.GeneratedText, error) {
	validateRequirements(req)
	outputValidator := schema.Compile(req.OutputSchema.Name, req.OutputSchema.Schema)
	if req.OutputValidationRules == "" {
		return e.generateUnreviewed(ctx, req, outputValidator)
	}
	var failures []model.FailedAttempt
	messages := []chat.Message{chat.UserMessage(req.Prompt)}

	maxRounds := 1 + maxCorrections(req)
	for round := 1; round <= maxRounds; round++ {
		result, roundFailures, ok := e.runRound(ctx, req, outputValidator, messages, round, maxRounds)
		failures = append(failures, roundFailures...)
		if err := ctx.Err(); err != nil {
			return model.GeneratedText{}, err
		}
		if !ok {
			return model.GeneratedText{}, &model.AttemptsExhaustedError{Attempts: failures}
		}
		if !quality.Below(result.quality, req.TargetQuality) {
			return result.generatedText(), nil
		}
		failures = append(failures, model.FailedAttempt{Outcome: model.OutcomeBelowTarget, Model: result.gen.model, GenerationID: result.gen.generationID, Quality: result.quality, Reason: review.FormatNotes(result.verdict.Notes), ReviewGenerationID: result.rev.generationID})
		messages = []chat.Message{chat.UserMessage(req.Prompt), chat.AssistantMessage(string(result.gen.content)), chat.UserMessage(review.CorrectionPrompt(result.verdict.Notes))}
	}
	return model.GeneratedText{}, &model.AttemptsExhaustedError{Attempts: failures}
}

// generateUnreviewed returns the first schema-valid output. With no rules to
// fail, it is recorded as high.
func (e *Engine) generateUnreviewed(ctx context.Context, req model.TextGenerationRequirements, outputValidator *schema.Validator) (model.GeneratedText, error) {
	e.logger.Info("openrouter: generating without review")
	gen, failures, ok := e.hedge(ctx, req, generatorLabel(req), []chat.Message{chat.UserMessage(req.Prompt)}, req.OutputSchema, outputValidator.Validate)
	if err := ctx.Err(); err != nil {
		return model.GeneratedText{}, err
	}
	if !ok {
		return model.GeneratedText{}, &model.AttemptsExhaustedError{Attempts: failures}
	}
	e.recordGeneration(gen, req, model.QualityHigh, "no validation rules; assumed high")
	return model.GeneratedText{Content: gen.content, Model: gen.model, GenerationID: gen.generationID}, nil
}

func (e *Engine) Rate(ctx context.Context, generationID string, q model.Quality, reason string) error {
	return e.history.RecordManual(generationID, q, reason, time.Now())
}

// runRound generates from messages and reviews the winner. It returns false
// when either the generation or the review had no winner.
func (e *Engine) runRound(ctx context.Context, req model.TextGenerationRequirements, outputValidator *schema.Validator, messages []chat.Message, round, maxRounds int) (reviewedRound, []model.FailedAttempt, bool) {
	e.logger.Info("openrouter: generating", "round", round, "maxRounds", maxRounds)
	gen, failures, ok := e.hedge(ctx, req, generatorLabel(req), messages, req.OutputSchema, outputValidator.Validate)
	if !ok {
		return reviewedRound{}, failures, false
	}
	result, reviewFailures, ok := e.reviewGeneration(ctx, req, gen, round)
	return result, append(failures, reviewFailures...), ok
}

// reviewGeneration rates gen against the rules and records it with that
// rating, or with none if the review had no winner.
func (e *Engine) reviewGeneration(ctx context.Context, req model.TextGenerationRequirements, gen attempt, round int) (reviewedRound, []model.FailedAttempt, bool) {
	messages := review.Messages(req.Prompt, gen.content, req.OutputValidationRules)
	rev, failures, ok := e.hedge(ctx, req, raceLabel{role: history.RoleReviewer, tag: req.Tag, timeout: req.Timeout, reviewed: gen.generationID}, messages, review.Schema(), review.Validate)
	if !ok {
		e.recordGeneration(gen, req, "", "never reviewed")
		return reviewedRound{}, failures, false
	}

	verdict := review.Parse(rev.content)
	rated := quality.FromBadScore(verdict.TotalBadScore, req.ReviewToleranceThreshold)
	e.recordGeneration(gen, req, rated, review.FormatNotes(verdict.Notes))
	e.logger.Info("openrouter: reviewed", "round", round, "model", gen.model, "reviewer", rev.model, "quality", rated, "target", req.TargetQuality, "notes", len(verdict.Notes), "badScore", verdict.TotalBadScore, "threshold", req.ReviewToleranceThreshold)
	return reviewedRound{gen: gen, rev: rev, verdict: verdict, quality: rated}, failures, true
}

// Close blocks until every straggler from an already-won race is settled and recorded.
func (e *Engine) Close() {
	e.background.Wait()
}

// hedge returns as soon as an attempt wins, recording the rest in the
// background; failures are returned only when nothing won. Each attempt asks
// a model picked at random from the candidates. The generator's winner is
// left unrecorded, because its quality comes from the review.
func (e *Engine) hedge(ctx context.Context, req model.TextGenerationRequirements, label raceLabel, messages []chat.Message, schema model.JSONSchema, validate func(json.RawMessage) error) (attempt, []model.FailedAttempt, bool) {
	maxTokens := maxOutputTokens(req)
	candidates := e.candidateModels(req.ModelTier, label.tag, promptTokens(messages, schema), maxTokens)
	race := hedge.Run(ctx, e.timing(req), func(attemptCtx context.Context, num int) (attempt, bool) {
		modelID := candidates[rand.IntN(len(candidates))]
		return e.runAttempt(attemptCtx, modelID, chat.BuildPayload(messages, schema, modelID, maxTokens), validate)
	})
	winner, ok := race.Winner()
	if !ok {
		return attempt{}, e.recordAttempts(race.Settled(), label), false
	}
	e.background.Add(1)
	go func() {
		defer e.background.Done()
		e.recordAttempts(race.Settled(), label)
	}()
	return winner, nil, true
}

func (e *Engine) timing(req model.TextGenerationRequirements) hedge.Timing {
	return hedge.Timing{MaxAttempts: e.settings.MaxAttempts, Stagger: req.Timeout, AttemptTimeout: req.Timeout + e.settings.GraceAfterTimeout}
}

func (e *Engine) recordAttempts(outcome hedge.Outcome[attempt], label raceLabel) []model.FailedAttempt {
	role := label.role
	var failures []model.FailedAttempt
	for i, a := range outcome.Results {
		if a.canceled {
			continue
		}
		if a.outcome != history.OutcomeSuccess {
			e.logger.Warn("openrouter: attempt", "role", role, "n", i+1, "model", a.model, "outcome", a.outcome, "latency", a.latency.Round(time.Millisecond), "resends", a.resends, "reason", a.reason)
			failures = append(failures, model.FailedAttempt{Outcome: publicOutcome(a.outcome), Model: a.model, GenerationID: a.generationID, Quality: model.QualityUnusable, Reason: rolePrefix(role) + a.reason})
			if isRecordableFailure(a, outcome.HasWinner()) {
				e.history.Append(e.entry(a, label, model.QualityUnusable, a.reason))
			}
			continue
		}
		e.logger.Info("openrouter: attempt", "role", role, "n", i+1, "model", a.model, "outcome", a.outcome, "latency", a.latency.Round(time.Millisecond), "resends", a.resends)
		switch {
		case role == history.RoleReviewer:
			e.history.Append(reviewerEntry(a, label, outcome.IsWinner(i)))
		case !outcome.IsWinner(i):
			e.history.Append(e.entry(a, label, "", "succeeded after another attempt won"))
		}
	}
	return failures
}

// A refusal counts against its model only when another attempt at the same
// request won, proving the request itself was servable.
func isRecordableFailure(a attempt, raceWon bool) bool {
	if a.outcome == history.OutcomeRefused {
		return raceWon
	}
	return a.generationID != ""
}

func (e *Engine) recordGeneration(gen attempt, req model.TextGenerationRequirements, rated model.Quality, reason string) {
	e.history.Append(e.entry(gen, generatorLabel(req), rated, reason))
}

// Only the winner's verdict rated the generation, so only it links there.
func reviewerEntry(a attempt, label raceLabel, won bool) history.Entry {
	fields := entryFields(a, label, "", "")
	if won {
		fields.ReviewedGenerationID = label.reviewed
	}
	return history.NewEntry(fields)
}

func (e *Engine) entry(a attempt, label raceLabel, q model.Quality, reason string) history.Entry {
	return history.NewEntry(entryFields(a, label, q, reason))
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
// estimated cost. It panics when nothing is left to ask.
func (e *Engine) candidateModels(tier model.ModelTier, tag string, promptTokens, maxTokens int) []string {
	excluded := e.excludedModels(tag)
	available := e.catalog.Candidates(tier)
	if len(available) == 0 {
		panic(fmt.Sprintf("engine: OpenRouter's catalog lists no %s structured-output models", tier))
	}
	requestTokens := promptTokens + maxTokens
	var fitting []catalog.Model
	for _, m := range available {
		if requestTokens <= m.ContextTokens {
			fitting = append(fitting, m)
		}
	}
	if len(fitting) == 0 {
		panic(fmt.Sprintf("engine: no %s structured-output model has context for ~%d prompt + %d output tokens", tier, promptTokens, maxTokens))
	}
	var models []catalog.Model
	for _, m := range fitting {
		if !slices.Contains(excluded, m.ID) {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		panic(fmt.Sprintf("engine: every %s structured-output model with enough context is excluded: %v", tier, excluded))
	}
	if tier == model.ModelTierPaid {
		pool, ceilingUSD := cost.CheapestPool(models, promptTokens, maxTokens)
		e.logger.Info("openrouter: paid pool", "size", len(pool), "of", len(models), "maxEstimateUSD", ceilingUSD)
		models = pool
	}
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids
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

func maxCorrections(req model.TextGenerationRequirements) int {
	if req.MaxCorrections == 0 {
		return model.DefaultMaxCorrections
	}
	return req.MaxCorrections
}

func (e *Engine) excludedModels(tag string) []string {
	exclusions := e.history.Exclusions(time.Now(), tag)
	if len(exclusions.BelowCap) > 0 || len(exclusions.Excluded) > 0 {
		e.logger.Info("openrouter: model failures", "tag", tag, "excluded", strings.Join(exclusions.Excluded, ", "), "belowCap", strings.Join(exclusions.BelowCap, ", "))
	}
	return exclusions.Excluded
}

func validateRequirements(req model.TextGenerationRequirements) {
	if req.Prompt == "" || req.OutputSchema.Name == "" || len(req.OutputSchema.Schema) == 0 || req.Tag == "" {
		panic(fmt.Sprintf("engine: Prompt, OutputSchema.Name, OutputSchema.Schema, and Tag are all required: %+v", req))
	}
	if !quality.IsRating(req.TargetQuality) {
		panic(fmt.Sprintf("engine: TargetQuality must be high, medium, or low, got %q", req.TargetQuality))
	}
	if req.OutputValidationRules != "" && req.ReviewToleranceThreshold < 1 {
		panic(fmt.Sprintf("engine: ReviewToleranceThreshold must be at least 1 when OutputValidationRules is set, got %d", req.ReviewToleranceThreshold))
	}
	if req.OutputValidationRules == "" && req.ReviewToleranceThreshold != 0 {
		panic(fmt.Sprintf("engine: ReviewToleranceThreshold must be 0 when OutputValidationRules is empty, got %d", req.ReviewToleranceThreshold))
	}
	if req.MaxCorrections < 0 {
		panic(fmt.Sprintf("engine: MaxCorrections must not be negative, got %d", req.MaxCorrections))
	}
	if req.OutputValidationRules == "" && req.MaxCorrections != 0 {
		panic(fmt.Sprintf("engine: MaxCorrections must be 0 when OutputValidationRules is empty, got %d", req.MaxCorrections))
	}
	if req.ModelTier != model.ModelTierFree && req.ModelTier != model.ModelTierPaid {
		panic(fmt.Sprintf("engine: ModelTier must be free or paid, got %q", req.ModelTier))
	}
	if req.Timeout <= 0 {
		panic(fmt.Sprintf("engine: Timeout must be positive, got %v", req.Timeout))
	}
	if req.MaxOutputTokens < 0 {
		panic(fmt.Sprintf("engine: MaxOutputTokens must not be negative, got %d", req.MaxOutputTokens))
	}
}

func publicOutcome(o history.Outcome) model.AttemptOutcome {
	switch o {
	case history.OutcomeFailed:
		return model.OutcomeFailed
	case history.OutcomeTimeout:
		return model.OutcomeTimeout
	case history.OutcomeInvalidOutput:
		return model.OutcomeInvalidOutput
	case history.OutcomeRefused:
		return model.OutcomeRefused
	}
	panic(fmt.Sprintf("engine: no public outcome for %q", o))
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
