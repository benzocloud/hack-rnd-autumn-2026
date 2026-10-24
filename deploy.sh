#!/bin/sh
set -eu
command -v gh >/dev/null 2>&1 || { echo 'Установи GitHub CLI (gh) и выполни gh auth login.' >&2; exit 1; }
gh auth status >/dev/null 2>&1 || { gh auth login; }
exec sh "$(dirname "$0")/deploy/frontend/publish.sh"
