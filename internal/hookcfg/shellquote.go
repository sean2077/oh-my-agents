package hookcfg

import (
	"runtime"
	"strings"
)

// PosixQuote single-quotes s for a POSIX shell. A single quote inside s
// is closed, backslash-escaped, and reopened (quote-backslash-quote-
// quote), the only escape POSIX single quoting needs; everything else
// ($, backticks, double quotes, spaces, backslashes) is literal inside
// single quotes. JSON marshaling is NOT shell quoting (review 099
// must-fix 2) — callers quote first, then marshal the completed command.
func PosixQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// GuardedCommand builds the host hook/statusLine command for the
// executable at exe with the given invocation args (e.g. "relay hook
// Stop"). On unix the command embeds the absolute path behind an
// existence guard so a removed/moved binary silently no-ops instead of
// spamming command-not-found, and a stripped host PATH cannot make the
// hook silently miss (v1 parity). Windows hosts get the bare quoted
// command — no POSIX guard exists there (documented limitation).
func GuardedCommand(exe, invocation string) string {
	return guardedCommand(runtime.GOOS, exe, invocation)
}

func guardedCommand(goos, exe, invocation string) string {
	if goos == "windows" {
		return `"` + exe + `" ` + invocation
	}
	q := PosixQuote(exe)
	return "[ -x " + q + " ] || exit 0; exec " + q + " " + invocation
}
