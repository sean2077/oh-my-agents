#!/usr/bin/env bash
# Install oma. By default this downloads the prebuilt binary for a GitHub
# release and verifies BOTH its SHA-256 against the release checksums.txt AND
# the installed binary's reported version against the requested release
# (docs/reference/security-contract.md §5 — the same asset/checksum contract
# self-update consumes). This prebuilt path is the only default.
#
# The installer is FAIL-CLOSED: if it cannot resolve a release, find a matching
# prebuilt asset, verify the checksum, or confirm the installed version, it
# stops with an actionable error. It NEVER silently downgrades to a source
# build or to the unreleased 'main' branch. A source build happens only when
# you opt in with OMA_INSTALL_FROM_SOURCE=1, and even then it builds the newest
# released tag (not main) unless you override the ref.
#
# On Windows, run from Git Bash (installs oma.exe), or use scripts/install.ps1
# from PowerShell.
#
# Overrides:
#   OMA_INSTALL_REPO=sean2077/oh-my-agents   owner/name slug
#   OMA_INSTALL_VERSION=latest               'latest' or a tag like v0.1.0
#   OMA_INSTALL_BIN_DIR=$HOME/.local/bin     install directory
#   OMA_INSTALL_BIN_NAME=oma[.exe]           installed binary name
#   OMA_INSTALL_FROM_SOURCE=0                 set 1 to opt into a source build
#   OMA_INSTALL_REF                           source-build git ref (default: the
#                                             pinned tag, else the newest
#                                             release, else main)
#   OMA_INSTALL_FILE                          install a local prebuilt binary at
#                                             this path instead of downloading
#                                             (requires OMA_INSTALL_VERSION=<tag>
#                                             as the expected version; used by CI
#                                             to smoke-test the just-built asset)
set -euo pipefail

REPO="${OMA_INSTALL_REPO:-sean2077/oh-my-agents}"
VERSION="${OMA_INSTALL_VERSION:-latest}"
BIN_DIR="${OMA_INSTALL_BIN_DIR:-$HOME/.local/bin}"
FROM_SOURCE="${OMA_INSTALL_FROM_SOURCE:-0}"
FILE="${OMA_INSTALL_FILE:-}"

case "$(uname -s 2>/dev/null || true)" in
  MINGW*|MSYS*|CYGWIN*) OS="windows"; EXT=".exe" ;;
  Linux)                OS="linux";   EXT="" ;;
  Darwin)               OS="darwin";  EXT="" ;;
  *)                    OS="";        EXT="" ;;
esac
BIN_NAME="${OMA_INSTALL_BIN_NAME:-oma${EXT}}"

case "$(uname -m 2>/dev/null || true)" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)             ARCH="" ;;
esac

