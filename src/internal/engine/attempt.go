package engine

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Darida/openrouterclient/src/internal/chat"
	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/model"
)

// attempt is one settled chat request.
type attempt struct {
	model string
	// Empty when OpenRouter never accepted the request; such an attempt is
	// reported but not recorded, since nothing reached the model, unless it
	// was refused.
	generationID string
	outcome      history.Outcome
	// The caller's context ended it; it's no evidence about any model.
	canceled bool
	content  json.RawMessage
	latency  time.Duration
	reason   string
	// Provider rejections resent within this attempt before it settled.
	resends int
	// Something no model causes, such as an undocumented reply; it aborts the race.
	fatal error
}

// raceLabel is what every history entry from one race shares.
type raceLabel struct {
	role history.Role
	// Empty for the reviewer, whose output has no target.
	target model.Quality
	tag    string
	// The configured Timeout, against which history judges a success slow.
	timeout time.Duration
	// Empty for the generator and for a review of caller-supplied content;
	// otherwise the generation the reviewer judges.
	reviewed string
}

// race is one hedged chat request: what to send, to which models, and how
// to judge and record each reply.
type race struct {
	label     raceLabel
	models    model.ModelSelection
	messages  []chat.Message
	schema    model.JSONSchema
	validate  validator
	maxTokens int
}

// reviewedRound is one generation plus the automatic review that rated it.
type reviewedRound struct {
	gen, rev attempt
	verdict  model.ReviewVerdict
	quality  model.Quality
}

// validator reports whether content is the model's valid output. Only invalid
// is the model's fault; err aborts the call.
type validator func(content json.RawMessage) (invalid, err error)

func (r reviewedRound) reviewedText() model.ReviewedText {
	return model.ReviewedText{
		GeneratedText: r.gen.generatedText(),
		Review:        *r.review(),
	}
}

func (a attempt) generatedText() model.GeneratedText {
	return model.GeneratedText{Content: a.content, Model: a.model, GenerationID: a.generationID}
}

func (r reviewedRound) rejection(output, review string) model.FailedAttempt {
	return model.FailedAttempt{
		Outcome:      model.OutcomeBelowTarget,
		Model:        r.gen.model,
		GenerationID: r.gen.generationID,
		Quality:      r.quality,
		Reason:       fmt.Sprintf("rejected output %s; review %s", output, review),
		Content:      r.gen.content,
		Review:       r.review(),
	}
}

func (r reviewedRound) review() *model.Review {
	return &model.Review{Verdict: r.verdict, Quality: r.quality, Model: r.rev.model, GenerationID: r.rev.generationID}
}
