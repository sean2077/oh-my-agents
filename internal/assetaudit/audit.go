// Package assetaudit derives an advisory bloat audit over the asset catalog
// (borrow-3). It sits ABOVE both internal/asset and internal/budget to avoid
// the asset<->budget import cycle (budget already imports asset). The audit is
// advisory only: it classifies assets (KEEP/ORPHAN/OVERSIZED/RETIRE) from
// deterministic metrics but never deletes — the judgment of whether to retire
// or consolidate stays with a human.
package assetaudit

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"

	"github.com/sean2077/oh-my-agents/internal/asset"
	"github.com/sean2077/oh-my-agents/internal/budget"
)

// Audit labels.
const (
	LabelKeep      = "KEEP"      // active, referenced, within budget
	LabelOrphan    = "ORPHAN"    // active with no inbound asset refs; direct use is out of scope
	LabelOversized = "OVERSIZED" // active but description exceeds its manifest budget
	LabelRetire    = "RETIRE"    // deprecated / merged / alias — slated for removal
)

// defaultDescriptionBudget mirrors the manifest default when omitted.
const defaultDescriptionBudget = 80

// AuditEntry is one row of the advisory audit. It is intentionally SEPARATE
// from asset.CatalogEntry so `oma asset catalog`'s generated view keeps its
// stable contract (codex gate-1 change #2).
type AuditEntry struct {
	Name                    string `json:"name"`
	Type                    string `json:"type"`
	Status                  string `json:"status"`
	LOC                     int    `json:"loc"`
	ResidentTokens          int    `json:"resident_tokens"`
	DescriptionTokens       int    `json:"description_tokens"`
	DescriptionBudgetTokens int    `json:"description_budget_tokens"`
	BodyTokens              int    `json:"body_tokens"`
	RefCount                int    `json:"ref_count"`
	ReferrerCount           int    `json:"referrer_count"`
	Label                   string `json:"label"`
	Reason                  string `json:"reason"`
}

// Audit scans a source root and returns the name-sorted advisory audit. It
// inherits the catalog's fail-closed manifest validation (malformed manifest,
// name/dir mismatch, duplicate name) and never mutates anything.
func Audit(root string) ([]AuditEntry, error) {
	cat, err := asset.Catalog(root) // universe + fail-closed validation
	if err != nil {
		return nil, err
	}
	out := make([]AuditEntry, 0, len(cat))
	for i := range cat {
		e := cat[i]
		main := mainFilePath(root, e.Type, e.Name)
		loc := countLines(main)
		resident := residentTokens(root, e.Type, e.Name)
		description := descriptionTokens(root, e.Type, e.Name)
		body := bodyTokens(root, e.Type, e.Name)
		budgetTok := manifestBudget(root, e.Type, e.Name)
		refs, referrers := referenceCounts(root, cat, e.Name)
		label, reason := classify(e, description, referrers, budgetTok)
		out = append(out, AuditEntry{
			Name: e.Name, Type: e.Type, Status: e.Status,
			LOC: loc, ResidentTokens: resident, DescriptionTokens: description,
			DescriptionBudgetTokens: budgetTok, BodyTokens: body, RefCount: refs,
			ReferrerCount: referrers, Label: label, Reason: reason,
		})
	}
	return out, nil // Catalog already sorts by name
}

// typeDirFor maps a manifest type to its assets/ subdir (codex gate-1 #4:
// type-specific selection so non-skills are not misread as skills).
func typeDirFor(typ string) string {
	switch typ {
	case asset.TypeSkill:
		return "skills"
	case asset.TypeSubagent:
		return "agents"
	case asset.TypeHook:
		return "hooks"
	case asset.TypePrompt:
		return "prompts"
	}
	return ""
}

// mainFilePath returns the type-specific primary file used for LOC and ref
// scanning. Skills carry SKILL.md; subagents carry <name>.md; hooks/prompts
// have no resident prose body, so their manifest.json is the measurable file.
func mainFilePath(root, typ, name string) string {
	dir := filepath.Join(root, typeDirFor(typ), name)
	switch typ {
	case asset.TypeSkill:
		return filepath.Join(dir, "SKILL.md")
	case asset.TypeSubagent:
		return filepath.Join(dir, name+".md")
	default: // hook, prompt — non-resident; manifest is the measurable file
		return filepath.Join(dir, "manifest.json")
	}
}

// residentTokens approximates the always-loaded prompt surface (name +
// description, plus whenToUse for subagents) via the pinned budget.Tokens.
// Hooks/prompts are non-resident → 0. A missing/unreadable file → 0 (advisory).
func residentTokens(root, typ, name string) int {
	switch typ {
	case asset.TypeSkill, asset.TypeSubagent:
		fm, err := budget.ReadFrontmatterFile(mainFilePath(root, typ, name))
		if err != nil {
			return 0
		}
		t := budget.Tokens(fm["name"]) + budget.Tokens(fm["description"])
		if typ == asset.TypeSubagent {
			t += budget.Tokens(fm["whenToUse"])
		}
		return t
	default:
		return 0
	}
}

