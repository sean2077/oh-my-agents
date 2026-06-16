package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ReceiptSchema versions the completion-receipt payload (A1).
const ReceiptSchema = "oma-completion-receipt/1"

// Receipt is the canonical completion-receipt embedded in a kind:decision
// artifact. It makes "done" falsifiable (A1, docs/borrow-from-omx-omc.md):
// hashing the receipt — and re-hashing the artifacts it names at close
// time — proves the decision was reviewed against the EXACT approved plan,
// the non-lead approve review, and the ledger head recorded, defeating the
// "agent declared done after the plan drifted" failure mode.
type Receipt struct {
	Schema      string    `json:"schema"`
	Pair        string    `json:"pair"`
	DecisionSeq int       `json:"decision_seq"`
	PlanRef     Ref       `json:"plan_ref"`
	QualityGate GateRef   `json:"quality_gate_ref"`
	LedgerHead  Ref       `json:"ledger_head"`
	VerifiedAt  time.Time `json:"verified_at"`
}

// Ref binds an artifact by seq and the sha256 of its rendered bytes.
type Ref struct {
	Seq  int    `json:"seq"`
	Hash string `json:"hash"`
}

// GateRef binds the non-lead approve review by seq, verdict and hash.
type GateRef struct {
	Seq     int    `json:"seq"`
	Verdict string `json:"verdict"`
	Hash    string `json:"hash"`
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
	planSeq, gateSeq, headSeq := r.PlanRef.Seq, r.QualityGate.Seq, r.LedgerHead.Seq
	t := r.VerifiedAt
	fm.ReceiptID = r.ID()
	fm.PlanRefSeq, fm.PlanRefHash = &planSeq, r.PlanRef.Hash
	fm.QualityGateSeq, fm.QualityGateHash = &gateSeq, r.QualityGate.Hash
	fm.LedgerHeadSeq, fm.LedgerHeadHash = &headSeq, r.LedgerHead.Hash
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

// buildDecisionReceipt assembles the receipt for a decision at decisionSeq.
// It returns (nil, nil) when no approve-receipt can be formed — i.e. there
// is no approved plan plus a non-lead `kind:review` with verdict approve —
// so a decision may still be published (e.g. summarizing a rejected pair),
// it just will not satisfy `close --outcome approve`.
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
	var plan, review, head *readyArtifact
	for i := range arts {
		a := &arts[i]
		head = a // sorted by seq → last wins = latest ready
		switch {
		case a.Kind == "plan":
			plan = a
		case a.Kind == "review" && a.Author != lead && a.FM.Verdict == VerdictApprove:
			review = a
		}
	}
	if plan == nil || review == nil || head == nil {
		return nil, nil
	}
	return &Receipt{
		Schema: ReceiptSchema, Pair: slug, DecisionSeq: decisionSeq,
		PlanRef:     Ref{Seq: plan.Seq, Hash: plan.Hash},
		QualityGate: GateRef{Seq: review.Seq, Verdict: review.FM.Verdict, Hash: review.Hash},
		LedgerHead:  Ref{Seq: head.Seq, Hash: head.Hash},
		VerifiedAt:  l.Now().UTC().Truncate(time.Second),
	}, nil
}

// verifyApproveClose is the A2 close gate: `close --outcome approve` is
// fail-closed unless the latest lead kind:decision carries a valid receipt
// whose referenced plan and non-lead approve review still hash-match.
// approve-with-changes never satisfies it. reject/abandon do not call this.
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
		return fmt.Errorf("%w: close --outcome approve needs a lead kind:decision carrying a completion receipt; publish one after a non-lead approve review (or close --outcome reject|abandon)", ErrRelay)
	}
	fm := decision.FM
	if fm.ReceiptID == "" || fm.PlanRefSeq == nil || fm.QualityGateSeq == nil || fm.LedgerHeadSeq == nil || fm.VerifiedAt == nil {
		return fmt.Errorf("%w: lead decision seq %d carries no completion receipt; approve is not falsifiable", ErrRelay, fm.Seq)
	}
	// Re-derive the receipt id from the stored fields: a mismatch means the
	// decision header was hand-edited or is malformed.
	r := &Receipt{
		Schema: ReceiptSchema, Pair: slug, DecisionSeq: fm.Seq,
		PlanRef:     Ref{Seq: *fm.PlanRefSeq, Hash: fm.PlanRefHash},
		QualityGate: GateRef{Seq: *fm.QualityGateSeq, Verdict: VerdictApprove, Hash: fm.QualityGateHash},
		LedgerHead:  Ref{Seq: *fm.LedgerHeadSeq, Hash: fm.LedgerHeadHash},
		VerifiedAt:  fm.VerifiedAt.UTC(),
	}
	if r.ID() != fm.ReceiptID {
		return fmt.Errorf("%w: decision seq %d receipt id mismatch (tampered or malformed)", ErrRelay, fm.Seq)
	}
	rev := bySeq[*fm.QualityGateSeq]
	if rev == nil || rev.Kind != "review" {
		return fmt.Errorf("%w: receipt references seq %d which is not a ready review", ErrRelay, *fm.QualityGateSeq)
	}
	if rev.Author == lead {
		return fmt.Errorf("%w: the approve review (seq %d) must be by the non-lead reviewer, not the lead", ErrRelay, rev.Seq)
	}
	if rev.FM.Verdict != VerdictApprove {
		return fmt.Errorf("%w: review seq %d verdict is %q, not approve (approve-with-changes does not satisfy close)", ErrRelay, rev.Seq, rev.FM.Verdict)
	}
	if rev.Hash != fm.QualityGateHash {
		return fmt.Errorf("%w: approve review seq %d changed since the receipt (hash mismatch)", ErrRelay, rev.Seq)
	}
	plan := bySeq[*fm.PlanRefSeq]
	if plan == nil || plan.Hash != fm.PlanRefHash {
		return fmt.Errorf("%w: receipt plan seq %d missing or changed since the receipt (hash mismatch)", ErrRelay, *fm.PlanRefSeq)
	}
	return nil
}
