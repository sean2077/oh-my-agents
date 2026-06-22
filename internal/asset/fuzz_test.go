package asset

import (
	"path/filepath"
	"strings"
	"testing"
)

// FuzzValidName asserts the asset-name guard never panics and — the property
// that matters for security — never accepts a name that could escape the
// canonical store. Every name ValidName approves must be a single,
// non-traversing path component (this is the guard relied on before any
// filesystem work; see safety_test.go).
func FuzzValidName(f *testing.F) {
	f.Add("deep-interview")
	f.Add("ralph")
	f.Add("../escape")
	f.Add("a/b")
	f.Add("..")
	f.Add("UPPER")
	f.Add("")

	f.Fuzz(func(t *testing.T, name string) {
		if !ValidName(name) {
			return
		}
		if name == "" {
			t.Fatalf("ValidName(%q) = true, want false for empty", name)
		}
		if strings.ContainsAny(name, `/\`) {
			t.Fatalf("ValidName(%q) = true but contains a path separator", name)
		}
		if name == "." || name == ".." || strings.Contains(name, "..") {
			t.Fatalf("ValidName(%q) = true but is or contains a traversal", name)
		}
		if filepath.Base(name) != name {
			t.Fatalf("ValidName(%q) = true but is not a single path component", name)
		}
		if !filepath.IsLocal(name) {
			t.Fatalf("ValidName(%q) = true but filepath.IsLocal = false", name)
		}
	})
}
