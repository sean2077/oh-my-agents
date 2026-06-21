// Package state implements oma's generic project-level key/value store
// (docs/reference/command-tree.md §3, docs/reference/schemas.md §3). Keys are "namespace/field";
// each namespace is one JSON file under <project>/.oma/state/. Values are
// always strings — structured data is the caller's concern.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sean2077/oh-my-agents/internal/atomicfile"
)

// Schema is the persisted state-file schema (docs/reference/schemas.md §3).
const Schema = "oma-state/1"

// ErrState marks fail-closed state errors (bad key, unknown schema, IO).
var ErrState = errors.New("invalid state")

// ErrConflict reports an expected revision mismatch.
var ErrConflict = errors.New("revision conflict")

var (
	nsRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	fieldRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)
)

const stateLockTimeout = 30 * time.Second

// File is one namespace's on-disk document.
type File struct {
	Schema    string            `json:"schema"`
	Namespace string            `json:"namespace"`
	Revision  int64             `json:"revision"`
	Data      map[string]string `json:"data"`
	Updated   string            `json:"updated"`
}

// Entry is one validated state namespace returned by List.
type Entry struct {
	Namespace string            `json:"namespace"`
	Revision  int64             `json:"revision"`
	Data      map[string]string `json:"data"`
	Updated   string            `json:"updated"`
	Path      string            `json:"path"`
}

// Store resolves and mutates state files for one project.
type Store struct {
	ProjectRoot string           // "" when outside a project (then --file is required)
	Now         func() time.Time // injected for deterministic timestamps in tests
}

// New builds a Store; Now defaults to time.Now.
func New(projectRoot string) *Store {
	return &Store{ProjectRoot: projectRoot, Now: time.Now}
}

// splitKey parses "namespace/field" with traversal-safe components.
func splitKey(key string) (ns, field string, err error) {
	ns, field, ok := strings.Cut(key, "/")
	if !ok {
		return "", "", fmt.Errorf("%w: key %q must be namespace/field", ErrState, key)
	}
	if !nsRe.MatchString(ns) {
		return "", "", fmt.Errorf("%w: namespace %q (want lowercase letters, digits, dashes)", ErrState, ns)
	}
	if !fieldRe.MatchString(field) {
		return "", "", fmt.Errorf("%w: field %q (want letters, digits, _-)", ErrState, field)
	}
	return ns, field, nil
}

// path resolves the file for a namespace: an explicit override wins,
// otherwise <project>/.oma/state/<namespace>.json.
func (s *Store) path(ns, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if s.ProjectRoot == "" {
		return "", fmt.Errorf("%w: no project root (run inside a git project or pass --file)", ErrState)
	}
	return filepath.Join(s.ProjectRoot, ".oma", "state", ns+".json"), nil
}

// load reads and validates a state file; missing → empty File for ns.
func load(path, ns string) (*File, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &File{Schema: Schema, Namespace: ns, Data: map[string]string{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %v", ErrState, path, err)
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s not valid JSON: %v", ErrState, path, err)
	}
	if major, ok := schemaMajor(f.Schema, "oma-state"); !ok || major != 1 {
		return nil, fmt.Errorf("%w: %s schema %q, want %s", ErrState, path, f.Schema, Schema)
	}
	// namespace and updated are part of the oma-state/1 contract: a file
	// whose namespace disagrees with its key (or whose timestamp is not
	// RFC3339) is corrupt state, not a soft default (B5 review 036).
	if f.Namespace != ns {
		return nil, fmt.Errorf("%w: %s namespace %q does not match key namespace %q (corrupt or misplaced state file)",
			ErrState, path, f.Namespace, ns)
	}
	if _, terr := time.Parse(time.RFC3339, f.Updated); terr != nil {
		return nil, fmt.Errorf("%w: %s updated %q is not RFC3339: %v", ErrState, path, f.Updated, terr)
	}
	if f.Data == nil {
		f.Data = map[string]string{}
	}
	return &f, nil
}

// Get returns the value for key; ok=false when the field is absent.
func (s *Store) Get(key, override string) (value string, ok bool, err error) {
	ns, field, err := splitKey(key)
	if err != nil {
		return "", false, err
	}
	path, err := s.path(ns, override)
	if err != nil {
		return "", false, err
	}
	f, err := load(path, ns)
	if err != nil {
		return "", false, err
	}
	v, ok := f.Data[field]
	return v, ok, nil
}

// List returns validated state namespaces under the project state directory,
// optionally filtered by namespace prefix. It fails closed on any matching
// corrupt state file so discovery cannot silently route a workflow at the
// wrong run.
func (s *Store) List(prefix string) ([]Entry, error) {
	if prefix != "" && !nsRe.MatchString(prefix) {
		return nil, fmt.Errorf("%w: namespace prefix %q (want lowercase letters, digits, dashes)", ErrState, prefix)
	}
	if s.ProjectRoot == "" {
		return nil, fmt.Errorf("%w: no project root (run inside a git project)", ErrState)
	}
	dir := filepath.Join(s.ProjectRoot, ".oma", "state")
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(matches))
	for _, path := range matches {
		ns := strings.TrimSuffix(filepath.Base(path), ".json")
		if prefix != "" && !strings.HasPrefix(ns, prefix) {
			continue
		}
		f, err := load(path, ns)
		if err != nil {
			return nil, err
		}
		data := make(map[string]string, len(f.Data))
		for k, v := range f.Data {
			data[k] = v
		}
		out = append(out, Entry{Namespace: f.Namespace, Revision: f.Revision, Data: data, Updated: f.Updated, Path: path})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Namespace < out[j].Namespace })
	return out, nil
}

