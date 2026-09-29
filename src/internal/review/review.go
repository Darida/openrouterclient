package review

import (
	"encoding/json"
	"fmt"
	"strings"

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

const reviewInstructions = `Review your previous reply strictly against the rules below, and nothing else.
Each rule may state a bad score for violating it; a rule that states none has a bad score of 1.
Return one note per rule violation: "rule" names the rule as it appears below, and "text" describes the violation and how to fix it.
Set "totalBadScore" to the sum of the bad scores of the rules your notes name, counting each note once.
If the reply violates no rule, return an empty "notes" array and a "totalBadScore" of 0.

Rules:
`

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

func Prompt(rules string) string { return reviewInstructions + rules }

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
