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
    "violations": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "rule": {"type": "string", "description": "The violated rule, quoted or named as it appears in the rules."},
          "evidence": {"type": "string", "description": "A verbatim excerpt of about five words that demonstrates the violation: from the output, or from the task for a missing part."},
          "explanation": {"type": "string", "description": "Why the evidence breaks the rule."},
          "recommendedAction": {"type": "string", "description": "The specific change that fixes this violation."},
          "badScore": {"type": "integer", "description": "The bad score the violated rule states, or 1 if it states none."}
        },
        "required": ["rule", "evidence", "explanation", "recommendedAction", "badScore"],
        "additionalProperties": false
      }
    },
    "totalBadScore": {"type": "integer", "description": "The sum of badScore over all violations."}
  },
  "required": ["violations", "totalBadScore"],
  "additionalProperties": false
}`),
}

var verdictValidator = schema.Compile(verdictSchema.Name, verdictSchema.Schema)

// The reviewer is framed as a senior checking someone else's work: shown the
// output as its own assistant turn, a model defends it instead of checking it.
const reviewInstructions = `You are a senior reviewer with twenty years of experience checking junior work before it ships. You are meticulous, skeptical, and exact: you assume nothing is right until you have checked it against the task and the output's actual text. You don't rewrite the work, you don't praise it, and you don't comment on it. Your only job is to report rule violations.

How you review:
- Read the task first, then the junior's output. Judge the output's actual text, never what the junior probably meant.
- Go through every rule below, one by one, and check the whole output against it. For a rule about coverage, walk through the task's source material section by section and confirm each section appears in the output.
- Report ONLY violations of the rules below. Never report style preferences, possible improvements, general observations, or anything no rule covers, however much you'd like to.
- Report a violation only when you can point to it. If you can't quote evidence that demonstrates it, it is not a violation.
- Report each offending instance as its own violation. A rule broken in three places is three violations.
- In each violation:
  - "rule" names the rule as written below.
  - "evidence" is a verbatim excerpt of about five words that demonstrates the problem. Quote the output; for something missing, quote the task where the missing part is required.
  - "explanation" says in one or two sentences why the evidence breaks the rule.
  - "recommendedAction" says exactly what to change to fix this instance.
  - "badScore" is the bad score the rule states for violating it; a rule that states none has a bad score of 1.
- Set "totalBadScore" to the sum of "badScore" over all your violations.
- If the output violates no rule, return an empty "violations" array and a "totalBadScore" of 0.

Rules:
`

const taskHeading = "The task the junior was given:\n\n"

const outputHeading = "The junior's output:\n\n"

const correctionInstructions = `A review of your previous reply found the rule violations below.
Reply again to the original request with a corrected answer that fixes every violation, in the same output format.

Violations:
`

func Schema() model.JSONSchema { return verdictSchema }

// Validate also rejects a total that contradicts the violations, which strict
// mode can't express, so the reviewer is rated like any off-schema output.
func Validate(content json.RawMessage) error {
	if err := verdictValidator.Validate(content); err != nil {
		return err
	}
	verdict := Parse(content)
	switch {
	case verdict.TotalBadScore < 0:
		return fmt.Errorf("review: totalBadScore %d is negative", verdict.TotalBadScore)
	case verdict.TotalBadScore != sumBadScores(verdict.Violations):
		return fmt.Errorf("review: totalBadScore %d differs from the violations' badScore sum %d", verdict.TotalBadScore, sumBadScores(verdict.Violations))
	case len(verdict.Violations) > 0 && verdict.TotalBadScore == 0:
		return fmt.Errorf("review: totalBadScore 0 with %d violations", len(verdict.Violations))
	}
	return nil
}

func sumBadScores(violations []model.ReviewViolation) int {
	sum := 0
	for _, v := range violations {
		sum += v.BadScore
	}
	return sum
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

func CorrectionPrompt(violations []model.ReviewViolation) string {
	return correctionInstructions + FormatViolations(violations)
}

func FormatViolations(violations []model.ReviewViolation) string {
	lines := make([]string, len(violations))
	for i, v := range violations {
		lines[i] = fmt.Sprintf("- [%s] %q: %s Fix: %s", v.Rule, v.Evidence, v.Explanation, v.RecommendedAction)
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
