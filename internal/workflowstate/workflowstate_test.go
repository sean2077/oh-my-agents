package workflowstate

import (
	"testing"

	"github.com/sean2077/oh-my-agents/internal/state"
)

func TestScopeIDAndStateKey(t *testing.T) {
	scope := Scope{Session: "release"}

	if got, err := scope.ID("same"); err != nil || got != "same-release" {
		t.Fatalf("ID = %q err=%v", got, err)
	}
	if got, err := scope.StateKey("autopilot/phase"); err != nil || got != "autopilot-release/phase" {
		t.Fatalf("StateKey = %q err=%v", got, err)
	}
}

func TestScopeCurrentUsesPlatformSession(t *testing.T) {
	scope := Scope{
		Session: "current",
		Getenv: func(k string) string {
			if k == "CODEX_THREAD_ID" {
				return "thread-123"
			}
			return ""
		},
	}

	got, err := scope.ID("ralph")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ralph-codex-34bdc44fd758" {
		t.Fatalf("ID = %q", got)
	}
}

func TestScopeDefaultUsesCurrent(t *testing.T) {
	scope := Scope{
		Getenv: func(k string) string {
			if k == "OMA_SESSION_ID" {
				return "default"
			}
			return ""
		},
	}

	got, err := scope.StateKey("autopilot/phase")
	if err != nil {
		t.Fatal(err)
	}
	if got != "autopilot-default/phase" {
		t.Fatalf("StateKey = %q", got)
	}
}

func TestFilterEntriesBySessionSuffix(t *testing.T) {
	scope := Scope{Session: "codex-abc"}
	entries := []state.Entry{
		{Namespace: "autopilot-codex-abc"},
		{Namespace: "autopilot-extra-codex-abc"},
		{Namespace: "autopilot-claude-def"},
	}

	got, err := scope.FilterEntries(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Namespace != "autopilot-codex-abc" || got[1].Namespace != "autopilot-extra-codex-abc" {
		t.Fatalf("FilterEntries = %+v", got)
	}
}
