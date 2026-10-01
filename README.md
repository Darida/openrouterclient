# openrouterclient

A Go library for OpenRouter text generation. You send a prompt and get
back JSON that matches a schema you supply. It is ported from
`assetloom/src/clients/openrouter` and supports text generation only.

`Client` has three calls. `Generate` runs step 1 alone. `Review` runs
step 2 alone, on content the caller supplies. `GenerateReviewed` runs
steps 1 to 3. All three track outcomes as in step 4.

1. **Generate.** Each attempt picks a model at random from the request's
   `ModelSelection` (below), minus models whose context length can't hold
   the estimated prompt tokens (chars ÷ 4, schema included) plus
   `MaxOutputTokens`, minus models the history marks as unreliable (none when
   disabled). It sends the prompt with a strict `json_schema` response
   format and `MaxOutputTokens` (default 10,000) as `max_tokens`. Output
   that fails the schema is a failed attempt. `Generate` returns the first
   schema-valid output and records it in history as high against its
   `TargetQuality`, so only a later manual rating can count it against its
   model.
2. **Review.** A separate request asks a model to review the output as
   someone else's work, never as its own reply. A fixed system prompt
   casts it as a skeptical senior reviewer checking a junior's work and
   carries the caller's `ReviewCriteria.Rules` (required). One user message
   then holds the task (the prompt, or `ReviewRequest.Task`) as the junior's
   task, and a second holds the output (the generation, or
   `ReviewRequest.Content`, which must be valid JSON). Each rule may
   state a bad score for violating it; a rule that states none counts 1.
   The reply must match the fixed `ReviewVerdict` schema: a list of
   violations plus their total bad score. The reviewer
   reports only violations of the listed rules, never suggestions or
   observations, and only ones it can quote. Each offending instance is its
   own violation, carrying the rule, a verbatim evidence excerpt of about
   five words, an explanation, a recommended action, and its rule's bad
   score. The total is the sum of every violation's bad score, so a rule
   broken twice counts twice. A total of 0 is high, up
   to `ReviewCriteria.ToleranceThreshold` (at least 1) is medium, and above
   it is low. A total that is negative, differs from the sum of the
   violations' bad scores, or is 0 despite violations counts as reviewer
   output that fails the schema. `Review` returns the verdict whatever its
   quality.
3. **Correct.** If that quality is below `TargetQuality`, a correction
   request sends the original prompt, the previous reply, and the review's
   violations, and asks the model to fix them. The correction is then
   reviewed again. Each correction sends only the latest reply and its
   violations. `MaxCorrections` caps the corrections: 0 reviews the first
   output without correcting it, and a negative value is an error.
   `GenerateReviewedRequest` takes separate `GenerationModels` and
   `ReviewModels` selections.
4. **Track.** Every generation's quality is recorded against the model
   that produced it: the automatic review's rating, `unusable` for
   failures, timeouts, and aborts, and any manual rating from
   `Client.Rate`. `Client.Rate` also takes a review's generation id. A
   review rated low counts as a failure against the reviewer's model, and
   the automatic rating it gave is cleared, so that generation no longer
   counts for or against its model. A standalone `Review` rates no
   generation in history, so rating it low clears nothing. Models that
   fail too often are excluded from later calls.

## Model selection

A `ModelSelection` sets at most one of its fields; setting more than one is
an error. Setting none, like an empty `Denied`, means every structured-output
model, free and paid.

- `Tier`: the free (`:free`) or paid structured-output models.
- `Allowed`: only these exact model IDs, free or paid.
- `Denied`: every structured-output model, free and paid, except these.

Whatever the selection, candidates are narrowed to the cheapest last: each
is priced as prompt chars ÷ 4 × prompt price plus `MaxOutputTokens` ×
completion price, and only those at most 10% above the 30th-percentile
estimate remain. Free models price at $0, so they always remain, and when at
least 30% of the candidates are free, only free models remain.

An `Allowed` or `Denied` ID that isn't a structured-output model in the
catalog is an error. The model list comes from OpenRouter's catalog, cached
in memory for an hour. Catalog entries priced at exactly `-1` (prompt or
completion) are routers, not models, and are never candidates; any other
negative or unparseable price is an unexpected error.

## Layout

The repo root holds only module and tooling files (`go.mod`, `git/`,
`bin/`, docs). All Go code lives under `src/`:

- `src/model/` holds data types only, with no logic: the request and
  response types, `ModelSelection`, `ReviewVerdict`, `Quality`, and the
  error types `AttemptsExhaustedError` and `UnexpectedError`.
- `src/api/` holds the client contract: `Client`, `Config`, the `History`
  and `Replies` choices, and a thin `New` that wires up `src/internal/`.
