#!/usr/bin/env bash
# Regression smoke for scripts/install.sh's verify-before-replace contract
# (review v0.9.2 P1): a downloaded binary that passes the checksum gate but
# reports the WRONG internal version must never replace an existing install, and
# must leave no .tmp/.old residue. Exercised through the local-file install mode
# (OMA_INSTALL_FILE) so no network is needed. Runs in the `scripts` CI job.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

bindir="$work/bin"
mkdir -p "$bindir"

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
before="$(sha256sum "$dest" | awk '{print $1}')"

# 1. A bad artifact (reports v0.0.1) installed as v2.0.0 must fail closed and
#    leave the existing binary byte-identical.
bad="$work/bad-oma"
fake_oma "$bad" "v0.0.1"
if OMA_INSTALL_FILE="$bad" OMA_INSTALL_VERSION="v2.0.0" OMA_INSTALL_BIN_DIR="$bindir" \
     bash "$here/install.sh" >/dev/null 2>&1; then
  fail "install of a wrong-version artifact should have failed"
fi
[ "$(sha256sum "$dest" | awk '{print $1}')" = "$before" ] \
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

echo "install.sh local-file smoke OK"
