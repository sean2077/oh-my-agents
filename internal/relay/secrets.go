package relay

import (
	"fmt"
	"regexp"
	"strings"
)

// secretPatterns is the mandatory publish-time scan set (protocol §10,
// security-contract.md §6): common token/key shapes. v1 ships NO bypass
// switch; false positives are resolved by editing the artifact or by
// registering a narrow allow pattern in the security contract appendix
// (none registered yet).
var secretPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"private-key-block", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"aws-access-key-id", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"github-token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`)},
	{"slack-token", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)},
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.`)},
	{"assignment", regexp.MustCompile(`(?i)\b(api[_-]?key|secret[_-]?key|access[_-]?token|password|passwd)\b\s*[:=]\s*['"][^'"\s]{8,}['"]`)},
}

// ScanSecrets returns one finding line per hit ("line N: <pattern>").
// Allow patterns (security-contract appendix) would filter here; the
// registry is currently empty.
func ScanSecrets(content []byte) []string {
	var findings []string
	for i, line := range strings.Split(string(content), "\n") {
		for _, p := range secretPatterns {
			if p.re.MatchString(line) {
				findings = append(findings, fmt.Sprintf("line %d: %s", i+1, p.name))
			}
		}
	}
	return findings
}