- `src/internal/` holds all behavior, split into these packages:
  - `chat`: builds the wire payload and parses responses.
  - `hedge`: runs staggered parallel attempts.
  - `catalog`: caches OpenRouter's model list and names the candidates.
  - `cost`: estimates a request's cost per model and keeps the cheapest.
  - `history`: stores outcomes and computes exclusions, or does nothing
    when disabled.
  - `review`: holds the fixed review prompt and schema.
  - `quality`: ranks qualities and derives one from a review's total bad score.
  - `schema`: validates output against the requested JSON Schema.
  - `replyfile`: saves raw replies to files so messages can name them, or
    names the generation id instead when disabled.
  - `engine`: runs each `Client` call: generate, review, or the generate,
    review, and correct loop.

  These packages may import `src/model/` but never `src/api/`.
- `src/cmd/generate/`, `src/cmd/models/`, and `src/cmd/rate/` are the
  command-line programs behind `bin/generate.sh`, `bin/models.sh`, and
  `bin/rate.sh`; `src/cmd/internal/` holds the requirements-file reader
  `bin/generate.sh` uses.

## Behavior

- **Hedged attempts.** The first attempt starts immediately. Another
  attempt starts when the previous one fails, or when it has been pending
  for `Config.Timeout` with no response. `Config.Timeout` is required and
  has no default. There are at most 3 attempts. Every attempt gets `Timeout` plus
  1s, just past the point at which a success already counts as a failure,
  whether or not another attempt has won; one still running then is a
  timeout. Both generation and review calls are hedged this way. A call
  moves on the moment an attempt wins; stragglers finish and are recorded
  in the background, and `Client.Close` waits for that before exit.
- **Attribution.** Every attempt names its model, so a failure is recorded
  against the model that attempt asked. A request OpenRouter never
  accepted, which has no `X-Generation-Id`, never reached the model, so it
  is reported in the returned error but not written to history. A refusal
  (below) is the one exception.
- **Refusals.** A 400, 404, or 422 means the attempt's model or provider
  refused the request, for example over a schema keyword it doesn't
  support or an account data policy that excludes its only endpoint. The
  attempt fails as `refused`, and the verdict waits for the race: if
  another attempt won, the request was servable, so the refusal is recorded
  against the model as `unusable`, with or without a generation id. If
  nothing won, the refusal is no evidence about any model: it is returned
  in `AttemptsExhaustedError`, naming the file with its full raw body (see
  Failure policy), and not recorded.
- **Provider errors.** OpenRouter sends a 200 status as soon as it accepts
  a request, so an upstream failure can arrive as an `error` object inside
  a 200 body as well as with a transient non-200 status. A 200 whose body
  is a provider error other than a rate limit arrives within a second,
  before any generation exists, so the same attempt resends it after a 1s
  pause. That doesn't use up an attempt or count against the model; the
  attempt's own timeout still bounds it. A rate limit (429) or a transient
  non-200 status counts as a failure of the attempt's model. Requests go
  out with provider fallbacks disabled, so a failing provider fails that
  attempt instead of being silently retried elsewhere.
- **Correction rounds.** There are at most 1 + `MaxCorrections` rounds,
  counting the first generation. Failed or timed-out review attempts are recorded against the
  reviewer's model. Output that doesn't match the schema, whether from the
  generator or the reviewer, is rated `unusable`.
- **Exclusion.** A result counts as a failure if its quality is below the
  target of the request that produced it, or if it took at least the
  `Config.Timeout` it ran under.
  Every generation entry carries `Config.Tag` + `-generate` and every
  review entry `Config.Tag` + `-review`. When picking models for a call, a
  failure under that call's tag counts 1 and one under any other tag
  counts 0.5, so a model's failures as a reviewer weigh half when it is
  picked as a generator, and the other way around. A
  model is excluded once its weighted failures exceed any of these limits: more
  than 3 today (UTC), more than 6 in the last 7 days, more than 12 in the
  last 30 days, or more than 24 in total. Excluded models are never
  picked; if every candidate is excluded, the call returns an unexpected error.

## Command line

```sh
bin/generate.sh --key=YOUR_OPENROUTER_KEY --tag=bakery bin/example-requirements.json
```

The script calls `Client.Generate` only; it never reviews. It takes one
requirements file with the fields `prompt`, `outputSchema` (`name` and
`schema`), `targetQuality`, and optionally `maxOutputTokens` and
`excludedModels` (exact model IDs never asked). It picks from every
structured-output model, free and paid, minus `excludedModels`, narrowed to
the cheapest like any selection. The script drops an empty `outputValidationRules` before reading the
file; a non-empty one is an unknown field, since the script never reviews. See
`bin/example-requirements.json`. The OpenRouter API key is required as
`--key=...`, and the client's history tag as `--tag=...`. `Config.Timeout`
is fixed at 60s. History goes to
`~/.local/state/openrouterclient/history.json`, which is per user and per
machine and never inside the repo.

