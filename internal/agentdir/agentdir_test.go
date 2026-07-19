package agentdir

import "testing"

func TestCodexSubagentSkipReasonIsScopedToAssetProjection(t *testing.T) {
	_, ok, reason := For(t.TempDir(), "codex", "subagent", "explorer")
	if ok {
		t.Fatal("codex subagent asset projection unexpectedly supported")
	}
	const want = "oma subagent asset projection to codex is unsupported"
	if reason != want {
		t.Fatalf("reason = %q, want %q", reason, want)
	}
}
