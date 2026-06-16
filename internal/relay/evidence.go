package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// EvidenceSchema versions the canonical review-evidence block (R5). It lives
// in the review body as a single fenced ```oma-review-evidence/1 block;
// Publish validates it by verdict, computes evidence_hash over its canonical
// (re-marshalled) form, and the close gate re-derives and compares that hash.
const EvidenceSchema = "oma-review-evidence/1"

// Closed enum sets for evidence fields (R5, codex gate-1 directions).
var (
	RefTypes    = []string{"repo", "official", "source_reference", "supplemental"}
	Severities  = []string{"critical", "high", "medium", "low"}
	Confidences = []string{"high", "medium", "low"}
)

// EvidenceRef is one structured reference backing a finding or an approval
// basis: a repo path:line, or an external URL, with optional version/date.
// A finding may cite several refs of different types (codex gate-1 #4).
type EvidenceRef struct {
	Type          string `json:"type"`
	Ref           string `json:"ref"`
	VersionOrDate string `json:"version_or_date,omitempty"`
}

// Finding is one reviewed issue with its severity, confidence and refs.
type Finding struct {
	Severity   string        `json:"severity"`
	Confidence string        `json:"confidence"`
	Claim      string        `json:"claim"`
	Refs       []EvidenceRef `json:"refs"`
}

// Evidence is the canonical oma-review-evidence/1 payload carried in a review
// body. Field order here IS the canonical hash order (json.Marshal is stable).
type Evidence struct {
	Schema      string        `json:"schema"`
	Findings    []Finding     `json:"findings"`
	BasisRefs   []EvidenceRef `json:"basis_refs"`
	CommandsRun []string      `json:"commands_run"`
	Limitations []string      `json:"limitations"`
}

// placeholderRe matches obvious filler that must not pass for real evidence
// (codex gate-1 #4). It is anchored, so a legitimate claim that merely
// mentions "todo" is unaffected — only a value that IS the filler is rejected.
var placeholderRe = regexp.MustCompile(`(?i)^(todo|tbd|stub|fake[ _-]?pass|not[ _-]?implemented|n/?a|x{3,}|\.{2,}|placeholder|tk)$`)

func isPlaceholder(s string) bool {
	t := strings.TrimSpace(s)
	return t == "" || placeholderRe.MatchString(t)
}

// repoRefRe matches a repo-relative path:line or path:line-line. The leading
// [^/] rejects absolute paths; ".." is rejected separately.
var repoRefRe = regexp.MustCompile(`^[^/].*:\d+(-\d+)?$`)

// extractEvidenceBlock pulls the single fenced oma-review-evidence/1 block out
// of a review body. Zero, multiple, or an unterminated block all fail closed.
func extractEvidenceBlock(body string) (string, error) {
	lines := strings.Split(body, "\n")
	var inBlock bool
	var collected []string
	var seen int
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if !inBlock {
			if strings.HasPrefix(trimmed, "```") && strings.Contains(trimmed, EvidenceSchema) {
				inBlock = true
				seen++
			}
			continue
		}
		if strings.TrimSpace(ln) == "```" {
			inBlock = false
			continue
		}
		collected = append(collected, ln)
	}
	if seen == 0 {
		return "", fmt.Errorf("%w: review body has no fenced %s evidence block", ErrRelay, EvidenceSchema)
	}
	if seen > 1 {
		return "", fmt.Errorf("%w: review body has %d %s blocks (want exactly 1)", ErrRelay, seen, EvidenceSchema)
	}
	if inBlock {
		return "", fmt.Errorf("%w: %s evidence block is not closed with ```", ErrRelay, EvidenceSchema)
	}
	return strings.Join(collected, "\n"), nil
}

// ParseEvidence extracts and strictly decodes the evidence payload from a
// review body. Unknown JSON keys and trailing content fail closed.
func ParseEvidence(body string) (*Evidence, error) {
	jsonRaw, err := extractEvidenceBlock(body)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(jsonRaw))
	dec.DisallowUnknownFields()
	var ev Evidence
	if err := dec.Decode(&ev); err != nil {
		return nil, fmt.Errorf("%w: evidence JSON: %v", ErrRelay, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: evidence block has trailing content after the JSON object", ErrRelay)
	}
	if ev.Schema != EvidenceSchema {
		return nil, fmt.Errorf("%w: evidence schema %q, want %s", ErrRelay, ev.Schema, EvidenceSchema)
	}
	return &ev, nil
}

