package hookcfg

import (
	"encoding/json"
	"strings"
	"testing"
)

// Review 099 must-fix 2: the exe path must survive POSIX shells with
// spaces, quotes, expansion characters, and backticks — single-quoted,
// not JSON-escaped.
func TestPosixQuoteHostileNames(t *testing.T) {
	cases := map[string]string{
		"/opt/o m a/oma":        "'/opt/o m a/oma'",
		"/opt/don't/oma":        `'/opt/don'\''t/oma'`,
		`/opt/say"hi"/oma`:      `'/opt/say"hi"/oma'`,
		"/opt/$HOME-fake/oma":   "'/opt/$HOME-fake/oma'",
		"/opt/`whoami`/oma":     "'/opt/`whoami`/oma'",
		`/opt/back\slash/oma`:   `'/opt/back\slash/oma'`,
		"/plain/oma":            "'/plain/oma'",
		"/opt/'already'/oma":    `'/opt/'\''already'\''/oma'`,
		"/opt/new\nline/oma":    "'/opt/new\nline/oma'",
		"/opt/semi;colon/oma":   "'/opt/semi;colon/oma'",
		"/opt/pipe|amp&/oma":    "'/opt/pipe|amp&/oma'",
		"/opt/redirect>out/oma": "'/opt/redirect>out/oma'",
	}
	for in, want := range cases {
		if got := PosixQuote(in); got != want {
			t.Errorf("PosixQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGuardedCommandUnix(t *testing.T) {
	got := guardedCommand("linux", "/opt/don't/oma", "relay hook Stop")
	want := `[ -x '/opt/don'\''t/oma' ] || exit 0; exec '/opt/don'\''t/oma' relay hook Stop`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// The completed command string must round-trip JSON marshaling
	// unchanged (quote-then-marshal order).
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var back string
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back != got {
		t.Fatalf("JSON round-trip changed the command: %q -> %q", got, back)
	}
}

func TestGuardedCommandWindowsBare(t *testing.T) {
	got := guardedCommand("windows", `C:\Tools\oma.exe`, "relay statusline")
	if strings.Contains(got, "exit 0") || !strings.HasPrefix(got, `"C:\Tools\oma.exe"`) {
		t.Fatalf("windows command must be bare quoted, got %q", got)
	}
}
