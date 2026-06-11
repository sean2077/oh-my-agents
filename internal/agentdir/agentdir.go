// Package agentdir owns the per-agent directory conventions: where each
// asset type projects for claude and codex (docs/adapter-conformance.md §2).
// It is a pure path table — no filesystem writes — so the asset engine can
// plan and verify projections without import cycles.
package agentdir

import "path/filepath"

// Projection kinds.
const (
	KindSymlink = "symlink"
	KindInject  = "inject" // hook fragments merged into the agent's host config
)

// Target describes where one asset projects for one agent.
type Target struct {
	Agent string
	Path  string
	Kind  string
}

// For resolves the projection target for (agent, assetType, assetName).
// ok=false with a human-readable reason when the agent does not support
// the asset type (the caller reports, never silently expands —
// docs/config.md §4b).
//
// Codex paths follow the npx-skills ecosystem conventions; they are
// fixture-verified offline and smoke-verified on a real codex install in
// Phase D (spec: Codex 真机验收后置).
func For(home, agent, assetType, assetName string) (Target, bool, string) {
	switch agent {
	case "claude":
		switch assetType {
		case "skill":
			return t(agent, home, KindSymlink, ".claude", "skills", assetName), true, ""
		case "subagent":
			return t(agent, home, KindSymlink, ".claude", "agents", assetName+".md"), true, ""
		case "prompt":
			return t(agent, home, KindSymlink, ".claude", "commands", assetName+".md"), true, ""
		case "hook":
			return t(agent, home, KindInject, ".claude", "settings.json"), true, ""
		}
	case "codex":
		switch assetType {
		case "skill":
			return t(agent, home, KindSymlink, ".codex", "skills", assetName), true, ""
		case "prompt":
			return t(agent, home, KindSymlink, ".codex", "prompts", assetName+".md"), true, ""
		case "subagent":
			return Target{}, false, "codex has no subagent mechanism (manifest fallback applies)"
		case "hook":
			return t(agent, home, KindInject, ".codex", "hooks.json"), true, ""
		}
	}
	return Target{}, false, "unknown agent or asset type"
}

// HookWrapKey returns the JSON key wrapping the event map in the agent's
// host config file: claude keeps events under settings.json's "hooks"
// key; codex's hooks.json root IS the event map ("").
func HookWrapKey(agent string) string {
	if agent == "claude" {
		return "hooks"
	}
	return ""
}

// AgentRoot is the trusted root for one agent's projections; projection
// writes must stay inside it (docs/security-contract.md §3).
func AgentRoot(home, agent string) string {
	switch agent {
	case "claude":
		return filepath.Join(home, ".claude")
	case "codex":
		return filepath.Join(home, ".codex")
	default:
		return ""
	}
}

func t(agent, home, kind string, parts ...string) Target {
	return Target{Agent: agent, Path: filepath.Join(append([]string{home}, parts...)...), Kind: kind}
}
