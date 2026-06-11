package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/spf13/cobra"
)

func TestVersionJSONContract(t *testing.T) {
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"version", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version --json: %v", err)
	}
	var out struct {
		Schema     string            `json:"schema"`
		Schemas    map[string]string `json:"schemas"`
		Algorithms map[string]string `json:"algorithms"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, buf.String())
	}
	if out.Schema != "oma-cli/1" {
		t.Fatalf("schema = %q, want oma-cli/1", out.Schema)
	}
	if out.Schemas["relay"] != "oma-relay/2" {
		t.Fatalf("relay schema = %q, want oma-relay/2", out.Schemas["relay"])
	}
	if out.Schemas["config"] != "oma-config/1" {
		t.Fatalf("config schema = %q, want oma-config/1", out.Schemas["config"])
	}
	if _, mixed := out.Schemas["budget_algo"]; mixed {
		t.Fatal("schemas registry must stay schema-only")
	}
	if out.Algorithms["budget"] != "approx-b4/1" {
		t.Fatalf("budget algorithm = %q, want approx-b4/1", out.Algorithms["budget"])
	}
}

// testRoot builds a root with synthetic commands exercising each error path.
func testRoot() *cobra.Command {
	root := newRootCmd()
	root.AddCommand(&cobra.Command{
		Use:  "boom-uncoded",
		RunE: run(func(*cobra.Command, []string) error { return errors.New("disk on fire") }),
	})
	root.AddCommand(&cobra.Command{
		Use:  "boom-gate",
		RunE: run(func(*cobra.Command, []string) error { return Errf(ExitGate, "ambiguity above threshold") }),
	})
	return root
}

func TestExecuteExitCodeMapping(t *testing.T) {
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"version"}, ExitOK},
		{[]string{"definitely-not-a-command"}, ExitUsage},
		{[]string{"version", "--definitely-not-a-flag"}, ExitUsage},
		{[]string{"boom-uncoded"}, ExitState},
		{[]string{"boom-gate"}, ExitGate},
	}
	for _, tc := range cases {
		root := testRoot()
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs(tc.args)
		if got := executeWith(root, io.Discard); got != tc.want {
			t.Errorf("args %v: exit = %d, want %d", tc.args, got, tc.want)
		}
	}
}
