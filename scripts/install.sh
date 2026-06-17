#!/usr/bin/env bash
# Install oma. By default this downloads the prebuilt binary for the latest
# GitHub release and verifies its SHA-256 against the release checksums.txt
# (docs/reference/security-contract.md §5 — the same asset/checksum contract self-update
# consumes). It falls back to a source build (git + go) when no prebuilt binary
# matches the platform, when no release can be resolved, or when
# OMA_INSTALL_FROM_SOURCE=1. A checksum mismatch is a hard, fail-closed error —
# never a silent downgrade to source.
#
# On Windows, run from Git Bash; the default binary name is oma.exe.
#
# Overrides:
#   OMA_INSTALL_REPO=sean2077/oh-my-agents   owner/name slug
#   OMA_INSTALL_VERSION=latest               'latest' or a tag like v0.1.0
#   OMA_INSTALL_BIN_DIR=$HOME/.local/bin     install directory
#   OMA_INSTALL_BIN_NAME=oma[.exe]           installed binary name
#   OMA_INSTALL_FROM_SOURCE=0                 set 1 to force a source build
#   OMA_INSTALL_REF                           source-build git ref (default: the
#                                             pinned version, else main)
set -euo pipefail

REPO="${OMA_INSTALL_REPO:-sean2077/oh-my-agents}"
VERSION="${OMA_INSTALL_VERSION:-latest}"
BIN_DIR="${OMA_INSTALL_BIN_DIR:-$HOME/.local/bin}"
FROM_SOURCE="${OMA_INSTALL_FROM_SOURCE:-0}"

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

# Source-build ref: explicit override wins, else the pinned tag, else main.
if [ -n "${OMA_INSTALL_REF:-}" ]; then
  REF="$OMA_INSTALL_REF"
elif [ "$VERSION" != "latest" ]; then
  REF="$VERSION"
else
  REF="main"
fi

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
cleanup() {
  rm -rf "$tmpdir"
  if [ -n "$tmpbin" ] && [ -e "$tmpbin" ]; then
    rm -f "$tmpbin"
  fi
}
trap cleanup EXIT

install_atomic() {
  # $1 = a ready binary file; place it at $BIN_DIR/$BIN_NAME atomically.
  mkdir -p "$BIN_DIR"
  tmpbin="$BIN_DIR/.${BIN_NAME}.tmp.$$"
  cp "$1" "$tmpbin"
  chmod 0755 "$tmpbin"
  mv "$tmpbin" "$BIN_DIR/$BIN_NAME"
  tmpbin=""
  echo "installed $BIN_DIR/$BIN_NAME"
  if "$BIN_DIR/$BIN_NAME" version >/dev/null 2>&1; then
    echo "  $("$BIN_DIR/$BIN_NAME" version 2>/dev/null | head -1)"
  fi
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

build_from_source() {
  log "building oma from source (ref: $REF)"
  need git
  need go
  git -c advice.detachedHead=false clone --quiet --depth 1 --branch "$REF" "https://github.com/$REPO.git" "$tmpdir/src"
  local commit source_version
  commit="$(git -C "$tmpdir/src" rev-parse --short HEAD 2>/dev/null || echo none)"
  if [ "$VERSION" != "latest" ]; then
    source_version="$VERSION"
  elif [ "$REF" != "main" ]; then
    source_version="$REF"
  else
    source_version="$(git -C "$tmpdir/src" describe --tags --exact-match 2>/dev/null || true)"
    if [ -z "$source_version" ]; then
      source_version="$(git -C "$tmpdir/src" describe --tags --abbrev=0 2>/dev/null || true)"
    fi
    if [ -z "$source_version" ]; then
      source_version="$REF"
    fi
  fi
  (
    cd "$tmpdir/src"
    go build -trimpath \
      -ldflags "-s -w \
        -X github.com/sean2077/oh-my-agents/internal/version.Version=$source_version \
        -X github.com/sean2077/oh-my-agents/internal/version.Commit=$commit" \
      -o "$tmpdir/built" ./cmd/oma
  )
  install_atomic "$tmpdir/built"
  path_note
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

install_from_release() {
  need curl
  local tag="$VERSION"
  if [ "$tag" = "latest" ]; then
    tag="$(resolve_latest_tag || true)"
    if [ -z "$tag" ]; then
      log "could not resolve the latest release for $REPO; falling back to source"
      return 1
    fi
    REF="$tag"
  fi

  local asset="oma_${tag}_${OS}_${ARCH}${EXT}"
  local base="https://github.com/$REPO/releases/download/$tag"
  log "downloading $asset ($tag)"
  if ! curl -fsSL -o "$tmpdir/$asset" "$base/$asset"; then
    log "no prebuilt asset $asset for $tag; falling back to source"
    return 1
  fi

  # From here a binary exists, so integrity is mandatory and fail-closed.
  curl -fsSL -o "$tmpdir/checksums.txt" "$base/checksums.txt" \
    || err "release $tag has no checksums.txt (unverifiable)"
  local want got
  want="$(awk -v a="$asset" '$2 == a {print $1}' "$tmpdir/checksums.txt")"
  [ -n "$want" ] || err "checksums.txt has no entry for $asset"
  got="$(sha256_of "$tmpdir/$asset")"
  [ "$got" = "$want" ] || err "checksum mismatch for $asset (want $want, got $got)"
  log "checksum ok"

  install_atomic "$tmpdir/$asset"
  path_note
  return 0
}

# --- main ---
if [ "$FROM_SOURCE" = "1" ]; then
  build_from_source
  exit 0
fi

if [ -z "$OS" ] || [ -z "$ARCH" ]; then
  log "no prebuilt binary for $(uname -s 2>/dev/null || echo unknown)/$(uname -m 2>/dev/null || echo unknown); building from source"
  build_from_source
  exit 0
fi

if ! install_from_release; then
  build_from_source
fi
