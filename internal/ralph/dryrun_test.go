package ralph

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dirDigest(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		_, _ = fmt.Fprintf(h, "%s|%v|%d\n", path, info.IsDir(), info.Size())
		if !info.IsDir() {
			raw, _ := os.ReadFile(path)
			h.Write(raw)
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

func TestDryRunMutatorsValidateButNeverWrite(t *testing.T) {
	// review 060 blocker 1.
	e := testEngine(t)
	mustStartLoop(t, e, 3, 3)
	before := dirDigest(t, e.Dir)

	if st, v, err := e.Next("r1", true); err != nil || !v.Continue || !v.Mutated || st.Round != 1 {
		t.Fatalf("dry-run next: %+v err=%v", v, err)
	}
	if _, v, err := e.RecordCheck("r1", 1, nil, "sig", true); err != nil || !v.Mutated {
		t.Fatalf("dry-run check: %+v err=%v", v, err)
	}
	if _, err := e.Abort("r1", true); err != nil {
		t.Fatal(err)
	}
	if s, err := e.Start("fresh", StartOpts{Goal: "another goal", MaxRounds: 5, StallWindow: 2}, true); err != nil || s.ID != "fresh" {
		t.Fatalf("dry-run start: %+v err=%v", s, err)
	}
	if got := dirDigest(t, e.Dir); got != before {
		t.Fatal("dry-run mutators wrote state (review 060 blocker 1)")
	}
	// Persisted state is untouched: round still 0.
	s, err := e.Load("r1")
	if err != nil || s.Round != 0 || s.Phase != PhaseRunning {
		t.Fatalf("persisted state = %+v err=%v", s, err)
	}
}

func TestDryRunStillFailsValidation(t *testing.T) {
	e := testEngine(t)
	// no state: next/check/abort refuse under dry-run too
	if _, _, err := e.Next("", true); !errors.Is(err, ErrRalph) {
		t.Fatalf("dry-run next with no loop: %v", err)
	}
	if _, _, err := e.RecordCheck("missing", 1, nil, "x", true); !errors.Is(err, ErrRalph) {
		t.Fatalf("dry-run check missing id: %v", err)
	}
	if _, err := e.Start("bad*id", StartOpts{Goal: "goal", MaxRounds: 10, StallWindow: 3}, true); !errors.Is(err, ErrRalph) {
		t.Fatalf("dry-run start bad id: %v", err)
	}
	if _, err := e.Start("ok", StartOpts{Goal: "", MaxRounds: 10, StallWindow: 3}, true); !errors.Is(err, ErrRalph) {
		t.Fatalf("dry-run start blank goal: %v", err)
	}
}

func TestResolveFailsClosedOnCorruptCandidate(t *testing.T) {
	// review 060 blocker 2.
	e := testEngine(t)
	mustStartLoop(t, e, 10, 3)
	bad := filepath.Join(e.Dir, "ralph-bad.json")
	if err := os.WriteFile(bad, []byte(`{"schema":"oma-ralph/9","id":"bad","phase":"running"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Resolve(""); err == nil || !strings.Contains(err.Error(), "ralph-bad.json") {
		t.Fatalf("corrupt candidate: err = %v, want fail-closed naming it", err)
	}
	if _, err := e.Resolve("r1"); err != nil {
		t.Fatalf("explicit id must bypass: %v", err)
	}
}
