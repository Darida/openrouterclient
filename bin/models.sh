#!/bin/sh
# Usage: bin/models.sh --tag=<history-tag> [--paid] <requirements.json>
# Prints the models bin/generate.sh would pick from, one per line; logs go to stderr.
set -eu

usage() {
    echo "usage: bin/models.sh --tag=<history-tag> [--paid] <requirements.json>" >&2
    exit 2
}

TAG=""
PAID=""
INPUT=""
for arg in "$@"; do
    case "$arg" in
        --tag=*) TAG="${arg#--tag=}" ;;
        --paid) PAID="--paid" ;;
        -*) echo "error: unknown option: $arg" >&2; usage ;;
        *)
            [ -z "$INPUT" ] || usage
            INPUT="$arg"
            ;;
    esac
done
if [ -z "$TAG" ] || [ -z "$INPUT" ]; then
    usage
fi
INPUT="$(realpath "$INPUT")"
REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
. "$REPO_ROOT/bin/history-path.sh"
mkdir -p "$HISTORY_DIR"

exec go -C "$REPO_ROOT" run ./src/cmd/models --input="$INPUT" --history="$HISTORY_PATH" --tag="$TAG" $PAID
