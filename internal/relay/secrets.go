package relay

import (
	"fmt"
	"regexp"
	"strings"
)

// The mandatory publish-time scan set (protocol §10, security-contract.md
// §6): common token/key shapes. v1 ships NO bypass switch; false
// positives are resolved by editing the artifact or by registering a
// narrow allow pattern in the security contract appendix (none
// registered yet).
var secretPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"private-key-block", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"aws-access-key-id", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"github-token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`)},
	{"slack-token", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)},
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.`)},
	// Provider secret-key prefixes: OpenAI sk-/sk-proj-, Anthropic
	// sk-ant-, Stripe sk_live_/sk_test_ (review 054 blocker 4).
	{"provider-secret-key", regexp.MustCompile(`\bsk[-_](proj-|ant-|live_|test_)?[A-Za-z0-9_-]{16,}\b`)},
	{"assignment-quoted", regexp.MustCompile(`(?i)\b(api[_-]?key|secret[_-]?key|access[_-]?token|password|passwd)\b\s*[:=]\s*['"][^'"\s]{8,}['"]`)},
}

// assignmentRe catches UNQUOTED `*KEY=...` / `*TOKEN=...` style
// assignments (review 054 blocker 4). RE2 has no lookahead, so the value
// shape is validated in code by looksLikeSecret — that two-stage check is
// what keeps benign prose ("token = approximately …") out.
var assignmentRe = regexp.MustCompile(`(?i)\b[A-Z0-9_]*(API[_-]?KEY|SECRET|TOKEN|PASSWORD|PASSWD)[A-Z0-9_]*\s*[:=]\s*([^\s'"]+)`)

// looksLikeSecret reports whether an unquoted assignment value has the
// shape of a real credential: long enough, mixed letters+digits, and
// drawn from the usual token alphabet. Plain words ("approximately"),
// small numbers and YAML counts ("budget_tokens: 2000") all fail this.
func looksLikeSecret(v string) bool {
	if len(v) < 16 {
		return false
	}
	hasLetter, hasDigit := false, false
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == '_' || r == '-' || r == '+' || r == '/' || r == '=' || r == '.':
		default:
			return false // outside the token alphabet: prose, paths with spaces, CJK …
		}
	}
	return hasLetter && hasDigit
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
		for _, m := range assignmentRe.FindAllStringSubmatch(line, -1) {
			if looksLikeSecret(m[2]) {
				findings = append(findings, fmt.Sprintf("line %d: assignment-unquoted", i+1))
			}
		}
	}
	return findings
}
