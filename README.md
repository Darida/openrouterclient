# openrouterclient

A Go library for OpenRouter text generation. You send a prompt and get
back JSON that matches a schema you supply. It is ported from
`assetloom/src/clients/openrouter` and supports text generation only.

Each call goes through three steps:

1. **Generate.** The prompt is sent to `openrouter/free` with a strict
   `json_schema` response format. Models the local history marks as
   unreliable are excluded.
2. **Review.** A follow-up request continues the same conversation. It
   sends the generated answer back, followed by a fixed review
   instruction plus the caller's `ReviewRulesPrompt`. The reply must match
   the fixed `ReviewVerdict` schema. A rejected result is regenerated from
   scratch.
3. **Track.** Every generation's outcome is recorded against the model
   that produced it: success, failure, timeout, automatic rejection, or
   manual up/down vote (`Client.Rate`). Models that fail too often are
   excluded from later calls.

## Layout

- The root package holds the public contract only: `Client`, `Config`,
  the request and response types, `ReviewVerdict`, `Vote`, and
  `AttemptsExhaustedError`, plus a thin `New` that wires up `internal/`.
- `internal/` holds all behavior, split into these packages:
  - `chat`: builds the wire payload and parses responses.
  - `hedge`: runs staggered parallel attempts.
  - `generationlog`: polls OpenRouter's `/api/v1/generation` log.
  - `history`: stores outcomes and computes exclusions.
  - `review`: holds the fixed review prompt and schema.
  - `engine`: orchestrates the generate, review, and retry loop.

  These packages never import the root package. The root converts
  between its public types and theirs.

## Behavior

- **Hedged attempts.** The first attempt starts immediately. Another
  attempt starts when the previous one fails, or when it has been pending
  60s with no response. There are at most 3 attempts. Once one wins, the
  others get whichever is later of 30s past the win or 60s of their own
  runtime, and are then aborted. Each attempt also has a hard timeout.
  Both generation and review calls are hedged this way.
- **Attribution.** Every attempt OpenRouter accepted, which is known from
  its `X-Generation-Id` header, is attributed to a real model. For an
  attempt that times out, is aborted, or breaks mid-body, the model is
  resolved by polling the generation log for that id. If the log doesn't
  resolve within its window, the process panics. A history entry never
  records a guessed or placeholder model. A request OpenRouter never
  accepted has no model, so it is reported in the returned error but not
  written to history.
- **Review rounds.** When the review rejects a result, that generation is
  recorded as `auto_rejected` and a fresh generation runs, up to a fixed
  number of rounds. Failed or timed-out review attempts are recorded
  against the reviewer's model.
- **Exclusion.** A model is excluded if its count of failures, timeouts,
  rejections, or successes that took 60s or longer exceeds any of these
  limits: more than 3 today (UTC), more than 6 in the last 7 days, more
  than 12 in the last 30 days, or more than 24 in total. A down vote from
  `Rate` counts toward the same limits.

## Failure policy

This library never falls back and never swallows a failure.

- `GenerateText` returns an error in exactly one case: every allowed
  attempt failed, timed out, or was rejected (`*AttemptsExhaustedError`,
  which carries every attempt's raw reason).
- Anything unexpected panics, with the full raw body in the message. That
  includes a response with no `model`, content that is not JSON despite
  the strict schema, a generation log that never resolves, and an
  unreadable or malformed history file.
- `New` returns an error for any missing `Config` field. Every field is
  required and none has a default.

## History file

`Config.HistoryPath` points to a JSON array with one object per generation
outcome. Each object holds the timestamp, model, generation id, outcome,
latency, and, for votes, the reason. Writes are serialized within the
process and protected with a file lock across processes. The caller owns
where this file lives and whether it is committed.