# Make BIN_DIR absolute.
case "$BIN_DIR" in
  /*) ;;
  *)  BIN_DIR="$(pwd)/$BIN_DIR" ;;
esac

err() { echo "ERR $*" >&2; exit 1; }
log() { echo "$*" >&2; }
need() { command -v "$1" >/dev/null 2>&1 || err "missing required command: $1"; }

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    err "missing required command: sha256sum or shasum"
  fi
}

tmpdir="$(mktemp -d)"
tmpbin=""
backup=""
cleanup() {
  rm -rf "$tmpdir"
  if [ -n "$tmpbin" ] && [ -e "$tmpbin" ]; then
    rm -f "$tmpbin"
  fi
  # A backup still set here means we aborted mid-swap: restore the previous
  # binary so a failed install never leaves the target missing or half-written.
  if [ -n "$backup" ] && [ -e "$backup" ]; then
    mv -f "$backup" "$BIN_DIR/$BIN_NAME" 2>/dev/null || rm -f "$backup"
  fi
}
trap cleanup EXIT

# Read a binary file's reported version via the --json surface (parsed without
# jq to keep the installer dependency-free). Prints the version, or nothing.
probe_version() {
  "$1" version --json 2>/dev/null \
    | sed -nE 's/.*"version"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/p' | head -n1
}

# Validate a downloaded/built artifact BEFORE it is allowed to replace the
# target, so a wrong, stale, or unstartable binary can never clobber a working
# install (self-update validates the temp binary the same way before swapping).
verify_artifact_version() {
  local file="$1" want="$2" got
  got="$(probe_version "$file")"
  [ -n "$got" ] || err "downloaded binary did not report a version (wanted $want); target left untouched"
  [ "$got" = "$want" ] || err "version mismatch: downloaded binary reports '$got', expected '$want'; target left untouched"
}

# Place an ALREADY-VERIFIED binary at $BIN_DIR/$BIN_NAME atomically: back up any
# existing target, rename into place, re-check the installed binary, and roll
# the previous one back on failure (the install-time analogue of self-update's
# backup + post-replace self-check + auto-rollback).
install_atomic() {
  local src="$1" want="$2" dest="$BIN_DIR/$BIN_NAME" got
  mkdir -p "$BIN_DIR"
  tmpbin="$BIN_DIR/.${BIN_NAME}.tmp.$$"
  cp "$src" "$tmpbin"
  chmod 0755 "$tmpbin"

  if [ -e "$dest" ]; then
    backup="$BIN_DIR/.${BIN_NAME}.old.$$"
    cp -p "$dest" "$backup"
  fi

  mv "$tmpbin" "$dest"
  tmpbin=""

  got="$(probe_version "$dest")"
  if [ "$got" != "$want" ]; then
    if [ -n "$backup" ]; then
      mv -f "$backup" "$dest"
      backup=""
      err "post-install check failed (installed binary reports '${got:-none}', expected '$want'); previous binary restored"
    fi
    rm -f "$dest"
    err "post-install check failed (installed binary reports '${got:-none}', expected '$want'); removed the bad install"
  fi

  if [ -n "$backup" ]; then
    rm -f "$backup"
    backup=""
  fi
  echo "installed $dest"
  log "version verified: $got"
}

path_note() {
  case ":$PATH:" in
    *":$BIN_DIR:"*) ;;
    *)
      echo "NOTE: $BIN_DIR is not on PATH."
      echo "Add it to your shell profile (Git Bash on Windows uses the same line):"
      echo "  export PATH=\"$BIN_DIR:\$PATH\""
      ;;
  esac
}

tag_from_release_url() {
  local url="$1"
  url="${url%%\?*}"
  case "$url" in
    */releases/tag/*)
      local tag="${url##*/releases/tag/}"
      [ -n "$tag" ] || return 1
      printf '%s\n' "$tag"
      ;;
    *)
      return 1
      ;;
  esac
}

resolve_latest_tag_from_redirect() {
  # The public release redirect is more reliable than the anonymous API and
  # keeps the no-jq installer path small.
  local final
  final="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" 2>/dev/null)" || return 1
  tag_from_release_url "$final"
}

resolve_latest_tag_from_api() {
  local body
  body="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null)" || return 1
  printf '%s\n' "$body" \
    | sed -nE 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/p' \
    | head -n 1
}

resolve_latest_tag() {
  local tag
  tag="$(resolve_latest_tag_from_redirect || true)"
  if [ -n "$tag" ]; then
    printf '%s\n' "$tag"
    return 0
  fi
  tag="$(resolve_latest_tag_from_api || true)"
  if [ -n "$tag" ]; then
    printf '%s\n' "$tag"
    return 0
  fi
  return 1
}

# Which git ref an opt-in source build should use. We prefer a real released
# tag over 'main' so a source build still tracks a release by default; 'main'
# is only ever a last resort, and only on an explicit source build.
resolve_source_ref() {
  if [ -n "${OMA_INSTALL_REF:-}" ]; then
    printf '%s\n' "$OMA_INSTALL_REF"
    return 0
  fi
  if [ "$VERSION" != "latest" ]; then
    printf '%s\n' "$VERSION"
    return 0
  fi
  local tag
  tag="$(resolve_latest_tag || true)"
  if [ -n "$tag" ]; then
    printf '%s\n' "$tag"
    return 0
  fi
  printf 'main\n'
}

