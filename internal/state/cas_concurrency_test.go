package state

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

// TestConcurrentCASExactlyOneWinnerAtRevision pins the in-lock compare-and-swap
// arbiter at state.go:264-266: many goroutines that all target the SAME
// expectedRevision must resolve to EXACTLY ONE winner. The first goroutine to
// take the state lock bumps the revision to R+1 and writes; every later
// goroutine re-loads inside the lock (state.go:260), observes revision R+1 !=
// expected R, and returns ErrConflict (state.go:265). If that in-lock check
// were dropped, the losers would re-acquire the lock and overwrite the winner,
// yielding multiple err==nil results and a final revision > R+1.
func TestConcurrentCASExactlyOneWinnerAtRevision(t *testing.T) {
	s := newStore(t)

	// Establish a known starting revision R for the namespace by writing one
	// field; the file now exists at revision 1.
	if _, err := s.Set("cas/seed", "0", "", false); err != nil {
		t.Fatalf("seed set: %v", err)
	}
	_, _, R, err := s.GetWithRevision("cas/seed", "")
	if err != nil {
		t.Fatal(err)
	}
	if R != 1 {
		t.Fatalf("seed revision = %d, want 1", R)
	}

	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			expected := R // all writers target the SAME expected revision
			field := fmt.Sprintf("w%02d", i)
			<-start // release all goroutines together to maximize contention
			_, errs[i] = s.SetExpected("cas/"+field, field, "", false, &expected)
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	winnerIdx := -1
	for i, e := range errs {
		switch {
		case e == nil:
			winners++
			winnerIdx = i
		case errors.Is(e, ErrConflict):
			// expected loser
		default:
			t.Fatalf("goroutine %d: unexpected error = %v, want nil or ErrConflict", i, e)
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1 (CAS arbiter let %d writers through)", winners, winners)
	}

	// Exactly one increment: the file must be at R+1, no more.
	_, _, rev, err := s.GetWithRevision("cas/seed", "")
	if err != nil {
		t.Fatal(err)
	}
	if rev != R+1 {
		t.Fatalf("revision = %d, want %d (R+1) — extra increments mean multiple writers won", rev, R+1)
	}

	// The persisted file must contain the winner's field with its value.
	winnerField := fmt.Sprintf("w%02d", winnerIdx)
	if v, ok, err := s.Get("cas/"+winnerField, ""); err != nil || !ok || v != winnerField {
		t.Fatalf("winner field %s = %q ok=%v err=%v, want %q", winnerField, v, ok, err, winnerField)
	}
}

// TestCASStaleExpectedRevisionConflicts pins the same CAS check deterministically
// and single-threaded: once the file advances past R, a writer still using the
// stale expectedRevision R must get ErrConflict (state.go:265, also pre-checked
// at state.go:251). If the revision comparison were removed, the stale write
// would silently succeed.
func TestCASStaleExpectedRevisionConflicts(t *testing.T) {
	s := newStore(t)

	if _, err := s.Set("cas/seed", "0", "", false); err != nil {
		t.Fatalf("seed set: %v", err)
	}
	_, _, R, err := s.GetWithRevision("cas/seed", "")
	if err != nil {
		t.Fatal(err)
	}

	// Advance the file to R+1 with a correct expected revision.
	if _, err := s.SetExpected("cas/a", "1", "", false, &R); err != nil {
		t.Fatalf("first expected set: %v", err)
	}
	_, _, rev, err := s.GetWithRevision("cas/seed", "")
	if err != nil {
		t.Fatal(err)
	}
	if rev != R+1 {
		t.Fatalf("revision after advance = %d, want %d", rev, R+1)
	}

	// A second write reusing the now-stale expectedRevision R must conflict.
	stale := R
	if _, err := s.SetExpected("cas/b", "2", "", false, &stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale expected revision: err = %v, want ErrConflict", err)
	}

	// The conflicting write must not have landed and must not have bumped rev.
	if _, ok, err := s.Get("cas/b", ""); err != nil || ok {
		t.Fatalf("stale write must not persist: ok=%v err=%v", ok, err)
	}
	_, _, rev, err = s.GetWithRevision("cas/seed", "")
	if err != nil {
		t.Fatal(err)
	}
	if rev != R+1 {
		t.Fatalf("revision after conflict = %d, want %d (conflict must not write)", rev, R+1)
	}
}
