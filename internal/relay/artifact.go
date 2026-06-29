package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sean2077/oh-my-agents/internal/schemaver"
)

// Artifact kinds and statuses (protocol §5).
var (
	Kinds    = []string{"plan", "review", "fix", "note", "question", "decision", "correction", "addendum"}
	Statuses = []string{"ready", "closed", "cancelled", "failed", "timed_out"}
)

// Verdict values for kind:review (A2). approve-with-changes does NOT satisfy
// the close gate — only approve does.
const (
	VerdictApprove            = "approve"
	VerdictApproveWithChanges = "approve-with-changes"
	VerdictRevise             = "revise"
)

// Verdicts is the closed set a review verdict may take.
var Verdicts = []string{VerdictApprove, VerdictApproveWithChanges, VerdictRevise}

// ValidKind / ValidStatus report membership in the §5 sets.
func ValidKind(k string) bool   { return contains(Kinds, k) }
func ValidStatus(s string) bool { return contains(Statuses, s) }

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// Frontmatter is the oma-relay/4 artifact header (protocol §5). The A1/A2
// fields (verdict on kind:review, receipt on kind:decision) are optional
// and absent (zero) on every other kind; the strict parser still rejects
// any key it does not know.
type Frontmatter struct {
	Schema        string
	Seq           int
	Author        string
	AuthorSession string
	Peer          string
	Kind          string
	Status        string
	Created       time.Time
	InReplyTo     *int
	PromptForNext string
	TouchedPaths  []string
	Corrects      *int

	// A1/A2 review-gate fields — set on kind:review only:
	Verdict         string // approve | approve-with-changes | revise
	ReviewTargetSeq *int   // the seq this review judges
	// Completion-receipt fields — set on kind:decision only. The receipt
	// makes "done" falsifiable: it binds the approved plan, the non-lead
	// approve review and the ledger head by content hash, so a decision can
	// be proven to have been reviewed against the exact artifacts named.
	ReceiptID        string     // sha256:<hex> of the canonical receipt JSON
	ReviewedHeadSeq  *int       // the work being approved (latest non-review/non-decision artifact)
	ReviewedHeadHash string     // sha256:<hex> of that artifact's rendered bytes
	QualityGateSeq   *int       // the non-lead approve review that targets the reviewed head
	QualityGateHash  string     // sha256:<hex> of that review's rendered bytes
	VerifiedAt       *time.Time // receipt timestamp

	// R5 (oma-relay/4). EvidenceHash is the canonical hash of the review
	// body's oma-review-evidence/1 block — set on kind:review only.
	// QualityGateEvidenceHash binds that evidence hash into the decision
	// receipt — set on kind:decision only (additive over the A1/A2 receipt).
	EvidenceHash            string
	QualityGateEvidenceHash string
}

// Validate enforces the §5 contract on a parsed or about-to-render header.
func (f *Frontmatter) Validate() error {
	if major, ok := schemaver.Major(f.Schema, "oma-relay"); !ok || major != 4 {
		return fmt.Errorf("%w: artifact schema %q, want %s", ErrRelay, f.Schema, ArtifactSchema)
	}
	if f.Seq < 1 || f.Seq > 999 {
		return fmt.Errorf("%w: seq %d out of range 1..999", ErrRelay, f.Seq)
	}
	if !authorRe.MatchString(f.Author) || !authorRe.MatchString(f.Peer) {
		return fmt.Errorf("%w: author/peer %q/%q invalid", ErrRelay, f.Author, f.Peer)
	}
	if !sessionKeyRe.MatchString(f.AuthorSession) {
		return fmt.Errorf("%w: author_session %q invalid", ErrRelay, f.AuthorSession)
	}
	if f.Author == f.Peer {
		return fmt.Errorf("%w: author equals peer (%s)", ErrRelay, f.Author)
	}
	if !ValidKind(f.Kind) {
		return fmt.Errorf("%w: kind %q not in %v", ErrRelay, f.Kind, Kinds)
	}
	if !ValidStatus(f.Status) {
		return fmt.Errorf("%w: status %q not in %v", ErrRelay, f.Status, Statuses)
	}
	if f.Created.IsZero() {
		return fmt.Errorf("%w: created timestamp missing", ErrRelay)
	}
	for _, p := range f.TouchedPaths {
		clean := filepath.ToSlash(filepath.Clean(p))
		if p == "" || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("%w: touched path %q must be repo-relative without escapes", ErrRelay, p)
		}
	}
	return f.validateGateFields()
}

