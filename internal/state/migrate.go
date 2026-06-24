package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/sean2077/oh-my-agents/internal/atomicfile"
	"github.com/sean2077/oh-my-agents/internal/session"
)

// preMigrationDir holds a copy of every file the scope migration rewrites,
// so an operator can recover the pre-migration state by hand if needed.
const preMigrationDir = ".pre-migration"

// oldScopedNameRe matches a v0.7.0 scoped name of the form
// "<logical-name>-<session-suffix>" where the suffix is one of the hashed
// session forms session.hashSlug produces ("claude-"/"codex-"/"session-" +
// 12 hex). These are the only old names whose name/suffix boundary can be
// recovered unambiguously; default instances (the bare suffix) and
// explicit-slug sessions are left untouched on purpose.
var oldScopedNameRe = regexp.MustCompile(`^(.+)-((?:claude|codex|session)-[0-9a-f]{12})$`)

// ScopeMigration is one planned or applied namespace/id rename.
type ScopeMigration struct {
	Kind    string `json:"kind"` // "state" | "interview" | "ralph"
	OldName string `json:"old"`
	NewName string `json:"new"`
	Applied bool   `json:"applied"`
}

// migrateScopedToken converts a v0.7.0 "name-suffix" scoped token to the
// current "name--s-suffix" form. It is a no-op (changed=false) for tokens
// that are already in the new form or whose boundary cannot be recovered.
func migrateScopedToken(token string) (string, bool) {
	if strings.Contains(token, session.ScopeSeparator) {
		return token, false
	}
	m := oldScopedNameRe.FindStringSubmatch(token)
	if m == nil {
		return token, false
	}
	return m[1] + session.ScopeSeparator + m[2], true
}

// workflowFilePrefix classifies a state file base name into the embedded
// scoped token and the JSON field that holds it. interview/ralph store the
// scoped value in "id"; generic state stores it in "namespace".
func workflowFilePrefix(base string) (prefix, token, field string) {
	switch {
	case strings.HasPrefix(base, "interview-"):
		return "interview-", strings.TrimPrefix(base, "interview-"), "id"
	case strings.HasPrefix(base, "ralph-"):
		return "ralph-", strings.TrimPrefix(base, "ralph-"), "id"
	default:
		return "", base, "namespace"
	}
}

// MigrateSessionScope rewrites v0.7.0 session-scoped state files (name-suffix)
// to the current reserved-separator form (name--s-suffix) so they remain
// discoverable. Without apply it returns the plan and writes nothing. With
// apply it backs each original up under .oma/state/.pre-migration, fails
// closed on any target-name collision, and is idempotent.
func MigrateSessionScope(projectRoot string, apply bool) ([]ScopeMigration, error) {
	if projectRoot == "" {
		return nil, fmt.Errorf("%w: no project root (run inside a git project)", ErrState)
	}
	dir := filepath.Join(projectRoot, ".oma", "state")
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	var actions []ScopeMigration
	for _, path := range matches {
		base := strings.TrimSuffix(filepath.Base(path), ".json")
		prefix, token, field := workflowFilePrefix(base)
		newToken, changed := migrateScopedToken(token)
		if !changed {
			continue
		}
		newBase := prefix + newToken
		kind := "state"
		if prefix != "" {
			kind = strings.TrimSuffix(prefix, "-")
		}
		act := ScopeMigration{Kind: kind, OldName: base, NewName: newBase}
		if !apply {
			actions = append(actions, act)
			continue
		}
		newPath := filepath.Join(dir, newBase+".json")
		if existing, serr := os.ReadFile(newPath); serr == nil {
			// Crash-idempotency: a prior run may have written newPath but not
			// yet removed the old file. If newPath is byte-for-byte this
			// migration's own output, finish the cleanup instead of failing;
			// only a genuinely different target is a real collision.
			raw, rerr := os.ReadFile(path)
			if rerr != nil {
				return actions, rerr
			}
			want, werr := rewriteScopedField(raw, field, newToken)
			if werr != nil {
				return actions, fmt.Errorf("%w: rewrite %s: %v", ErrState, base, werr)
			}
			if !bytes.Equal(existing, want) {
				return actions, fmt.Errorf("%w: cannot migrate %s -> %s, target already exists with different content; resolve by hand", ErrState, base, newBase)
			}
			if err := finishScopeMigration(path); err != nil {
				return actions, err
			}
		} else if !os.IsNotExist(serr) {
			return actions, serr
		} else if err := applyScopeMigration(dir, path, newPath, field, newToken); err != nil {
			return actions, err
		}
		act.Applied = true
		actions = append(actions, act)
	}
	return actions, nil
}

// applyScopeMigration rewrites one file's embedded scoped field, writes it
// under the new name, backs the original up, and removes the old file and its
// sidecars. It holds the old file's state lock for the duration.
func applyScopeMigration(dir, oldPath, newPath, field, newToken string) error {
	return withStateLock(oldPath, func() error {
		raw, err := os.ReadFile(oldPath)
		if err != nil {
			return fmt.Errorf("%w: read %s: %v", ErrState, oldPath, err)
		}
		out, err := rewriteScopedField(raw, field, newToken)
		if err != nil {
			return fmt.Errorf("%w: %s: %v", ErrState, oldPath, err)
		}
		backup := filepath.Join(dir, preMigrationDir)
		if err := os.MkdirAll(backup, 0o700); err != nil {
			return err
		}
		if err := atomicfile.Write(filepath.Join(backup, filepath.Base(oldPath)), raw, 0o600); err != nil {
			return fmt.Errorf("%w: back up %s: %v", ErrState, oldPath, err)
		}
		if err := atomicfile.Write(newPath, out, 0o600); err != nil {
			return fmt.Errorf("%w: write %s: %v", ErrState, newPath, err)
		}
		_ = os.Remove(oldPath)
		_ = os.Remove(oldPath + ".bak")
		return nil
	})
}

// rewriteScopedField rewrites the embedded scoped field (id|namespace) to
// newToken, preserving every other (possibly unknown) top-level field. The
// output is deterministic (MarshalIndent + trailing newline) so a crash-then-
// re-run can recognize its own prior output byte-for-byte.
func rewriteScopedField(raw []byte, field, newToken string) ([]byte, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("not valid JSON: %v", err)
	}
	encoded, err := json.Marshal(newToken)
	if err != nil {
		return nil, err
	}
	obj[field] = encoded
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// finishScopeMigration removes the old file (and its .bak) under its state
// lock — the cleanup tail an idempotent re-run completes when newPath already
// holds this migration's own output.
func finishScopeMigration(oldPath string) error {
	return withStateLock(oldPath, func() error {
		_ = os.Remove(oldPath)
		_ = os.Remove(oldPath + ".bak")
		return nil
	})
}
