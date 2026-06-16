package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrGate marks an UNSATISFIED quality gate (R4): an expected "not approvable
// yet" condition — no lead decision/receipt, no non-lead approve review of the
// latest work, wrong verdict, or a target that does not match the reviewed
// head. The CLI maps it to exit 4. Corruption (malformed receipt, missing
// referenced artifact, hash mismatch, tamper) stays ErrRelay → exit 3.
var ErrGate = errors.New("relay quality gate not satisfied")

// ReceiptSchema versions the completion-receipt payload. /2 (R5) adds the
// quality-gate review's evidence hash to the bound GateRef.
const ReceiptSchema = "oma-completion-receipt/2"

// Receipt is the canonical completion-receipt embedded in a kind:decision
// artifact. It makes "done" falsifiable (A1/A2): it binds the WORK being
// approved (reviewed_head — the latest non-review/non-decision artifact) to
// the non-lead approve review that TARGETED it, by content hash.
// close --outcome approve re-verifies all of it and refuses if newer
// unreviewed work has appeared since.
type Receipt struct {
	Schema       string    `json:"schema"`
	Pair         string    `json:"pair"`
	DecisionSeq  int       `json:"decision_seq"`
	ReviewedHead Ref       `json:"reviewed_head"`
	QualityGate  GateRef   `json:"quality_gate_ref"`
	VerifiedAt   time.Time `json:"verified_at"`
}

// Ref binds an artifact by seq and the sha256 of its rendered bytes.
type Ref struct {
	Seq  int    `json:"seq"`
	Hash string `json:"hash"`
}

// GateRef binds the non-lead approve review by seq, verdict, artifact hash,
// and (R5) the canonical hash of its oma-review-evidence/1 block.
type GateRef struct {
	Seq          int    `json:"seq"`
	Verdict      string `json:"verdict"`
	Hash         string `json:"hash"`
	EvidenceHash string `json:"evidence_hash"`
}

// ID is the sha256 of the canonical receipt JSON (struct field order is
// stable, so json.Marshal is deterministic). VerifiedAt is truncated to a
// whole second at build time so the id reproduces from the frontmatter.
func (r *Receipt) ID() string {
	raw, _ := json.Marshal(r)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// applyTo writes the receipt's fields onto a decision frontmatter.
func (r *Receipt) applyTo(fm *Frontmatter) {
	headSeq, gateSeq := r.ReviewedHead.Seq, r.QualityGate.Seq
	t := r.VerifiedAt
	fm.ReceiptID = r.ID()
	fm.ReviewedHeadSeq, fm.ReviewedHeadHash = &headSeq, r.ReviewedHead.Hash
	fm.QualityGateSeq, fm.QualityGateHash = &gateSeq, r.QualityGate.Hash
	fm.QualityGateEvidenceHash = r.QualityGate.EvidenceHash
	fm.VerifiedAt = &t
}

// readyArtifact is a published artifact plus its content hash.
type readyArtifact struct {
	Seq    int
	Author string
	Kind   string
	Path   string
	Hash   string // sha256:<hex> of the rendered bytes
	FM     *Frontmatter
}

func hashFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// readyArtifacts lists this pair's published artifacts (sidecar-verified),
// sorted by seq, each with its content hash.
func (l *Ledger) readyArtifacts(slug string) ([]readyArtifact, error) {
	pairDir := l.PairDir(slug)
	names, err := publishedArtifacts(pairDir, true)
	if err != nil {
		return nil, err
	}
	var out []readyArtifact
	for _, name := range names {
		path := filepath.Join(pairDir, name)
		fm, _, err := ReadArtifact(path) // verifies .ready + .sha256 integrity
		if err != nil {
			return nil, err
		}
		h, err := hashFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, readyArtifact{Seq: fm.Seq, Author: fm.Author, Kind: fm.Kind, Path: path, Hash: h, FM: fm})
	}
	return out, nil
}

// latestWork returns the latest artifact that is neither a review nor a
// decision — the substantive work a close --outcome approve must have had
// reviewed. nil when only reviews/decisions exist.
func latestWork(arts []readyArtifact) *readyArtifact {
	for i := len(arts) - 1; i >= 0; i-- {
		if k := arts[i].Kind; k != "review" && k != "decision" {
			return &arts[i]
		}
	}
	return nil
}

// buildDecisionReceipt assembles the receipt for a decision at decisionSeq.
// It returns (nil, nil) when no approve-receipt can be formed — i.e. the
// latest work has no non-lead `kind:review` with verdict approve that
// TARGETS it (R1). A decision may still be published (e.g. summarizing a
// rejected pair); it just will not satisfy close --outcome approve.
func (l *Ledger) buildDecisionReceipt(slug string, decisionSeq int) (*Receipt, error) {
	s, err := l.LoadSession(slug)
	if err != nil {
		return nil, err
	}
	lead := s.Roles["lead"]
	arts, err := l.readyArtifacts(slug)
	if err != nil {
		return nil, err
	}
	head := latestWork(arts)
	if head == nil {
		return nil, nil
	}
	var review *readyArtifact
	for i := range arts {
		a := &arts[i]
		if a.Kind == "review" && a.Author != lead && a.FM.Verdict == VerdictApprove &&
			a.FM.ReviewTargetSeq != nil && *a.FM.ReviewTargetSeq == head.Seq && a.Seq > head.Seq {
			review = a // sorted ascending → keep the latest matching review
		}
	}
	if review == nil {
		return nil, nil
	}
	return &Receipt{
		Schema: ReceiptSchema, Pair: slug, DecisionSeq: decisionSeq,
		ReviewedHead: Ref{Seq: head.Seq, Hash: head.Hash},
		QualityGate:  GateRef{Seq: review.Seq, Verdict: review.FM.Verdict, Hash: review.Hash, EvidenceHash: review.FM.EvidenceHash},
		VerifiedAt:   l.Now().UTC().Truncate(time.Second),
	}, nil
}

