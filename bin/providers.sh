#!/bin/sh
# Usage: bin/providers.sh --model=<model-id> --input-tokens=<n> --output-tokens=<n>
# Picks the cheapest provider endpoint of one model able to serve a request of
# that size and prints it with its prices and estimated cost as JSON; logs go
# to stderr.
set -eu

usage() {
    echo "usage: bin/providers.sh --model=<model-id> --input-tokens=<n> --output-tokens=<n>" >&2
    exit 2
}

MODEL=""
INPUT_TOKENS=""
OUTPUT_TOKENS=""
for arg in "$@"; do
    case "$arg" in
        --model=?*) [ -z "$MODEL" ] || usage; MODEL="$arg" ;;
        --input-tokens=?*) [ -z "$INPUT_TOKENS" ] || usage; INPUT_TOKENS="$arg" ;;
        --output-tokens=?*) [ -z "$OUTPUT_TOKENS" ] || usage; OUTPUT_TOKENS="$arg" ;;
        *) usage ;;
    esac
done
if [ -z "$MODEL" ] || [ -z "$INPUT_TOKENS" ] || [ -z "$OUTPUT_TOKENS" ]; then
    usage
fi
REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"

exec go -C "$REPO_ROOT" run ./src/cmd/providers "$MODEL" "$INPUT_TOKENS" "$OUTPUT_TOKENS"
