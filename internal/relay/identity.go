package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
)

// Identity is the resolved author of this oma process plus a stable
// per-session key used to name its pair binding file.
type Identity struct {
	Author     string
	SessionKey string // hex prefix of sha256(author:platform-session-id)
}

// authorRe keeps author names path- and filename-safe by construction.
var authorRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ResolveIdentity applies the protocol §4 precedence: platform signal
// (CLAUDE_CODE_SESSION_ID → claude, CODEX_THREAD_ID → codex) beats
// OMA_RELAY_AUTHOR; both platform signals present without an arbiter is
// ambiguous and refused fail-closed; no signal at all is refused.
func ResolveIdentity(getenv func(string) string) (Identity, error) {
	claudeID := getenv("CLAUDE_CODE_SESSION_ID")
	codexID := getenv("CODEX_THREAD_ID")
	envAuthor := getenv("OMA_RELAY_AUTHOR")

	switch {
	case claudeID != "" && codexID != "":
		// Both platform signals: only an explicit arbiter naming one of
		// the two platforms resolves the ambiguity.
		switch envAuthor {
		case "claude":
			return makeIdentity("claude", claudeID)
		case "codex":
			return makeIdentity("codex", codexID)
		default:
			return Identity{}, fmt.Errorf("%w: both CLAUDE_CODE_SESSION_ID and CODEX_THREAD_ID are set; set OMA_RELAY_AUTHOR=claude|codex to arbitrate", ErrRelay)
		}
	case claudeID != "":
		return makeIdentity("claude", claudeID)
	case codexID != "":
		return makeIdentity("codex", codexID)
	case envAuthor != "":
		return makeIdentity(envAuthor, "")
	default:
		return Identity{}, fmt.Errorf("%w: cannot resolve author (no platform session signal and OMA_RELAY_AUTHOR unset)", ErrRelay)
	}
}

func makeIdentity(author, sessionID string) (Identity, error) {
	if !authorRe.MatchString(author) {
		return Identity{}, fmt.Errorf("%w: author %q (want %s)", ErrRelay, author, authorRe)
	}
	sum := sha256.Sum256([]byte(author + ":" + sessionID))
	return Identity{Author: author, SessionKey: hex.EncodeToString(sum[:])[:12]}, nil
}
