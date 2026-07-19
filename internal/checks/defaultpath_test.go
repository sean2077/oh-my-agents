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

	writeSkill(t, root, "bad-wrong-block", `"claude","codex"`,
		"# bad\n\n> **CC acceleration**: fan out subagents.\n")

	// claude-only skill may use the affordance on its default path → exempt.
	writeSkill(t, root, "ok-claude-only", `"claude"`,
		"# ok\n\nUse AskUserQuestion and a subagent freely.\n")

	writeSkill(t, root, "ok-clean", `"codex"`,
		"# ok\n\n> **CC acceleration**: ask with AskUserQuestion.\n\n> **Parallel acceleration (optional, capability-gated)**: delegate to subagents.\n")

	findings := DefaultPathConformance(root)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one wrong-block finding", findings)
	}
	f := findings[0]
	if f.Level != LevelFail {
		t.Errorf("finding level = %q, want fail: %s", f.Level, f.Message)
	}
	for _, want := range []string{
		"bad-wrong-block SKILL.md:3",
		`references "subagent" in a CC acceleration block`,
		"`> **Parallel acceleration (optional, capability-gated)**:`",
	} {
		if !strings.Contains(f.Message, want) {
			t.Errorf("message %q does not contain %q", f.Message, want)
		}
	}
}

func TestDefaultPathViolationsSeparateExactMarkers(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantMarker  string
		wantBlock   accelerationBlock
		wantAllowed accelerationBlock
		wantLine    int
	}{
		{
			name:        "AskUserQuestion on default path",
			body:        "# bad\nAsk via AskUserQuestion.\n",
			wantMarker:  "AskUserQuestion",
			wantAllowed: ccBlock,
			wantLine:    2,
		},
		{
			name:        "subagent on default path",
			body:        "# bad\nDelegate to a subagent.\n",
			wantMarker:  "subagent",
			wantAllowed: parallelBlock,
			wantLine:    2,
		},
		{
			name:        "spawn_agent tool on default path",
			body:        "# bad\nCall spawn_agent for each subsystem.\n",
			wantMarker:  "subagent",
			wantAllowed: parallelBlock,
			wantLine:    2,
		},
		{
			name:        "hyphenated sub-agent on default path",
			body:        "# bad\nSpawn a sub-agent for each subsystem.\n",
			wantMarker:  "subagent",
			wantAllowed: parallelBlock,
			wantLine:    2,
		},
		{
			name:        "AskUserQuestion under Parallel marker",
			body:        "> **Parallel acceleration (optional, capability-gated)**: ask via AskUserQuestion.\n",
			wantMarker:  "AskUserQuestion",
			wantBlock:   parallelBlock,
			wantAllowed: ccBlock,
			wantLine:    1,
		},
		{
			name:        "subagent under CC marker",
			body:        "> **CC acceleration**: delegate to subagents.\n",
			wantMarker:  "subagent",
			wantBlock:   ccBlock,
			wantAllowed: parallelBlock,
			wantLine:    1,
		},
		{
			name:        "old parenthetical CC marker is stale",
			body:        "> **CC acceleration (optional, Claude Code only)**: ask via AskUserQuestion.\n",
			wantMarker:  "AskUserQuestion",
			wantAllowed: ccBlock,
			wantLine:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := defaultPathViolations(tc.body)
			if len(got) != 1 {
				t.Fatalf("violations = %+v, want one", got)
			}
			if got[0].marker != tc.wantMarker || got[0].block != tc.wantBlock || got[0].allowed != tc.wantAllowed || got[0].line != tc.wantLine {
				t.Fatalf("violation = %+v, want marker=%q block=%q allowed=%q line=%d", got[0], tc.wantMarker, tc.wantBlock, tc.wantAllowed, tc.wantLine)
			}
		})
	}
}

func TestDefaultPathViolationsAllowExactBlocksAndOrdinarySpawn(t *testing.T) {
	body := strings.Join([]string{
		"# ok",
		"Plain markdown may spawn a process or say do not spawn a nested loop.",
		"> **CC acceleration**: ask through AskUserQuestion.",
		">",
		"> Continue to use AskUserQuestion in this quoted paragraph.",
		"",
		"> **Parallel acceleration (optional, capability-gated)**: delegate bounded lanes.",
		">",
		"> A subagent returns evidence to the parent.",
		"",
	}, "\n")
	if got := defaultPathViolations(body); len(got) != 0 {
		t.Fatalf("violations = %+v, want none", got)
	}
}

func TestDefaultPathConformanceMissingRoot(t *testing.T) {
	if findings := DefaultPathConformance(filepath.Join(t.TempDir(), "missing")); len(findings) != 0 {
		t.Fatalf("findings = %+v, want none", findings)
	}
}

func TestDefaultPathConformanceReportsEveryGovernedAffordance(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "bad", `"codex"`,
		"# bad\n\nUse AskUserQuestion and a subagent.\n")
	findings := DefaultPathConformance(root)
	if len(findings) != 2 {
		t.Fatalf("findings = %+v, want two", findings)
	}
	flagged := map[string]bool{}
	for _, f := range findings {
		if f.Level != LevelFail {
			t.Errorf("finding level = %q, want fail: %s", f.Level, f.Message)
		}
		for _, marker := range []string{"AskUserQuestion", "subagent"} {
			if strings.Contains(f.Message, `"`+marker+`"`) {
				flagged[marker] = true
			}
		}
	}
	if !flagged["AskUserQuestion"] || !flagged["subagent"] {
		t.Errorf("governed affordances = %v, want both", flagged)
	}
}
