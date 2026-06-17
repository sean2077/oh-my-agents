package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPairDeliverySkillCodexStopHookFirst(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "assets", "skills", "pair-delivery", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"the Stop hook is the main self-continuation path",
		"Stop hook dispatcher is wired in `~/.codex/hooks.json` and trusted through `/hooks`",
		"Use held/re-polled `oma relay wait` only as the fallback path",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("pair-delivery skill lost Codex Stop-hook-first guardrail %q", want)
		}
	}
}