// validateGateFields enforces the A1/A2 optional fields: verdict shape,
// kind-locality (review fields only on kind:review, receipt fields only on
// kind:decision) and sha256 hash shape — so the close gate can trust what
// it reads without re-deriving it (fail-closed).
func (f *Frontmatter) validateGateFields() error {
	if f.Verdict != "" {
		if !contains(Verdicts, f.Verdict) {
			return fmt.Errorf("%w: verdict %q not in %v", ErrRelay, f.Verdict, Verdicts)
		}
		if f.Kind != "review" {
			return fmt.Errorf("%w: verdict is only valid on kind:review (got %s)", ErrRelay, f.Kind)
		}
	}
	if f.ReviewTargetSeq != nil {
		if f.Kind != "review" {
			return fmt.Errorf("%w: review_target_seq is only valid on kind:review (got %s)", ErrRelay, f.Kind)
		}
		if *f.ReviewTargetSeq < 1 {
			return fmt.Errorf("%w: review_target_seq %d must be >= 1", ErrRelay, *f.ReviewTargetSeq)
		}
	}
	if f.EvidenceHash != "" && f.Kind != "review" {
		return fmt.Errorf("%w: evidence_hash is only valid on kind:review (got %s)", ErrRelay, f.Kind)
	}
	receiptSet := f.ReceiptID != "" || f.ReviewedHeadSeq != nil || f.ReviewedHeadHash != "" ||
		f.QualityGateSeq != nil || f.QualityGateHash != "" || f.QualityGateEvidenceHash != "" || f.VerifiedAt != nil
	if receiptSet && f.Kind != "decision" {
		return fmt.Errorf("%w: receipt fields are only valid on kind:decision (got %s)", ErrRelay, f.Kind)
	}
	for _, h := range []string{f.ReceiptID, f.ReviewedHeadHash, f.QualityGateHash, f.EvidenceHash, f.QualityGateEvidenceHash} {
		if h != "" && !strings.HasPrefix(h, "sha256:") {
			return fmt.Errorf("%w: hash %q must be sha256:<hex>", ErrRelay, h)
		}
	}
	return nil
}

// ArtifactName is the canonical published filename for a header.
func ArtifactName(seq int, author, kind string) string {
	return fmt.Sprintf("%03d-%s-%s.md", seq, author, kind)
}

var nameParseRe = regexp.MustCompile(`^(\d{3})-([a-z0-9][a-z0-9-]{0,31})-([a-z]+)\.md$`)

// ParseArtifactName splits NNN-<author>-<kind>.md.
func ParseArtifactName(name string) (seq int, author, kind string, ok bool) {
	m := nameParseRe.FindStringSubmatch(name)
	if m == nil {
		return 0, "", "", false
	}
	seq, err := strconv.Atoi(m[1])
	if err != nil || seq < 1 || !ValidKind(m[3]) {
		return 0, "", "", false
	}
	return seq, m[2], m[3], true
}

