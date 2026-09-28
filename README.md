# openrouterclient

A Go library for OpenRouter text generation. You send a prompt and get
back JSON that matches a schema you supply. It is ported from
`assetloom/src/clients/openrouter` and supports text generation only.

Each call goes through these steps:

1. **Generate.** The prompt is sent to `openrouter/free` with a strict
   `json_schema` response format. Models the local history marks as
   unreliable are excluded.
2. **Review.** A follow-up request continues the same conversation. It
   sends the generated answer back, followed by a fixed review
   instruction plus the caller's `ReviewRulesPrompt`. The reply must match
   the fixed `ReviewVerdict` schema, which is a list of notes. The number
   of notes sets the generation's `Quality`.
3. **Correct.** If that quality is below `TargetQuality`, a correction
   request sends the original prompt, the previous reply, and the review
   notes, and asks the model to address the notes. The correction is then
   reviewed again.
4. **Track.** Every generation's quality is recorded against the model
   that produced it: the automatic review's rating, `unusable` for
   failures, timeouts, and aborts, and any manual rating from
   `Client.Rate`. Models that fail too often are excluded from later
   calls.

## Layout

The repo root holds only module and tooling files (`go.mod`, `git/`,
`bin/`, docs). All Go code lives under `src/`:

- `src/model/` holds data types only, with no logic: the request and
  response types, `ReviewVerdict`, `Quality`, and `AttemptsExhaustedError`.
- `src/api/` holds the client contract: `Client`, `Config`, and a thin
  `New` that wires up `src/internal/`.
- `src/internal/` holds all behavior, split into these packages:
  - `chat`: builds the wire payload and parses responses.
  - `hedge`: runs staggered parallel attempts.
  - `generationlog`: polls OpenRouter's `/api/v1/generation` log.
  - `history`: stores outcomes and computes exclusions.
  - `review`: holds the fixed review prompt and schema.
  - `quality`: ranks qualities and derives one from a review's note count.
  - `schema`: validates output against the requested JSON Schema.
  - `engine`: orchestrates the generate, review, and correct loop.

  These packages may import `src/model/` but never `src/api/`.
- `src/cmd/generate/` is the command-line program behind `bin/generate`.

## Behavior

- **Hedged attempts.** The first attempt starts immediately. Another
  attempt starts when the previous one fails, or when it has been pending
  60s with no response. There are at most 3 attempts. Once one wins, the
  others get whichever is later of 30s past the win or 60s of their own
  runtime, and are then aborted. Each attempt also has a hard timeout.
  Both generation and review calls are hedged this way.
- **Attribution.** Every attempt OpenRouter accepted, which is known from
  its `X-Generation-Id` header, is attributed to a real model. An attempt
  that times out, breaks mid-body, or is aborted after another attempt won
  is aborted for good. Its model is then resolved by polling the
  generation log for that id, and it is recorded as a failure. If the log doesn't
  resolve within its window, the process panics. A history entry never
  records a guessed or placeholder model. A request OpenRouter never
  accepted has no model, so it is reported in the returned error but not
  written to history.
- **Correction rounds.** There are at most 3 rounds, counting the first
  generation. Failed or timed-out review attempts are recorded against the
  reviewer's model. Output that doesn't match the schema, whether from the
  generator or the reviewer, is rated `unusable`.
- **Exclusion.** A result counts as a failure if its quality is below the
  target of the request that produced it, or if it took 60s or longer. A
  model is excluded once its failures exceed any of these limits: more
  than 3 today (UTC), more than 6 in the last 7 days, more than 12 in the
  last 30 days, or more than 24 in total. The exclusion list is passed to
  `openrouter/free`'s router, which then picks a model at random.

## Command line

```sh
bin/generate bin/example-requirements.json
```

The script takes one requirements file with the fields `prompt`,
`reviewRulesPrompt`, `outputSchema` (`name` and `schema`), and
`targetQuality`. See `bin/example-requirements.json`. It reads the API key
from `git config --get openrouter.githubapikey`; set it with
`git config --local openrouter.githubapikey 'YOUR_KEY'`. History goes to
`history.json` at the repo root, which is tracked in git so exclusions
carry across machines.

It prints the reviewed result as JSON on stdout and logs on stderr. If
every attempt fails, it prints the failed attempts on stderr and exits 1.

## Failure policy

This library never falls back and never swallows a failure.

- `GenerateText` returns an error in exactly two cases: every allowed
  attempt failed, timed out, or fell below `TargetQuality`
  (`*AttemptsExhaustedError`, which carries every attempt's raw reason), or
  the caller's context ended (`ctx.Err()`). Attempts cut short by the
  caller's context are no evidence about any model, so they are not
  recorded.
- Only transient chat statuses (408, 429, 500, 502, 503, 504) count as a
  model failure. Any other non-200 status means the request or the key is
  wrong, so it panics.
- Anything unexpected panics, with the full raw body in the message. That
  includes a response with no `model`, a 200 response with no
  `X-Generation-Id`, a generation log that never resolves or answers with
  an unexpected status, an invalid `OutputSchema` or `TargetQuality`, and
  an unreadable or malformed history file. A panic inside a parallel
  attempt crashes the process.
- `New` returns an error for any missing `Config` field. Every field is
  required and none has a default.

## History file

`Config.HistoryPath` points to a JSON array with one object per generation
outcome. Each object holds the timestamp, model, generation id, whether
the rating was automatic or manual, the quality, the request's target
quality, latency, and a reason for failures and manual ratings. Writes are serialized within the
process and protected with a file lock across processes. The caller owns
where this file lives and whether it is committed.
