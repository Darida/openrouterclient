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
	"github.com/Darida/openrouterclient/src/internal/generationlog"
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
	log      *generationlog.Client
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
		log:      &generationlog.Client{URL: settings.GenerationLogURL, APIKey: apiKey, HTTP: httpClient, Timing: settings.GenerationLog},
		catalog:  &catalog.Catalog{URL: settings.CatalogURL, HTTP: httpClient},
		history:  store,
		logger:   logger,
	}
}

// GenerateText generates, reviews, and corrects until a result meets
// TargetQuality or MaxRounds runs out. Every correction resends the original
// prompt, the latest reply, and its review notes.
func (e *Engine) GenerateText(ctx context.Context, req model.TextGenerationRequirements) (model.GeneratedText, error) {
	validateRequirements(req)
	outputValidator := schema.Compile(req.OutputSchema.Name, req.OutputSchema.Schema)
	if req.OutputValidationRules == "" {
		return e.generateUnreviewed(ctx, req, outputValidator)
	}
	var failures []model.FailedAttempt
	messages := []chat.Message{chat.UserMessage(req.Prompt)}

	for round := 1; round <= e.settings.MaxRounds; round++ {
		result, roundFailures, ok := e.runRound(ctx, req, outputValidator, messages, round)
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
		failures = append(failures, model.FailedAttempt{Outcome: model.OutcomeBelowTarget, Model: result.gen.model, GenerationID: result.gen.generationID, Quality: result.quality, Reason: review.FormatNotes(result.verdict.Notes)})
		messages = []chat.Message{chat.UserMessage(req.Prompt), chat.AssistantMessage(string(result.gen.content)), chat.UserMessage(review.CorrectionPrompt(result.verdict.Notes))}
	}
	return model.GeneratedText{}, &model.AttemptsExhaustedError{Attempts: failures}
}