// Render produces the full artifact bytes: strict YAML frontmatter in a
// fixed key order, then the body. Parse reads back exactly this subset
// and nothing more (fail-closed on anything fancier).
func Render(f *Frontmatter, body string) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "schema: %s\n", f.Schema)
	fmt.Fprintf(&b, "seq: %d\n", f.Seq)
	fmt.Fprintf(&b, "author: %s\n", f.Author)
	fmt.Fprintf(&b, "author_session: %s\n", f.AuthorSession)
	fmt.Fprintf(&b, "peer: %s\n", f.Peer)
	fmt.Fprintf(&b, "kind: %s\n", f.Kind)
	fmt.Fprintf(&b, "status: %s\n", f.Status)
	fmt.Fprintf(&b, "created: %s\n", f.Created.UTC().Format(time.RFC3339))
	if f.InReplyTo != nil {
		fmt.Fprintf(&b, "in_reply_to: %d\n", *f.InReplyTo)
	} else {
		b.WriteString("in_reply_to: null\n")
	}
	if f.PromptForNext == "" {
		b.WriteString("prompt_for_next: \"\"\n")
	} else {
		b.WriteString("prompt_for_next: |\n")
		for _, line := range strings.Split(strings.TrimRight(f.PromptForNext, "\n"), "\n") {
			b.WriteString("  " + line + "\n")
		}
	}
	if len(f.TouchedPaths) == 0 {
		b.WriteString("touched_paths: []\n")
	} else {
		b.WriteString("touched_paths:\n")
		for _, p := range f.TouchedPaths {
			b.WriteString("  - " + p + "\n")
		}
	}
	if f.Corrects != nil {
		fmt.Fprintf(&b, "corrects: %d\n", *f.Corrects)
	} else {
		b.WriteString("corrects: null\n")
	}
	// A1/A2 fields render only when set, in a fixed order, after the base
	// header (the strict parser reads them back by key, so order is for
	// humans / stable digests).
	renderScalar(&b, "verdict", f.Verdict)
	renderOptInt(&b, "review_target_seq", f.ReviewTargetSeq)
	renderScalar(&b, "evidence_hash", f.EvidenceHash)
	renderScalar(&b, "receipt_id", f.ReceiptID)
	renderOptInt(&b, "reviewed_head_seq", f.ReviewedHeadSeq)
	renderScalar(&b, "reviewed_head_hash", f.ReviewedHeadHash)
	renderOptInt(&b, "quality_gate_seq", f.QualityGateSeq)
	renderScalar(&b, "quality_gate_hash", f.QualityGateHash)
	renderScalar(&b, "quality_gate_evidence_hash", f.QualityGateEvidenceHash)
	if f.VerifiedAt != nil {
		fmt.Fprintf(&b, "verified_at: %s\n", f.VerifiedAt.UTC().Format(time.RFC3339))
	}
	b.WriteString("---\n")
	b.WriteString(body)
	return []byte(b.String())
}

