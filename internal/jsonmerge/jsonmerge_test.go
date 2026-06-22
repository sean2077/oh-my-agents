package jsonmerge

import (
	"encoding/json"
	"testing"
)

type sample struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Skip   string `json:"-"`
}

func TestExtraAndMarshalRoundTrip(t *testing.T) {
	data := []byte(`{"schema":"x/1","name":"a","future":42,"nested":{"k":1}}`)
	var s sample
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	extra, err := Extra(data, &s)
	if err != nil {
		t.Fatal(err)
	}
	if len(extra) != 2 {
		t.Fatalf("extra = %v, want future + nested", extra)
	}

	s.Name = "b" // mutate a known field
	out, err := Marshal(&s, extra)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]json.RawMessage
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	if string(back["name"]) != `"b"` {
		t.Errorf("known field not updated: %s", back["name"])
	}
	if string(back["future"]) != `42` {
		t.Errorf("unknown scalar dropped: %s", back["future"])
	}
	if _, ok := back["nested"]; !ok {
		t.Error("unknown object dropped")
	}
}

func TestMarshalNoExtraIsByteIdenticalToStruct(t *testing.T) {
	s := sample{Schema: "x/1", Name: "a"}
	got, err := Marshal(&s, nil)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.MarshalIndent(&s, "", "  ")
	if string(got) != string(want) {
		t.Fatalf("Marshal with no extra must equal struct marshal:\n got %s\nwant %s", got, want)
	}
}
