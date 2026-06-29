package schemaver

import "testing"

func TestSchemaMajorParser(t *testing.T) {
	cases := []struct {
		schema string
		major  int
		ok     bool
	}{
		{"oma-asset/1", 1, true},
		{"oma-asset/2", 2, true},
		{"oma-asset/-1", 0, false},
		{"oma-asset/0", 0, false},
		{"oma-asset/+1", 0, false},
		{"oma-asset/01", 0, false},
		{"oma-asset/1x", 0, false},
		{"oma-asset/", 0, false},
		{"oma-asset", 0, false},
		{"oma-registry/1", 0, false}, // wrong domain for want=oma-asset
	}
	for _, tc := range cases {
		major, ok := Major(tc.schema, "oma-asset")
		if ok != tc.ok || (ok && major != tc.major) {
			t.Errorf("Major(%q) = (%d,%v), want (%d,%v)", tc.schema, major, ok, tc.major, tc.ok)
		}
	}
}
