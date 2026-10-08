#!/bin/sh
# Usage: bin/estimate.sh --input-tokens=<n> --output-tokens=<n> [--exclude=<model-id>,...] [--min-intelligence-index=<0-100>]
# Picks one model as a first bin/generate.sh attempt would, ignoring history,
# and prints it with its prices and estimated cost as JSON; logs go to stderr.
set -eu

usage() {
    echo "usage: bin/estimate.sh --input-tokens=<n> --output-tokens=<n> [--exclude=<model-id>,...] [--min-intelligence-index=<0-100>]" >&2
    exit 2
}

INPUT_TOKENS=""
OUTPUT_TOKENS=""
EXCLUDE=""
MIN_INDEX=""
for arg in "$@"; do
    case "$arg" in
        --input-tokens=?*) [ -z "$INPUT_TOKENS" ] || usage; INPUT_TOKENS="$arg" ;;
        --output-tokens=?*) [ -z "$OUTPUT_TOKENS" ] || usage; OUTPUT_TOKENS="$arg" ;;
        --exclude=?*) [ -z "$EXCLUDE" ] || usage; EXCLUDE="$arg" ;;
        --min-intelligence-index=?*) [ -z "$MIN_INDEX" ] || usage; MIN_INDEX="$arg" ;;
        *) usage ;;
    esac
done
if [ -z "$INPUT_TOKENS" ] || [ -z "$OUTPUT_TOKENS" ]; then
    usage
fi
REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"

# Unset options expand to nothing, so the Go program sees only those given.
exec go -C "$REPO_ROOT" run ./src/cmd/estimate "$INPUT_TOKENS" "$OUTPUT_TOKENS" ${EXCLUDE:+"$EXCLUDE"} ${MIN_INDEX:+"$MIN_INDEX"}
