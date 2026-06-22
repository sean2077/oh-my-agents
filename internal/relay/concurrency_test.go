package relay

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestConcurrentProcessDraftsProduceDistinctArtifacts races two real OS
// processes (claude and codex), each drafting + publishing once. The pair lock
// and O_EXCL seq reservation must hand them distinct seqs and produce two
// clean, sidecar-verified artifacts — no torn write, no seq collision. This is
// the real-process complement to the in-goroutine TestConcurrentDraftsDistinctSeqs.
func TestConcurrentProcessDraftsProduceDistinctArtifacts(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "race")
	codex := testLedger(t, root, "codex", ck)
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}

	start := filepath.Join(t.TempDir(), "start")
	spawn := func(author string) (*exec.Cmd, *bytes.Buffer) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRelayPublishHelperProcess$")
		cmd.Env = append(os.Environ(),
			"OMA_RELAY_HELPER=1",
			"OMA_RELAY_ROOT="+root,
			"OMA_RELAY_PAIR="+s.Pair,
			"OMA_RELAY_AUTHOR_NAME="+author,
			"OMA_RELAY_START="+start,
		)
		out := &bytes.Buffer{}
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Start(); err != nil {
			t.Fatalf("start %s helper: %v", author, err)
		}
		return cmd, out
	}
	cCmd, cOut := spawn("claude")
	xCmd, xOut := spawn("codex")
	if err := os.WriteFile(start, []byte("go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cCmd.Wait(); err != nil {
		t.Fatalf("claude helper: %v\n%s", err, cOut.String())
	}
	if err := xCmd.Wait(); err != nil {
		t.Fatalf("codex helper: %v\n%s", err, xOut.String())
	}

	arts, err := claude.readyArtifacts(s.Pair)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 2 {
		t.Fatalf("ready artifacts = %d, want 2: %+v", len(arts), arts)
	}
	if arts[0].Seq == arts[1].Seq {
		t.Fatalf("seq collision: %d == %d", arts[0].Seq, arts[1].Seq)
	}
	authors := map[string]bool{arts[0].Author: true, arts[1].Author: true}
	if !authors["claude"] || !authors["codex"] {
		t.Fatalf("expected one artifact per author, got %v", authors)
	}
}

func TestRelayPublishHelperProcess(t *testing.T) {
	if os.Getenv("OMA_RELAY_HELPER") != "1" {
		return
	}
	deadline := time.Now().Add(5 * time.Second)
	for startPath := os.Getenv("OMA_RELAY_START"); ; {
		if _, err := os.Stat(startPath); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "timed out waiting for start file")
			os.Exit(2)
		}
		time.Sleep(5 * time.Millisecond)
	}
	author := os.Getenv("OMA_RELAY_AUTHOR_NAME")
	id, err := makeIdentity(author, "session-"+author)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	l := NewLedger(os.Getenv("OMA_RELAY_ROOT"), id)
	pair := os.Getenv("OMA_RELAY_PAIR")
	draft, err := l.CreateDraft(pair, "note", nil, nil, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if _, err := l.Publish(draft, PublishInput{Body: author + " checking in", Prompt: "your turn"}, false); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}
