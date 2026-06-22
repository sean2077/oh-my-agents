package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanWorkflowsAcrossSessionsAndTypes(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".oma", "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("ralph-codex-aaaaaaaaaaaa.json", `{"schema":"oma-ralph/2","id":"codex-aaaaaaaaaaaa","revision":3,"phase":"running","worktree_root":"/wt/x"}`)
	write("interview-spec--s-claude-bbbbbbbbbbbb.json", `{"schema":"oma-interview/1","id":"spec--s-claude-bbbbbbbbbbbb","revision":7,"phase":"interviewing"}`)
	write("autopilot--s-codex-aaaaaaaaaaaa.json", `{"schema":"oma-state/1","namespace":"autopilot--s-codex-aaaaaaaaaaaa","revision":2,"data":{"phase":"verify"},"updated":"2026-06-22T00:00:00Z"}`)

	rows, err := scanWorkflows(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3: %+v", len(rows), rows)
	}
	byKey := map[string]workflowRow{}
	for _, r := range rows {
		byKey[r.Workflow+"/"+r.Session] = r
	}
	if r := byKey["ralph/codex-aaaaaaaaaaaa"]; r.ID != "default" || r.Phase != "running" || r.Worktree != "/wt/x" || r.Revision != 3 {
		t.Errorf("ralph row = %+v", r)
	}
	if r := byKey["interview/claude-bbbbbbbbbbbb"]; r.ID != "spec" || r.Phase != "interviewing" || r.Revision != 7 {
		t.Errorf("interview row = %+v", r)
	}
	if r := byKey["autopilot/codex-aaaaaaaaaaaa"]; r.ID != "-" || r.Phase != "verify" || r.Revision != 2 {
		t.Errorf("autopilot row = %+v", r)
	}
}

func TestWorkflowListCurrentSessionDefault(t *testing.T) {
	t.Setenv("OMA_SESSION_ID", "sessone")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	if code, out := runOma(t, "ralph", "start", "--goal", "go test ./... passes"); code != ExitOK {
		t.Fatalf("ralph start exit %d: %s", code, out)
	}
	code, out := runOma(t, "workflow", "list", "--json")
	if code != ExitOK {
		t.Fatalf("workflow list exit %d: %s", code, out)
	}
	if !strings.Contains(out, "ralph") || !strings.Contains(out, "sessone") {
		t.Errorf("current-session list missing the ralph loop: %s", out)
	}
}