// descriptionTokens measures only the frontmatter description against the
// manifest's description_budget_tokens. The resident metric above deliberately
// remains name + description (+ whenToUse for subagents).
func descriptionTokens(root, typ, name string) int {
	switch typ {
	case asset.TypeSkill, asset.TypeSubagent:
		fm, err := budget.ReadFrontmatterFile(mainFilePath(root, typ, name))
		if err != nil {
			return 0
		}
		return budget.Tokens(fm["description"])
	default:
		return 0
	}
}

// bodyTokens approximates the prompt tokens loaded from a skill's body while
// excluding its YAML frontmatter. Other asset types do not have a SKILL body.
func bodyTokens(root, typ, name string) int {
	if typ != asset.TypeSkill {
		return 0
	}
	raw, err := os.ReadFile(mainFilePath(root, typ, name))
	if err != nil {
		return 0
	}
	body := withoutYAMLFrontmatter(raw)
	return budget.Tokens(string(canonicalTextLineEndings(body)))
}

// withoutYAMLFrontmatter returns raw unchanged when it has no complete leading
// frontmatter block. It recognizes both LF and CRLF; bodyTokens canonicalizes
// the remaining Markdown separately before measuring it.
func withoutYAMLFrontmatter(raw []byte) []byte {
	firstEnd := bytes.IndexByte(raw, '\n')
	if firstEnd < 0 || !isYAMLDelimiter(raw[:firstEnd]) {
		return raw
	}

	for start := firstEnd + 1; start <= len(raw); {
		relEnd := bytes.IndexByte(raw[start:], '\n')
		end, next := len(raw), len(raw)
		if relEnd >= 0 {
			end = start + relEnd
			next = end + 1
		}
		if isYAMLDelimiter(raw[start:end]) {
			return raw[next:]
		}
		if next == len(raw) {
			break
		}
		start = next
	}
	return raw
}

// canonicalTextLineEndings makes the advisory loaded-body estimate independent
// of Git checkout policy. CommonMark treats CRLF, CR, and LF as line endings, so
// measuring their canonical LF form compares the same Markdown across hosts.
func canonicalTextLineEndings(raw []byte) []byte {
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(raw, []byte("\r"), []byte("\n"))
}

// isYAMLDelimiter mirrors budget.ReadFrontmatterFile: Scanner removes the line
// ending and the parser accepts spaces or tabs after the three dashes.
func isYAMLDelimiter(line []byte) bool {
	return bytes.Equal(bytes.TrimRight(line, " \t\r"), []byte("---"))
}

// manifestBudget reads the asset's description_budget_tokens (default 80).
func manifestBudget(root, typ, name string) int {
	m, err := asset.LoadManifest(filepath.Join(root, typeDirFor(typ), name, "manifest.json"))
	if err != nil || m.DescriptionBudgetTokens <= 0 {
		return defaultDescriptionBudget
	}
	return m.DescriptionBudgetTokens
}

func countLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	if len(data) == 0 {
		return 0
	}
	n := 0
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	if data[len(data)-1] != '\n' { // last line without trailing newline
		n++
	}
	return n
}

// referenceCounts returns both reference occurrences and distinct OTHER assets
// that refer to target. Text references are exact case-sensitive name tokens
// (boundaries excluding [A-Za-z0-9_-]); a manifest canonical edge contributes
// one occurrence but never a second referrer for the same asset. Self references
// are excluded; docs/ is intentionally NOT scanned (historical plans would mask
// real orphans).
func referenceCounts(root string, cat []asset.CatalogEntry, target string) (occurrences, referrers int) {
	re := regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(target) + `(?:$|[^A-Za-z0-9_-])`)
	for i := range cat {
		e := cat[i]
		if e.Name == target {
			continue // exclude self
		}
		fromAsset := 0
		if data, err := os.ReadFile(mainFilePath(root, e.Type, e.Name)); err == nil {
			fromAsset += len(re.FindAllString(string(data), -1))
		}
		if e.Canonical == target { // merged/alias successor edge
			fromAsset++
		}
		occurrences += fromAsset
		if fromAsset > 0 {
			referrers++
		}
	}
	return occurrences, referrers
}

// classify maps status + metrics to an advisory label (deterministic).
func classify(e asset.CatalogEntry, descriptionTokens, referrerCount, budgetTok int) (label, reason string) {
	switch e.Status {
	case asset.StatusDeprecated, asset.StatusMerged, asset.StatusAlias:
		return LabelRetire, "status " + e.Status + " — slated for removal/consolidation"
	}
	if referrerCount == 0 {
		return LabelOrphan, "no inbound references from other assets — direct-use evidence is outside this audit"
	}
	if descriptionTokens > budgetTok {
		return LabelOversized, "description exceeds its manifest token budget — streamline"
	}
	return LabelKeep, "active, referenced, description within budget"
}
