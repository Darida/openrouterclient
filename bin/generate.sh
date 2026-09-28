#!/bin/sh
# Usage: bin/generate.sh --key=<openrouter-api-key> --tag=<history-tag> <requirements.json>
# Prints the reviewed result as JSON on stdout; logs go to stderr.
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

# Per-user state, never in the repo: see the XDG Base Directory spec's state dir.
HISTORY_DIR="$HOME/.local/state/openrouterclient"
mkdir -p "$HISTORY_DIR"

# Passed by environment so the key stays out of the Go program's argv.
OPENROUTER_API_KEY="$KEY" exec go -C "$REPO_ROOT" run ./src/cmd/generate --input="$INPUT" --history="$HISTORY_DIR/history.json" --tag="$TAG"
