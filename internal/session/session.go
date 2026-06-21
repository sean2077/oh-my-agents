// Package session provides workflow-session scoping for project-level state.
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// Resolve returns a path-safe session suffix. The default session value is
// "current"; workflow state always has a session boundary.
func Resolve(value string, getenv func(string) string) (string, error) {
	value = strings.TrimSpace(value)
	switch value {
	case "", "current":
		return current(getenv)
	default:
		return explicit(value), nil
	}
}

// ScopeName appends the session suffix to a workflow namespace or id.
func ScopeName(name, suffix string) (string, error) {
	name = strings.TrimSpace(name)
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return "", fmt.Errorf("session suffix is required")
	}
	if name == "" {
		return "", nil
	}
	scoped := name + "-" + suffix
	if len(scoped) > 64 {
		return "", fmt.Errorf("session-scoped name %q is too long (max 64)", scoped)
	}
	return scoped, nil
}

func current(getenv func(string) string) (string, error) {
	if raw := strings.TrimSpace(getenv("OMA_SESSION_ID")); raw != "" {
		return explicit(raw), nil
	}
	claudeID := strings.TrimSpace(getenv("CLAUDE_CODE_SESSION_ID"))
	codexID := strings.TrimSpace(getenv("CODEX_THREAD_ID"))
	switch {
	case claudeID != "" && codexID != "":
		return "", fmt.Errorf("both CLAUDE_CODE_SESSION_ID and CODEX_THREAD_ID are set; pass --session <slug> or set OMA_SESSION_ID")
	case claudeID != "":
		return hashSlug("claude", claudeID), nil
	case codexID != "":
		return hashSlug("codex", codexID), nil
	default:
		return "", fmt.Errorf("no current session signal (set CODEX_THREAD_ID, CLAUDE_CODE_SESSION_ID, OMA_SESSION_ID, or pass --session <slug>)")
	}
}

func explicit(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.ReplaceAll(raw, "_", "-")
	if slugRe.MatchString(raw) {
		return raw
	}
	return hashSlug("session", raw)
}

func hashSlug(prefix, raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return prefix + "-" + hex.EncodeToString(sum[:])[:12]
}
