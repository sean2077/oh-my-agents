package state

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
	// file lands at the namespace path with 0600 on POSIX. Windows exposes ACLs
	// through approximate mode bits, so the exact permission assertion is not
	// meaningful there.
	path := filepath.Join(s.ProjectRoot, ".oma", "state", "autopilot.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
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

func TestListFiltersByNamespacePrefix(t *testing.T) {
	s := newStore(t)
	for _, key := range []string{"autopilot-alpha/phase", "autopilot-beta/phase", "interview-x/phase"} {
		if _, err := s.Set(key, "running", "", false); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := s.List("autopilot")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Namespace != "autopilot-alpha" || entries[1].Namespace != "autopilot-beta" {
		t.Fatalf("entries = %+v, want sorted autopilot namespaces", entries)
	}
	if entries[0].Data["phase"] != "running" || entries[0].Path == "" {
		t.Fatalf("entry data/path not populated: %+v", entries[0])
	}
}

func TestListFailsClosedOnMatchingCorruptState(t *testing.T) {
	s := newStore(t)
	if _, err := s.Set("autopilot-good/phase", "running", "", false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.ProjectRoot, ".oma", "state", "autopilot-bad.json")
	if err := os.WriteFile(path, []byte(`{"schema":"oma-state/9","namespace":"autopilot-bad","data":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List("autopilot"); !errors.Is(err, ErrState) {
		t.Fatalf("list corrupt matching namespace: err = %v, want ErrState", err)
	}
	if entries, err := s.List("interview"); err != nil || len(entries) != 0 {
		t.Fatalf("nonmatching corrupt state should be ignored by prefix: entries=%+v err=%v", entries, err)
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

func TestNamespaceMismatchFailsClosed(t *testing.T) {
	s := newStore(t)
	path := filepath.Join(s.ProjectRoot, ".oma", "state", "autopilot.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"schema":"oma-state/1","namespace":"other","data":{"phase":"wrong"},"updated":"2026-06-11T12:00:00Z"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Get("autopilot/phase", ""); !errors.Is(err, ErrState) {
		t.Fatalf("namespace mismatch: err = %v, want ErrState", err)
	}
}

func TestBadUpdatedTimestampFailsClosed(t *testing.T) {
	s := newStore(t)
	path := filepath.Join(s.ProjectRoot, ".oma", "state", "ns.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"schema":"oma-state/1","namespace":"ns","data":{},"updated":"yesterday"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Get("ns/x", ""); !errors.Is(err, ErrState) {
		t.Fatalf("bad updated: err = %v, want ErrState", err)
	}
}

func TestDryRunValidatesExistingFile(t *testing.T) {
	s := newStore(t)
	path := filepath.Join(s.ProjectRoot, ".oma", "state", "ns.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema":"oma-state/9"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Set("ns/x", "v", "", true); !errors.Is(err, ErrState) {
		t.Fatalf("dry-run over bad file: err = %v, want ErrState (validation must run)", err)
	}
}

func TestOverwriteKeepsSingleGenerationBak(t *testing.T) {
	s := newStore(t)
	if _, err := s.Set("ns/x", "one", "", false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.ProjectRoot, ".oma", "state", "ns.json")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Set("ns/x", "two", "", false); err != nil {
		t.Fatal(err)
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf(".bak missing: %v", err)
	}
	if string(bak) != string(first) {
		t.Fatal(".bak must hold the prior generation")
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
