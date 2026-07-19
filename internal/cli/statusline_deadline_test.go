package cli

import (
	"testing"
	"time"
)

func TestStatuslineSegmentWithinReturnsFastProbe(t *testing.T) {
	want := &wfSegment{Workflow: "ralph"}
	got := statuslineSegmentWithin(make(chan struct{}, 1), time.Second, func() *wfSegment {
		return want
	})
	if got != want {
		t.Fatalf("segment = %#v, want %#v", got, want)
	}
}

func TestStatuslineSegmentWithinBoundsAndDeduplicatesSlowProbe(t *testing.T) {
	slot := make(chan struct{}, 1)
	started := make(chan struct{})
	release := make(chan struct{})

	start := time.Now()
	got := statuslineSegmentWithin(slot, 25*time.Millisecond, func() *wfSegment {
		close(started)
		<-release
		return &wfSegment{Workflow: "relay"}
	})
	if got != nil {
		t.Fatalf("timed-out segment = %#v, want nil", got)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("slow probe returned after %s, want a hard deadline", elapsed)
	}
	select {
	case <-started:
	default:
		t.Fatal("slow probe never started")
	}

	ranSecond := false
	start = time.Now()
	got = statuslineSegmentWithin(slot, time.Second, func() *wfSegment {
		ranSecond = true
		return &wfSegment{Workflow: "interview"}
	})
	if got != nil || ranSecond {
		t.Fatalf("overlapping probe = %#v, ran=%v; want immediate fail-soft nil", got, ranSecond)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("overlapping probe returned after %s, want immediate fail-soft result", elapsed)
	}

	close(release)
	select {
	case slot <- struct{}{}:
		<-slot
	case <-time.After(time.Second):
		t.Fatal("slow probe did not release its in-flight slot")
	}
}
