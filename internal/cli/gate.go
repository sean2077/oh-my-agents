package cli

import (
	"regexp"
	"strings"
)

// fuzzyStartAnchors match signals that a goal is concrete enough to execute
// against (ralplan fuzzy-start gate, wave2): a file with an extension, an
// issue/PR ref, a camelCase or snake_case symbol, a test/build runner, or a
// numbered step list. Any one present → the goal is anchored.
var fuzzyStartAnchors = []*regexp.Regexp{
	regexp.MustCompile(`\b[\w./-]+\.[a-zA-Z]{2,}\b`),                          // a file: internal/relay/hook.go
	regexp.MustCompile(`#\d+`),                                                // issue / PR reference
	regexp.MustCompile(`[a-z]+[A-Z][A-Za-z]+`),                                // camelCase symbol
	regexp.MustCompile(`[A-Za-z]\w*_[A-Za-z]\w*`),                             // snake_case symbol
	regexp.MustCompile(`\b(go test|go build|pytest|npm|cargo|jest|vitest)\b`), // test/build runner
	regexp.MustCompile(`(?m)^\s*\d+[.)]\s`),                                   // a numbered step
}

// fuzzyStartAdvisory returns a non-empty advisory when text looks too vague
// to execute a loop against: short (≤15 words) AND carrying no concrete
// anchor. It is ADVISORY — the caller prints it, it never blocks the start
// (the CLI already owns the start boundary; this only nudges toward
// clarification/planning first).
func fuzzyStartAdvisory(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}
	for _, re := range fuzzyStartAnchors {
		if re.MatchString(t) {
			return ""
		}
	}
	if len(strings.Fields(t)) > 15 {
		return ""
	}
	return "note: goal looks underspecified (short, no file/issue/symbol/test-runner anchor). " +
		"A vague goal weakens the stop judgment — clarify with deep-interview or plan with ralplan before looping."
}
