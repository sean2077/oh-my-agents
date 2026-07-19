package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sean2077/oh-my-agents/internal/asset"
)

const (
	parallelAccelerationHeading = "Parallel acceleration (optional, capability-gated)"
	ccAccelerationHeading       = "CC acceleration"
)

type accelerationBlock string

const (
	noAccelerationBlock accelerationBlock = ""
	parallelBlock       accelerationBlock = "parallel"
	ccBlock             accelerationBlock = "cc"
)

// governedAffordances are runtime instructions whose location is contractual
// (docs/reference/adapter-conformance.md §6). Runtime-native subagent
// instructions belong only in a capability-gated Parallel block, while
// AskUserQuestion remains Claude-Code-only and belongs only in a CC block.
// Deliberately NOT included: bare "spawn", which skills use in generic prose
// unrelated to subagent delegation.
var governedAffordances = []struct {
	name    string
	re      *regexp.Regexp
	allowed accelerationBlock
}{
	{"AskUserQuestion", regexp.MustCompile(`AskUserQuestion`), ccBlock},
	{"subagent", regexp.MustCompile(`(?i)\b(?:sub[-_ ]?agents?|spawn_agent)\b`), parallelBlock},
}

// DefaultPathConformance enforces the adapter-conformance default-path rule for
// every skill under skillsRoot whose manifest targets codex. Runtime subagent
// instructions and Claude-Code-only affordances must appear under their exact,
// distinct acceleration markers. Claude-only skills remain exempt. A missing
// skills root is silently ok (nothing installed).
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
				Message: fmt.Sprintf("%s SKILL.md:%d references %q in %s; %q is allowed only in an exact `%s` block", name, v.line, v.marker, v.location(), v.marker, markerLine(v.allowed))})
		}
	}
	return fs
}

type dpViolation struct {
	line    int
	marker  string
	block   accelerationBlock
	allowed accelerationBlock
}

func (v dpViolation) location() string {
	switch v.block {
	case parallelBlock:
		return "a Parallel acceleration block"
	case ccBlock:
		return "a CC acceleration block"
	default:
		return "default-path text"
	}
}

func markerLine(block accelerationBlock) string {
	switch block {
	case parallelBlock:
		return "> **" + parallelAccelerationHeading + "**:"
	case ccBlock:
		return "> **" + ccAccelerationHeading + "**:"
	default:
		return ""
	}
}

// defaultPathViolations returns every governed affordance outside its exact
// acceleration block. Quoted blank lines preserve the same CommonMark
// blockquote; an unquoted line ends it, and a new exact heading switches it.
func defaultPathViolations(content string) []dpViolation {
	var out []dpViolation
	block := noAccelerationBlock
	for i, line := range strings.Split(content, "\n") {
		block = accelerationBlockForLine(line, block)
		for _, affordance := range governedAffordances {
			if affordance.re.MatchString(line) && block != affordance.allowed {
				out = append(out, dpViolation{
					line:    i + 1,
					marker:  affordance.name,
					block:   block,
					allowed: affordance.allowed,
				})
			}
		}
	}
	return out
}

func accelerationBlockForLine(line string, current accelerationBlock) accelerationBlock {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, ">") {
		return noAccelerationBlock
	}
	quoted := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
	if quoted == "" {
		return current
	}
	if strings.HasPrefix(quoted, "**"+parallelAccelerationHeading+"**:") {
		return parallelBlock
	}
	if strings.HasPrefix(quoted, "**"+ccAccelerationHeading+"**:") {
		return ccBlock
	}
	return current
}
