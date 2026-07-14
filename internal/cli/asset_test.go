package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCLIAssetSource(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"oma-asset/1","name":"` + name + `","type":"skill","targets":["claude","codex"]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
}

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
			Name                    string `json:"name"`
			Label                   string `json:"label"`
			DescriptionTokens       int    `json:"description_tokens"`
			DescriptionBudgetTokens int    `json:"description_budget_tokens"`
			BodyTokens              int    `json:"body_tokens"`
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
	if out.Audit[0].BodyTokens != 2 { // "body\n" is 5 bytes under approx-b4/1
		t.Fatalf("demo body_tokens = %d, want 2", out.Audit[0].BodyTokens)
	}
	if out.Audit[0].DescriptionTokens != 2 || out.Audit[0].DescriptionBudgetTokens != 80 {
		t.Fatalf("demo description budget = %d/%d, want 2/80", out.Audit[0].DescriptionTokens, out.Audit[0].DescriptionBudgetTokens)
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
	if !bytes.Contains(buf2.Bytes(), []byte("demo")) || !bytes.Contains(buf2.Bytes(), []byte("ORPHAN")) ||
		!bytes.Contains(buf2.Bytes(), []byte("resident=3")) || !bytes.Contains(buf2.Bytes(), []byte("desc=2  /80")) ||
		!bytes.Contains(buf2.Bytes(), []byte("body=2")) {
		t.Fatalf("text output missing demo/ORPHAN/context metrics: %s", buf2.String())
	}
}

func withAssetDownloadBase(t *testing.T, base string) {
	t.Helper()
	old := assetReleaseDownloadBase
	assetReleaseDownloadBase = base
	t.Cleanup(func() { assetReleaseDownloadBase = old })
}

func TestAssetInstallRemoteDryRunFetchesValidatesAndReportsPaths(t *testing.T) {
	ref := "v1.2.3"
	tarball := makeTarGz(t, map[string]string{
		"skills/demo/manifest.json": `{"schema":"oma-asset/1","name":"demo","type":"skill","targets":["claude","codex"]}`,
		"skills/demo/SKILL.md":      "body",
	})
	srv := serveAssets(t, ref, tarball, false)
	withAssetDownloadBase(t, srv.URL)
	home := t.TempDir()
	t.Setenv("OMA_HOME", home)

	code, out := runOma(t, "--dry-run", "asset", "install", "--ref", ref, "demo")
	if code != ExitOK {
		t.Fatalf("dry-run remote install exit %d: %s", code, out)
	}
	for _, want := range []string{
		"[dry-run] create  " + filepath.Join(home, ".agents", "skills", "demo"),
		"[dry-run] link    " + filepath.Join(home, ".claude", "skills", "demo"),
		"[dry-run] link    " + filepath.Join(home, ".codex", "skills", "demo"),
		"[dry-run] replace " + filepath.Join(home, ".config", "oma", "registry.json"),
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("remote dry-run wrote canonical root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "oma", "registry.json")); !os.IsNotExist(err) {
		t.Fatalf("remote dry-run wrote registry: %v", err)
	}
}

func TestAssetInstallRemoteDryRunRejectsMissingAsset(t *testing.T) {
	ref := "v1.2.3"
	tarball := makeTarGz(t, map[string]string{
		"skills/demo/manifest.json": `{"schema":"oma-asset/1","name":"demo","type":"skill","targets":["claude","codex"]}`,
		"skills/demo/SKILL.md":      "body",
	})
	srv := serveAssets(t, ref, tarball, false)
	withAssetDownloadBase(t, srv.URL)
	home := t.TempDir()
	t.Setenv("OMA_HOME", home)

	code, out := runOma(t, "--dry-run", "asset", "install", "--ref", ref, "not-exist")
	if code != ExitState || !strings.Contains(out, `asset "not-exist" not found`) {
		t.Fatalf("dry-run missing asset exit %d output:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("missing-asset dry-run wrote canonical root: %v", err)
	}
}

func TestAssetInstallValidatesAllAssetsBeforeWriting(t *testing.T) {
	srcRoot := t.TempDir()
	writeCLIAssetSource(t, srcRoot, "demo")
	home := t.TempDir()
	t.Setenv("OMA_HOME", home)

	code, out := runOma(t, "asset", "install", "--from", srcRoot, "demo", "not-exist")
	if code != ExitState || !strings.Contains(out, `asset "not-exist" not found`) {
		t.Fatalf("install missing asset exit %d output:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "demo")); !os.IsNotExist(err) {
		t.Fatalf("first asset was written before validating the full request: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "oma", "registry.json")); !os.IsNotExist(err) {
		t.Fatalf("registry was written before validating the full request: %v", err)
	}
}

func TestAssetInstallFromAndRefAreMutuallyExclusive(t *testing.T) {
	srcRoot := t.TempDir()
	writeCLIAssetSource(t, srcRoot, "demo")

	code, out := runOma(t, "asset", "install", "--from", srcRoot, "--ref", "v1.2.3", "demo")
	if code != ExitUsage || !strings.Contains(out, "from ref") || !strings.Contains(out, "were all set") {
		t.Fatalf("from/ref exit %d output:\n%s", code, out)
	}
}
