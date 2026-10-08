#!/bin/sh
# Usage: bin/models.sh [--exclude=<model-id>,<model-id>,...] [--min-intelligence-index=<0-100>] [--cost-percentile=<0-100>] [--input-tokens=<n> --output-tokens=<n>]
# Prints the free and paid models a generation would pick from for a fixed
# sample request, ignoring history; logs go to stderr. With --exclude, lists
# one pool over every model minus those IDs, as bin/generate.sh picks. With
# --min-intelligence-index, drops models below that index or without one.
# --cost-percentile sets the cheapest pool's price percentile (default 10th).
# --input-tokens and --output-tokens, given together, price that size instead
# of the sample request, as bin/estimate.sh does.
set -eu

usage() {
    echo "usage: bin/models.sh [--exclude=<model-id>,<model-id>,...] [--min-intelligence-index=<0-100>] [--cost-percentile=<0-100>] [--input-tokens=<n> --output-tokens=<n>]" >&2
    exit 2
}

EXCLUDE=""
MIN_INDEX=""
COST_PERCENTILE=""
INPUT_TOKENS=""
OUTPUT_TOKENS=""
for arg in "$@"; do
    case "$arg" in
        --exclude=?*) [ -z "$EXCLUDE" ] || usage; EXCLUDE="$arg" ;;
        --min-intelligence-index=?*) [ -z "$MIN_INDEX" ] || usage; MIN_INDEX="$arg" ;;
        --cost-percentile=?*) [ -z "$COST_PERCENTILE" ] || usage; COST_PERCENTILE="$arg" ;;
        --input-tokens=?*) [ -z "$INPUT_TOKENS" ] || usage; INPUT_TOKENS="$arg" ;;
        --output-tokens=?*) [ -z "$OUTPUT_TOKENS" ] || usage; OUTPUT_TOKENS="$arg" ;;
        *) usage ;;
    esac
done
REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"

# Unset options expand to nothing, so the Go program sees only those given.
exec go -C "$REPO_ROOT" run ./src/cmd/models ${EXCLUDE:+"$EXCLUDE"} ${MIN_INDEX:+"$MIN_INDEX"} ${COST_PERCENTILE:+"$COST_PERCENTILE"} ${INPUT_TOKENS:+"$INPUT_TOKENS"} ${OUTPUT_TOKENS:+"$OUTPUT_TOKENS"}
