package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s := New(t.TempDir())
	s.Now = func() time.Time { return time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC) }
	return s
}

func TestSetThenGet(t *testing.T) {
	s := newStore(t)
	if _, err := s.Set("autopilot/phase", "planning", "", false); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, ok, err := s.Get("autopilot/phase", "")
	if err != nil || !ok || v != "planning" {
		t.Fatalf("get = %q ok=%v err=%v", v, ok, err)
	}
	// file lands at the namespace path with 0600
	path := filepath.Join(s.ProjectRoot, ".oma", "state", "autopilot.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestGetMissingFieldNotOK(t *testing.T) {
	s := newStore(t)
	if _, err := s.Set("ns/a", "1", "", false); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.Get("ns/b", ""); err != nil || ok {
		t.Fatalf("missing field: ok=%v err=%v, want ok=false", ok, err)
	}
}

func TestMultipleFieldsSameNamespace(t *testing.T) {
	s := newStore(t)
	if _, err := s.Set("wf/a", "1", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Set("wf/b", "2", "", false); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"wf/a": "1", "wf/b": "2"} {
		if v, _, _ := s.Get(k, ""); v != want {
			t.Errorf("%s = %q, want %q", k, v, want)
		}
	}
}

func TestBadKeysRejected(t *testing.T) {
	s := newStore(t)
	for _, key := range []string{"nokeyslash", "../escape/x", "ns/../../x", "UP/x", "ns/with space", "/x", "ns/"} {
		if _, err := s.Set(key, "v", "", false); !errors.Is(err, ErrState) {
			t.Errorf("key %q: err = %v, want ErrState", key, err)
		}
	}
}

func TestUnknownSchemaMajorFailsClosed(t *testing.T) {
	s := newStore(t)
	path := filepath.Join(s.ProjectRoot, ".oma", "state", "ns.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema":"oma-state/9","namespace":"ns","data":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Get("ns/x", ""); !errors.Is(err, ErrState) {
		t.Fatalf("unknown major: err = %v, want ErrState", err)
	}
}

func TestNoProjectRootRequiresFile(t *testing.T) {
	s := New("") // no project
	s.Now = func() time.Time { return time.Unix(0, 0) }
	if _, err := s.Set("ns/x", "v", "", false); !errors.Is(err, ErrState) {
		t.Fatalf("no project, no --file: err = %v, want ErrState", err)
	}
	// explicit file works without a project root
	override := filepath.Join(os.TempDir(), "oma-state-test-"+t.Name()+".json")
	t.Cleanup(func() { _ = os.Remove(override) })
	if _, err := s.Set("ns/x", "v", override, false); err != nil {
		t.Fatalf("explicit file: %v", err)
	}
	if v, ok, _ := s.Get("ns/x", override); !ok || v != "v" {
		t.Fatalf("explicit file get: %q ok=%v", v, ok)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	s := newStore(t)
	path, err := s.Set("ns/x", "v", "", true)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry-run must not write")
	}
}

func TestOverwriteUpdatesValueAndTimestamp(t *testing.T) {
	s := newStore(t)
	if _, err := s.Set("ns/x", "old", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Set("ns/x", "new", "", false); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := s.Get("ns/x", ""); v != "new" {
		t.Fatalf("overwrite = %q, want new", v)
	}
}
