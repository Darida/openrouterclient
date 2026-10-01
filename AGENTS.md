# openrouterclient

**Human commander:** Darya Prokurat.

See `README.md` for what this library does, its package layout, and its
failure policy. None of that is repeated here. Baseline agent behavior
(personality/alignment, no `cd`, README-first reading,
fail-fast/no-fallbacks, git and testing conventions) is added separately
at launch and isn't repeated here either.

---

## Git

Run `git/push-all "<message>"` after every small milestone. It's a
symlink to the shared checkpoint script at `ai-cli-config/git/push-all`,
which commits and pushes everything that changed, on whatever branch the
repo is already on. You never need to check or switch branches yourself.

`push-all` runs `git add -A`, which takes the whole working tree. You
can't pick individual files. Before your next `push-all` after adding a
tool that generates build output (`bin/`, coverage reports, `.cache/`),
update `.gitignore`.

Intermediate commits on the working branch are always fine. The branch
history is meant to be a sequence of checkpoints, not one atomic unit. If
a checkpoint reveals a small, non-blocking side effect, fix it in a
following `push-all` rather than stopping mid-task.

## Testing, builds, and codegen

Never hand-run `go build`, `go test`, `go vet`, or `go generate` yourself,
for any reason. That includes "verifying before I commit". The pre-push
hook (`git/hooks/pre-push`, which delegates to
`ai-cli-config/git/hooks/lib/`) runs `go-generate`, `go-test`,
`cassette-check`, and `require-clean` on every push. `git/push-all` is
your only verification tool.

The one exception: once `git/push-all` has reported a specific failing
test or build step, you may hand-run that same command while iterating on
a fix. Go back to `git/push-all` as the real check once you believe it's
fixed. A hand-run pass never substitutes for it.

Tests never call the live OpenRouter API. They run against `httptest`
fakes with millisecond timings, because the hook gives the whole test run
30s.

## Failure policy is the product

This library exists to fail loudly. Never add a retry, default, fallback
model, placeholder value (`"unknown"`), swallowed error, or
best-effort-and-continue path that `README.md` doesn't already describe.
If an OpenRouter response doesn't match what the code expects, abort the
call with a `*model.UnexpectedError` whose message names the file holding
the full raw body. Never panic: every failure propagates as an error. Never
quote a raw reply in a log or error message. An unexpected shape is a bug
to research and fix, not a case to paper over.

## Tooling gaps never drive design

A tool, package, or include path being present or missing in the current
environment is never a reason to pick a simpler design over the correct
one. If something the right design needs is missing, stop and ask the
human to fix the environment. Don't quietly route around it with a weaker
design.

## Comments

A comment may describe the behavior or contract of the file it's in, even
if a future change to that file will need to update it. A comment must
not assert a fact about a *different* file's current state. Nothing forces
whoever changes that other file to come fix this comment.

Doc comments on exported identifiers in `src/api/` and `src/model/` are
the caller's only documentation surface. They must stand alone and never
point the reader to `src/internal/`.

## Automated post-push code review

Every `push-all` triggers a separate AI review of the diff
(`ACTION_REQUIRED`/`LGTM`). It's advisory and runs apart from the
test/build gate. Neither blindly comply with its findings nor blindly
dismiss them. Judge each finding on its merits against the actual current
file content.

The rules (`ai-cli-config/human_tools/review.prompt.md`) are fixed and
written by humans. You may not reject a rule because it's stricter than
general idiom. You may reject a specific finding only by proving it's a
false positive against the rule's literal text: quote the clause and show
the flagged code doesn't violate it. "This matches idiomatic Go" is not a
valid rejection. Provenance ("I only changed two words", "a sibling
function already does this") is not a valid reason either way.

If a finding is factually wrong about what the file contains, verify
against the file and reject it outright. If a comment exists only to stop
a name from being misread, rename the identifier when that has no
meaningful compatibility cost.

Apply the findings that survive evaluation and decline the rest with
specific reasoning. Land the result as its own small `push-all`
checkpoint.
