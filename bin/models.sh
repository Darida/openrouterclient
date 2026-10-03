#!/bin/sh
# Usage: bin/models.sh [requirements.json]
# Prints the free and paid models a generation would pick from, ignoring
# history; logs go to stderr. With a requirements file, lists the pool
# bin/generate.sh would pick from for it, minus its excludedModels.
set -eu

if [ "$#" -gt 1 ]; then
    echo "usage: bin/models.sh [requirements.json]" >&2
    exit 2
fi
REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
if [ "$#" -eq 0 ]; then
    exec go -C "$REPO_ROOT" run ./src/cmd/models
fi

# TODO: remove once callers stop sending an empty outputValidationRules.
REQUEST="$(mktemp)"
trap 'rm -f "$REQUEST"' EXIT
jq 'if .outputValidationRules == "" then del(.outputValidationRules) else . end' "$1" > "$REQUEST"

go -C "$REPO_ROOT" run ./src/cmd/models --input="$REQUEST"