// GetWithRevision returns the value and current file revision for key.
func (s *Store) GetWithRevision(key, override string) (value string, ok bool, revision int64, err error) {
	ns, field, err := splitKey(key)
	if err != nil {
		return "", false, 0, err
	}
	path, err := s.path(ns, override)
	if err != nil {
		return "", false, 0, err
	}
	f, err := load(path, ns)
	if err != nil {
		return "", false, 0, err
	}
	v, ok := f.Data[field]
	return v, ok, f.Revision, nil
}

// Set writes key=value atomically (lock+unique tmp+rename, 0600, dirs 0700).
// When dryRun is set it returns the resolved path and writes nothing.
func (s *Store) Set(key, value, override string, dryRun bool) (path string, err error) {
	return s.SetExpected(key, value, override, dryRun, nil)
}

// SetExpected writes key=value only when expectedRevision is nil or matches
// the state file's current revision.
func (s *Store) SetExpected(key, value, override string, dryRun bool, expectedRevision *int64) (path string, err error) {
	ns, field, err := splitKey(key)
	if err != nil {
		return "", err
	}
	return s.PatchExpected(ns, map[string]string{field: value}, override, dryRun, expectedRevision)
}

// PatchExpected writes several fields in one namespace under a single lock,
// revision bump, and optional CAS check.
func (s *Store) PatchExpected(ns string, values map[string]string, override string, dryRun bool, expectedRevision *int64) (path string, err error) {
	if !nsRe.MatchString(ns) {
		return "", fmt.Errorf("%w: namespace %q (want lowercase letters, digits, dashes)", ErrState, ns)
	}
	if len(values) == 0 {
		return "", fmt.Errorf("%w: patch requires at least one field", ErrState)
	}
	for field := range values {
		if !fieldRe.MatchString(field) {
			return "", fmt.Errorf("%w: field %q (want letters, digits, _-)", ErrState, field)
		}
	}
	path, err = s.path(ns, override)
	if err != nil {
		return "", err
	}
	// Validation runs before the dry-run return: --dry-run performs the
	// full computation/validation path and only skips writes
	// (docs/reference/security-contract.md §1; B5 review 036).
	f, err := load(path, ns)
	if err != nil {
		return "", err
	}
	if expectedRevision != nil && f.Revision != *expectedRevision {
		return "", fmt.Errorf("%w: %s revision is %d, expected %d", ErrConflict, path, f.Revision, *expectedRevision)
	}
	if dryRun {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("%w: %v", ErrState, err)
	}
	return path, withStateLock(path, func() error {
		f, err := load(path, ns)
		if err != nil {
			return err
		}
		if expectedRevision != nil && f.Revision != *expectedRevision {
			return fmt.Errorf("%w: %s revision is %d, expected %d", ErrConflict, path, f.Revision, *expectedRevision)
		}
		f.Schema, f.Namespace = Schema, ns
		f.Revision++
		for field, value := range values {
			f.Data[field] = value
		}
		f.Updated = s.Now().UTC().Format(time.RFC3339)
		if err := writeAtomic(path, f); err != nil {
			return err
		}
		return nil
	})
}

func writeAtomic(path string, f *File) error {
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicfile.WriteWithBackup(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("%w: write %s: %v", ErrState, path, err)
	}
	return nil
}

func withStateLock(path string, fn func() error) error {
	err := atomicfile.WithLock(path+".lock", stateLockTimeout, fn)
	if errors.Is(err, atomicfile.ErrLockHeld) {
		return fmt.Errorf("%w: %v", ErrState, err)
	}
	return err
}

// schemaMajor mirrors the strict parser used elsewhere (digits only, >= 1).
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
