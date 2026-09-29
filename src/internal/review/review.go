package review

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Darida/openrouterclient/src/internal/chat"
	"github.com/Darida/openrouterclient/src/internal/schema"
	"github.com/Darida/openrouterclient/src/model"
)

// Mirrors model.ReviewVerdict exactly; strict mode rejects any other field.
var verdictSchema = model.JSONSchema{
	Name: "review_verdict",
	Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "notes": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "rule": {"type": "string", "description": "The violated rule, quoted or named as it appears in the rules."},
          "text": {"type": "string", "description": "An actionable description of the violation."}
        },
        "required": ["rule", "text"],
        "additionalProperties": false
      }
    },
    "totalBadScore": {"type": "integer", "description": "The sum of the bad scores of the rules the notes name, one per note."}
  },
  "required": ["notes", "totalBadScore"],
  "additionalProperties": false
}`),
}

var verdictValidator = schema.Compile(verdictSchema.Name, verdictSchema.Schema)

// The reviewer is framed as a senior checking someone else's work: shown the
// output as its own assistant turn, a model defends it instead of checking it.
const reviewInstructions = `You are a senior reviewer with twenty years of experience checking junior work before it ships. You are meticulous, skeptical, and fair: you assume nothing is right until you have checked it against the task and the output's actual text. You don't rewrite the work, and you don't praise it. You find what's wrong and say exactly how to fix it.

How you review:
- Read the task first, then the junior's output. Judge the output's actual text, never what the junior probably meant.
- Go through every rule below, one by one. For a rule about coverage, walk through the task's source material section by section and confirm each section appears in the output.
- When in doubt whether a rule is broken, report it. A false alarm costs less than a missed error.
- Write one note per violated rule. In it, "rule" names the rule as written below, and "text" quotes every offending passage (or names every missing part) and says how to fix it.
- Each rule may state a bad score for violating it; a rule that states none has a bad score of 1. Set "totalBadScore" to the sum of the bad scores of the rules your notes name, counting each rule once.
- If the output violates no rule, return an empty "notes" array and a "totalBadScore" of 0.

Rules:
`

const taskHeading = "The task the junior was given:\n\n"

const outputHeading = "The junior's output:\n\n"

const correctionInstructions = `A review of your previous reply found the issues below.
Reply again to the original request with a corrected answer that addresses every issue, in the same output format.

Issues:
`

func Schema() model.JSONSchema { return verdictSchema }

// Validate also rejects a total that contradicts the notes, which strict
// mode can't express, so the reviewer is rated like any off-schema output.
func Validate(content json.RawMessage) error {
	if err := verdictValidator.Validate(content); err != nil {
		return err
	}
	verdict := Parse(content)
	switch {
	case verdict.TotalBadScore < 0:
		return fmt.Errorf("review: totalBadScore %d is negative", verdict.TotalBadScore)
	case len(verdict.Notes) == 0 && verdict.TotalBadScore != 0:
		return fmt.Errorf("review: totalBadScore %d with no notes", verdict.TotalBadScore)
	case len(verdict.Notes) > 0 && verdict.TotalBadScore == 0:
		return fmt.Errorf("review: totalBadScore 0 with %d notes", len(verdict.Notes))
	}
	return nil
}

// Messages builds a review request that never presents output as an
// assistant turn, so the reviewer can't mistake it for its own reply.
func Messages(task string, output json.RawMessage, rules string) []chat.Message {
	return []chat.Message{
		chat.SystemMessage(reviewInstructions + rules),
		chat.UserMessage(taskHeading + task),
		chat.UserMessage(outputHeading + string(output)),
	}
}

func CorrectionPrompt(notes []model.ReviewNote) string {
	return correctionInstructions + FormatNotes(notes)
}

func FormatNotes(notes []model.ReviewNote) string {
	lines := make([]string, len(notes))
	for i, n := range notes {
		lines[i] = fmt.Sprintf("- [%s] %s", n.Rule, n.Text)
	}
	return strings.Join(lines, "\n")
}

// Parse expects content that already passed Validate.
func Parse(content json.RawMessage) model.ReviewVerdict {
	var verdict model.ReviewVerdict
	if err := json.Unmarshal(content, &verdict); err != nil {
		panic(fmt.Sprintf("review: validated verdict does not unmarshal: %v — content: %s", err, content))
	}
	return verdict
}
