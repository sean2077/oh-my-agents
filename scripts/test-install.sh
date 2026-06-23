#!/usr/bin/env bash
# Regression smoke for scripts/install.sh's verify-before-replace contract.
# It exercises both the local-file path used by CI artifacts and the release
# download path users hit via `curl .../install.sh | bash`, without reaching the
# network (the release base is a file:// fixture).
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

bindir="$work/bin"
mkdir -p "$bindir"

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

fake_oma() { # $1=path $2=version-it-reports
  cat >"$1" <<EOF
#!/usr/bin/env bash
[ "\$1" = version ] && echo '{"version":"$2"}'
EOF
  chmod +x "$1"
}

fail() { echo "FAIL: $*" >&2; exit 1; }

# An existing, working install reporting v1.0.0.
dest="$bindir/oma"
fake_oma "$dest" "v1.0.0"
before="$(digest "$dest")"

# 1. A bad artifact (reports v0.0.1) installed as v2.0.0 must fail closed and
#    leave the existing binary byte-identical.
bad="$work/bad-oma"
fake_oma "$bad" "v0.0.1"
if OMA_INSTALL_FILE="$bad" OMA_INSTALL_VERSION="v2.0.0" OMA_INSTALL_BIN_DIR="$bindir" \
     bash "$here/install.sh" >/dev/null 2>&1; then
  fail "install of a wrong-version artifact should have failed"
fi
[ "$(digest "$dest")" = "$before" ] \
  || fail "existing binary was modified by a refused install"
residue="$(find "$bindir" -maxdepth 1 -name '.*' -type f)"
[ -z "$residue" ] || fail "left residue after a refused install: $residue"

# 2. A matching-version artifact installs cleanly and replaces the binary.
good="$work/good-oma"
fake_oma "$good" "v2.0.0"
# GitHub Actions artifacts are downloaded from a zip and do not preserve the
# executable bit. Local-file installs must still verify and install them.
chmod 0644 "$good"
OMA_INSTALL_FILE="$good" OMA_INSTALL_VERSION="v2.0.0" OMA_INSTALL_BIN_DIR="$bindir" \
  bash "$here/install.sh" >/dev/null 2>&1 || fail "install of a matching artifact should succeed"
[ "$("$dest" version)" = '{"version":"v2.0.0"}' ] || fail "matching artifact was not installed"
residue="$(find "$bindir" -maxdepth 1 -name '.*' -type f)"
[ -z "$residue" ] || fail "left residue after a successful install: $residue"

case "$(uname -s 2>/dev/null || true)" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *)      os="" ;;
esac
case "$(uname -m 2>/dev/null || true)" in
  x86_64|amd64)  arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *)             arch="" ;;
esac

if [ -n "$os" ] && [ -n "$arch" ]; then
  make_release() { # $1=tag $2=reported-version
    local tag="$1" reported="$2" release_dir asset
    release_dir="$work/releases/sean2077/oh-my-agents/releases/download/$tag"
    mkdir -p "$release_dir"
    asset="oma_${tag}_${os}_${arch}"
    fake_oma "$release_dir/$asset" "$reported"
    chmod 0644 "$release_dir/$asset"
    printf '%s  %s\n' "$(digest "$release_dir/$asset")" "$asset" >"$release_dir/checksums.txt"
  }

  # 3. The release-download path receives a 0644 curl output, prepares it with
  #    chmod before version probing, and installs successfully.
  make_release "v3.0.0" "v3.0.0"
  OMA_INSTALL_DOWNLOAD_BASE="file://$work/releases" \
  OMA_INSTALL_VERSION="v3.0.0" \
  OMA_INSTALL_BIN_DIR="$bindir" \
    bash "$here/install.sh" >/dev/null 2>&1 || fail "release-path install of a 0644 artifact should succeed"
  [ "$("$dest" version)" = '{"version":"v3.0.0"}' ] || fail "release-path artifact was not installed"
  residue="$(find "$bindir" -maxdepth 1 -name '.*' -type f)"
  [ -z "$residue" ] || fail "left residue after release-path install: $residue"

  # 4. A release artifact that passes checksum but reports the wrong version
  #    must fail closed and leave the existing binary byte-identical.
  before="$(digest "$dest")"
  make_release "v4.0.0" "v0.0.1"
  if OMA_INSTALL_DOWNLOAD_BASE="file://$work/releases" \
       OMA_INSTALL_VERSION="v4.0.0" \
       OMA_INSTALL_BIN_DIR="$bindir" \
         bash "$here/install.sh" >/dev/null 2>&1; then
    fail "release-path wrong-version artifact should have failed"
  fi
  [ "$(digest "$dest")" = "$before" ] || fail "release-path refusal modified the existing binary"
  residue="$(find "$bindir" -maxdepth 1 -name '.*' -type f)"
  [ -z "$residue" ] || fail "left residue after release-path refusal: $residue"

  # 5. Duplicate checksum entries for the consumed asset are ambiguous and
  #    must fail closed instead of accepting whichever line awk saw last.
  before="$(digest "$dest")"
  make_release "v4.1.0" "v4.1.0"
  dup_asset="oma_v4.1.0_${os}_${arch}"
  printf '%s  %s\n' "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff" "$dup_asset" \
    >>"$work/releases/sean2077/oh-my-agents/releases/download/v4.1.0/checksums.txt"
  if OMA_INSTALL_DOWNLOAD_BASE="file://$work/releases" \
       OMA_INSTALL_VERSION="v4.1.0" \
       OMA_INSTALL_BIN_DIR="$bindir" \
         bash "$here/install.sh" >/dev/null 2>&1; then
    fail "release-path duplicate checksum entry should have failed"
  fi
  [ "$(digest "$dest")" = "$before" ] || fail "duplicate checksum refusal modified the existing binary"
  residue="$(find "$bindir" -maxdepth 1 -name '.*' -type f)"
  [ -z "$residue" ] || fail "left residue after duplicate checksum refusal: $residue"
fi

# 6. Backup creation is two-phase: if copying the previous binary fails after
#    creating a partial candidate, cleanup must not restore that partial over
#    the good install.
shim="$work/shim"
mkdir -p "$shim"
cat >"$shim/cp" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
last="${@: -1}"
if [[ "$last" == *".oma.old."*".tmp" ]]; then
  printf partial >"$last"
  exit 1
fi
exec /bin/cp "$@"
EOF
chmod +x "$shim/cp"

before="$(digest "$dest")"
backup_good="$work/backup-good"
fake_oma "$backup_good" "v5.0.0"
if PATH="$shim:$PATH" \
     OMA_INSTALL_FILE="$backup_good" \
     OMA_INSTALL_VERSION="v5.0.0" \
     OMA_INSTALL_BIN_DIR="$bindir" \
       bash "$here/install.sh" >/dev/null 2>&1; then
  fail "install should fail when backup creation fails"
fi
[ "$(digest "$dest")" = "$before" ] || fail "partial backup was restored over the existing binary"
residue="$(find "$bindir" -maxdepth 1 -name '.*' -type f)"
[ -z "$residue" ] || fail "left residue after failed backup creation: $residue"

echo "install.sh smoke OK"
