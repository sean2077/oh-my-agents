package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	markdownLinkPattern      = regexp.MustCompile(`!?\[[^\]]*\]\(([^)\n]+)\)`)
	markdownBlankLinePattern = regexp.MustCompile(`\n[ \t]*\n`)
	destinationSchemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
)

func TestShippedSkillsKeepWorkflowHeadingOrder(t *testing.T) {
	tests := []struct {
		skill string
		want  []string
	}{
		{
			skill: "deep-interview",
			want: []string{
				"## Start",
				"## Interview loop (one question per round)",
				"## Gate, crystallize, complete",
			},
		},
		{
			skill: "ralph",
			want: []string{
				"## Start",
				"## Each round",
				"## Terminal states are instructions, not labels",
			},
		},
		{
			skill: "autopilot",
			want: []string{
				"## Start and persist the run",
				"## Phases",
				"## Hard rules",
			},
		},
		{
			skill: "pair-delivery",
			want: []string{
				"## Every turn: orient, then act",
				"## Delivery gates (docs/reference/workflows.md §4)",
				"## Publishing a turn",
				"## Continuing after handoff",
			},
		},
		{
			skill: "analyze",
			want: []string{
				"## Non-negotiable contract",
				"## Working method",
				"## Output contract",
			},
		},
		{
			skill: "research-mission",
			want: []string{
				"## 1. Scaffold the mission",
				"## 2. Define the evaluator contract",
				"## 3. Drive it with ralph",
				"## 4. Keep a candidate ledger",
				"## Terminal states",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.skill, func(t *testing.T) {
			path := filepath.Join("..", "..", "assets", "skills", tc.skill, "SKILL.md")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if violation := orderedHeadingViolation(markdownHeadings(string(raw)), tc.want); violation != "" {
				t.Fatalf("%s: %s", filepath.ToSlash(path), violation)
			}
		})
	}
}

func TestShippedSkillReferencesStayOneHopAndConditional(t *testing.T) {
	root := filepath.Join("..", "..", "assets", "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			for _, violation := range skillReferenceViolations(filepath.Join(root, entry.Name())) {
				t.Error(violation)
			}
		})
	}
}

func TestSkillStructureContractRejectsMutations(t *testing.T) {
	t.Run("heading reorder", func(t *testing.T) {
		text := "# demo\n\n## Verify\n\n```md\n## Implement\n```\n\n## Implement\n"
		want := []string{"# demo", "## Implement", "## Verify"}
		if violation := orderedHeadingViolation(markdownHeadings(text), want); violation == "" {
			t.Fatal("reordered workflow headings passed the ordered-subsequence contract")
		}
	})

	writeFixture := func(t *testing.T, skill, reference string) string {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "demo")
		if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skill), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "references", "detail.md"), []byte(reference), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	t.Run("valid one-hop reference", func(t *testing.T) {
		dir := writeFixture(t,
			"See the ordinary [workflow docs](../../docs/workflows.md).\n\nLoad [detail](references/detail.md) only when needed.\n",
			"# Detail\n\nSee the ordinary [schema docs](../../../docs/schemas.md).\n",
		)
		if violations := skillReferenceViolations(dir); len(violations) != 0 {
			t.Fatalf("valid fixture violations: %v", violations)
		}
	})

	t.Run("reference link needs load predicate", func(t *testing.T) {
		dir := writeFixture(t, "Read [detail](references/detail.md).\n", "# Detail\n")
		assertViolationContains(t, skillReferenceViolations(dir), "explicit load predicate")
	})

	t.Run("reference cannot chain", func(t *testing.T) {
		dir := writeFixture(t, "Load [detail](references/detail.md) only when needed.\n", "See [other](other.md).\n")
		if err := os.WriteFile(filepath.Join(dir, "references", "other.md"), []byte("# Other\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		assertViolationContains(t, skillReferenceViolations(dir), "links to another references path")
	})

	t.Run("reference must be directly discoverable", func(t *testing.T) {
		dir := writeFixture(t, "# Demo\n", "# Detail\n")
		assertViolationContains(t, skillReferenceViolations(dir), "is not directly linked")
	})

	t.Run("reference link must exist", func(t *testing.T) {
		dir := writeFixture(t, "Load [missing](references/missing.md) only when needed.\n", "# Detail\n")
		assertViolationContains(t, skillReferenceViolations(dir), "links to missing reference")
	})
}

func markdownHeadings(text string) []string {
	var headings []string
	for _, line := range strings.Split(markdownOutsideFences(text), "\n") {
		candidate := strings.TrimLeft(line, " \t")
		level := 0
		for level < len(candidate) && level < 6 && candidate[level] == '#' {
			level++
		}
		if level == 0 || level >= len(candidate) || candidate[level] != ' ' {
			continue
		}
		title := strings.TrimSpace(candidate[level+1:])
		if title != "" {
			headings = append(headings, strings.Repeat("#", level)+" "+title)
		}
	}
	return headings
}

func orderedHeadingViolation(got, want []string) string {
	next := 0
	for _, heading := range got {
		if next < len(want) && heading == want[next] {
			next++
		}
	}
	if next == len(want) {
		return ""
	}
	after := "start of document"
	if next > 0 {
		after = fmt.Sprintf("heading %q", want[next-1])
	}
	return fmt.Sprintf("required heading %q is missing or out of order after %s", want[next], after)
}

func skillReferenceViolations(skillDir string) []string {
	skillPath := filepath.Join(skillDir, "SKILL.md")
	raw, err := os.ReadFile(skillPath)
	if err != nil {
		return []string{fmt.Sprintf("read %s: %v", filepath.ToSlash(skillPath), err)}
	}

	linked := map[string]bool{}
	var violations []string
	for _, paragraph := range markdownParagraphs(string(raw)) {
		for _, destination := range markdownDestinations(paragraph) {
			rel, ok := referencesRelativePath(destination)
			if !ok {
				continue
			}
			linked[rel] = true
			path := filepath.Join(skillDir, filepath.FromSlash(rel))
			info, statErr := os.Stat(path)
			if statErr != nil || info.IsDir() {
				violations = append(violations, fmt.Sprintf("%s links to missing reference %s", filepath.ToSlash(skillPath), rel))
			}
			if !hasExplicitLoadPredicate(paragraph) {
				violations = append(violations, fmt.Sprintf("%s link to %s lacks an explicit load predicate such as 'only when' or 'when needed'", filepath.ToSlash(skillPath), rel))
			}
		}
	}

	referencesRoot := filepath.Join(skillDir, "references")
	if _, statErr := os.Stat(referencesRoot); statErr != nil {
		if os.IsNotExist(statErr) {
			return violations
		}
		return append(violations, fmt.Sprintf("stat %s: %v", filepath.ToSlash(referencesRoot), statErr))
	}
	_ = filepath.WalkDir(referencesRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			violations = append(violations, fmt.Sprintf("walk %s: %v", filepath.ToSlash(path), walkErr))
			return nil
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}

		rel, relErr := filepath.Rel(skillDir, path)
		if relErr != nil {
			violations = append(violations, fmt.Sprintf("resolve reference %s: %v", filepath.ToSlash(path), relErr))
			return nil
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if !linked[rel] {
			violations = append(violations, fmt.Sprintf("reference %s is not directly linked from %s", rel, filepath.ToSlash(skillPath)))
		}

		refRaw, readErr := os.ReadFile(path)
		if readErr != nil {
			violations = append(violations, fmt.Sprintf("read %s: %v", filepath.ToSlash(path), readErr))
			return nil
		}
		for _, paragraph := range markdownParagraphs(string(refRaw)) {
			for _, destination := range markdownDestinations(paragraph) {
				if referenceTargetIsInside(referencesRoot, filepath.Dir(path), destination) {
					violations = append(violations, fmt.Sprintf("reference %s links to another references path %q; references must stay one hop from SKILL.md", rel, destination))
				}
			}
		}
		return nil
	})
	return violations
}

