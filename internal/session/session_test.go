package session

import "testing"

func TestResolveCurrentCodexSession(t *testing.T) {
	got, err := Resolve("current", func(k string) string {
		if k == "CODEX_THREAD_ID" {
			return "thread-123"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "codex-34bdc44fd758" {
		t.Fatalf("Resolve current = %q", got)
	}
}

func TestResolveExplicitSlug(t *testing.T) {
	got, err := Resolve("release-qa", func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if got != "release-qa" {
		t.Fatalf("Resolve explicit = %q", got)
	}
}

func TestResolveCurrentAmbiguousPlatformSignals(t *testing.T) {
	_, err := Resolve("current", func(k string) string {
		switch k {
		case "CODEX_THREAD_ID":
			return "codex"
		case "CLAUDE_CODE_SESSION_ID":
			return "claude"
		default:
			return ""
		}
	})
	if err == nil {
		t.Fatal("Resolve current with two platform signals must fail")
	}
}

func TestScopeName(t *testing.T) {
	got, err := ScopeName("autopilot", "codex-abc")
	if err != nil {
		t.Fatal(err)
	}
	if got != "autopilot-codex-abc" {
		t.Fatalf("ScopeName = %q", got)
	}
}
