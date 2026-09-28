package engine

import (
	"encoding/json"
	"time"

	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/model"
)

// attempt is one settled chat request.
type attempt struct {
	// Empty when OpenRouter never accepted the request, or until
	// recordAttempts resolves it from the generation log.
	model        string
	generationID string
	outcome      history.Outcome
	// The caller's context ended it; it's no evidence about any model.
	canceled bool
	content  json.RawMessage
	latency  time.Duration
	reason   string
}

// reviewedRound is one generation plus the automatic review that rated it.
type reviewedRound struct {
	gen, rev attempt
	verdict  model.ReviewVerdict
	quality  model.Quality
}

func (r reviewedRound) generatedText() model.GeneratedText {
	return model.GeneratedText{
		Content:      r.gen.content,
		Model:        r.gen.model,
		GenerationID: r.gen.generationID,
		Review:       &model.Review{Verdict: r.verdict, Quality: r.quality, Model: r.rev.model, GenerationID: r.rev.generationID},
	}
}
