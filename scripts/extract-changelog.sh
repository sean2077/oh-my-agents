#!/usr/bin/env bash
# Extract one version's section from CHANGELOG.md for the GitHub Release
# body: lines between the `## <tag>` heading (exact-prefix match, no
# regex surprises from dots in tags) and the next `## ` heading. A tag
# without a non-empty section fails the release (fail-closed): tagging
# is only legal after the changelog was written for that version.
set -euo pipefail

TAG="${1:?usage: extract-changelog.sh <tag> [changelog-file]}"
FILE="${2:-CHANGELOG.md}"

if [[ ! -f "$FILE" ]]; then
  echo "ERR $FILE not found" >&2
  exit 1
fi

section="$(awk -v tag="$TAG" '
  !found && ($0 == "## " tag || index($0, "## " tag " ") == 1) { found = 1; next }
  found && /^## / { exit }
  found { print }
' "$FILE")"

# Trim leading/trailing blank lines, then require substance.
section="$(printf '%s\n' "$section" | sed -e '/./,$!d' | sed -e ':a' -e '/^\n*$/{$d;N;ba' -e '}')"

if [[ -z "${section//[[:space:]]/}" ]]; then
  echo "ERR no changelog section for tag '$TAG' in $FILE — write the release notes before tagging" >&2
  exit 1
fi

printf '%s\n' "$section"
