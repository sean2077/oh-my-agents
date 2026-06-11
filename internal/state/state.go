// Package state implements oma's generic project-level key/value store
// (docs/command-tree.md §3, docs/schemas.md §3). Keys are "namespace/field";
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
	"strconv"
	"strings"
	"time"
)

// Schema is the persisted state-file schema (docs/schemas.md §3).
const Schema = "oma-state/1"

// ErrState marks fail-closed state errors (bad key, unknown schema, IO).
var ErrState = errors.New("invalid state")

var (
	nsRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	fieldRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)
)

// File is one namespace's on-disk document.
type File struct {
	Schema    string            `json:"schema"`
	Namespace string            `json:"namespace"`
	Data      map[string]string `json:"data"`
	Updated   string            `json:"updated"`
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

// Set writes key=value atomically (tmp+rename, 0600, dirs 0700). When
// dryRun is set it returns the resolved path and writes nothing.
func (s *Store) Set(key, value, override string, dryRun bool) (path string, err error) {
	ns, field, err := splitKey(key)
	if err != nil {
		return "", err
	}
	path, err = s.path(ns, override)
	if err != nil {
		return "", err
	}
	// Validation runs before the dry-run return: --dry-run performs the
	// full computation/validation path and only skips writes
	// (docs/security-contract.md §1; B5 review 036).
	f, err := load(path, ns)
	if err != nil {
		return "", err
	}
	if dryRun {
		return path, nil
	}
	f.Schema, f.Namespace = Schema, ns
	f.Data[field] = value
	f.Updated = s.Now().UTC().Format(time.RFC3339)
	if err := writeAtomic(path, f); err != nil {
		return "", err
	}
	return path, nil
}

func writeAtomic(path string, f *File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("%w: %v", ErrState, err)
	}
	// single-generation .bak of the prior file (docs/schemas.md §1)
	if prev, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", prev, 0o600); err != nil {
			return fmt.Errorf("%w: write %s.bak: %v", ErrState, path, err)
		}
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("%w: write %s: %v", ErrState, path, err)
	}
	return os.Rename(tmp, path)
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
