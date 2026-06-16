package relay

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PublishInput carries the publish-time fill of a draft. Body and Prompt
// are file contents already read by the caller; empty means "keep what
// the draft holds" (interrupted-publish re-runs may omit them).
type PublishInput struct {
	Body    string
	Prompt  string
	Touched []string
	Status  string // default "ready"
	// A1/A2 (kind:review): the typed verdict and the seq it judges. An
	// empty Verdict leaves the draft's value untouched (idempotent re-run);
	// ReviewTarget defaults to the draft's in_reply_to when omitted.
	Verdict      string
	ReviewTarget *int
}

// Publish runs the §7 transaction: render the draft into its final form
// (durable publish intent), then formal.tmp → rename → .sha256 → .ready
// → delete draft + .seq reservation, in that strict order. Every step is
// idempotent so re-running the same command after a kill converges; a
// formal file that disagrees with the draft render fails closed.
func (l *Ledger) Publish(draftPath string, in PublishInput, dryRun bool) (string, error) {
	pairDir, seq, kind, err := l.ownDraftPath(draftPath)
	if err != nil {
		return "", err
	}
	slug := filepath.Base(pairDir)
	s, err := l.LoadSession(slug)
	if err != nil {
		return "", err
	}
	if s.Terminal() {
		return "", fmt.Errorf("%w: pair %s is %s", ErrRelay, slug, s.Status)
	}
	raw, err := os.ReadFile(draftPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w: draft %s does not exist (already published and cleaned, or never created)", ErrRelay, draftPath)
	}
	if err != nil {
		return "", err
	}
	fm, body, err := Parse(raw)
	if err != nil {
		return "", fmt.Errorf("draft %s: %w", filepath.Base(draftPath), err)
	}
	if fm.Seq != seq || fm.Author != l.Identity.Author || fm.Kind != kind {
		return "", fmt.Errorf("%w: draft frontmatter (seq %d author %s kind %s) does not match its filename", ErrRelay, fm.Seq, fm.Author, fm.Kind)
	}

	// Fill from publish inputs (re-renders the durable intent).
	if in.Body != "" {
		body = in.Body
	}
	if in.Prompt != "" {
		fm.PromptForNext = in.Prompt
	}
	if len(in.Touched) > 0 {
		fm.TouchedPaths = in.Touched
	}
	if in.Status != "" {
		fm.Status = in.Status
	}
	if fm.Status == "" {
		fm.Status = "ready"
	}
	// A1/A2: a review carries its verdict + target; a decision auto-builds
	// the completion receipt the close gate verifies. The receipt is built
	// only once (guarded by ReceiptID): an interrupted re-publish re-parses
	// the already-rendered draft and must reproduce identical bytes, so its
	// VerifiedAt timestamp must not change between runs.
	if fm.Kind == "review" {
		if in.Verdict != "" {
			fm.Verdict = in.Verdict
		}
		if in.ReviewTarget != nil {
			fm.ReviewTargetSeq = in.ReviewTarget
		} else if fm.ReviewTargetSeq == nil {
			fm.ReviewTargetSeq = fm.InReplyTo
		}
	}
	if fm.Kind == "decision" && fm.ReceiptID == "" {
		rcpt, rerr := l.buildDecisionReceipt(slug, fm.Seq)
		if rerr != nil {
			return "", rerr
		}
		if rcpt != nil {
			rcpt.applyTo(fm)
		}
	}
	if err := fm.Validate(); err != nil {
		return "", err
	}
	if strings.Contains(body, "TODO:") {
		return "", fmt.Errorf("%w: draft body still contains a TODO: placeholder; provide --body-file", ErrRelay)
	}
	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("%w: artifact body is empty; provide --body-file", ErrRelay)
	}
	if strings.TrimSpace(fm.PromptForNext) == "" {
		return "", fmt.Errorf("%w: prompt_for_next is empty; provide --prompt-file (a blank prompt wastes the peer's round)", ErrRelay)
	}
	rendered := Render(fm, body)
	if findings := ScanSecrets(rendered); len(findings) > 0 {
		return "", fmt.Errorf("%w: secret patterns detected (publish blocked, no bypass — security-contract.md §6):\n  %s",
			ErrRelay, strings.Join(findings, "\n  "))
	}

	formal := filepath.Join(pairDir, ArtifactName(fm.Seq, fm.Author, fm.Kind))
	// Fail-closed divergence check BEFORE any write: an existing formal
	// file must equal this render byte-for-byte (interrupted publish), or
	// the ledger needs doctor attention.
	if existing, err := os.ReadFile(formal); err == nil {
		if !bytes.Equal(existing, rendered) {
			return "", fmt.Errorf("%w: %s already exists and differs from the draft render — run `oma doctor relay --clean-stale` to quarantine the incomplete formal file", ErrRelay, filepath.Base(formal))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if dryRun {
		return formal, nil
	}
	l.touchHeartbeat(slug)

	// Step 1: persist the rendered draft (durable intent).
	if !bytes.Equal(raw, rendered) {
		if err := writeFileAtomic(draftPath, rendered, 0o600); err != nil {
			return "", err
		}
	}
	if err := l.step("draft-rendered"); err != nil {
		return "", err
	}
	// Step 2: formal tmp + rename (skipped when an identical formal
	// already exists from an interrupted run).
	if existing, err := os.ReadFile(formal); err != nil || !bytes.Equal(existing, rendered) {
		if err := writeFileAtomic(formal, rendered, 0o600); err != nil {
			return "", err
		}
	}
	if err := l.step("formal-renamed"); err != nil {
		return "", err
	}
	// Step 3: .sha256 sidecar.
	sum := sha256.Sum256(rendered)
	if err := writeFileAtomic(formal+".sha256", []byte(hex.EncodeToString(sum[:])+"\n"), 0o600); err != nil {
		return "", err
	}
	if err := l.step("sha256-written"); err != nil {
		return "", err
	}
	// Step 4: .ready — the single publication criterion.
	if err := writeFileAtomic(formal+".ready", []byte(l.Now().UTC().Format("2006-01-02T15:04:05Z07:00")+"\n"), 0o600); err != nil {
		return "", err
	}
	if err := l.step("ready-written"); err != nil {
		return "", err
	}
	// Step 5 (last): clean the draft and the .seq reservation (only when
	// the marker content confirms our ownership).
	if err := os.Remove(draftPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	seqMarker := filepath.Join(pairDir, ".seq", fmt.Sprintf("%03d", fm.Seq))
	if raw, err := os.ReadFile(seqMarker); err == nil {
		if owner, _, _ := strings.Cut(strings.TrimSpace(string(raw)), " "); owner == fm.Author {
			if err := os.Remove(seqMarker); err != nil && !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
	}
	return formal, nil
}

// ownDraftPath validates that draftPath names a draft of OUR author
// inside a pair under this ledger root, and returns its parts.
func (l *Ledger) ownDraftPath(draftPath string) (pairDir string, seq int, kind string, err error) {
	abs, err := filepath.Abs(draftPath)
	if err != nil {
		return "", 0, "", err
	}
	dir := filepath.Dir(abs)
	if filepath.Base(dir) != ".draft" {
		return "", 0, "", fmt.Errorf("%w: %s is not a .draft/ path", ErrRelay, draftPath)
	}
	pairDir = filepath.Dir(dir)
	root, err := filepath.Abs(l.Root)
	if err != nil {
		return "", 0, "", err
	}
	if filepath.Dir(pairDir) != root {
		return "", 0, "", fmt.Errorf("%w: draft %s is not under ledger root %s", ErrRelay, draftPath, l.Root)
	}
	seq, author, kind, ok := ParseArtifactName(filepath.Base(abs))
	if !ok {
		return "", 0, "", fmt.Errorf("%w: draft filename %q is not NNN-<author>-<kind>.md", ErrRelay, filepath.Base(abs))
	}
	if author != l.Identity.Author {
		return "", 0, "", fmt.Errorf("%w: draft belongs to %s, not %s (drafts are private to their author)", ErrRelay, author, l.Identity.Author)
	}
	return pairDir, seq, kind, nil
}