func validateRef(r EvidenceRef, ctx string) error {
	if !contains(RefTypes, r.Type) {
		return fmt.Errorf("%w: %s ref type %q not in %v", ErrRelay, ctx, r.Type, RefTypes)
	}
	if isPlaceholder(r.Ref) {
		return fmt.Errorf("%w: %s ref %q is empty or placeholder", ErrRelay, ctx, r.Ref)
	}
	if r.Type == "repo" {
		if strings.Contains(r.Ref, "..") || !repoRefRe.MatchString(r.Ref) {
			return fmt.Errorf("%w: %s repo ref %q must be repo-relative path:line[-line] (no absolute path, no ..)", ErrRelay, ctx, r.Ref)
		}
		return nil
	}
	if !strings.HasPrefix(r.Ref, "http://") && !strings.HasPrefix(r.Ref, "https://") {
		return fmt.Errorf("%w: %s %s ref %q must be an http(s) URL", ErrRelay, ctx, r.Type, r.Ref)
	}
	return nil
}

// ValidateEvidence enforces the R5 contract for a verdict (codex gate-1 #1/#4):
// revise / approve-with-changes need non-empty findings; approve may have empty
// findings but must carry basis_refs + commands_run + limitations so an approve
// is never evidence-free. Every finding needs a concrete ref; nothing may be a
// placeholder.
func ValidateEvidence(ev *Evidence, verdict string) error {
	switch verdict {
	case VerdictRevise, VerdictApproveWithChanges:
		if len(ev.Findings) == 0 {
			return fmt.Errorf("%w: verdict %q requires at least one finding", ErrRelay, verdict)
		}
	case VerdictApprove:
		if len(ev.BasisRefs) == 0 {
			return fmt.Errorf("%w: an approve review must carry at least one basis_ref — an approve cannot be evidence-free", ErrRelay)
		}
		if len(ev.CommandsRun) == 0 {
			return fmt.Errorf("%w: an approve review must record commands_run (validation evidence, or a stated non-execution reason)", ErrRelay)
		}
		if len(ev.Limitations) == 0 {
			return fmt.Errorf("%w: an approve review must state limitations (what was not checked)", ErrRelay)
		}
	default:
		return fmt.Errorf("%w: evidence requires a known verdict, got %q", ErrRelay, verdict)
	}
	for i := range ev.Findings {
		f := &ev.Findings[i]
		if !contains(Severities, f.Severity) {
			return fmt.Errorf("%w: finding %d severity %q not in %v", ErrRelay, i, f.Severity, Severities)
		}
		if !contains(Confidences, f.Confidence) {
			return fmt.Errorf("%w: finding %d confidence %q not in %v", ErrRelay, i, f.Confidence, Confidences)
		}
		if isPlaceholder(f.Claim) {
			return fmt.Errorf("%w: finding %d claim is empty or placeholder", ErrRelay, i)
		}
		if len(f.Refs) == 0 {
			return fmt.Errorf("%w: finding %d must have at least one ref", ErrRelay, i)
		}
		for j := range f.Refs {
			if err := validateRef(f.Refs[j], fmt.Sprintf("finding %d ref %d", i, j)); err != nil {
				return err
			}
		}
	}
	for j := range ev.BasisRefs {
		if err := validateRef(ev.BasisRefs[j], fmt.Sprintf("basis_ref %d", j)); err != nil {
			return err
		}
	}
	for _, c := range ev.CommandsRun {
		if isPlaceholder(c) {
			return fmt.Errorf("%w: commands_run entry %q is empty or placeholder", ErrRelay, c)
		}
	}
	for _, l := range ev.Limitations {
		if isPlaceholder(l) {
			return fmt.Errorf("%w: limitations entry %q is empty or placeholder", ErrRelay, l)
		}
	}
	return nil
}

// EvidenceHashFor computes the canonical evidence hash: sha256 over the
// re-marshalled payload (stable struct field order), prefixed sha256:. Both
// Publish and the close gate derive it the same way, so a tampered body
// re-parses to a different hash and fails the gate.
func EvidenceHashFor(ev *Evidence) (string, error) {
	raw, err := json.Marshal(ev)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// reviewEvidenceHash is the publish/close shared helper: parse + validate the
// body's evidence for the given verdict, then return its canonical hash.
func reviewEvidenceHash(body, verdict string) (string, error) {
	ev, err := ParseEvidence(body)
	if err != nil {
		return "", err
	}
	if err := ValidateEvidence(ev, verdict); err != nil {
		return "", err
	}
	return EvidenceHashFor(ev)
}
