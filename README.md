# openrouterclient

A Go library for OpenRouter text generation. You send a prompt and get
back JSON that matches a schema you supply. It is ported from
`assetloom/src/clients/openrouter` and supports text generation only.

Each call goes through these steps:

1. **Generate.** Each attempt picks a model at random from the models of
   the request's `ModelTier` that support structured output, minus those
   whose context length can't hold the estimated prompt tokens (chars ÷ 4,
   schema included) plus `MaxOutputTokens`, minus those
   the history marks as unreliable (none when disabled), and minus the request's
   `ExcludedModels` (exact IDs, applied to review too), and sends the prompt to it with a
   strict `json_schema` response format and `MaxOutputTokens` (default
   10,000) as `max_tokens`. For the paid tier, candidates are first
   narrowed to the cheapest: each is priced as prompt chars ÷ 4 × prompt
   price plus `MaxOutputTokens` × completion price, and only those at most
   10% above the 30th-percentile estimate remain. The model list comes from
   OpenRouter's catalog, cached in memory for an hour.
2. **Review.** A separate request asks a model to review the output as
   someone else's work, never as its own reply. A fixed system prompt
   casts it as a skeptical senior reviewer checking a junior's work and
   carries the caller's `OutputValidationRules`. One user message then
   holds the caller's prompt as the junior's task, and a second holds the
   generated output. Each rule may
   state a bad score for violating it; a rule that states none counts 1.
   The reply must match the fixed `ReviewVerdict` schema: a list of
   violations plus their total bad score. The reviewer
   reports only violations of the listed rules, never suggestions or
   observations, and only ones it can quote. Each offending instance is its
   own violation, carrying the rule, a verbatim evidence excerpt of about
   five words, an explanation, a recommended action, and its rule's bad
   score. The total is the sum of every violation's bad score, so a rule
   broken twice counts twice. A total of 0 is high, up
   to the request's `ReviewToleranceThreshold` is medium, and above it is
   low. A total that is negative, differs from the sum of the violations'
   bad scores, or is 0 despite violations counts as reviewer output that
   fails the schema. With empty
   `OutputValidationRules`, review and correction are skipped and the first
   schema-valid output is returned, recorded in history as high.
   `ReviewToleranceThreshold` is required (at least 1) with rules and must
   be 0 without them. Output that fails the schema
   is a failed attempt and never reaches review.
3. **Correct.** If that quality is below `TargetQuality`, a correction
   request sends the original prompt, the previous reply, and the review's
   violations, and asks the model to fix them. The correction is then
   reviewed again. Each correction sends only the latest reply and its
   violations. `MaxCorrections` caps the corrections and is required with
   rules: 0 reviews the first output without correcting it, a negative
   value panics, and it must be 0 without rules.
4. **Track.** Every generation's quality is recorded against the model
   that produced it: the automatic review's rating, `unusable` for
   failures, timeouts, and aborts, and any manual rating from
   `Client.Rate`. `Client.Rate` also takes a review's generation id. A
   review rated low counts as a failure against the reviewer's model, and
   the automatic rating it gave is cleared, so that generation no longer
   counts for or against its model. Models that fail too often are
   excluded from later calls.

## Layout

The repo root holds only module and tooling files (`go.mod`, `git/`,
`bin/`, docs). All Go code lives under `src/`:

- `src/model/` holds data types only, with no logic: the request and
  response types, `ReviewVerdict`, `Quality`, and `AttemptsExhaustedError`.
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
  - `engine`: orchestrates the generate, review, and correct loop.

  These packages may import `src/model/` but never `src/api/`.
- `src/cmd/generate/` and `src/cmd/models/` are the command-line programs
  behind `bin/generate.sh` and `bin/models.sh`; `src/cmd/internal/` holds
  what they share, such as reading the requirements file.

## Behavior

