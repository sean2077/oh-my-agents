package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, name, targets, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"oma-asset/1","name":"` + name + `","type":"skill","targets":[` + targets + `]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultPathConformance(t *testing.T) {
	root := t.TempDir()

	// codex-targeted, Claude-only affordance on the default path → violations.
	writeSkill(t, root, "bad-ask", `"claude","codex"`,
		"# bad\n\nAsk the user via AskUserQuestion with 2-4 options.\n")
	writeSkill(t, root, "bad-sub", `"codex"`,
		"# bad\n\nFan out a subagent per subsystem and synthesize the results.\n")

	// codex-targeted but the affordance is inside a CC-acceleration block → ok.
	writeSkill(t, root, "ok-accel", `"claude","codex"`,
		"# ok\n\nDefault path is plain markdown.\n\n> **CC acceleration (optional, Claude Code only)**: present options through AskUserQuestion and fan out subagents.\n")

	// claude-only skill may use the affordance on its default path → exempt.
	writeSkill(t, root, "ok-claude-only", `"claude"`,
		"# ok\n\nUse AskUserQuestion and a subagent freely.\n")

	// clean codex skill → ok. "do not spawn" is generic, not a subagent ref.
	writeSkill(t, root, "ok-clean", `"codex"`,
		"# ok\n\nPlain `oma` commands and markdown only; do not spawn a nested loop.\n")

	flagged := map[string]int{}
	for _, f := range DefaultPathConformance(root) {
		if f.Level != LevelFail {
			t.Errorf("finding level = %q, want fail: %s", f.Level, f.Message)
		}
		flagged[strings.SplitN(f.Message, " ", 2)[0]]++
	}

	if flagged["bad-ask"] == 0 {
		t.Errorf("AskUserQuestion in codex default-path must be flagged; flagged=%v", flagged)
	}
	if flagged["bad-sub"] == 0 {
		t.Errorf("subagent in codex default-path must be flagged; flagged=%v", flagged)
	}
	for _, ok := range []string{"ok-accel", "ok-claude-only", "ok-clean"} {
		if flagged[ok] != 0 {
			t.Errorf("%s must not be flagged; flagged=%v", ok, flagged)
		}
	}
}
