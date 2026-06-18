package asset

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RegistrySchema is the persisted registry schema (docs/reference/schemas.md §2).
const RegistrySchema = "oma-registry/1"

// ErrNotManaged marks operations on assets the registry does not own.
var ErrNotManaged = errors.New("asset is not managed by oma")

// Projection records one per-agent projection of an installed asset.
type Projection struct {
	Agent string `json:"agent"`
	Path  string `json:"path"`
	Kind  string `json:"kind"` // symlink | junction | copy
}

// Backup records one pre-overwrite backup snapshot.
type Backup struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// Entry is one managed asset in the registry.
type Entry struct {
	Name          string       `json:"name"`
	Type          string       `json:"type"`
	Version       string       `json:"version,omitempty"`
	InstalledAt   time.Time    `json:"installed_at"`
	Source        string       `json:"source"` // release | dev-link | dir
	CanonicalPath string       `json:"canonical_path"`
	Digest        string       `json:"digest"`                  // DigestTree of canonical content; drift = non-managed
	RestoredFrom  string       `json:"restored_from,omitempty"` // backup ID of the last rollback, provenance for list/doctor
	Projections   []Projection `json:"projections"`
	Backups       []Backup     `json:"backups,omitempty"`
	Manifest      Manifest     `json:"manifest"`
}

// Registry is the oma-registry/1 document. It records only oma-managed
// entries; foreign assets in shared directories are never touched.
type Registry struct {
	Schema string  `json:"schema"`
	Assets []Entry `json:"assets"`
}

// LoadRegistry reads the registry, returning an empty one when absent and
// failing closed on unknown schema majors.
func LoadRegistry(path string) (*Registry, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Registry{Schema: RegistrySchema}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}
	var r Registry
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("%w: registry not valid JSON: %v", ErrInvalid, err)
	}
	if major, ok := schemaMajor(r.Schema, "oma-registry"); !ok || major != 1 {
		return nil, fmt.Errorf("%w: registry schema %q, want %s", ErrUnknownSchema, r.Schema, RegistrySchema)
	}
	return &r, nil
}

// Save writes the registry atomically (tmp+rename, 0600, dirs 0700) with a
// single-generation .bak of the previous version (docs/reference/schemas.md §1).
func (r *Registry) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create registry dir: %w", err)
	}
	if prev, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", prev, 0o600); err != nil {
			return fmt.Errorf("write registry backup: %w", err)
		}
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write registry: %w", err)
	}
	return os.Rename(tmp, path)
}

// Find returns the entry for name, or nil.
func (r *Registry) Find(name string) *Entry {
	for i := range r.Assets {
		if r.Assets[i].Name == name {
			return &r.Assets[i]
		}
	}
	return nil
}

// Upsert replaces or appends the entry, preserving recorded backups when
// the asset is reinstalled.
func (r *Registry) Upsert(e Entry) {
	for i := range r.Assets {
		if r.Assets[i].Name == e.Name {
			e.Backups = append(r.Assets[i].Backups, e.Backups...)
			r.Assets[i] = e
			return
		}
	}
	r.Assets = append(r.Assets, e)
}

// Drop removes the entry by name and reports whether it existed.
func (r *Registry) Drop(name string) bool {
	for i := range r.Assets {
		if r.Assets[i].Name == name {
			r.Assets = append(r.Assets[:i], r.Assets[i+1:]...)
			return true
		}
	}
	return false
}
