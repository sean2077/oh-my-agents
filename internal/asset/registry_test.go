package asset

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRegistrySaveRoundTripAndBackup pins the registry persistence contract
// (S11 moved Save onto atomicfile.WriteWithBackup): Save creates missing parent
// dirs, round-trips through LoadRegistry, writes no .bak on the first save, and
// refreshes a single-generation .bak with the PREVIOUS version on the next save.
func TestRegistrySaveRoundTripAndBackup(t *testing.T) {
	dir := t.TempDir()
	// A nested path proves Save creates the parent directory.
	path := filepath.Join(dir, "sub", "registry.json")

	r1 := &Registry{Schema: RegistrySchema, Assets: []Entry{{Name: "alpha", Type: "skill"}}}
	if err := r1.Save(path); err != nil {
		t.Fatalf("first save: %v", err)
	}
	got, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("load after first save: %v", err)
	}
	if len(got.Assets) != 1 || got.Assets[0].Name != "alpha" {
		t.Fatalf("round-trip mismatch: %+v", got.Assets)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("no .bak expected after the first save, stat err = %v", err)
	}

	r2 := &Registry{Schema: RegistrySchema, Assets: []Entry{{Name: "beta", Type: "agent"}}}
	if err := r2.Save(path); err != nil {
		t.Fatalf("second save: %v", err)
	}
	got, err = LoadRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Assets) != 1 || got.Assets[0].Name != "beta" {
		t.Fatalf("file must hold the new generation: %+v", got.Assets)
	}
	// The .bak now holds the previous (alpha) generation.
	bak, err := LoadRegistry(path + ".bak")
	if err != nil {
		t.Fatalf("load .bak: %v", err)
	}
	if len(bak.Assets) != 1 || bak.Assets[0].Name != "alpha" {
		t.Fatalf(".bak must hold the previous generation (alpha): %+v", bak.Assets)
	}
}