- **Hedged attempts.** The first attempt starts immediately. Another
  attempt starts when the previous one fails, or when it has been pending
  for the request's `Timeout` with no response. `Timeout` defaults to 60s
  when it's 0; `Client.GenerateText` fills that in, and a negative value
  panics. There are at most 3 attempts. Every attempt gets `Timeout` plus
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
  target of the request that produced it, or if it took at least that
  request's `Timeout`.
  Every entry carries its request's `Tag`; for a new request, a failure
  under the same tag counts 1 and one under another tag counts 0.5. A
  model is excluded once its weighted failures exceed any of these limits: more
  than 3 today (UTC), more than 6 in the last 7 days, more than 12 in the
  last 30 days, or more than 24 in total. Excluded models are never
  picked; if every candidate is excluded, the call panics.

## Command line

```sh
bin/generate.sh --key=YOUR_OPENROUTER_KEY --tag=bakery [--paid] bin/example-requirements.json
```

The script takes one requirements file with the fields `prompt`,
`outputSchema` (`name` and `schema`), `outputValidationRules`,
`reviewToleranceThreshold` (required with rules, absent without),
`targetQuality`, and optionally `timeoutSeconds`, `maxOutputTokens`,
`excludedModels` (a list of exact model IDs), and
`maxCorrections` (required with rules, 0 or more; absent without). `--paid` switches from
free models to the cheapest paid ones. See `bin/example-requirements.json`. The OpenRouter API
key is required as `--key=...`, and the request's history tag as
`--tag=...`. History goes to
`~/.local/state/openrouterclient/history.json`, which is per user and per
machine and never inside the repo.

It prints the reviewed result as JSON on stdout and logs on stderr. The
last log lines hold ready-to-paste `bin/rate.sh` commands: one rates the
result low with the reason "human rejected output", and one rates its
review low with the reason "human rejected review". If every attempt
fails, it logs a "human rejected review" command for each round's review
and then prints the failed attempts on stderr and exits 1.

```sh
bin/models.sh --tag=bakery [--paid] bin/example-requirements.json
```

`bin/models.sh` takes the same requirements file and prints, one per line,
the models a first generation attempt would pick from: the tier's
structured-output models that fit the request's context, minus the
history's exclusions for `--tag` and the file's `excludedModels`, narrowed
to the cheapest pool with `--paid`. It sends no chat request and needs no
API key, since the model catalog is public.

```sh
bin/rate.sh --id=GENERATION_ID --quality=high|medium|low --reason=WHY
```

`bin/rate.sh` records a manual rating in the same history file. It needs
no API key, since rating never contacts OpenRouter.

## Failure policy

This library never falls back and never swallows a failure.

- `GenerateText` returns an error in exactly two cases: every allowed
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
  wrong, so it panics.
- Log lines, error messages, and panics never quote a raw OpenRouter reply.
  With `Config.Replies` set to `LocalReplies()`, each reply they refer to
  is saved in full to its own file under `openrouterclient/` in the system
  temp directory (`$TMPDIR`, else `/tmp`), and the message names that file.
  With `DisabledReplies()`, nothing is saved: the message names the reply's
  OpenRouter generation id, to look it up on OpenRouter, or says the body
  wasn't saved when there is no id (a catalog failure, or a chat response
  without `X-Generation-Id`). That covers invalid output, refusals, and for a
  rejected output, both the output and its review.
- Anything unexpected panics, naming the file with the full raw body. That
  includes a 200 body that isn't JSON, a non-200 body that isn't an
  OpenRouter error object, a 200 response with no `X-Generation-Id`, a
  model list that can't be fetched, lists a candidate without a context
  length, or leaves no candidate after the context-length filter and
  exclusions, an `ExcludedModels` ID that isn't a structured-output model
  of the request's tier in the catalog, an invalid `OutputSchema`, `TargetQuality`, `ReviewToleranceThreshold`, or `Tag`, and an
  unreadable or malformed history file. A panic inside a parallel
  attempt crashes the process.
- `New` returns an error for any missing `Config` field. Every field is
  required and none has a default; `History` and `Replies` each name their
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
quality, tag, and timeout, latency, and a reason for failures and manual ratings.
The winning reviewer's entry also names the generation it reviewed. Rating
that reviewer low rewrites the reviewed generation's automatic entry in
place, clearing its quality and noting why in its reason.
Writes are serialized within the process and protected with a file lock
across processes. A file with entries that lack a tag or timeout panics on load. The caller owns
where this file lives and whether it is committed.
