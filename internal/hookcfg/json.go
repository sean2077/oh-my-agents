package hookcfg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// The ordered token tree: a JSON object as an ordered member list whose
// values stay json.RawMessage (tokens byte-verbatim, never re-encoded).
// Rendering normalizes only inter-token whitespace via json.Indent, so a
// canonical-form document (2-space indent) round-trips byte-identically.

type member struct {
	key string
	raw json.RawMessage
}

type obj []member

func (o obj) find(key string) (json.RawMessage, bool) {
	for _, m := range o {
		if m.key == key {
			return m.raw, true
		}
	}
	return nil, false
}

// set replaces the member in place or appends it at the end.
func (o obj) set(key string, raw json.RawMessage) obj {
	for i := range o {
		if o[i].key == key {
			o[i].raw = raw
			return o
		}
	}
	return append(o, member{key, raw})
}

func (o obj) drop(key string) obj {
	for i := range o {
		if o[i].key == key {
			return append(o[:i], o[i+1:]...)
		}
	}
	return o
}

// parseObj decodes one JSON object preserving member order; values are
// captured raw. Trailing data after the closing brace is rejected.
//
// Duplicate member names are rejected (review 046 blocker 1): the ordered
// tree resolves lookups first-wins while JSON runtimes (encoding/json, jq,
// the agents themselves) resolve last-wins, so tolerating duplicates lets
// oma verify/count a hook tree the runtime never consumes. Comparison
// happens on DECODED names — Token() unescapes, so a plain key and its
// unicode-escaped spelling collide here exactly as they do at runtime.
func parseObj(raw []byte) (obj, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("expected object, got %v", tok)
	}
	var o obj
	seen := map[string]bool{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("expected object key, got %v", keyTok)
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate object key %q (first-wins/last-wins ambiguity; fail-closed)", key)
		}
		seen[key] = true
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		o = append(o, member{key, v})
	}
	if _, err := dec.Token(); err != nil { // consume '}'
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data after JSON object")
	}
	return o, nil
}

// parseArr decodes one JSON array; elements are captured raw.
func parseArr(raw []byte) ([]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, fmt.Errorf("expected array, got %v", tok)
	}
	items := []json.RawMessage{}
	for dec.More() {
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	if _, err := dec.Token(); err != nil { // consume ']'
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data after JSON array")
	}
	return items, nil
}

// renderObj writes an object at the given indent depth: ordered members,
// 2-space indent, values re-indented by json.Indent (tokens untouched).
func renderObj(o obj, indent string) []byte {
	if len(o) == 0 {
		return []byte("{}")
	}
	var buf bytes.Buffer
	buf.WriteString("{\n")
	inner := indent + "  "
	for i, m := range o {
		buf.WriteString(inner)
		key, _ := json.Marshal(m.key)
		buf.Write(key)
		buf.WriteString(": ")
		writeIndented(&buf, m.raw, inner)
		if i < len(o)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString(indent)
	buf.WriteByte('}')
	return buf.Bytes()
}

// renderArr writes an array at the given indent depth.
func renderArr(items []json.RawMessage, indent string) []byte {
	if len(items) == 0 {
		return []byte("[]")
	}
	var buf bytes.Buffer
	buf.WriteString("[\n")
	inner := indent + "  "
	for i, item := range items {
		buf.WriteString(inner)
		writeIndented(&buf, item, inner)
		if i < len(items)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString(indent)
	buf.WriteByte(']')
	return buf.Bytes()
}

// renderObjRaw / renderArrRaw produce a value to re-seat into a parent;
// the parent's render re-indents it to its final depth.
func renderObjRaw(o obj) json.RawMessage                   { return renderObj(o, "") }
func renderArrRaw(items []json.RawMessage) json.RawMessage { return renderArr(items, "") }

// writeIndented appends one raw value re-indented for the given prefix;
// invalid raw bytes are written verbatim (they cannot occur: every raw
// passed parsing first).
func writeIndented(buf *bytes.Buffer, raw json.RawMessage, prefix string) {
	var tmp bytes.Buffer
	if err := json.Indent(&tmp, raw, prefix, "  "); err != nil {
		buf.Write(raw)
		return
	}
	buf.Write(tmp.Bytes())
}
