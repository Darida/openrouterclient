#!/bin/sh
# Usage: bin/generate.sh --key=<openrouter-api-key> --tag=<history-tag> <requirements.json>
# Prints the generated result as JSON on stdout; logs go to stderr. Never reviews.
set -eu

usage() {
    echo "usage: bin/generate.sh --key=<openrouter-api-key> --tag=<history-tag> <requirements.json>" >&2
    exit 2
}

KEY=""
TAG=""
INPUT=""
for arg in "$@"; do
    case "$arg" in
        --key=*) KEY="${arg#--key=}" ;;
        --tag=*) TAG="${arg#--tag=}" ;;
        -*) echo "error: unknown option: $arg" >&2; usage ;;
        *)
            [ -z "$INPUT" ] || usage
            INPUT="$arg"
            ;;
    esac
done
if [ -z "$KEY" ] || [ -z "$TAG" ] || [ -z "$INPUT" ]; then
    usage
fi
INPUT="$(realpath "$INPUT")"
REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
. "$REPO_ROOT/bin/history-path.sh"
mkdir -p "$HISTORY_DIR"

# TODO: remove once callers stop sending an empty outputValidationRules.
# A non-empty one is kept, so the reader rejects it as an unknown field.
REQUEST="$(mktemp)"
trap 'rm -f "$REQUEST"' EXIT
jq 'if .outputValidationRules == "" then del(.outputValidationRules) else . end' "$INPUT" > "$REQUEST"

# Passed by environment so the key stays out of the Go program's argv.
OPENROUTER_API_KEY="$KEY" go -C "$REPO_ROOT" run ./src/cmd/generate --input="$REQUEST" --history="$HISTORY_PATH" --tag="$TAG" --rate-script="$REPO_ROOT/bin/rate.sh"
