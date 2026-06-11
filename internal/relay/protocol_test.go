package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// clock is a shared simulated clock: every now() call advances a small
// step so poll deadlines progress; heartbeat mtimes live in the same
// simulated domain via Chtimes.
type clock struct {
	mu   sync.Mutex
	t    time.Time
	step time.Duration
}

func newClock() *clock {
	return &clock{t: time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC), step: 100 * time.Millisecond}
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(c.step)
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// testLedger builds a ledger for one author over a shared root + clock.
func testLedger(t *testing.T, root, author string, ck *clock) *Ledger {
	t.Helper()
	id, err := makeIdentity(author, "session-"+author)
	if err != nil {
		t.Fatal(err)
	}
	l := NewLedger(root, id)
	l.Now = ck.now
	l.Getenv = func(string) string { return "" }
	l.PollInterval = time.Millisecond
	return l
}

// initRoot creates an initialized v2 root inside a fake git checkout.
func initRoot(t *testing.T, ck *clock) (root string, top string) {
	t.Helper()
	top = t.TempDir()
	if err := os.MkdirAll(filepath.Join(top, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(top, ".oma", "relay")
	l := testLedger(t, root, "claude", ck)
	if err := l.Init(false); err != nil {
		t.Fatal(err)
	}
	return root, top
}

func mustPair(t *testing.T, l *Ledger, topic string) *Session {
	t.Helper()
	s, err := l.NewPair(topic, "", "oh-my-agents", false)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustPublish(t *testing.T, l *Ledger, slug, kind, body, prompt string) string {
	t.Helper()
	draft, err := l.CreateDraft(slug, kind, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	formal, err := l.Publish(draft, PublishInput{Body: body, Prompt: prompt}, false)
	if err != nil {
		t.Fatal(err)
	}
	return formal
}

// --- matrix 1 + 10: v1 trees are refused, zero writes ---

func TestV1TreeRefusedZeroWrites(t *testing.T) {
	ck := newClock()
	for name, build := range map[string]func(string){
		"_relay dir": func(root string) {
			if err := os.MkdirAll(filepath.Join(root, "_relay"), 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"v1 session.json": func(root string) {
			dir := filepath.Join(root, "20260101-old")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "session.json"), []byte(`{"schema_version": 2, "topic": "old"}`), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	} {
		root := filepath.Join(t.TempDir(), ".shared-like")
		build(root)
		before := treeDigest(t, root)
		l := testLedger(t, root, "claude", ck)
		if err := l.Init(false); !errors.Is(err, ErrRelay) {
			t.Fatalf("%s: Init err = %v, want v1 refusal", name, err)
		}
		if err := l.Open(); !errors.Is(err, ErrRelay) {
			t.Fatalf("%s: Open err = %v, want v1 refusal", name, err)
		}
		if _, err := l.CreateDraft("", "note", nil, nil, false); err == nil {
			t.Fatalf("%s: draft into v1 tree must refuse", name)
		}
		if got := treeDigest(t, root); got != before {
			t.Fatalf("%s: v1 tree was modified", name)
		}
	}
}

// --- matrix 2 + 11: unknown sentinel schema major refused ---

func TestUnknownSentinelMajorRefused(t *testing.T) {
	ck := newClock()
	root := filepath.Join(t.TempDir(), "relay")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, sentinelName), []byte(`{"schema":"oma-relay/3","created":"2026-06-11T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	l := testLedger(t, root, "claude", ck)
	if err := l.Open(); err == nil || !strings.Contains(err.Error(), "oma-relay/3") {
		t.Fatalf("err = %v, want unknown-major refusal", err)
	}
	// Non-empty foreign dir without sentinel also refuses.
	root2 := filepath.Join(t.TempDir(), "foreign")
	if err := os.MkdirAll(filepath.Join(root2, "stuff"), 0o700); err != nil {
		t.Fatal(err)
	}
	l2 := testLedger(t, root2, "claude", ck)
	if err := l2.Init(false); err == nil || !strings.Contains(err.Error(), "no v2 sentinel") {
		t.Fatalf("foreign dir: err = %v", err)
	}
}

// --- matrix 3: append-only ---

func TestAppendOnlyFormalNeverOverwritten(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "topic")
	formal := mustPublish(t, claude, s.Pair, "plan", "the plan body", "review it")

	// A hand-crafted draft at the SAME seq with different content must
	// fail closed, never overwrite the published artifact.
	draftPath := filepath.Join(claude.PairDir(s.Pair), ".draft", filepath.Base(formal))
	fm := &Frontmatter{Schema: Schema, Seq: 1, Author: "claude", Peer: "codex", Kind: "plan",
		Status: "ready", Created: ck.now(), TouchedPaths: []string{}, PromptForNext: "different"}
	if err := os.WriteFile(draftPath, Render(fm, "DIFFERENT body"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := claude.Publish(draftPath, PublishInput{}, false); err == nil || !strings.Contains(err.Error(), "differs from the draft render") {
		t.Fatalf("err = %v, want divergence fail-closed", err)
	}
	raw, _ := os.ReadFile(formal)
	if !strings.Contains(string(raw), "the plan body") {
		t.Fatal("published artifact was modified")
	}
}

// --- matrix 4: sidecar/hash verification ---

func TestTamperedArtifactWithheld(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "topic")
	formal := mustPublish(t, claude, s.Pair, "plan", "honest body", "ok")

	raw, _ := os.ReadFile(formal)
	if err := os.WriteFile(formal, append(raw, []byte("tampered\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadArtifact(formal); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("err = %v, want hash fail-closed", err)
	}
	st, err := claude.Status(s.Pair, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Residue) == 0 {
		t.Fatal("status must report the corrupt artifact")
	}
}

// --- matrix 5: identity ambiguity ---

func TestIdentityAmbiguityFailsClosed(t *testing.T) {
	env := map[string]string{"CLAUDE_CODE_SESSION_ID": "a", "CODEX_THREAD_ID": "b"}
	getenv := func(k string) string { return env[k] }
	if _, err := ResolveIdentity(getenv); err == nil || !strings.Contains(err.Error(), "OMA_RELAY_AUTHOR") {
		t.Fatalf("ambiguous identity: err = %v", err)
	}
	env["OMA_RELAY_AUTHOR"] = "codex"
	id, err := ResolveIdentity(getenv)
	if err != nil || id.Author != "codex" {
		t.Fatalf("arbited identity = %+v err=%v", id, err)
	}
	if _, err := ResolveIdentity(func(string) string { return "" }); err == nil {
		t.Fatal("no signal must refuse")
	}
}

// --- matrix 6: stale intent detection, cleanup, seq holes ---

func TestStaleIntentCleanupAndSeqHoles(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, claude, s.Pair, "plan", "body", "review")

	// codex creates an intent (draft+seq) then goes silent.
	if _, err := codex.CreateDraft(s.Pair, "review", nil, nil, false); err != nil {
		t.Fatal(err)
	}
	ck.advance(20 * time.Minute)

	res, err := claude.Wait(s.Pair, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != WaitStaleIntent {
		t.Fatalf("wait code = %d (%s), want 11", res.Code, res.Reason)
	}

	actions, err := claude.CleanStale(s.Pair, false)
	if err != nil || len(actions) == 0 {
		t.Fatalf("clean-stale actions=%v err=%v", actions, err)
	}
	if n := claude.ReservationCount(s.Pair); n != 0 {
		t.Fatalf("reservations after clean = %d", n)
	}
	// codex publishes after cleanup: a seq hole (002 cleaned) is legal.
	formal := mustPublish(t, codex, s.Pair, "review", "late but alive", "next")
	if _, _, err := ReadArtifact(formal); err != nil {
		t.Fatalf("read across seq hole: %v", err)
	}
}

// --- matrix 7: concurrent drafts get distinct seqs ---

func TestConcurrentDraftsDistinctSeqs(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)
	codex := testLedger(t, root, "codex", ck)
	s := mustPair(t, claude, "topic")
	if _, err := codex.Join(s.Pair, false); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	paths := make([]string, 4)
	for i, l := range []*Ledger{claude, codex, claude, codex} {
		wg.Add(1)
		go func(i int, l *Ledger) {
			defer wg.Done()
			p, err := l.CreateDraft(s.Pair, "note", nil, nil, false)
			if err != nil {
				t.Errorf("draft %d: %v", i, err)
				return
			}
			paths[i] = p
		}(i, l)
	}
	wg.Wait()
	seen := map[int]bool{}
	for _, p := range paths {
		seq, _, _, ok := ParseArtifactName(filepath.Base(p))
		if !ok || seen[seq] {
			t.Fatalf("duplicate or invalid seq in %v", paths)
		}
		seen[seq] = true
	}
}

// --- matrix 8: interrupted publish converges at every step ---

func TestInterruptedPublishConvergesAtEveryStep(t *testing.T) {
	steps := []string{"draft-rendered", "formal-renamed", "sha256-written", "ready-written"}
	for _, killAfter := range steps {
		t.Run(killAfter, func(t *testing.T) {
			ck := newClock()
			root, _ := initRoot(t, ck)
			claude := testLedger(t, root, "claude", ck)
			codex := testLedger(t, root, "codex", ck)
			s := mustPair(t, claude, "topic")
			if _, err := codex.Join(s.Pair, false); err != nil {
				t.Fatal(err)
			}
			draft, err := claude.CreateDraft(s.Pair, "plan", nil, nil, false)
			if err != nil {
				t.Fatal(err)
			}
			boom := errors.New("simulated kill")
			claude.StepHook = func(step string) error {
				if step == killAfter {
					return boom
				}
				return nil
			}
			if _, err := claude.Publish(draft, PublishInput{Body: "body", Prompt: "next"}, false); !errors.Is(err, boom) {
				t.Fatalf("interrupted publish err = %v", err)
			}
			// The durable intent survives every interruption.
			if _, err := os.Stat(draft); err != nil {
				t.Fatalf("draft must survive the kill after %s: %v", killAfter, err)
			}
			// Peers never see a partial publish (ready is the only criterion)…
			formal := filepath.Join(claude.PairDir(s.Pair), ArtifactName(1, "claude", "plan"))
			if killAfter != "ready-written" {
				if _, _, err := ReadArtifact(formal); err == nil {
					t.Fatal("partial publish must not be readable")
				}
			} else {
				// …and a kill AFTER .ready means the artifact IS published:
				// the peer's wait gets exit 0, leftovers are doctor warnings.
				res, err := codex.Wait(s.Pair, time.Hour)
				if err != nil || res.Code != WaitNewArtifact {
					t.Fatalf("ready-beats-stale: wait = %+v err=%v", res, err)
				}
			}
			// Re-running the same publish converges.
			claude.StepHook = nil
			formalPath, err := claude.Publish(draft, PublishInput{Body: "body", Prompt: "next"}, false)
			if err != nil {
				t.Fatalf("converging publish after %s: %v", killAfter, err)
			}
			if _, _, err := ReadArtifact(formalPath); err != nil {
				t.Fatalf("converged artifact unreadable: %v", err)
			}
			if _, err := os.Stat(draft); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("draft must be cleaned after a completed publish")
			}
			if n := claude.ReservationCount(s.Pair); n != 0 {
				t.Fatalf("seq reservation leaked: %d", n)
			}
		})
	}
}

// --- matrix 9: v1 coexistence ---

func TestV1CoexistenceUntouched(t *testing.T) {
	ck := newClock()
	root, top := initRoot(t, ck)
	v1 := filepath.Join(top, ".shared")
	if err := os.MkdirAll(filepath.Join(v1, "_relay"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v1, "_relay", "state.json"), []byte(`{"v":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	before := treeDigest(t, v1)

	claude := testLedger(t, root, "claude", ck)
	s := mustPair(t, claude, "topic")
	mustPublish(t, claude, s.Pair, "plan", "body", "next")
	st, err := claude.Status(s.Pair, 0)
	if err != nil {
		t.Fatal(err)
	}
	if st.LegacyV1 != v1 {
		t.Fatalf("legacy v1 not reported: %q", st.LegacyV1)
	}
	if got := treeDigest(t, v1); got != before {
		t.Fatal("v1 tree was touched")
	}
}

// --- matrix 12: pair binding resolution ---

func TestBindingResolution(t *testing.T) {
	ck := newClock()
	root, _ := initRoot(t, ck)
	claude := testLedger(t, root, "claude", ck)

	// No active pair: refuse with guidance, zero binding written.
	if _, err := claude.ResolvePair("", true); err == nil || !strings.Contains(err.Error(), "pair new") {
		t.Fatalf("no pair: err = %v", err)
	}
	s1 := mustPair(t, claude, "alpha")
	// NewPair binds the creator; drop the binding to test auto-adopt.
	if err := os.Remove(claude.bindingPath()); err != nil {
		t.Fatal(err)
	}
	got, err := claude.ResolvePair("", true)
	if err != nil || got.Pair != s1.Pair {
		t.Fatalf("single-active auto-adopt: %+v err=%v", got, err)
	}
	if _, err := os.Stat(claude.bindingPath()); err != nil {
		t.Fatal("auto-adopt must persist the binding")
	}

	// Second active pair: explicit --pair works; without it the binding
	// pins; with the binding removed it is ambiguous, zero writes.
	ck2 := newClock()
	ck2.advance(48 * time.Hour) // distinct date prefix for the second slug
	claude2 := testLedger(t, root, "claude", ck2)
	s2, err := claude2.NewPair("beta", "", "p", false)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := claude.ResolvePair(s2.Pair, true); err != nil || got.Pair != s2.Pair {
		t.Fatalf("explicit --pair: %+v err=%v", got, err)
	}
	if err := os.Remove(claude.bindingPath()); err != nil {
		t.Fatal(err)
	}
	_, err = claude.ResolvePair("", true)
	if err == nil || !strings.Contains(err.Error(), s1.Pair) || !strings.Contains(err.Error(), s2.Pair) {
		t.Fatalf("ambiguous: err = %v, want candidate list", err)
	}
	if _, statErr := os.Stat(claude.bindingPath()); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("ambiguous resolution must not write a binding")
	}

	// Binding at a terminal pair: refused with rebind guidance.
	if _, err := claude.Join(s1.Pair, false); err != nil {
		t.Fatal(err)
	}
	if err := claude.Close(s1.Pair, "approve", "done", false); err != nil {
		t.Fatal(err)
	}
	if _, err := claude.ResolvePair("", true); err == nil || !strings.Contains(err.Error(), "rebind") {
		t.Fatalf("terminal binding: err = %v", err)
	}
}

// treeDigest hashes a directory tree (paths + contents) for
// untouched-bytes assertions.
func treeDigest(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s|%v|%d\n", path, info.IsDir(), info.Size())
		if !info.IsDir() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			h.Write(raw)
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}