build_from_source() {
  need git
  need go
  local ref commit source_version
  ref="$(resolve_source_ref)"
  if [ "$ref" = "main" ]; then
    log "WARNING: building from the unreleased 'main' branch — no pinned tag and no release could be resolved."
  fi
  log "building oma from source (ref: $ref)"
  git -c advice.detachedHead=false clone --quiet --depth 1 --branch "$ref" "https://github.com/$REPO.git" "$tmpdir/src"
  commit="$(git -C "$tmpdir/src" rev-parse --short HEAD 2>/dev/null || echo none)"
  if [ "$ref" != "main" ]; then
    source_version="$ref"
  else
    source_version="$(git -C "$tmpdir/src" describe --tags --always 2>/dev/null || echo main)"
  fi
  (
    cd "$tmpdir/src"
    go build -trimpath \
      -ldflags "-s -w \
        -X github.com/sean2077/oh-my-agents/internal/version.Version=$source_version \
        -X github.com/sean2077/oh-my-agents/internal/version.Commit=$commit" \
      -o "$tmpdir/built" ./cmd/oma
  )
  verify_artifact_version "$tmpdir/built" "$source_version"
  install_atomic "$tmpdir/built" "$source_version"
  path_note
}

install_from_release() {
  need curl
  local tag="$VERSION"
  if [ "$tag" = "latest" ]; then
    tag="$(resolve_latest_tag || true)"
    [ -n "$tag" ] || err "could not resolve the latest release for $REPO. Check your network, pin OMA_INSTALL_VERSION=vX.Y.Z, or set OMA_INSTALL_FROM_SOURCE=1 to build from source."
  fi

  local asset="oma_${tag}_${OS}_${ARCH}${EXT}"
  local base="https://github.com/$REPO/releases/download/$tag"
  log "downloading $asset ($tag)"
  curl -fsSL -o "$tmpdir/$asset" "$base/$asset" \
    || err "no prebuilt asset $asset for $tag — this platform has no published binary. Set OMA_INSTALL_FROM_SOURCE=1 to build $tag from source (needs git + go)."

  # A binary now exists, so integrity is mandatory and fail-closed.
  curl -fsSL -o "$tmpdir/checksums.txt" "$base/checksums.txt" \
    || err "release $tag has no checksums.txt (unverifiable)"
  local want got
  want="$(awk -v a="$asset" '$2 == a {print $1}' "$tmpdir/checksums.txt")"
  [ -n "$want" ] || err "checksums.txt has no entry for $asset"
  got="$(sha256_of "$tmpdir/$asset")"
  [ "$got" = "$want" ] || err "checksum mismatch for $asset (want $want, got $got)"
  log "checksum ok"

  verify_artifact_version "$tmpdir/$asset" "$tag"
  install_atomic "$tmpdir/$asset" "$tag"
  path_note
}

# Install a local prebuilt binary (OMA_INSTALL_FILE) instead of downloading it.
# The expected version must be given via OMA_INSTALL_VERSION so the same
# verify-before-replace + rollback contract applies; this is how CI smoke-tests
# the artifact it just built through the real installer path.
install_from_file() {
  [ -f "$FILE" ] || err "OMA_INSTALL_FILE=$FILE is not a file"
  [ "$VERSION" != "latest" ] || err "OMA_INSTALL_FILE requires OMA_INSTALL_VERSION=<expected tag>"
  log "installing local binary $FILE ($VERSION)"
  verify_artifact_version "$FILE" "$VERSION"
  install_atomic "$FILE" "$VERSION"
  path_note
}

# --- main ---
if [ -n "$FILE" ]; then
  install_from_file
  exit 0
fi

if [ "$FROM_SOURCE" = "1" ]; then
  build_from_source
  exit 0
fi

# Default path: a verified prebuilt release binary, or a fail-closed stop. We
# never silently fall back to a source build or to the 'main' branch — that is
# an explicit opt-in (OMA_INSTALL_FROM_SOURCE=1).
if [ -z "$OS" ] || [ -z "$ARCH" ]; then
  err "no prebuilt binary for $(uname -s 2>/dev/null || echo unknown)/$(uname -m 2>/dev/null || echo unknown). Set OMA_INSTALL_FROM_SOURCE=1 to build from source (needs git + go)."
fi

install_from_release
