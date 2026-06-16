package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetAuditCLI(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "skills", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"schema":"oma-asset/1","name":"demo","type":"skill","targets":["claude","codex"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: demo\ndescription: short\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// --json output carries the oma-cli/1 envelope and the audit rows.
	rc := newRootCmd()
	var buf bytes.Buffer
	rc.SetOut(&buf)
	rc.SetErr(&buf)
	rc.SetArgs([]string{"asset", "audit", "--from", root, "--json"})
	if err := rc.Execute(); err != nil {
		t.Fatalf("asset audit --json: %v\n%s", err, buf.String())
	}
	var out struct {
		Schema string `json:"schema"`
		Audit  []struct {
			Name  string `json:"name"`
			Label string `json:"label"`
		} `json:"audit"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, buf.String())
	}
	if out.Schema != "oma-cli/1" || len(out.Audit) != 1 || out.Audit[0].Name != "demo" {
		t.Fatalf("unexpected audit json: %+v", out)
	}
	if out.Audit[0].Label != "ORPHAN" { // a lone unreferenced skill
		t.Fatalf("demo label = %s, want ORPHAN", out.Audit[0].Label)
	}

	// text output is human-scannable and names the asset + label.
	rc2 := newRootCmd()
	var buf2 bytes.Buffer
	rc2.SetOut(&buf2)
	rc2.SetErr(&buf2)
	rc2.SetArgs([]string{"asset", "audit", "--from", root})
	if err := rc2.Execute(); err != nil {
		t.Fatalf("asset audit text: %v\n%s", err, buf2.String())
	}
	if !bytes.Contains(buf2.Bytes(), []byte("demo")) || !bytes.Contains(buf2.Bytes(), []byte("ORPHAN")) {
		t.Fatalf("text output missing demo/ORPHAN: %s", buf2.String())
	}
}
