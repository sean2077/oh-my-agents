// Package schemaver parses the strict "oma-<domain>/<major>" schema string
// shared by every persisted-data reader (registry/state/ledger/manifest/
// interview/ralph/config). It was previously a byte-identical per-package copy
// (B2 review finding 1); this is the single source of truth. Leaf package: it
// imports only the standard library, so domain packages depend on it without a
// cycle.
package schemaver

import (
	"strconv"
	"strings"
)

// Major extracts and validates the major version of a schema string: it must be
// "<wantDomain>/<major>" where major is a digit sequence with no sign and no
// leading zero, parsing to an integer >= 1. Anything else returns (0, false).
func Major(schema, wantDomain string) (int, bool) {
	domain, ver, found := strings.Cut(schema, "/")
	if !found || domain != wantDomain || ver == "" || ver[0] == '0' {
		return 0, false
	}
	for i := 0; i < len(ver); i++ {
		if ver[i] < '0' || ver[i] > '9' {
			return 0, false
		}
	}
	major, err := strconv.Atoi(ver)
	if err != nil || major < 1 {
		return 0, false
	}
	return major, true
}
