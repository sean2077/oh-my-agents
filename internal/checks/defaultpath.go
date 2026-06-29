package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sean2077/oh-my-agents/internal/asset"
)

// claudeOnlyMarkers are the host-specific affordances a Codex-targeted skill's
// default-path text must not reference (docs/reference/adapter-conformance.md §6):
// they belong only inside an explicit `> **CC acceleration` block. This is the
// spec's named set (AskUserQuestion, subagent invocation); extend it here if the
// conformance rule grows. Deliberately NOT included: bare "spawn", which skills
// use in generic prohibitions ("do not spawn a nested loop") unrelated to a
// Claude subagent.
var claudeOnlyMarkers = []struct {
	name string
	re   *regexp.Regexp
}{
	{"AskUserQuestion", regexp.MustCompile(`AskUserQuestion`)},
	{"subagent", regexp.MustCompile(`(?i)\bsubagents?\b`)},
}

// DefaultPathConformance enforces the adapter-conformance default-path rule for
// every skill under skillsRoot whose manifest targets codex: outside a
// `> **CC acceleration` block, the SKILL.md must not reference a Claude-only
// affordance. Claude-only skills are exempt (they may use those affordances on
// their default path). A missing skills root is silently ok (nothing installed).
func DefaultPathConformance(skillsRoot string) []Finding {
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []Finding{{Check: "default-path-conformance", Level: LevelFail,
			Message: fmt.Sprintf("read skills root %s: %v", skillsRoot, err)}}
	}
	var fs []Finding
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		m, err := asset.LoadManifest(filepath.Join(skillsRoot, name, "manifest.json"))
		if err != nil || !m.HasTarget("codex") {
			continue // unmanifested (other checks own that) or claude-only (exempt)
		}
		raw, err := os.ReadFile(filepath.Join(skillsRoot, name, "SKILL.md"))
		if err != nil {
			continue
		}
		for _, v := range defaultPathViolations(string(raw)) {
			fs = append(fs, Finding{Check: "default-path-conformance", Level: LevelFail,
				Message: fmt.Sprintf("%s SKILL.md:%d references Claude-only %q in default-path text; move it into a `> **CC acceleration` block", name, v.line, v.marker)})
		}
	}
	return fs
}

type dpViolation struct {
	line   int
	marker string
}

// defaultPathViolations returns every Claude-only reference outside a
// CC-acceleration blockquote. A blockquote (a run of consecutive lines whose
// trimmed text begins with ">") whose run contains the "CC acceleration" marker
// is the exempt acceleration block; every other line is default-path text.
func defaultPathViolations(content string) []dpViolation {
	var out []dpViolation
	inAccel := false
	for i, line := range strings.Split(content, "\n") {
		isQuote := strings.HasPrefix(strings.TrimSpace(line), ">")
		switch {
		case isQuote && strings.Contains(line, "CC acceleration"):
			inAccel = true
		case !isQuote:
			inAccel = false
		}
		if inAccel {
			continue
		}
		for _, mk := range claudeOnlyMarkers {
			if mk.re.MatchString(line) {
				out = append(out, dpViolation{line: i + 1, marker: mk.name})
				break
			}
		}
	}
	return out
}
