#!/usr/bin/env bash
# Install the latest oma from the main branch into ~/.local/bin by default.
set -euo pipefail

REPO_URL="${OMA_INSTALL_REPO:-https://github.com/sean2077/oh-my-agents.git}"
REF="${OMA_INSTALL_REF:-main}"
BIN_DIR="${OMA_INSTALL_BIN_DIR:-$HOME/.local/bin}"
BIN_NAME="${OMA_INSTALL_BIN_NAME:-oma}"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERR missing required command: $1" >&2
    exit 1
  fi
}

need git
need go

case "$BIN_DIR" in
  /*) ;;
  *) BIN_DIR="$(pwd)/$BIN_DIR" ;;
esac

tmpdir="$(mktemp -d)"
tmpbin=""
cleanup() {
  rm -rf "$tmpdir"
  if [[ -n "$tmpbin" && -e "$tmpbin" ]]; then
    rm -f "$tmpbin"
  fi
}
trap cleanup EXIT

git clone --quiet --depth 1 --branch "$REF" "$REPO_URL" "$tmpdir/oh-my-agents"
mkdir -p "$BIN_DIR"
tmpbin="$BIN_DIR/.${BIN_NAME}.tmp.$$"

(
  cd "$tmpdir/oh-my-agents"
  go build -trimpath -o "$tmpbin" ./cmd/oma
)

chmod 0755 "$tmpbin"
mv "$tmpbin" "$BIN_DIR/$BIN_NAME"
tmpbin=""

echo "installed $BIN_DIR/$BIN_NAME"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *)
    echo "NOTE: $BIN_DIR is not on PATH."
    echo "Add it to your shell profile, for example:"
    echo "  export PATH=\"$BIN_DIR:\$PATH\""
    ;;
esac
