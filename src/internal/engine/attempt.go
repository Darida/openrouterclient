package engine

import (
	"encoding/json"
	"time"

	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/model"
)

// attempt is one settled chat request.
type attempt struct {
	model string
	// Empty when OpenRouter never accepted the request; such an attempt is
	// reported but not recorded, since nothing reached the model.
	generationID string
	outcome      history.Outcome
	// The caller's context ended it; it's no evidence about any model.
	canceled bool
	content  json.RawMessage
	latency  time.Duration
	reason   string
	// Provider rejections resent within this attempt before it settled.
	resends int
}

// raceLabel is what every history entry from one race shares.
type raceLabel struct {
	role history.Role
	// Empty for the reviewer, whose output has no target.
	target model.Quality
	tag    string
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
