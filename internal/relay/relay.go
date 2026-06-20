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

	"github.com/sean2077/oh-my-agents/internal/atomicfile"
	"github.com/sean2077/oh-my-agents/internal/projectroot"
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

var (
	// ErrPairNotFound marks a missing active/archived pair.
	ErrPairNotFound = errors.New("pair not found")
	// ErrPairArchived marks an operation that refuses because a pair is archived.
	ErrPairArchived = errors.New("pair archived")
	// ErrPairClosing marks an operation that refuses because close is in progress.
	ErrPairClosing = errors.New("pair closing")
	// ErrConflict marks a concurrent mutation conflict.
	ErrConflict = errors.New("relay conflict")
)

const pairLockTimeout = 30 * time.Second

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

// DefaultRoot resolves `<primary project root>/.oma/relay` starting from dir
// (protocol §1). Linked worktrees map back to the primary checkout, so all
// sessions for one repository share the same project-level ledger by default.
func DefaultRoot(dir string) (string, error) {
	top, err := projectroot.ProjectRoot(dir)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrRelay, err)
	}
	return filepath.Join(top, ".oma", "relay"), nil
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

// writeFileAtomic writes via a unique same-directory temp, rename, and
// parent directory sync.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	return atomicfile.Write(path, data, mode)
}

func relayError(kind error, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if kind == nil {
		return fmt.Errorf("%w: %s", ErrRelay, msg)
	}
	return fmt.Errorf("%w: %w: %s", ErrRelay, kind, msg)
}

func (l *Ledger) pairLock(slug string) (*atomicfile.Lock, error) {
	lockRoot := filepath.Join(l.Root, ".locks")
	if err := os.MkdirAll(lockRoot, 0o700); err != nil {
		return nil, err
	}
	lock, err := atomicfile.AcquireLock(filepath.Join(lockRoot, slug+".lock"), pairLockTimeout)
	if err != nil {
		switch {
		case errors.Is(err, atomicfile.ErrLockHeld):
			return nil, relayError(ErrConflict, "pair %s is being mutated by another process", slug)
		case errors.Is(err, os.ErrNotExist):
			return nil, relayError(ErrPairNotFound, "pair %q not found under %s", slug, l.Root)
		default:
			return nil, err
		}
	}
	return lock, nil
}

func (l *Ledger) withPairLock(slug string, fn func() error) error {
	lock, err := l.pairLock(slug)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()
	return fn()
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