// generateUnreviewed returns the first schema-valid output. With no rules to
// fail, it is recorded as high.
func (e *Engine) generateUnreviewed(ctx context.Context, req model.TextGenerationRequirements, outputValidator *schema.Validator) (model.GeneratedText, error) {
	e.logger.Info("openrouter: generating without review")
	gen, failures, ok := e.hedge(ctx, []chat.Message{chat.UserMessage(req.Prompt)}, req.OutputSchema, outputValidator.Validate, generatorLabel(req))
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
func (e *Engine) runRound(ctx context.Context, req model.TextGenerationRequirements, outputValidator *schema.Validator, messages []chat.Message, round int) (reviewedRound, []model.FailedAttempt, bool) {
	e.logger.Info("openrouter: generating", "round", round, "maxRounds", e.settings.MaxRounds)
	gen, failures, ok := e.hedge(ctx, messages, req.OutputSchema, outputValidator.Validate, generatorLabel(req))
	if !ok {
		return reviewedRound{}, failures, false
	}
	result, reviewFailures, ok := e.reviewGeneration(ctx, req, gen, round)
	return result, append(failures, reviewFailures...), ok
}

// reviewGeneration rates gen against the rules and records it with that
// rating, or with none if the review had no winner.
func (e *Engine) reviewGeneration(ctx context.Context, req model.TextGenerationRequirements, gen attempt, round int) (reviewedRound, []model.FailedAttempt, bool) {
	messages := []chat.Message{chat.UserMessage(req.Prompt), chat.AssistantMessage(string(gen.content)), chat.UserMessage(review.Prompt(req.OutputValidationRules))}
	rev, failures, ok := e.hedge(ctx, messages, review.Schema(), review.Validate, raceLabel{role: history.RoleReviewer, tag: req.Tag})
	if !ok {
		e.recordGeneration(gen, req, "", "never reviewed")
		return reviewedRound{}, failures, false
	}

	verdict := review.Parse(rev.content)
	rated := quality.FromNoteCount(len(verdict.Notes))
	e.recordGeneration(gen, req, rated, review.FormatNotes(verdict.Notes))
	e.logger.Info("openrouter: reviewed", "round", round, "model", gen.model, "reviewer", rev.model, "quality", rated, "target", req.TargetQuality, "notes", len(verdict.Notes))
	return reviewedRound{gen: gen, rev: rev, verdict: verdict, quality: rated}, failures, true
}

// Close blocks until every straggler from an already-won race is settled and recorded.
func (e *Engine) Close() {
	e.background.Wait()
}

// hedge returns as soon as an attempt wins, recording the rest in the
// background; failures are returned only when nothing won. Each attempt asks
// a model picked at random from the non-excluded candidates. The generator's
// winner is left unrecorded, because its quality comes from the review.
func (e *Engine) hedge(ctx context.Context, messages []chat.Message, schema model.JSONSchema, validate func(json.RawMessage) error, label raceLabel) (attempt, []model.FailedAttempt, bool) {
	candidates := e.candidateModels(label.tag)
	race := hedge.Run(ctx, e.settings.Hedge, func(attemptCtx context.Context, num int) (attempt, bool) {
		modelID := candidates[rand.IntN(len(candidates))]
		e.logger.Info("openrouter: attempt launched", "role", label.role, "attempt", num, "maxAttempts", e.settings.Hedge.MaxAttempts, "model", modelID)
		return e.runAttempt(attemptCtx, chat.BuildPayload(messages, schema, modelID), validate)
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

func (e *Engine) recordAttempts(outcome hedge.Outcome[attempt], label raceLabel) []model.FailedAttempt {
	role := label.role
	var failures []model.FailedAttempt
	for i, a := range outcome.Results {
		if a.canceled {
			continue
		}
		if a.model == "" && a.generationID != "" {
			e.logger.Warn("openrouter: resolving model from generation log", "role", role, "attempt", i+1, "generationId", a.generationID, "outcome", a.outcome, "reason", a.reason)
			a.model = e.catalog.ModelID(e.log.ResolveModel(a.generationID))
		}
		if a.outcome != history.OutcomeSuccess {
			e.logger.Warn("openrouter: attempt failed", "role", role, "attempt", i+1, "model", a.model, "generationId", a.generationID, "outcome", a.outcome, "latency", a.latency, "reason", a.reason)
			failures = append(failures, model.FailedAttempt{Outcome: publicOutcome(a.outcome), Model: a.model, GenerationID: a.generationID, Quality: model.QualityUnusable, Reason: rolePrefix(role) + a.reason})
			if a.model != "" {
				e.history.Append(e.entry(a, label, model.QualityUnusable, a.reason))
			}
			continue
		}
		e.logger.Info("openrouter: attempt succeeded", "role", role, "attempt", i+1, "model", a.model, "latency", a.latency)
		switch {
		case role == history.RoleReviewer:
			e.history.Append(e.entry(a, label, "", ""))
		case !outcome.IsWinner(i):
			e.history.Append(e.entry(a, label, "", "succeeded after another attempt won"))
		}
	}
	return failures
}

func (e *Engine) recordGeneration(gen attempt, req model.TextGenerationRequirements, rated model.Quality, reason string) {
	e.history.Append(e.entry(gen, generatorLabel(req), rated, reason))
}

func (e *Engine) entry(a attempt, label raceLabel, q model.Quality, reason string) history.Entry {
	return history.NewEntry(history.EntryFields{
		Timestamp:      time.Now().UTC(),
		Model:          a.model,
		GenerationID:   a.generationID,
		Role:           label.role,
		Source:         history.SourceAuto,
		Outcome:        a.outcome,
		Quality:        q,
		TargetQuality:  label.target,
		LatencySeconds: a.latency.Seconds(),
		Reason:         reason,
		Tag:            label.tag,
	})
}

// candidateModels panics when exclusions leave no model to ask.
func (e *Engine) candidateModels(tag string) []string {
	excluded := e.excludedModels(tag)
	var candidates []string
	for _, id := range e.catalog.FreeStructuredModels() {
		if !slices.Contains(excluded, id) {
			candidates = append(candidates, id)
		}
	}
	if len(candidates) == 0 {
		panic(fmt.Sprintf("engine: every free structured-output model is excluded: %v", excluded))
	}
	return candidates
}

func (e *Engine) excludedModels(tag string) []string {
	exclusions := e.history.Exclusions(time.Now(), tag)
	if len(exclusions.BelowCap) > 0 {
		e.logger.Info("openrouter: models with recent failures below the exclusion cap", "models", strings.Join(exclusions.BelowCap, ", "))
	}
	if len(exclusions.Excluded) > 0 {
		e.logger.Info("openrouter: excluding models with high failure rates", "models", strings.Join(exclusions.Excluded, ", "))
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
}

func publicOutcome(o history.Outcome) model.AttemptOutcome {
	switch o {
	case history.OutcomeFailed:
		return model.OutcomeFailed
	case history.OutcomeTimeout:
		return model.OutcomeTimeout
	case history.OutcomeAborted:
		return model.OutcomeAborted
	case history.OutcomeInvalidOutput:
		return model.OutcomeInvalidOutput
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
	return raceLabel{role: history.RoleGenerator, target: req.TargetQuality, tag: req.Tag}
}
