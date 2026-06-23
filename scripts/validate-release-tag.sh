#!/usr/bin/env bash
# Validate an oma release tag and report whether it is a prerelease.
set -euo pipefail

tag="${1:?usage: validate-release-tag.sh <tag>}"

fail() {
  echo "ERR invalid release tag '$tag': $1" >&2
  exit 2
}

if [[ ! "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$ ]]; then
  fail "want vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-prerelease without build metadata"
fi

prerelease="${BASH_REMATCH[5]:-}"
if [[ -n "$prerelease" ]]; then
  IFS='.' read -r -a ids <<<"$prerelease"
  for id in "${ids[@]}"; do
    if [[ "$id" =~ ^[0-9]+$ && "${#id}" -gt 1 && "$id" == 0* ]]; then
      fail "numeric prerelease identifier '$id' must not contain leading zeroes"
    fi
  done
  echo "is_prerelease=true"
else
  echo "is_prerelease=false"
fi
