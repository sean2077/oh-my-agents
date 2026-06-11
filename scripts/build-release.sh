#!/usr/bin/env bash
# Cross-compile release binaries with the asset naming + checksums.txt
# contract self-update consumes (docs/security-contract.md §5):
#   oma_<version>_<os>_<arch>[.exe]  (version = v-prefixed git tag)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${GITHUB_REF_NAME:-dev}}"
COMMIT="${COMMIT:-$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo none)}"
OUT_DIR="${OUT_DIR:-$ROOT/dist}"

if [[ "$OUT_DIR" != /* ]]; then
  OUT_DIR="$ROOT/$OUT_DIR"
fi
if [[ "$OUT_DIR" == *..* ]]; then
  echo "ERR OUT_DIR must not contain '..': $OUT_DIR" >&2
  exit 1
fi
case "$OUT_DIR" in
  "$ROOT/dist"|"$ROOT/dist"/*) ;;
  *)
    echo "ERR OUT_DIR must stay under $ROOT/dist: $OUT_DIR" >&2
    exit 1
    ;;
esac

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

build_one() {
  local goos="$1"
  local goarch="$2"
  local binary="oma_${VERSION}_${goos}_${goarch}"

  if [[ "$goos" == "windows" ]]; then
    binary="${binary}.exe"
  fi

  (
    cd "$ROOT"
    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
      go build -trimpath \
      -ldflags "-s -w \
        -X github.com/sean2077/oh-my-agents/internal/version.Version=$VERSION \
        -X github.com/sean2077/oh-my-agents/internal/version.Commit=$COMMIT" \
      -o "$OUT_DIR/$binary" ./cmd/oma
  )
}

build_one linux amd64
build_one linux arm64
build_one darwin amd64
build_one darwin arm64
build_one windows amd64
build_one windows arm64

(
  cd "$OUT_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum oma_"$VERSION"_* > checksums.txt
  else
    shasum -a 256 oma_"$VERSION"_* > checksums.txt
  fi
)
