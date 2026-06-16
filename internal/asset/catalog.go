package asset

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// CatalogEntry is one asset in the generated catalog view (A7). The catalog
// is DERIVED from the same manifest.json files install and the registry use;
// it never introduces a second source of truth that could drift.
type CatalogEntry struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Status    string   `json:"status"`
	Targets   []string `json:"targets"`
	Canonical string   `json:"canonical,omitempty"`
}

// TypeDirs mirrors the repo assets/ layout (plan §2): one subdir per type.
var TypeDirs = []string{"skills", "agents", "hooks", "prompts"}

// Catalog scans a source root (<root>/{skills,agents,hooks,prompts}/<name>/
// manifest.json) and returns the validated, name-sorted catalog. A malformed
// manifest, a name that disagrees with its directory, or a duplicate name
// across type dirs fails closed.
func Catalog(root string) ([]CatalogEntry, error) {
	seen := map[string]string{} // name -> the type dir it first appeared in
	var entries []CatalogEntry
	for _, td := range TypeDirs {
		dir := filepath.Join(root, td)
		ents, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			mfPath := filepath.Join(dir, e.Name(), "manifest.json")
			if _, statErr := os.Stat(mfPath); statErr != nil {
				continue
			}
			m, err := LoadManifest(mfPath)
			if err != nil {
				return nil, fmt.Errorf("catalog: %s: %w", filepath.Join(td, e.Name()), err)
			}
			if m.Name != e.Name() {
				return nil, fmt.Errorf("catalog: %s manifest name %q does not match its directory", filepath.Join(td, e.Name()), m.Name)
			}
			if prev, dup := seen[m.Name]; dup {
				return nil, fmt.Errorf("catalog: duplicate asset name %q (in %s and %s)", m.Name, prev, td)
			}
			seen[m.Name] = td
			entries = append(entries, CatalogEntry{
				Name:      m.Name,
				Type:      m.Type,
				Status:    m.StatusOrDefault(),
				Targets:   m.Targets,
				Canonical: m.Canonical,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}
