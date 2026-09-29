#!/bin/sh
# Usage: bin/rate.sh --id=<generation-id> --quality=<high|medium|low> --reason=<why>
# Records a manual rating in the same history bin/generate.sh writes.
set -eu

usage() {
    echo "usage: bin/rate.sh --id=<generation-id> --quality=<high|medium|low> --reason=<why>" >&2
    exit 2
}

ID=""
QUALITY=""
REASON=""
for arg in "$@"; do
    case "$arg" in
        --id=*) ID="${arg#--id=}" ;;
        --quality=*) QUALITY="${arg#--quality=}" ;;
        --reason=*) REASON="${arg#--reason=}" ;;
        *) echo "error: unknown argument: $arg" >&2; usage ;;
    esac
done
if [ -z "$ID" ] || [ -z "$QUALITY" ] || [ -z "$REASON" ]; then
    usage
fi
REPO_ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
. "$REPO_ROOT/bin/history-path.sh"

exec go -C "$REPO_ROOT" run ./src/cmd/rate --history="$HISTORY_PATH" --id="$ID" --quality="$QUALITY" --reason="$REASON"
