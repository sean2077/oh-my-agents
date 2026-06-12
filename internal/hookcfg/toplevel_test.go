package hookcfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTopLevelSetGetDeletePreservesBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	// Canonical-form host with a foreign key and a large-int scalar.
	original := `{
  "model": "opus",
  "feedbackSurveyState": {
    "lastShownTime": 1750000000000
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	// Set a new top-level key.
	if err := SetTopLevel(path, "statusLine", json.RawMessage(`{"type": "command", "command": "x"}`)); err != nil {
		t.Fatal(err)
	}
	v, found, err := GetTopLevel(path, "statusLine")
	if err != nil || !found || !strings.Contains(string(v), `"command"`) {
		t.Fatalf("get statusLine = %s found=%v err=%v", v, found, err)
	}
	// Foreign tokens preserved verbatim.
	mid, _ := os.ReadFile(path)
	for _, tok := range []string{"1750000000000", `"model": "opus"`} {
		if !strings.Contains(string(mid), tok) {
			t.Fatalf("foreign token %q lost:\n%s", tok, mid)
		}
	}
	// Delete restores byte-identical canonical form.
	if err := DeleteTopLevel(path, "statusLine"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != original {
		t.Fatalf("delete did not restore canonical bytes:\n--- got ---\n%s\n--- want ---\n%s", got, original)
	}
}

func TestTopLevelMissingFileAndKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if _, found, err := GetTopLevel(path, "statusLine"); err != nil || found {
		t.Fatalf("missing file: found=%v err=%v", found, err)
	}
	if err := DeleteTopLevel(path, "statusLine"); err != nil {
		t.Fatalf("delete on missing file must be a no-op: %v", err)
	}
	// Set on a missing file creates it.
	if err := SetTopLevel(path, "k", json.RawMessage(`"v"`)); err != nil {
		t.Fatal(err)
	}
	if v, found, _ := GetTopLevel(path, "k"); !found || string(v) != `"v"` {
		t.Fatalf("set-created file: %s found=%v", v, found)
	}
}

func TestTopLevelCorruptHostFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"k":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetTopLevel(path, "statusLine", json.RawMessage(`{}`)); err == nil {
		t.Fatal("corrupt host must fail closed")
	}
	if got, _ := os.ReadFile(path); string(got) != `{"k":` {
		t.Fatalf("corrupt host mutated: %q", got)
	}
}
