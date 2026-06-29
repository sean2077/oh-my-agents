package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TEST-3: self-update cobra wiring. These tests verify newSelfUpdateCmd
// (selfupdate.go) is correctly registered and that the no-network surfaces
// behave, WITHOUT performing any update/download. The running test binary's
// version is the dev stamp ("dev"), and every assertion here either stops in
// cobra (help) or fails closed inside update.Resolve BEFORE any HTTP fetch.

// findCommand walks the cobra tree for a direct subcommand by its first Use
// word (e.g. "self-update").
func findCommand(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// TestSelfUpdateCommandWiring pins newSelfUpdateCmd's registration and flag set
// (selfupdate.go:11-59). It is purely structural — no RunE, no network.
//
// fail-before rationale: the command is added via newRootCmd's AddCommand list
// (root.go:84). Remove newSelfUpdateCmd() there, or rename its Use, and the
// findCommand lookup returns nil and this fails. Each flag assertion pins the
// corresponding cmd.Flags().*Var call in selfupdate.go (lines 55-58); drop any
// and Lookup returns nil.
func TestSelfUpdateCommandWiring(t *testing.T) {
	root := newRootCmd()
	cmd := findCommand(root, "self-update")
	if cmd == nil {
		t.Fatal("self-update not registered on the root command")
	}
	if cmd.Use != "self-update" {
		t.Fatalf("Use = %q, want self-update", cmd.Use)
	}
	if !strings.HasPrefix(cmd.Short, "Update oma") {
		t.Fatalf("Short = %q, want it to start with %q", cmd.Short, "Update oma")
	}
	for _, name := range []string{"check", "allow-downgrade", "channel", "version"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("self-update missing --%s flag", name)
		}
	}
	// --check and --allow-downgrade are booleans; --channel/--version take values.
	if f := cmd.Flags().Lookup("check"); f != nil && f.Value.Type() != "bool" {
		t.Errorf("--check type = %q, want bool", f.Value.Type())
	}
	if f := cmd.Flags().Lookup("channel"); f != nil && f.Value.Type() != "string" {
		t.Errorf("--channel type = %q, want string", f.Value.Type())
	}
}

// TestSelfUpdateHelpIsOffline pins that the help path (cobra-handled, never
// enters RunE) exits ExitOK and documents the command without any network I/O.
//
// fail-before rationale: --help is intercepted by cobra before RunE, so it
// cannot touch update.New / CheckTarget. If the command were unregistered
// (root.go:84) cobra would error ExitUsage instead of ExitOK, failing this.
func TestSelfUpdateHelpIsOffline(t *testing.T) {
	code, out := runOma(t, "self-update", "--help")
	if code != ExitOK {
		t.Fatalf("self-update --help exit %d, want ExitOK: %s", code, out)
	}
	if !strings.Contains(out, "self-update") {
		t.Fatalf("help output missing command name: %s", out)
	}
	if !strings.Contains(out, "--check") {
		t.Fatalf("help output missing --check flag: %s", out)
	}
}

// TestSelfUpdateValidationRefusesBeforeNetwork pins the no-network refusal
// surface: bad --version / --channel values fail closed inside update.Resolve
// (update.go:146-156) before any HTTP request, surfacing through run() as
// ExitState. This exercises the RunE path while staying fully offline and
// deterministic.
//
// fail-before rationale: update.go:146 (`if !releaseTagRe.MatchString(version)`)
// rejects a slash-bearing pin, and update.go:155 rejects an unknown channel,
// both BEFORE fetchRelease. selfupdate.go wires --version/--channel straight
// into CheckTarget→Resolve, and run() (root.go:47) maps the uncoded refusal to
// ExitState. Break the wiring (e.g. stop passing pinVersion) and the invalid
// pin would instead try the network / change the exit, failing these asserts.
// "../etc/passwd" can never match the release-tag regex, so no fetch is
// attempted regardless of network availability.
func TestSelfUpdateValidationRefusesBeforeNetwork(t *testing.T) {
	// Invalid --version: rejected at the tag regex, no fetch.
	code, out := runOma(t, "self-update", "--version", "../etc/passwd")
	if code != ExitState {
		t.Fatalf("invalid --version exit %d, want ExitState(%d): %s", code, ExitState, out)
	}
	if !strings.Contains(out, "invalid --version") {
		t.Fatalf("invalid --version output missing reason: %s", out)
	}

	// Unknown --channel: rejected before any list/fetch.
	code, out = runOma(t, "self-update", "--channel", "weird")
	if code != ExitState {
		t.Fatalf("unknown --channel exit %d, want ExitState(%d): %s", code, ExitState, out)
	}
	if !strings.Contains(out, "unknown channel") {
		t.Fatalf("unknown --channel output missing reason: %s", out)
	}
}