// Parse reads artifact bytes rendered by Render (or hand-written within
// the same strict subset). Unknown keys, unknown shapes and a missing
// closing fence fail closed.
func Parse(raw []byte) (*Frontmatter, string, error) {
	lines := strings.Split(string(raw), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != "---" {
		return nil, "", fmt.Errorf("%w: artifact must start with ---", ErrRelay)
	}
	f := &Frontmatter{}
	seen := map[string]bool{}
	i := 1
	for ; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimRight(line, " \t") == "---" {
			body := strings.Join(lines[i+1:], "\n")
			if err := f.Validate(); err != nil {
				return nil, "", err
			}
			return f, body, nil
		}
		key, value, found := strings.Cut(line, ":")
		if !found || strings.HasPrefix(line, " ") {
			return nil, "", fmt.Errorf("%w: unexpected frontmatter line %q", ErrRelay, line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		// Duplicate keys are first-wins/last-wins ambiguity: fail closed,
		// same rule as the hook host-config parser (review 054 hardening).
		if seen[key] {
			return nil, "", fmt.Errorf("%w: duplicate frontmatter key %q", ErrRelay, key)
		}
		seen[key] = true
		var err error
		switch key {
		case "schema":
			f.Schema = value
		case "seq":
			f.Seq, err = strconv.Atoi(value)
		case "author":
			f.Author = value
		case "author_session":
			f.AuthorSession = value
		case "peer":
			f.Peer = value
		case "kind":
			f.Kind = value
		case "status":
			f.Status = value
		case "created":
			f.Created, err = time.Parse(time.RFC3339, value)
		case "in_reply_to":
			f.InReplyTo, err = parseOptInt(value)
		case "corrects":
			f.Corrects, err = parseOptInt(value)
		case "verdict":
			f.Verdict = value
		case "review_target_seq":
			f.ReviewTargetSeq, err = parseOptInt(value)
		case "evidence_hash":
			f.EvidenceHash = value
		case "receipt_id":
			f.ReceiptID = value
		case "reviewed_head_seq":
			f.ReviewedHeadSeq, err = parseOptInt(value)
		case "reviewed_head_hash":
			f.ReviewedHeadHash = value
		case "quality_gate_seq":
			f.QualityGateSeq, err = parseOptInt(value)
		case "quality_gate_hash":
			f.QualityGateHash = value
		case "quality_gate_evidence_hash":
			f.QualityGateEvidenceHash = value
		case "verified_at":
			f.VerifiedAt, err = parseOptTime(value)
		case "prompt_for_next":
			switch value {
			case "|", "|-":
				var block []string
				for i+1 < len(lines) && (strings.HasPrefix(lines[i+1], "  ") || strings.TrimSpace(lines[i+1]) == "") {
					if strings.TrimRight(lines[i+1], " \t") == "---" {
						break
					}
					block = append(block, strings.TrimPrefix(lines[i+1], "  "))
					i++
				}
				f.PromptForNext = strings.TrimRight(strings.Join(block, "\n"), "\n")
			case `""`, "''", "":
				f.PromptForNext = ""
			default:
				f.PromptForNext = strings.Trim(value, `"'`)
			}
		case "touched_paths":
			if value == "[]" {
				f.TouchedPaths = []string{}
				continue
			}
			if value != "" {
				return nil, "", fmt.Errorf("%w: touched_paths must be [] or a block sequence", ErrRelay)
			}
			for i+1 < len(lines) && strings.HasPrefix(lines[i+1], "  - ") {
				f.TouchedPaths = append(f.TouchedPaths, strings.TrimSpace(strings.TrimPrefix(lines[i+1], "  - ")))
				i++
			}
		default:
			return nil, "", fmt.Errorf("%w: unknown frontmatter key %q (fail-closed)", ErrRelay, key)
		}
		if err != nil {
			return nil, "", fmt.Errorf("%w: frontmatter %s: %v", ErrRelay, key, err)
		}
	}
	return nil, "", fmt.Errorf("%w: frontmatter never closed with ---", ErrRelay)
}

func parseOptInt(v string) (*int, error) {
	if v == "null" || v == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func parseOptTime(v string) (*time.Time, error) {
	if v == "null" || v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// renderScalar writes "key: val\n" only when val is non-empty (A1/A2
// optional fields render only when set).
func renderScalar(b *strings.Builder, key, val string) {
	if val != "" {
		fmt.Fprintf(b, "%s: %s\n", key, val)
	}
}

// renderOptInt writes "key: n\n" only when v is non-nil.
func renderOptInt(b *strings.Builder, key string, v *int) {
	if v != nil {
		fmt.Fprintf(b, "%s: %d\n", key, *v)
	}
}

// ReadArtifact loads and verifies one published artifact: the .ready
// sidecar must exist (the only publication criterion) and the .sha256
// sidecar must match the content — anything else is reported corrupt and
// the content is not returned (protocol §7).
func ReadArtifact(path string) (*Frontmatter, string, error) {
	if _, err := os.Stat(path + ".ready"); err != nil {
		return nil, "", fmt.Errorf("%w: %s has no .ready sidecar (unpublished or interrupted)", ErrRelay, filepath.Base(path))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	wantHex, err := os.ReadFile(path + ".sha256")
	if err != nil {
		return nil, "", fmt.Errorf("%w: %s has no .sha256 sidecar", ErrRelay, filepath.Base(path))
	}
	sum := sha256.Sum256(raw)
	if got, want := hex.EncodeToString(sum[:]), strings.TrimSpace(string(wantHex)); got != want {
		return nil, "", fmt.Errorf("%w: %s content does not match .sha256 (corrupt or tampered; content withheld)", ErrRelay, filepath.Base(path))
	}
	return parseAndReturn(raw)
}

func parseAndReturn(raw []byte) (*Frontmatter, string, error) {
	f, body, err := Parse(raw)
	if err != nil {
		return nil, "", err
	}
	return f, body, nil
}

// publishedArtifacts lists formal artifacts in a pair dir (sorted by
// filename => by seq). readyOnly filters to those with .ready sidecars.
func publishedArtifacts(pairDir string, readyOnly bool) ([]string, error) {
	entries, err := os.ReadDir(pairDir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		if _, _, _, ok := ParseArtifactName(ent.Name()); !ok {
			continue
		}
		if readyOnly {
			if _, err := os.Stat(filepath.Join(pairDir, ent.Name()+".ready")); err != nil {
				continue
			}
		}
		names = append(names, ent.Name())
	}
	sort.Strings(names)
	return names, nil
}
