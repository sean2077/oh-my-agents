package state

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	v, ok, rev, err := s.GetWithRevision("autopilot/phase", "")
	if err != nil || !ok || v != "planning" {
		t.Fatalf("get = %q ok=%v err=%v", v, ok, err)
	}
	if rev != 1 {
		t.Fatalf("revision = %d, want 1", rev)
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

func TestSetExpectedRevision(t *testing.T) {
	s := newStore(t)
	if _, err := s.Set("wf/a", "1", "", false); err != nil {
		t.Fatal(err)
	}
	_, _, rev, err := s.GetWithRevision("wf/a", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetExpected("wf/b", "2", "", false, &rev); err != nil {
		t.Fatalf("expected revision set: %v", err)
	}
	if _, err := s.SetExpected("wf/c", "3", "", false, &rev); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision set: err = %v, want ErrConflict", err)
	}
	_, _, rev, err = s.GetWithRevision("wf/a", "")
	if err != nil {
		t.Fatal(err)
	}
	if rev != 2 {
		t.Fatalf("revision after two writes = %d, want 2", rev)
	}
}

func TestConcurrentProcessSetsDoNotLoseFields(t *testing.T) {
	root := t.TempDir()
	start := filepath.Join(root, "start")
	const workers = 16
	type child struct {
		cmd *exec.Cmd
		out *bytes.Buffer
	}
	children := make([]child, 0, workers)
	for i := 0; i < workers; i++ {
		field := fmt.Sprintf("f%02d", i)
		cmd := exec.Command(os.Args[0], "-test.run=^TestStateSetHelperProcess$")
		cmd.Env = append(os.Environ(),
			"OMA_STATE_HELPER=1",
			"OMA_STATE_ROOT="+root,
			"OMA_STATE_FIELD="+field,
			"OMA_STATE_START="+start,
		)
		out := &bytes.Buffer{}
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Start(); err != nil {
			t.Fatalf("start helper %s: %v", field, err)
		}
		children = append(children, child{cmd: cmd, out: out})
	}
	if err := os.WriteFile(start, []byte("go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i, child := range children {
		if err := child.cmd.Wait(); err != nil {
			t.Fatalf("helper %02d: %v\n%s", i, err, child.out.String())
		}
	}
	s := New(root)
	for i := 0; i < workers; i++ {
		field := fmt.Sprintf("f%02d", i)
		if v, ok, err := s.Get("shared/"+field, ""); err != nil || !ok || v != field {
			t.Fatalf("%s = %q ok=%v err=%v", field, v, ok, err)
		}
	}
	_, _, rev, err := s.GetWithRevision("shared/f00", "")
	if err != nil {
		t.Fatal(err)
	}
	if rev != workers {
		t.Fatalf("revision = %d, want %d", rev, workers)
	}
}

func TestStateSetHelperProcess(t *testing.T) {
	if os.Getenv("OMA_STATE_HELPER") != "1" {
		return
	}
	root := os.Getenv("OMA_STATE_ROOT")
	field := os.Getenv("OMA_STATE_FIELD")
	start := os.Getenv("OMA_STATE_START")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(start); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "timed out waiting for start file")
			os.Exit(2)
		}
		time.Sleep(5 * time.Millisecond)
	}
	s := New(root)
	s.Now = func() time.Time { return time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC) }
	if _, err := s.Set("shared/"+field, field, "", false); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
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
