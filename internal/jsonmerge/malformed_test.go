package jsonmerge

import (
	"encoding/json"
	"testing"
)

// malformedSample mirrors the struct conventions in jsonmerge_test.go.
type malformedSample struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

// TestExtraRejectsMalformedJSON pins the fail-closed contract of Extra: any
// clearly-malformed JSON input must return a non-nil error, never a silently
// empty (nil) result.
//
// fail-before rationale: Extra ingests raw JSON at jsonmerge.go:19
//
//	if err := json.Unmarshal(data, &all); err != nil { return nil, err }
//
// If that error guard (jsonmerge.go:19-21) were dropped (the json.Unmarshal
// error ignored), malformed input would leave `all` as a nil/empty map, fall
// through to `len(all) == 0`, and return (nil, nil) -- silently reporting "no
// extra keys" for input that could not be parsed at all. A caller would then
// drop every preserved field instead of failing closed. These cases lock in
// that malformed input is rejected with an error.
func TestExtraRejectsMalformedJSON(t *testing.T) {
	cases := map[string]string{
		"unterminated_object":   `{invalid`,
		"missing_value":         `{"a":}`,
		"not_json":              `not json`,
		"trailing_garbage":      `{"schema":"x/1"} trailing`,
		"unterminated_array":    `[1,2`,
		"bare_unquoted_word":    `schema`,
		"unterminated_string":   `{"schema":"x`,
		"non_object_top_number": `42`,
		"non_object_top_array":  `[1,2,3]`,
		"empty_input":           ``,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			var s malformedSample
			extra, err := Extra([]byte(in), &s)
			if err == nil {
				t.Fatalf("Extra(%q) = (%v, nil); want a non-nil error (fail-closed). "+
					"A nil error on malformed input means fields would be silently dropped.", in, extra)
			}
			if extra != nil {
				t.Errorf("Extra(%q) returned extra=%v alongside error; want nil result on malformed input", in, extra)
			}
		})
	}
}

// TestExtraAcceptsWellFormedControl is the control: a well-formed object must
// succeed. Without it, TestExtraRejectsMalformedJSON could pass against a
// hypothetical always-failing Extra, so this distinguishes "rejects malformed"
// from "rejects everything".
func TestExtraAcceptsWellFormedControl(t *testing.T) {
	var s malformedSample
	extra, err := Extra([]byte(`{"schema":"x/1","name":"a","future":42}`), &s)
	if err != nil {
		t.Fatalf("Extra on well-formed input returned error: %v", err)
	}
	if len(extra) != 1 {
		t.Fatalf("Extra = %v; want exactly the unknown key {future}", extra)
	}
	if string(extra["future"]) != `42` {
		t.Errorf("extra[future] = %s; want 42", extra["future"])
	}
}

// TestMarshalAcceptsWellFormedExtraControl is the Marshal counterpart control:
// Marshal re-unmarshals its own MarshalIndent output (jsonmerge.go:48), not
// arbitrary caller JSON, so it cannot be fed malformed top-level input through
// the public signature. The reachable failure-closed path is a malformed
// json.RawMessage value supplied in the extra map. This control pins that a
// well-formed extra value round-trips successfully (so the rejection test below
// is not vacuously passing against an always-failing Marshal).
func TestMarshalAcceptsWellFormedExtraControl(t *testing.T) {
	s := malformedSample{Schema: "x/1", Name: "a"}
	out, err := Marshal(&s, map[string]json.RawMessage{"future": json.RawMessage(`42`)})
	if err != nil {
		t.Fatalf("Marshal with well-formed extra returned error: %v", err)
	}
	var back map[string]json.RawMessage
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("Marshal output is not valid JSON: %v", err)
	}
	if string(back["future"]) != `42` {
		t.Errorf("preserved extra future = %s; want 42", back["future"])
	}
}

// TestMarshalRejectsMalformedExtraValue pins that a malformed json.RawMessage
// in the extra map fails closed: Marshal re-encodes the merged map via
// json.MarshalIndent, which rejects an invalid RawMessage with a non-nil error
// rather than emitting corrupt output. This guards the merge path
// (jsonmerge.go:51-56) from silently producing invalid JSON.
func TestMarshalRejectsMalformedExtraValue(t *testing.T) {
	s := malformedSample{Schema: "x/1", Name: "a"}
	out, err := Marshal(&s, map[string]json.RawMessage{"future": json.RawMessage(`{invalid`)})
	if err == nil {
		t.Fatalf("Marshal(extra with malformed RawMessage) = (%q, nil); want a non-nil error (fail-closed)", out)
	}
}
