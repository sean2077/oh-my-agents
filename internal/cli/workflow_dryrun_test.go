package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runOma executes one invocation against the CWD-resolved project.
func runOma(t *testing.T, args ...string) (int, string) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	return executeWith(root, &out), out.String()
}

func treeFingerprint(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		_, _ = fmt.Fprintf(h, "%s|%v|%d\n", path, info.IsDir(), info.Size())
		if !info.IsDir() {
			raw, _ := os.ReadFile(path)
			h.Write(raw)
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

func TestWorkflowCLIDryRunSnapshot(t *testing.T) {
	// review 060 blocker 1 at the CLI layer: snapshot .oma/state around
	// every --dry-run mutator, including the passing gate path that
	// previously wrote state and a .bak.
	t.Setenv("OMA_SESSION_ID", "dryrun")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	if code, out := runOma(t, "interview", "start", "--id", "t1", "--depth", "deep"); code != ExitOK {
		t.Fatalf("start exit %d: %s", code, out)
	}
	writeJSON := func(name, content string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	topo := writeJSON("topo.json", `{"schema":"oma-interview-scores/1","round":0,"topology":{"components":[{"id":"a","name":"a","description":"d","status":"active"}]}}`)
	if code, out := runOma(t, "interview", "score", "--id", "t1", "--input", topo); code != ExitOK {
		t.Fatalf("topology exit %d: %s", code, out)
	}
	// ambiguity 0.10 == deep threshold: the passing edge case
	pass := writeJSON("r1.json", `{"schema":"oma-interview-scores/1","round":1,"component_scores":{"a":{"goal":0.9,"constraints":0.9,"criteria":0.9}}}`)
	if code, out := runOma(t, "interview", "score", "--id", "t1", "--input", pass); code != ExitOK {
		t.Fatalf("score exit %d: %s", code, out)
	}

	stateDir := filepath.Join(dir, ".oma", "state")
	before := treeFingerprint(t, stateDir)

	// The smoking gun from review 060: dry-run on a PASSING gate.
	code, out := runOma(t, "--dry-run", "interview", "gate", "--id", "t1")
	if code != ExitOK || !strings.Contains(out, "dry-run: would replace") {
		t.Fatalf("dry-run gate exit %d: %s", code, out)
	}
	if got := treeFingerprint(t, stateDir); got != before {
		t.Fatal("--dry-run interview gate wrote state")
	}
	if _, err := os.Stat(filepath.Join(stateDir, "interview-t1--s-dryrun.json.bak")); err == nil {
		raw, _ := os.ReadFile(filepath.Join(stateDir, "interview-t1--s-dryrun.json.bak"))
		// a .bak from the earlier REAL score command is fine; the dry-run
		// must not have refreshed it — covered by the fingerprint above.
		_ = raw
	}

	// Invalid dry-run inputs exit nonzero (same validation as real).
	if code, _ := runOma(t, "--dry-run", "interview", "crystallize", "--id", "missing", "--spec", "absent.md"); code == ExitOK {
		t.Fatal("dry-run crystallize on missing id must fail")
	}
	if code, _ := runOma(t, "--dry-run", "interview", "gate", "--waive"); code == ExitOK {
		t.Fatal("dry-run waive without reason must fail")
	}
	if code, _ := runOma(t, "--dry-run", "ralph", "next"); code == ExitOK {
		t.Fatal("dry-run ralph next with no loop must fail")
	}
	if code, _ := runOma(t, "--dry-run", "ralph", "start", "--goal", "x", "--id", "bad*id"); code == ExitOK {
		t.Fatal("dry-run ralph start with bad id must fail")
	}
	if got := treeFingerprint(t, stateDir); got != before {
		t.Fatal("failing dry-run validations wrote state")
	}

	// ralph dry-run round-trip: would-write reported, nothing written.
	if code, out := runOma(t, "ralph", "start", "--goal", "g", "--id", "r1"); code != ExitOK {
		t.Fatalf("ralph start exit %d: %s", code, out)
	}
	// Advance one real round so the dry-run check below is a legal transition
	// (a check needs a round to measure).
	if code, out := runOma(t, "ralph", "next", "--id", "r1"); code != ExitOK {
		t.Fatalf("ralph next exit %d: %s", code, out)
	}
	mid := treeFingerprint(t, stateDir)
	if code, out := runOma(t, "--dry-run", "ralph", "next", "--id", "r1"); code != ExitOK || !strings.Contains(out, "would replace") {
		t.Fatalf("dry-run next exit %d: %s", code, out)
	}
	if code, out := runOma(t, "--dry-run", "ralph", "check", "--verifier-exit", "1", "--note", "sig", "--id", "r1"); code != ExitOK || !strings.Contains(out, "would replace") {
		t.Fatalf("dry-run check exit %d: %s", code, out)
	}
	if got := treeFingerprint(t, stateDir); got != mid {
		t.Fatal("ralph dry-run mutators wrote state")
	}
}

func TestWorkflowCLIOmittedIDFailsClosedOnCorruptDefaultState(t *testing.T) {
	// Omitted --id now targets the current session's default instance
	// directly. A corrupt default file must fail closed instead of falling
	// back to another explicit instance.
	t.Setenv("OMA_SESSION_ID", "corrupt")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if code, out := runOma(t, "interview", "start", "--id", "good"); code != ExitOK {
		t.Fatalf("start exit %d: %s", code, out)
	}
	stateDir := filepath.Join(dir, ".oma", "state")
	if err := os.WriteFile(filepath.Join(stateDir, "interview-corrupt.json"), []byte(`{"schema":"oma-interview/9","id":"corrupt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out := runOma(t, "interview", "status"); code != ExitState || !strings.Contains(out, `schema "oma-interview/9"`) {
		t.Fatalf("omitted-id with corrupt default: exit %d out=%s", code, out)
	}
	if code, _ := runOma(t, "interview", "status", "--id", "good"); code != ExitOK {
		t.Fatal("explicit good id must still work")
	}
}

func TestWorkflowCLIOmittedIDUsesSessionDefaultInstance(t *testing.T) {
	t.Setenv("OMA_SESSION_ID", "defaultid")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	if code, out := runOma(t, "interview", "start"); code != ExitOK {
		t.Fatalf("interview start exit %d: %s", code, out)
	}
	topo := filepath.Join(dir, "topo.json")
	if err := os.WriteFile(topo, []byte(`{"schema":"oma-interview-scores/1","round":0,"topology":{"components":[{"id":"a","name":"a","description":"d","status":"active"}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out := runOma(t, "interview", "score", "--input", topo); code != ExitOK {
		t.Fatalf("omitted-id interview score exit %d: %s", code, out)
	}
	if code, out := runOma(t, "interview", "status"); code != ExitOK || !strings.Contains(out, "defaultid") {
		t.Fatalf("omitted-id interview status exit %d: %s", code, out)
	}
	if code, out := runOma(t, "interview", "start", "--id", "feature-a"); code != ExitOK {
		t.Fatalf("explicit interview start exit %d: %s", code, out)
	}
	if code, out := runOma(t, "interview", "status"); code != ExitOK || strings.Contains(out, "feature-a--s-defaultid") {
		t.Fatalf("omitted-id must stay on default interview, exit %d: %s", code, out)
	}

	if code, out := runOma(t, "ralph", "start", "--goal", "keep going"); code != ExitOK {
		t.Fatalf("ralph start exit %d: %s", code, out)
	}
	if code, out := runOma(t, "ralph", "next"); code != ExitOK || !strings.Contains(out, "round=1") {
		t.Fatalf("omitted-id ralph next exit %d: %s", code, out)
	}
	if code, out := runOma(t, "ralph", "start", "--id", "loop", "--goal", "explicit loop"); code != ExitOK {
		t.Fatalf("explicit ralph start exit %d: %s", code, out)
	}
	if code, out := runOma(t, "ralph", "status"); code != ExitOK || strings.Contains(out, "loop--s-defaultid") {
		t.Fatalf("omitted-id must stay on default ralph loop, exit %d: %s", code, out)
	}
}

func TestWorkflowStateUsesProjectRootAndSessionScope(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	main := t.TempDir()
	for _, name := range []string{"one", "two"} {
		gitdir := filepath.Join(main, ".git", "worktrees", name)
		if err := os.MkdirAll(gitdir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(gitdir, "commondir"), []byte("../..\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	worktree := func(name string) string {
		t.Helper()
		wt := t.TempDir()
		gitdir := filepath.Join(main, ".git", "worktrees", name)
		if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return wt
	}
	wt1 := worktree("one")
	wt2 := worktree("two")

	for _, tc := range []struct {
		root   string
		thread string
		phase  string
	}{
		{wt1, "thread-one", "implement"},
		{wt2, "thread-two", "verify"},
	} {
		t.Chdir(tc.root)
		t.Setenv("CODEX_THREAD_ID", tc.thread)
		if code, out := runOma(t, "state", "set", "autopilot/phase", tc.phase); code != ExitOK {
			t.Fatalf("%s state set exit %d: %s", tc.root, code, out)
		}
		if code, out := runOma(t, "interview", "start", "--id", "same", "--idea", "parallel session"); code != ExitOK {
			t.Fatalf("%s interview start exit %d: %s", tc.root, code, out)
		}
		if code, out := runOma(t, "ralph", "start", "--id", "same", "--goal", "parallel session verifier"); code != ExitOK {
			t.Fatalf("%s ralph start exit %d: %s", tc.root, code, out)
		}
	}

	type stateEntry struct {
		Namespace string            `json:"namespace"`
		Data      map[string]string `json:"data"`
	}
	readAutopilot := func(root, thread string) []stateEntry {
		t.Helper()
		t.Chdir(root)
		t.Setenv("CODEX_THREAD_ID", thread)
		args := []string{"state", "list", "autopilot", "--json"}
		code, out := runOma(t, args...)
		if code != ExitOK {
			t.Fatalf("%s state list exit %d: %s", root, code, out)
		}
		var got struct {
			States []stateEntry `json:"states"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("state list json: %v\n%s", err, out)
		}
		return got.States
	}
	if got := readAutopilot(wt1, "thread-one"); len(got) != 1 || got[0].Data["phase"] != "implement" {
		t.Fatalf("session one states = %+v, want implement only", got)
	}
	if got := readAutopilot(wt2, "thread-two"); len(got) != 1 || got[0].Data["phase"] != "verify" {
		t.Fatalf("session two states = %+v, want verify only", got)
	}
	t.Chdir(wt1)
	t.Setenv("CODEX_THREAD_ID", "thread-one")
	if code, out := runOma(t, "state", "set", "autopilot-extra/phase", "plan"); code != ExitOK {
		t.Fatalf("extra state set exit %d: %s", code, out)
	}
	if got := readAutopilot(wt1, "thread-one"); len(got) != 2 {
		t.Fatalf("session one prefix list = %+v, want both autopilot namespaces for this session", got)
	}
	if _, err := os.Stat(filepath.Join(main, ".oma", "state")); err != nil {
		t.Fatalf("primary repo .oma/state should hold workflow state: %v", err)
	}
	for _, wt := range []string{wt1, wt2} {
		if _, err := os.Stat(filepath.Join(wt, ".oma")); !os.IsNotExist(err) {
			t.Fatalf("linked worktree .oma should not be used: %s err=%v", wt, err)
		}
	}
}

func TestRalphBindsDefaultLoopToStartingWorktree(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	main := t.TempDir()
	for _, name := range []string{"one", "two"} {
		gitdir := filepath.Join(main, ".git", "worktrees", name)
		if err := os.MkdirAll(gitdir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(gitdir, "commondir"), []byte("../..\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	worktree := func(name string) string {
		t.Helper()
		wt := t.TempDir()
		gitdir := filepath.Join(main, ".git", "worktrees", name)
		if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return wt
	}
	wt1 := worktree("one")
	wt2 := worktree("two")

	t.Setenv("CODEX_THREAD_ID", "same-thread")
	t.Chdir(wt1)
	if code, out := runOma(t, "ralph", "start", "--goal", "verify this worktree"); code != ExitOK {
		t.Fatalf("ralph start exit %d: %s", code, out)
	}
	if code, out := runOma(t, "ralph", "status"); code != ExitOK {
		t.Fatalf("same-worktree status exit %d: %s", code, out)
	}

	t.Chdir(wt2)
	if code, out := runOma(t, "ralph", "status"); code != ExitState || !strings.Contains(out, "bound to worktree") {
		t.Fatalf("cross-worktree status exit %d out=%s", code, out)
	}
	if code, out := runOma(t, "ralph", "status", "--allow-worktree-change"); code != ExitOK {
		t.Fatalf("allow cross-worktree status exit %d: %s", code, out)
	}
}
