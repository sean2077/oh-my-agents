package hookcfg

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// canonicalSettings is a claude settings.json already in canonical form
// (2-space indent, trailing newline) with a foreign hook and a scalar
// (the large integer) that float64 re-encoding would mangle.
const canonicalSettings = `{
  "model": "opus",
  "feedbackSurveyState": {
    "lastShownTime": 1750000000000
  },
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "echo user-own-hook"
          }
        ]
      }
    ]
  }
}
`

func claudeEntries(command string) map[string][]json.RawMessage {
	entry := `{"hooks": [{"type": "command", "command": ` + string(mustJSON(command)) + `}]}`
	return map[string][]json.RawMessage{
		"PreToolUse": {json.RawMessage(entry)},
		"Stop":       {json.RawMessage(entry)},
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCanonicalParseRenderRoundTrip(t *testing.T) {
	o, err := parseObj([]byte(canonicalSettings))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := append(renderObj(o, ""), '\n')
	if string(out) != canonicalSettings {
		t.Fatalf("canonical render drifted:\n--- got ---\n%s\n--- want ---\n%s", out, canonicalSettings)
	}
}

func TestInjectThenRemoveRoundTripsCanonicalFileByteIdentically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, canonicalSettings)

	if err := Inject(path, WrapKeySettings, "ralph", claudeEntries("oma ralph tick")); err != nil {
		t.Fatalf("inject: %v", err)
	}
	mid := readFile(t, path)
	if !strings.Contains(mid, `"_oma_asset": "ralph"`) {
		t.Fatalf("marker missing after inject:\n%s", mid)
	}
	// Foreign tokens preserved verbatim: the large integer and user hook.
	for _, tok := range []string{"1750000000000", "echo user-own-hook"} {
		if !strings.Contains(mid, tok) {
			t.Fatalf("foreign token %q lost:\n%s", tok, mid)
		}
	}
	cmds, err := OwnCommands(path, WrapKeySettings, "ralph")
	if err != nil || len(cmds) != 2 {
		t.Fatalf("own commands = %v err=%v, want 2", cmds, err)
	}

	if err := Remove(path, WrapKeySettings, "ralph"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := readFile(t, path); got != canonicalSettings {
		t.Fatalf("remove did not restore canonical bytes:\n--- got ---\n%s\n--- want ---\n%s", got, canonicalSettings)
	}
	if _, err := os.Stat(path + ".oma-bak"); err != nil {
		t.Fatalf("single-generation backup missing: %v", err)
	}
}

func TestInjectIsIdempotentReplaceNotAppend(t *testing.T) {
	// review 044 forward note: install v1, install v2, exactly one marked
	// entry per event carrying the v2 command.
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, canonicalSettings)

	if err := Inject(path, WrapKeySettings, "ralph", claudeEntries("oma ralph v1")); err != nil {
		t.Fatal(err)
	}
	if err := Inject(path, WrapKeySettings, "ralph", claudeEntries("oma ralph v2")); err != nil {
		t.Fatal(err)
	}
	cmds, err := OwnCommands(path, WrapKeySettings, "ralph")
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 2 || cmds[0] != "oma ralph v2" || cmds[1] != "oma ralph v2" {
		t.Fatalf("after reinstall commands = %v, want exactly the v2 pair", cmds)
	}
	if got := readFile(t, path); strings.Contains(got, "oma ralph v1") {
		t.Fatalf("v1 entry still present:\n%s", got)
	}
}

func TestRemoveFiltersOnlyOwnMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	content := `{
  "hooks": {
    "Stop": [
      {
        "command": "user-stop"
      },
      {
        "command": "other-stop",
        "_oma_asset": "other-asset"
      },
      {
        "command": "mine-stop",
        "_oma_asset": "mine"
      }
    ]
  }
}
`
	writeFile(t, path, content)
	if err := Remove(path, WrapKeySettings, "mine"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if strings.Contains(got, "mine-stop") {
		t.Fatalf("own entry not removed:\n%s", got)
	}
	for _, keep := range []string{"user-stop", "other-stop", "other-asset"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("foreign entry %q lost:\n%s", keep, got)
		}
	}
}

func TestCorruptHostFailsClosedWithZeroWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	writeFile(t, path, `{"hooks": {`) // truncated JSON

	err := Inject(path, WrapKeySettings, "x", claudeEntries("cmd"))
	if err == nil {
		t.Fatal("corrupt host must refuse injection")
	}
	if got := readFile(t, path); got != `{"hooks": {` {
		t.Fatalf("corrupt host mutated: %q", got)
	}
	for _, leftover := range []string{path + ".oma-bak", path + ".oma-tmp"} {
		if _, err := os.Lstat(leftover); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("fail-closed path left %s behind", leftover)
		}
	}
}

func TestTrailingGarbageFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, "{} {}\n")
	if err := Inject(path, WrapKeySettings, "x", claudeEntries("cmd")); err == nil {
		t.Fatal("trailing garbage must refuse injection")
	}
}

func TestSymlinkHostRefused(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	writeFile(t, real, "{}\n")
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	err := Inject(link, WrapKeySettings, "x", claudeEntries("cmd"))
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlinked host: err = %v, want symlink refusal", err)
	}
	if got := readFile(t, real); got != "{}\n" {
		t.Fatalf("symlink target mutated: %q", got)
	}
}

func TestCodexShapeMissingFileLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".codex", "hooks.json")
	entries := map[string][]json.RawMessage{
		"Stop": {json.RawMessage(`{"command": "oma ralph tick"}`)},
	}
	if err := Inject(path, WrapKeyNone, "ralph", entries); err != nil {
		t.Fatalf("inject into missing file: %v", err)
	}
	cmds, err := OwnCommands(path, WrapKeyNone, "ralph")
	if err != nil || len(cmds) != 1 || cmds[0] != "oma ralph tick" {
		t.Fatalf("own commands = %v err=%v", cmds, err)
	}
	if _, err := os.Lstat(path + ".oma-bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("no backup may be written when no previous file existed")
	}
	if err := Remove(path, WrapKeyNone, "ralph"); err != nil {
		t.Fatal(err)
	}
	// Host files are never deleted: an emptied codex doc renders as {}.
	if got := readFile(t, path); got != "{}\n" {
		t.Fatalf("emptied codex doc = %q, want {} document", got)
	}
}

func TestNonCanonicalFileRoundTripsSemantically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := `{
    "model":   "opus",
    "limits": {"budget": 1e3},
    "hooks": {"Stop": [{"command": "user-stop", "timeout": 0.50}]}
}`
	writeFile(t, path, original)
	if err := Inject(path, WrapKeySettings, "x", claudeEntries("cmd")); err != nil {
		t.Fatal(err)
	}
	if err := Remove(path, WrapKeySettings, "x"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	// Whitespace is normalized, so bytes differ — but tokens survive
	// verbatim (no float re-encoding) and the document is JSON-equal.
	for _, tok := range []string{"1e3", "0.50", "user-stop"} {
		if !strings.Contains(got, tok) {
			t.Fatalf("token %q re-encoded or lost:\n%s", tok, got)
		}
	}
	var a, b any
	if err := json.Unmarshal([]byte(original), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(got), &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("semantic drift:\n%s\nvs\n%s", original, got)
	}
}

func TestWhitespaceOnlyHostTreatedAsEmptyDoc(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, " \n")
	if err := Inject(path, WrapKeySettings, "x", claudeEntries("cmd")); err != nil {
		t.Fatalf("whitespace-only host: %v", err)
	}
	cmds, err := OwnCommands(path, WrapKeySettings, "x")
	if err != nil || len(cmds) != 2 {
		t.Fatalf("cmds = %v err=%v", cmds, err)
	}
}

func TestInjectCreatedContainersRemovedWithLastEntry(t *testing.T) {
	// A file without a hooks key gains one on inject; remove must drop the
	// emptied event arrays AND the hooks key itself.
	path := filepath.Join(t.TempDir(), "settings.json")
	original := "{\n  \"model\": \"opus\"\n}\n"
	writeFile(t, path, original)
	if err := Inject(path, WrapKeySettings, "x", claudeEntries("cmd")); err != nil {
		t.Fatal(err)
	}
	if err := Remove(path, WrapKeySettings, "x"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != original {
		t.Fatalf("containers not cleaned up:\n--- got ---\n%s--- want ---\n%s", got, original)
	}
}

func TestFragmentValidation(t *testing.T) {
	valid := `{"schema": "oma-hook-fragment/1",
		"claude": {"Stop": [{"hooks": [{"type": "command", "command": "x"}]}]},
		"codex": {"Stop": [{"command": "x"}]}}`
	if _, err := ParseFragment([]byte(valid)); err != nil {
		t.Fatalf("valid fragment rejected: %v", err)
	}
	cases := map[string]string{
		"unknown major":   `{"schema": "oma-hook-fragment/2", "claude": {"Stop": [{"command": "x"}]}}`,
		"no sections":     `{"schema": "oma-hook-fragment/1"}`,
		"empty events":    `{"schema": "oma-hook-fragment/1", "claude": {}}`,
		"empty entries":   `{"schema": "oma-hook-fragment/1", "claude": {"Stop": []}}`,
		"reserved marker": `{"schema": "oma-hook-fragment/1", "claude": {"Stop": [{"command": "x", "_oma_asset": "y"}]}}`,
		"no command":      `{"schema": "oma-hook-fragment/1", "claude": {"Stop": [{"matcher": "Bash"}]}}`,
		"non-object":      `{"schema": "oma-hook-fragment/1", "claude": {"Stop": ["just a string"]}}`,
	}
	for name, doc := range cases {
		if _, err := ParseFragment([]byte(doc)); !errors.Is(err, ErrFragment) {
			t.Errorf("%s: err = %v, want ErrFragment", name, err)
		}
	}
}

func TestOwnCommandsCollectsNestedCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	content := `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "nested-one"
          },
          {
            "type": "command",
            "command": "nested-two"
          }
        ],
        "_oma_asset": "mine"
      }
    ]
  }
}
`
	writeFile(t, path, content)
	cmds, err := OwnCommands(path, WrapKeySettings, "mine")
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 2 || cmds[0] != "nested-one" || cmds[1] != "nested-two" {
		t.Fatalf("cmds = %v", cmds)
	}
}

func TestUnchangedEditSkipsWriteAndBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, canonicalSettings)
	// Removing an asset that was never injected is a byte-level no-op.
	if err := Remove(path, WrapKeySettings, "never-installed"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path + ".oma-bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("no-op edit must not churn a backup")
	}
	if got := readFile(t, path); got != canonicalSettings {
		t.Fatalf("no-op edit mutated file:\n%s", got)
	}
}
