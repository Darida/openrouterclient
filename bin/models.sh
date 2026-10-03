#!/bin/sh
# Usage: bin/models.sh [--exclude=<model-id>,<model-id>,...]
# Prints the free and paid models a generation would pick from for a fixed
# sample request, ignoring history; logs go to stderr. With --exclude, lists
# one pool over every model minus those IDs, as bin/generate.sh picks.
set -eu

usage() {
    echo "usage: bin/models.sh [--exclude=<model-id>,<model-id>,...]" >&2
    exit 2
}

if [ "$#" -gt 1 ]; then
    usage
fi
REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
if [ "$#" -eq 0 ]; then
    exec go -C "$REPO_ROOT" run ./src/cmd/models
fi
case "$1" in
    --exclude=?*) ;;
    *) usage ;;
esac

exec go -C "$REPO_ROOT" run ./src/cmd/models "$1"
