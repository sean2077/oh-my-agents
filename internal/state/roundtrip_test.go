package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestStatePreservesUnknownFields pins the schemas.md minor-additive contract:
// a reader that does not know a newer field must preserve it on write.
func TestStatePreservesUnknownFields(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".oma", "state")
	writeStateFixture(t, dir, "ns.json",
		`{"schema":"oma-state/1","namespace":"ns","revision":1,"data":{"a":"1"},"updated":"2026-06-20T00:00:00Z","future_field":"keep me","future_obj":{"x":1}}`)

	st := New(root)
	st.Now = func() time.Time { return time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC) }
	if _, err := st.Set("ns/b", "2", "", false); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "ns.json"))
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["future_field"] != "keep me" {
		t.Errorf("unknown scalar field dropped on write: %v", obj["future_field"])
	}
	if obj["future_obj"] == nil {
		t.Errorf("unknown object field dropped on write")
	}
	data, _ := obj["data"].(map[string]any)
	if data["a"] != "1" || data["b"] != "2" {
		t.Errorf("known data not preserved/updated: %v", data)
	}
}
