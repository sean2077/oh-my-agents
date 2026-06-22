// Package jsonmerge preserves unknown top-level JSON fields across a
// load/modify/save cycle, honoring the schemas.md minor-additive contract:
// "readers tolerate unknown fields (preserved and passed through, never
// dropped)". Without it, an older binary that reads a newer file and writes it
// back silently strips fields it does not know.
package jsonmerge

import (
	"encoding/json"
	"reflect"
	"strings"
)

// Extra returns the top-level keys in data that are not declared fields of v
// (matched by json tag, including embedded structs). It returns nil when there
// are none, so a same-version load carries no state.
func Extra(data []byte, v any) (map[string]json.RawMessage, error) {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	known := knownKeys(v)
	for k := range all {
		if known[k] {
			delete(all, k)
		}
	}
	if len(all) == 0 {
		return nil, nil
	}
	return all, nil
}

// Marshal renders v (indented, two-space) and merges any extra keys it does
// not already produce. With no extras the output is byte-identical to
// json.MarshalIndent(v, "", "  "), so existing files and golden tests are
// unaffected until an unknown field actually has to be preserved. Declared
// fields always win over a same-named extra.
func Marshal(v any, extra map[string]json.RawMessage) ([]byte, error) {
	base, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(extra) == 0 {
		return base, nil
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(base, &merged); err != nil {
		return nil, err
	}
	for k, val := range extra {
		if _, ok := merged[k]; !ok {
			merged[k] = val
		}
	}
	return json.MarshalIndent(merged, "", "  ")
}

// knownKeys collects the JSON object keys v serializes to (following struct
// tags, descending into embedded structs).
func knownKeys(v any) map[string]bool {
	out := map[string]bool{}
	collectKeys(reflect.TypeOf(v), out)
	return out
}

func collectKeys(t reflect.Type, out map[string]bool) {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported field: never serialized
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if f.Anonymous && name == "" {
			collectKeys(f.Type, out) // embedded struct: its fields are top-level
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = true
	}
}
