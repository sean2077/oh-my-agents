package relay

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CleanStale removes safe residue from one pair (protocol §6/§8, surfaced
// as `oma doctor relay --clean-stale`):
//   - .seq reservations and drafts whose seq already has a READY formal
//     artifact by the same author (post-publish leftovers from a kill
//     between .ready and cleanup) — always safe;
//   - drafts + reservations of an author whose heartbeat is STALE and
//     whose seq has no formal artifact (abandoned intent; the seq hole
//     is legal);
//   - formal files WITHOUT .ready and without a surviving draft are
//     quarantined (renamed *.stale) — with a draft alive the publish can
//     still resume, so those are left untouched.
//
// Live authors' in-flight reservations are never touched.
func (l *Ledger) CleanStale(slug string, dryRun bool) ([]string, error) {
	s, err := l.LoadSession(slug)
	if err != nil {
		return nil, err
	}
	pairDir := l.PairDir(slug)
	var actions []string
	remove := func(path, why string) error {
		actions = append(actions, fmt.Sprintf("remove %s (%s)", path, why))
		if dryRun {
			return nil
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}

	for _, author := range s.Participants {
		stale := l.heartbeatStale(slug, author)
		for _, seq := range l.reservations(slug, author) {
			seqPath := filepath.Join(pairDir, ".seq", fmt.Sprintf("%03d", seq))
			draftGlob, _ := filepath.Glob(filepath.Join(pairDir, ".draft", fmt.Sprintf("%03d-%s-*.md", seq, author)))
			switch {
			case l.hasReadyAt(slug, seq, author):
				if err := remove(seqPath, "published counterpart exists"); err != nil {
					return actions, err
				}
				for _, d := range draftGlob {
					if err := remove(d, "published counterpart exists"); err != nil {
						return actions, err
					}
				}
			case stale:
				// A surviving draft whose formal file already exists (no
				// .ready yet) is a RESUMABLE interrupted publish: rerunning
				// the same publish converges from the draft (protocol §7).
				// Cleaning it would destroy the recovery path (review 054
				// blocker 3). Resumability requires BOTH pieces — a formal
				// without any draft cannot be resumed (review 056 blocker 2):
				// the reservation is removed here and the quarantine pass
				// below renames the formal, so doctor converges.
				if l.hasFormalAt(slug, seq, author) && len(draftGlob) > 0 {
					actions = append(actions, fmt.Sprintf("keep .seq/%03d and draft (interrupted publish is resumable: rerun `oma relay publish`)", seq))
					continue
				}
				if err := remove(seqPath, "stale abandoned intent"); err != nil {
					return actions, err
				}
				for _, d := range draftGlob {
					if err := remove(d, "stale abandoned intent"); err != nil {
						return actions, err
					}
				}
			}
		}
	}

	// Formal files without .ready: quarantine only when no draft can
	// resume the publish.
	names, err := publishedArtifacts(pairDir, false)
	if err != nil {
		return actions, err
	}
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(pairDir, name+".ready")); err == nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(pairDir, ".draft", name)); err == nil {
			continue // resumable: publish re-run converges
		}
		actions = append(actions, fmt.Sprintf("quarantine %s -> %s.stale (no .ready, no draft)", name, name))
		if !dryRun {
			if err := os.Rename(filepath.Join(pairDir, name), filepath.Join(pairDir, name+".stale")); err != nil {
				return actions, err
			}
			_ = os.Remove(filepath.Join(pairDir, name+".sha256"))
		}
	}
	return actions, nil
}

// Restore moves an archived pair back to the active root (surfaced as
// `oma doctor relay --restore <slug>`).
func (l *Ledger) Restore(slug string, dryRun bool) error {
	src := filepath.Join(l.Root, "_archive", slug)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("%w: no archived pair %q", ErrRelay, slug)
	}
	dest := l.PairDir(slug)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%w: %s already exists in the active root", ErrRelay, slug)
	}
	if dryRun {
		return nil
	}
	// The session keeps its terminal status and CLOSED sentinel: restore
	// brings history back for inspection, it does not reactivate the pair.
	return os.Rename(src, dest)
}

// reservationSeqs parses ".seq" entries — exported for doctor reporting.
func (l *Ledger) ReservationCount(slug string) int {
	entries, err := os.ReadDir(filepath.Join(l.PairDir(slug), ".seq"))
	if err != nil {
		return 0
	}
	n := 0
	for _, ent := range entries {
		if num, _, found := strings.Cut(ent.Name(), "."); found {
			if _, err := strconv.Atoi(num); err == nil {
				n++
			}
		}
	}
	return n
}