It prints the generated result as JSON on stdout and logs on stderr. The
last log line holds a ready-to-paste `bin/rate.sh` command that rates the
result low with the reason "human rejected output". If every attempt
fails, it prints the failed attempts on stderr and exits 1.

```sh
bin/models.sh
```

`bin/models.sh` takes no arguments. It runs the model-selection pipeline
for a fixed sample request (a one-line prompt with the default
`MaxOutputTokens`) against each tier and prints the free
candidates, then the cheapest paid pool, one per line under a `free:` and
a `paid:` heading. It ignores history, so no model is excluded. It sends
no chat request and needs no API key, since the model catalog is public.

```sh
bin/rate.sh --id=GENERATION_ID --quality=high|medium|low --reason=WHY
```

`bin/rate.sh` records a manual rating in the same history file. It needs
no API key, since rating never contacts OpenRouter.

## Failure policy

This library never falls back and never swallows a failure.

- `Generate`, `Review`, and `GenerateReviewed` return an error in exactly
  three cases: something unexpected happened (`*UnexpectedError`, below), every allowed
  attempt failed, timed out, or fell below `TargetQuality`
  (`*AttemptsExhaustedError`, which carries every attempt's reason, and
  for each output the review rejected, that output and its review; its
  message says the review rejected the output if any attempt fell below
  `TargetQuality`, and that all attempts failed otherwise), or
  the caller's context ended (`ctx.Err()`). Attempts cut short by the
  caller's context are no evidence about any model, so they are not
  recorded.
- Only transient chat statuses (408, 429, 500, 502, 503, 504) count as a
  model failure, plus refusals (400, 404, 422) when another attempt in the
  same race won. Any other non-200 status means the request or the key is
  wrong, so it aborts the call with an unexpected error.
- Log lines and error messages never quote a raw OpenRouter reply.
  With `Config.Replies` set to `LocalReplies()`, each reply they refer to
  is saved in full to its own file under `openrouterclient/` in the system
  temp directory (`$TMPDIR`, else `/tmp`), and the message names that file.
  With `DisabledReplies()`, nothing is saved: the message names the reply's
  OpenRouter generation id, to look it up on OpenRouter, or says the body
  wasn't saved when there is no id (a catalog failure, or a chat response
  without `X-Generation-Id`). That covers invalid output, refusals, and for a
  rejected output, both the output and its review.
- Nothing panics. Anything unexpected aborts the call with
  `*model.UnexpectedError`, naming the file with the full raw body, and no
  further attempt starts; attempts already in flight are canceled. That
  includes a 200 body that isn't JSON, a non-200 body that isn't an
  OpenRouter error object, a 200 response with no `X-Generation-Id`, a
  model list that can't be fetched, lists a candidate without a context
  length or with an invalid price, or leaves no candidate after the
  context-length filter and exclusions, an `Allowed` or `Denied` ID that
  isn't a structured-output model in the catalog, an invalid request
  (including a `ModelSelection` that sets more than one field, an
  invalid `OutputSchema`, `TargetQuality`, or `ReviewCriteria`, and
  `ReviewRequest.Content` that isn't JSON), and an unreadable or malformed
  history file. A failure while recording a won
  race's stragglers in the background is returned by `Client.Close`.
- `New` returns an error for any missing `Config` field or a
  non-positive `Timeout`. Every field is required and none has a default; `History` and `Replies` each name their
  choice explicitly, local or disabled.

## History

`Config.History` is either `LocalHistory(path)` or `DisabledHistory()`.
Disabled history records nothing and excludes no model, so every call picks
from all candidates regardless of past failures, and `Client.Rate` and
`NewRater` return an error. Use it where no file outlives the process, such
as Cloud Run.

`LocalHistory` points to a JSON array with one object per generation
outcome. Each object holds the timestamp, model, generation id (empty only
for a `refused` outcome), whether
the rating was automatic or manual, the quality, the request's target
quality, the tag and timeout it ran under, latency, and a reason for
failures and manual ratings. `LocalHistory(path)` returns an error for an
empty path or an unreadable or malformed file.
The winning reviewer's entry in `GenerateReviewed` also names the
generation it reviewed. Rating
that reviewer low rewrites the reviewed generation's automatic entry in
place, clearing its quality and noting why in its reason.
Writes are serialized within the process and protected with a file lock
across processes. A file with entries that lack a tag or timeout fails to load. The caller owns
where this file lives and whether it is committed.
