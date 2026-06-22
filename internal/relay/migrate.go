package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SessionMigration is one planned or applied participant_sessions repair.
type SessionMigration struct {
	Pair    string `json:"pair"`
	Applied bool   `json:"applied"`
	Note    string `json:"note"`
}

// MigrateParticipantSessions repairs v0.7.0 (`oma-relay/2`) pairs whose
// session.json predates the participant_sessions field. The current
// Validate() requires that field, so such pairs are unreadable via
// LoadSession until repaired. The migration raw-reads each session.json
// (bypassing Validate), and when participant_sessions is absent or null sets
// it to an empty object — a valid "no session has claimed a seat yet" state.
// Both participants must re-run `oma relay pair join` afterwards to re-claim
// their seats. Without apply it returns the plan and writes nothing; with
// apply it works under each pair's lock and is idempotent.
func (l *Ledger) MigrateParticipantSessions(apply bool) ([]SessionMigration, error) {
	slugs, err := l.AllPairs()
	if err != nil {
		return nil, err
	}
	var actions []SessionMigration
	for _, slug := range slugs {
		path := filepath.Join(l.PairDir(slug), "session.json")
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return actions, err
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			// Not valid JSON: surfaced by status/doctor, not the migrator.
			continue
		}
		existing, present := obj["participant_sessions"]
		if present && string(existing) != "null" {
			continue // already has a (possibly empty) map — nothing to do
		}
		if !apply {
			actions = append(actions, SessionMigration{Pair: slug, Note: "add empty participant_sessions; both sides must re-join"})
			continue
		}
		if err := l.withPairLock(slug, func() error {
			// Re-read under the lock so a concurrent writer can't be lost.
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(raw, &obj); err != nil {
				return fmt.Errorf("%w: session.json of %s not valid JSON: %v", ErrRelay, slug, err)
			}
			if existing, present := obj["participant_sessions"]; present && string(existing) != "null" {
				return nil // another process migrated it first
			}
			obj["participant_sessions"] = json.RawMessage("{}")
			out, err := json.MarshalIndent(obj, "", "  ")
			if err != nil {
				return err
			}
			return writeFileAtomic(path, append(out, '\n'), 0o600)
		}); err != nil {
			return actions, err
		}
		actions = append(actions, SessionMigration{Pair: slug, Applied: true, Note: "participant_sessions reset; both sides must re-run `oma relay pair join`"})
	}
	return actions, nil
}