func markdownParagraphs(text string) []string {
	var paragraphs []string
	for _, paragraph := range markdownBlankLinePattern.Split(strings.TrimSpace(markdownOutsideFences(text)), -1) {
		if paragraph = strings.TrimSpace(paragraph); paragraph != "" {
			paragraphs = append(paragraphs, paragraph)
		}
	}
	return paragraphs
}

func markdownOutsideFences(text string) string {
	var out strings.Builder
	fence := ""
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			out.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			out.WriteByte('\n')
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}

func markdownDestinations(paragraph string) []string {
	var destinations []string
	for _, match := range markdownLinkPattern.FindAllStringSubmatch(paragraph, -1) {
		destination := strings.TrimSpace(match[1])
		if strings.HasPrefix(destination, "<") {
			if end := strings.Index(destination, ">"); end >= 0 {
				destination = destination[1:end]
			}
		} else if fields := strings.Fields(destination); len(fields) > 0 {
			destination = fields[0]
		}
		if destination != "" {
			destinations = append(destinations, destination)
		}
	}
	return destinations
}

func referencesRelativePath(destination string) (string, bool) {
	path := cleanMarkdownDestination(destination)
	if path == "" || destinationSchemePattern.MatchString(path) || strings.HasPrefix(path, "/") || filepath.IsAbs(filepath.FromSlash(path)) {
		return "", false
	}
	path = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))), "./")
	if path == "references" || !strings.HasPrefix(path, "references/") {
		return "", false
	}
	return path, true
}

func cleanMarkdownDestination(destination string) string {
	if cut := strings.IndexAny(destination, "?#"); cut >= 0 {
		destination = destination[:cut]
	}
	return strings.TrimSpace(destination)
}

func hasExplicitLoadPredicate(paragraph string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(paragraph), " "))
	return strings.Contains(normalized, "only when") ||
		strings.Contains(normalized, "when needed") ||
		strings.Contains(normalized, "load when") ||
		strings.Contains(normalized, "read when") ||
		strings.Contains(normalized, "consult when")
}

func referenceTargetIsInside(referencesRoot, sourceDir, destination string) bool {
	target := cleanMarkdownDestination(destination)
	if target == "" || strings.HasPrefix(target, "#") || destinationSchemePattern.MatchString(target) || strings.HasPrefix(target, "/") || filepath.IsAbs(filepath.FromSlash(target)) {
		return false
	}
	resolved := filepath.Clean(filepath.Join(sourceDir, filepath.FromSlash(target)))
	rel, err := filepath.Rel(referencesRoot, resolved)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func assertViolationContains(t *testing.T, violations []string, want string) {
	t.Helper()
	for _, violation := range violations {
		if strings.Contains(violation, want) {
			return
		}
	}
	t.Fatalf("violations %q do not contain %q", violations, want)
}
