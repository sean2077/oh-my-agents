// Package relay implements the oma relay v2 pair ledger
// (docs/reference/relay-v2-protocol.md): append-only artifacts with sidecar
// integrity markers, O_EXCL sequence reservation, draft-as-durable-
// publish-intent transactions, heartbeat liveness and pair bindings.
//
// v2 never reads or writes an agent-ledger v1 tree (.shared/): v1 roots
// are detected and refused; coexistence is report-only.
package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Schema constants (docs/reference/schemas.md §4).
const (
	Schema         = "oma-relay/2" // session + sentinel (ledger format)
	ArtifactSchema = "oma-relay/4" // artifact frontmatter (R5: + review evidence_hash / decision quality_gate_evidence_hash)
	BindingSchema  = "oma-relay-binding/1"
	sentinelName   = ".oma-relay-v2"
)

// ErrRelay marks fail-closed relay refusals (exit 3 at the CLI).
var ErrRelay = errors.New("relay refused (fail-closed)")

// Ledger is a v2 ledger rooted at Root. Now and StepHook are injected
// for deterministic tests; StepHook fires between publish transaction
// steps so the interruption matrix (protocol §11 test 8) can simulate a
// kill at every boundary — it is nil in production.
type Ledger struct {
	Root         string
	Identity     Identity
	Now          func() time.Time
	Getenv       func(string) string
	StepHook     func(step string) error
	PollInterval time.Duration // wait poll cadence; tests shrink it
}

// NewLedger builds a Ledger for the given root and identity.
func NewLedger(root string, id Identity) *Ledger {
	return &Ledger{Root: root, Identity: id, Now: time.Now, Getenv: os.Getenv, PollInterval: time.Second}
}

// sentinel is the v2 root marker content.
type sentinel struct {
	Schema  string    `json:"schema"`
	Created time.Time `json:"created"`
}

// DefaultRoot resolves `<git main worktree toplevel>/.oma/relay` starting
// from dir (protocol §1). A linked worktree's .git FILE is followed to
// the main repository so both sides of a same-host pair agree on one
// ledger root.
func DefaultRoot(dir string) (string, error) {
	top, err := gitToplevel(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(top, ".oma", "relay"), nil
}

func gitToplevel(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		gitPath := filepath.Join(dir, ".git")
		info, statErr := os.Stat(gitPath)
		if statErr == nil {
			if info.IsDir() {
				return dir, nil
			}
			// Linked worktree: .git is a file "gitdir: <main>/.git/worktrees/<name>"
			raw, readErr := os.ReadFile(gitPath)
			if readErr != nil {
				return "", readErr
			}
			gitdir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(raw)), "gitdir:"))
			if i := strings.Index(gitdir, string(filepath.Separator)+".git"+string(filepath.Separator)+"worktrees"+string(filepath.Separator)); i >= 0 {
				return gitdir[:i], nil
			}
			return dir, nil // unusual layout: fall back to this checkout
		}
		if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w: not inside a git checkout (no .git found above %s)", ErrRelay, dir)
		}
		dir = parent
	}
}

// Init creates the ledger root and sentinel (idempotent). It refuses v1
// trees and roots whose existing sentinel has a foreign schema major.
func (l *Ledger) Init(dryRun bool) error {
	if err := refuseV1(l.Root); err != nil {
		return err
	}
	sentinelPath := filepath.Join(l.Root, sentinelName)
	if raw, err := os.ReadFile(sentinelPath); err == nil {
		return validateSentinel(raw) // already initialized: revalidate only
	}
	if nonEmptyDir(l.Root) {
		return fmt.Errorf("%w: %s is non-empty but has no v2 sentinel (foreign directory; refusing to adopt)", ErrRelay, l.Root)
	}
	if dryRun {
		return nil
	}
	if err := os.MkdirAll(l.Root, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(sentinel{Schema: Schema, Created: l.Now().UTC()})
	if err != nil {
		return err
	}
	return writeFileAtomic(sentinelPath, append(raw, '\n'), 0o600)
}

// Open validates the root for use by every other command: v1 refusal,
// sentinel presence and schema major.
func (l *Ledger) Open() error {
	if err := refuseV1(l.Root); err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(l.Root, sentinelName))
	if errors.Is(err, os.ErrNotExist) {
		if nonEmptyDir(l.Root) {
			return fmt.Errorf("%w: %s has no v2 sentinel (run `oma relay init`, or this is not an oma ledger)", ErrRelay, l.Root)
		}
		return fmt.Errorf("%w: ledger not initialized at %s (run `oma relay init`)", ErrRelay, l.Root)
	}
	if err != nil {
		return err
	}
	return validateSentinel(raw)
}

func validateSentinel(raw []byte) error {
	var s sentinel
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("%w: sentinel not valid JSON: %v", ErrRelay, err)
	}
	if major, ok := schemaMajor(s.Schema, "oma-relay"); !ok || major != 2 {
		return fmt.Errorf("%w: sentinel schema %q, want %s (upgrade oma or check the directory)", ErrRelay, s.Schema, Schema)
	}
	return nil
}

// refuseV1 rejects agent-ledger v1 trees (protocol §1): a `_relay/`
// directory at the root, or a directly nested session.json whose
// schema_version is the v1 integer form (1-3).
func refuseV1(root string) error {
	if _, err := os.Stat(filepath.Join(root, "_relay")); err == nil {
		return fmt.Errorf("%w: %s is an agent-ledger v1 tree (_relay/ present); oma never reads or writes v1 — use a fresh root", ErrRelay, root)
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(root, ent.Name(), "session.json"))
		if readErr != nil {
			continue
		}
		var probe struct {
			SchemaVersion json.RawMessage `json:"schema_version"`
		}
		if json.Unmarshal(raw, &probe) != nil || probe.SchemaVersion == nil {
			continue
		}
		if v, convErr := strconv.Atoi(strings.TrimSpace(string(probe.SchemaVersion))); convErr == nil && v >= 1 && v <= 3 {
			return fmt.Errorf("%w: %s contains a v1 session (%s); oma never reads or writes v1 — use a fresh root", ErrRelay, root, ent.Name())
		}
	}
	return nil
}

func nonEmptyDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

// writeFileAtomic writes via tmp+rename with the given mode and fsyncs
// the parent directory.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// step fires the test failpoint hook between transaction steps.
func (l *Ledger) step(name string) error {
	if l.StepHook == nil {
		return nil
	}
	return l.StepHook(name)
}

// schemaMajor parses "oma-<domain>/<major>" with the strict digit-only
// rule shared by every persisted-schema reader (per-package copy by
// convention; B2 review finding 1).
func schemaMajor(schema, wantDomain string) (int, bool) {
	domain, ver, found := strings.Cut(schema, "/")
	if !found || domain != wantDomain || ver == "" || ver[0] == '0' {
		return 0, false
	}
	for i := 0; i < len(ver); i++ {
		if ver[i] < '0' || ver[i] > '9' {
			return 0, false
		}
	}
	major, err := strconv.Atoi(ver)
	if err != nil || major < 1 {
		return 0, false
	}
	return major, true
}
