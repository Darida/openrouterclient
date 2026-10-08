#!/bin/sh
# Usage: bin/models.sh [--exclude=<model-id>,<model-id>,...] [--min-intelligence-index=<0-100>]
# Prints the free and paid models a generation would pick from for a fixed
# sample request, ignoring history; logs go to stderr. With --exclude, lists
# one pool over every model minus those IDs, as bin/generate.sh picks. With
# --min-intelligence-index, drops models below that index or without one.
set -eu

usage() {
    echo "usage: bin/models.sh [--exclude=<model-id>,<model-id>,...] [--min-intelligence-index=<0-100>]" >&2
    exit 2
}

EXCLUDE=""
MIN_INDEX=""
for arg in "$@"; do
    case "$arg" in
        --exclude=?*) [ -z "$EXCLUDE" ] || usage; EXCLUDE="$arg" ;;
        --min-intelligence-index=?*) [ -z "$MIN_INDEX" ] || usage; MIN_INDEX="$arg" ;;
        *) usage ;;
    esac
done
REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"

# Unset options expand to nothing, so the Go program sees only those given.
exec go -C "$REPO_ROOT" run ./src/cmd/models ${EXCLUDE:+"$EXCLUDE"} ${MIN_INDEX:+"$MIN_INDEX"}