// verifyApproveClose is the A2 close gate: close --outcome approve is
// fail-closed unless the latest lead kind:decision carries a valid receipt
// over a non-lead approve review that targeted the reviewed head, the
// referenced artifacts still hash-match, and no newer unreviewed work exists
// (R1/R2). reject/abandon never call this.
func (l *Ledger) verifyApproveClose(slug string) error {
	s, err := l.LoadSession(slug)
	if err != nil {
		return err
	}
	lead := s.Roles["lead"]
	arts, err := l.readyArtifacts(slug)
	if err != nil {
		return err
	}
	bySeq := map[int]*readyArtifact{}
	var decision *readyArtifact
	for i := range arts {
		a := &arts[i]
		bySeq[a.Seq] = a
		if a.Kind == "decision" && a.Author == lead {
			decision = a // latest lead decision
		}
	}
	if decision == nil {
		return fmt.Errorf("%w: close --outcome approve needs a lead kind:decision with a completion receipt (publish one after a non-lead approve review of the latest work)", ErrGate)
	}
	fm := decision.FM
	if fm.ReceiptID == "" || fm.ReviewedHeadSeq == nil || fm.QualityGateSeq == nil || fm.VerifiedAt == nil {
		return fmt.Errorf("%w: lead decision seq %d carries no completion receipt", ErrGate, fm.Seq)
	}
	// Re-derive the receipt id from the stored fields: a mismatch means the
	// decision header was hand-edited or is malformed (corruption → exit 3).
	r := &Receipt{
		Schema: ReceiptSchema, Pair: slug, DecisionSeq: fm.Seq,
		ReviewedHead: Ref{Seq: *fm.ReviewedHeadSeq, Hash: fm.ReviewedHeadHash},
		QualityGate:  GateRef{Seq: *fm.QualityGateSeq, Verdict: VerdictApprove, Hash: fm.QualityGateHash, EvidenceHash: fm.QualityGateEvidenceHash},
		VerifiedAt:   fm.VerifiedAt.UTC(),
	}
	if r.ID() != fm.ReceiptID {
		return fmt.Errorf("%w: decision seq %d receipt id mismatch (tampered or malformed)", ErrRelay, fm.Seq)
	}
	// The reviewed head must still exist and hash-match (corruption → exit 3).
	head := bySeq[*fm.ReviewedHeadSeq]
	if head == nil {
		return fmt.Errorf("%w: receipt reviewed-head seq %d is not a ready artifact", ErrRelay, *fm.ReviewedHeadSeq)
	}
	if head.Hash != fm.ReviewedHeadHash {
		return fmt.Errorf("%w: reviewed-head seq %d changed since the receipt (hash mismatch)", ErrRelay, head.Seq)
	}
	// R1: no newer substantive work may exist after the reviewed head.
	if lw := latestWork(arts); lw != nil && lw.Seq > head.Seq {
		return fmt.Errorf("%w: seq %d (%s) was published after the reviewed head seq %d with no approve review", ErrGate, lw.Seq, lw.Kind, head.Seq)
	}
	// The quality-gate review must still exist (corruption → exit 3) ...
	rev := bySeq[*fm.QualityGateSeq]
	if rev == nil || rev.Kind != "review" {
		return fmt.Errorf("%w: receipt references seq %d which is not a ready review", ErrRelay, *fm.QualityGateSeq)
	}
	if rev.Hash != fm.QualityGateHash {
		return fmt.Errorf("%w: approve review seq %d changed since the receipt (hash mismatch)", ErrRelay, rev.Seq)
	}
	// ... and satisfy the gate: non-lead, approve, targeting the reviewed head
	// (gate miss → exit 4).
	if rev.Author == lead {
		return fmt.Errorf("%w: the approve review (seq %d) must be by the non-lead reviewer, not the lead", ErrGate, rev.Seq)
	}
	if rev.FM.Verdict != VerdictApprove {
		return fmt.Errorf("%w: review seq %d verdict is %q, not approve (approve-with-changes does not satisfy close)", ErrGate, rev.Seq, rev.FM.Verdict)
	}
	if rev.FM.ReviewTargetSeq == nil || *rev.FM.ReviewTargetSeq != head.Seq {
		return fmt.Errorf("%w: approve review seq %d does not target the reviewed head seq %d", ErrGate, rev.Seq, head.Seq)
	}
	if rev.Seq <= head.Seq {
		return fmt.Errorf("%w: approve review seq %d is not newer than the reviewed head seq %d (it could not have reviewed its content)", ErrGate, rev.Seq, head.Seq)
	}
	// R5: re-derive the quality-gate review's evidence hash from its body and
	// require it to agree with both the review frontmatter and the receipt
	// (additive over the artifact-hash checks above; tamper/corruption → exit 3).
	_, revBody, rerr := ReadArtifact(rev.Path)
	if rerr != nil {
		return rerr
	}
	recomputed, eerr := reviewEvidenceHash(revBody, VerdictApprove)
	if eerr != nil {
		return fmt.Errorf("%w: quality-gate review seq %d evidence is invalid: %v", ErrRelay, rev.Seq, eerr)
	}
	if recomputed != rev.FM.EvidenceHash {
		return fmt.Errorf("%w: quality-gate review seq %d body evidence does not match its evidence_hash (tampered)", ErrRelay, rev.Seq)
	}
	if rev.FM.EvidenceHash != fm.QualityGateEvidenceHash {
		return fmt.Errorf("%w: decision quality_gate_evidence_hash does not match approve review seq %d evidence_hash", ErrRelay, rev.Seq)
	}
	return nil
}
