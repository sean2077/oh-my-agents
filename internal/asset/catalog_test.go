package asset

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifestFile(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogGeneratesSortedView(t *testing.T) {
	root := t.TempDir()
	writeManifestFile(t, filepath.Join(root, "skills", "zeta"), `{"schema":"oma-asset/1","name":"zeta","type":"skill","targets":["claude","codex"]}`)
	writeManifestFile(t, filepath.Join(root, "skills", "alpha"), `{"schema":"oma-asset/1","name":"alpha","type":"skill","targets":["claude","codex"],"status":"deprecated","canonical":"zeta"}`)
	entries, err := Catalog(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name != "alpha" || entries[1].Name != "zeta" {
		t.Fatalf("entries = %+v (want sorted alpha, zeta)", entries)
	}
	if entries[0].Status != "deprecated" || entries[0].Canonical != "zeta" {
		t.Fatalf("alpha = %+v", entries[0])
	}
	if entries[1].Status != StatusActive { // empty status defaults to active
		t.Fatalf("zeta status = %q, want active default", entries[1].Status)
	}
}

func TestCatalogFailsClosedOnDuplicate(t *testing.T) {
	root := t.TempDir()
	writeManifestFile(t, filepath.Join(root, "skills", "dup"), `{"schema":"oma-asset/1","name":"dup","type":"skill","targets":["claude","codex"]}`)
	writeManifestFile(t, filepath.Join(root, "hooks", "dup"), `{"schema":"oma-asset/1","name":"dup","type":"hook","targets":["claude","codex"]}`)
	if _, err := Catalog(root); err == nil {
		t.Fatal("duplicate asset name across type dirs must fail closed")
	}
}

func TestCatalogNameMustMatchDir(t *testing.T) {
	root := t.TempDir()
	writeManifestFile(t, filepath.Join(root, "skills", "x"), `{"schema":"oma-asset/1","name":"y","type":"skill","targets":["claude","codex"]}`)
	if _, err := Catalog(root); err == nil {
		t.Fatal("a manifest name that disagrees with its directory must fail closed")
	}
}
