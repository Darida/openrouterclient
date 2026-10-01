#!/bin/sh
# Usage: bin/models.sh
# Prints the free and paid models a generation would pick from for a fixed
# sample request, ignoring history; logs go to stderr.
set -eu

if [ "$#" -ne 0 ]; then
    echo "usage: bin/models.sh (takes no arguments)" >&2
    exit 2
fi
REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"

exec go -C "$REPO_ROOT" run ./src/cmd/models
