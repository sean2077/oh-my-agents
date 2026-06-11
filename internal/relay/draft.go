package relay

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// draftPlaceholder marks an unfilled draft body; publish refuses it
// (docs/command-tree.md §6).
const draftPlaceholder = "TODO: write the artifact body before publishing."

// reserveSeq allocates the next sequence number with an O_EXCL marker
// file: NNN = max(published, reserved) + 1, retrying upward on races
// (protocol §6). The exclusive object is `.seq/NNN` itself — the owning
// author is recorded INSIDE the file, never in the filename: a per-author
// suffix would make O_EXCL exclusive only per (seq, author) and two
// authors could reserve the same seq concurrently (caught by protocol
// test 7; doc updated with B8).
func (l *Ledger) reserveSeq(slug string) (int, error) {
	pairDir := l.PairDir(slug)
	next, err := l.nextSeq(slug)
	if err != nil {
		return 0, err
	}
	for attempt := 0; attempt < 10; attempt++ {
		seq := next + attempt
		if seq > 999 {
			return 0, fmt.Errorf("%w: sequence space exhausted (>999)", ErrRelay)
		}
		marker := filepath.Join(pairDir, ".seq", fmt.Sprintf("%03d", seq))
		f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return 0, err
		}
		_, werr := fmt.Fprintf(f, "%s %s\n", l.Identity.Author, l.Now().UTC().Format(time.RFC3339))
		cerr := f.Close()
		if werr != nil || cerr != nil {
			return 0, errors.Join(werr, cerr)
		}
		return seq, nil
	}
	return 0, fmt.Errorf("%w: could not reserve a sequence number after 10 attempts (heavy contention?)", ErrRelay)
}

// nextSeq computes max(published formal, reserved) + 1.
func (l *Ledger) nextSeq(slug string) (int, error) {
	pairDir := l.PairDir(slug)
	maxSeq := 0
	names, err := publishedArtifacts(pairDir, false)
	if err != nil {
		return 0, err
	}
	for _, name := range names {
		if seq, _, _, ok := ParseArtifactName(name); ok && seq > maxSeq {
			maxSeq = seq
		}
	}
	reserved, err := os.ReadDir(filepath.Join(pairDir, ".seq"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	for _, ent := range reserved {
		if n, err := strconv.Atoi(ent.Name()); err == nil && n > maxSeq {
			maxSeq = n
		}
	}
	return maxSeq + 1, nil
}

// CreateDraft reserves a seq and writes the draft skeleton — the durable
// publish intent (protocol §7). The skeleton carries final frontmatter
// except status/prompt/touched (filled at publish) and a placeholder
// body that publish refuses verbatim.
func (l *Ledger) CreateDraft(slug, kind string, inReplyTo, corrects *int, dryRun bool) (string, error) {
	if !ValidKind(kind) {
		return "", fmt.Errorf("%w: kind %q not in %v", ErrRelay, kind, Kinds)
	}
	if kind == "correction" && corrects == nil {
		return "", fmt.Errorf("%w: kind correction requires --corrects <seq>", ErrRelay)
	}
	// Explicit --pair wins FIRST (§4a resolution order, review 054
	// blocker 1): binding state must never gate an explicit valid slug.
	s, err := l.ResolvePair(slug, !dryRun)
	if err != nil {
		return "", err
	}
	if s.Terminal() {
		return "", fmt.Errorf("%w: pair %s is %s", ErrRelay, s.Pair, s.Status)
	}
	peer, err := s.Peer(l.Identity.Author)
	if err != nil {
		return "", err
	}
	if dryRun {
		next, err := l.nextSeq(s.Pair)
		if err != nil {
			return "", err
		}
		return filepath.Join(l.PairDir(s.Pair), ".draft", ArtifactName(next, l.Identity.Author, kind)), nil
	}
	seq, err := l.reserveSeq(s.Pair)
	if err != nil {
		return "", err
	}
	fm := &Frontmatter{
		Schema: Schema, Seq: seq, Author: l.Identity.Author, Peer: peer,
		Kind: kind, Status: "ready", Created: l.Now().UTC(),
		InReplyTo: inReplyTo, Corrects: corrects, TouchedPaths: []string{},
	}
	if err := fm.Validate(); err != nil {
		return "", err
	}
	draftPath := filepath.Join(l.PairDir(s.Pair), ".draft", ArtifactName(seq, l.Identity.Author, kind))
	if err := writeFileAtomic(draftPath, Render(fm, draftPlaceholder+"\n"), 0o600); err != nil {
		return "", err
	}
	l.touchHeartbeat(s.Pair)
	return draftPath, nil
}
