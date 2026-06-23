#!/usr/bin/env bash
# Regression tests for the release tag gate used by release.yml and build-release.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VALIDATE="$ROOT/scripts/validate-release-tag.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

expect_ok() {
  local tag="$1"
  local want="$2"
  local out
  out="$("$VALIDATE" "$tag")"
  if [[ "$out" != "is_prerelease=$want" ]]; then
    echo "ERR $tag produced '$out', want is_prerelease=$want" >&2
    exit 1
  fi
}

expect_bad() {
  local tag="$1"
  if "$VALIDATE" "$tag" >"$TMP_DIR/out" 2>"$TMP_DIR/err"; then
    echo "ERR $tag passed but should be rejected" >&2
    exit 1
  fi
}

expect_ok v0.9.2 false
expect_ok v1.0.0 false
expect_ok v1.0.0-rc.1 true
expect_ok v1.0.0-beta.2 true
expect_ok v1.0.0-alpha01 true
expect_ok v1.0.0-0 true

expect_bad v1
expect_bad v1.0
expect_bad v01.0.0
expect_bad v1.02.0
expect_bad v1.0.03
expect_bad v1.0.0-rc.01
expect_bad v1.0.0rc1
expect_bad v1.0.0+
expect_bad v1.0.0+build.1
expect_bad 1.0.0

echo "release tag gate OK"
