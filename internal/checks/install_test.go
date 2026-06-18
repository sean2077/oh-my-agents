package checks

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// installDemo places a minimal claude-targeted skill in a temp home and
// returns the home path.
func installDemo(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	src := filepath.Join(t.TempDir(), "skills", "demo")
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"oma-asset/1","name":"demo","type":"skill","targets":["claude"],"fallback":"codex reads canonical"}`
	if err := os.WriteFile(filepath.Join(src, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("Use `oma state get demo/x`."), 0o600); err != nil {
		t.Fatal(err)
	}
	eng := newEngine(t, home)
	if _, err := eng.Install(src, installOptions()); err != nil {
		t.Fatalf("install: %v", err)
	}
	return home
}

func doctorWorst(t *testing.T, home string) (string, []Finding) {
	t.Helper()
	set := CommandSet{"oma": false, "oma state": false, "oma state get": true}
	res := RunAll(InstallChecks(home, "", set))
	return res.Worst, res.Findings
}

func TestDoctorDetectsProjectionRootDriftAfterInstall(t *testing.T) {
	home := installDemo(t)
	// post-install drift: swap .claude/skills for an outside symlink that
	// still contains the expected link (review 038 blocker 1 repro)
	outside := t.TempDir()
	skills := filepath.Join(home, ".claude", "skills")
	canonical := filepath.Join(home, ".agents", "skills", "demo")
	if err := os.Symlink(canonical, filepath.Join(outside, "demo")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := os.RemoveAll(skills); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, skills); err != nil {
		t.Fatal(err)
	}
	worst, findings := doctorWorst(t, home)
	if worst != LevelFail {
		t.Fatalf("worst = %s, want fail; findings: %+v", worst, findings)
	}
}

func TestDoctorDetectsAgentRootPermissionDrift(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX world-writable mode bits do not model Windows ACLs")
	}
	home := installDemo(t)
	if err := os.Chmod(filepath.Join(home, ".claude"), 0o777); err != nil {
		t.Fatal(err)
	}
	worst, findings := doctorWorst(t, home)
	if worst != LevelFail {
		t.Fatalf("worst = %s, want fail; findings: %+v", worst, findings)
	}
	found := false
	for _, f := range findings {
		if f.Check == "permissions" && f.Level == LevelFail && strings.Contains(f.Message, "world-writable") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing world-writable permissions finding: %+v", findings)
	}
}

func TestDoctorCleanInstallIsGreen(t *testing.T) {
	home := installDemo(t)
	worst, findings := doctorWorst(t, home)
	if worst != LevelOK {
		t.Fatalf("clean install worst = %s; findings: %+v", worst, findings)
	}
}
