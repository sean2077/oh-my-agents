package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sean2077/oh-my-agents/internal/ralph"
	"github.com/sean2077/oh-my-agents/internal/relay"
	"github.com/sean2077/oh-my-agents/internal/state"
	"github.com/spf13/cobra"
)

// unifiedStatuslineSchema versions the --json shape.
const unifiedStatuslineSchema = "oma-statusline/1"

// The host invokes statusline frequently, so one stalled filesystem probe must
// neither block the bar nor accumulate a goroutine on every --watch repaint.
// One-shot commands exit after the deadline; watch mode admits at most one
// still-running probe and renders idle until it returns.
const unifiedStatuslineDeadline = time.Second

var unifiedStatuslineProbeSlot = make(chan struct{}, 1)

// wfSegment is one core workflow's compact statusline contribution. Only an
// ACTIVE (in-flight, non-terminal) workflow produces a segment; the renderer
// shows the single most-recently-active one — "the workflow you are in" — each
// carrying the leading `oma` marker.
type wfSegment struct {
	Workflow string                  `json:"workflow"` // relay | ralph | interview | autopilot
	Body     string                  `json:"text"`     // uncolored body (also used for --json/minimal)
	Updated  time.Time               `json:"updated"`  // last-activity, for precedence
	render   func(color bool) string `json:"-"`        // styled body (workflow-specific)
}

// newStatuslineCmd is the unified top-level statusline: one compact line for
// whichever core workflow the current session is in (relay / ralph / interview
// / autopilot). It supersedes `oma relay statusline` — a richer status-line
// script can still call `oma statusline --json` and filter on .active. Like the
// relay line before it, it is PURE-READ and FAIL-SOFT: any setup problem (no
// git root, unresolved session, uninitialized state) degrades to a minimal
// line and exit 0 — a statusline must never error into the host's status bar.
func newStatuslineCmd() *cobra.Command {
	var rootFlag, pairOverride, preset string
	var asJSON, watch, noColor, activeOnly bool
	var interval int

	cmd := &cobra.Command{
		Use:   "statusline",
		Short: "Compact one-line state of the active core workflow (relay/ralph/interview/autopilot; pure-read, fail-soft)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			if watch {
				return watchUnifiedStatusline(cmd, rootFlag, pairOverride, preset, noColor, time.Duration(interval)*time.Second)
			}
			seg := boundedActiveSegment(rootFlag, pairOverride, preset)
			if asJSON {
				return printJSON(cmd, unifiedJSON(seg))
			}
			if activeOnly && seg == nil {
				return nil
			}
			color := !noColor && os.Getenv("NO_COLOR") == ""
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderUnified(seg, color))
			return nil
		}),
	}
	cmd.Flags().StringVar(&rootFlag, "root", "", "relay ledger root (default: resolved from the git checkout)")
	cmd.Flags().StringVar(&pairOverride, "pair", "", "relay pair slug override")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	cmd.Flags().BoolVar(&watch, "watch", false, "repaint a live line until Ctrl-C")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable ANSI styling")
	cmd.Flags().BoolVar(&activeOnly, "active-only", false, "emit no text when no workflow is active")
	cmd.Flags().IntVar(&interval, "interval", 2, "watch repaint interval in seconds")
	cmd.Flags().StringVar(&preset, "preset", "", "relay verbosity: minimal|focused|full (default focused)")
	cmd.MarkFlagsMutuallyExclusive("json", "active-only")
	cmd.MarkFlagsMutuallyExclusive("watch", "active-only")
	return cmd
}

func boundedActiveSegment(rootFlag, pairOverride, preset string) *wfSegment {
	return statuslineSegmentWithin(unifiedStatuslineProbeSlot, unifiedStatuslineDeadline, func() *wfSegment {
		return activeSegment(rootFlag, pairOverride, preset)
	})
}

func statuslineSegmentWithin(slot chan struct{}, deadline time.Duration, probe func() *wfSegment) *wfSegment {
	select {
	case slot <- struct{}{}:
	default:
		return nil
	}

	result := make(chan *wfSegment, 1)
	go func() {
		defer func() { <-slot }()
		result <- probe()
	}()

	timer := time.NewTimer(deadline)
	defer timer.Stop()
	select {
	case seg := <-result:
		return seg
	case <-timer.C:
		return nil
	}
}

// activeSegment probes every core workflow and returns the most-recently-active
// one (nil when none is in flight). Each probe is independently fail-soft.
func activeSegment(rootFlag, pairOverride, preset string) *wfSegment {
	var segs []*wfSegment
	for _, s := range []*wfSegment{
		relaySegment(rootFlag, pairOverride, preset),
		ralphSegment(),
		interviewSegment(),
		autopilotSegment(),
	} {
		if s != nil {
			segs = append(segs, s)
		}
	}
	if len(segs) == 0 {
		return nil
	}
	// Most-recently-updated wins ("the workflow you're currently in"); a stable
	// workflow order breaks exact ties deterministically.
	sort.SliceStable(segs, func(i, j int) bool {
		if !segs[i].Updated.Equal(segs[j].Updated) {
			return segs[i].Updated.After(segs[j].Updated)
		}
		return wfRank(segs[i].Workflow) < wfRank(segs[j].Workflow)
	})
	return segs[0]
}

func renderUnified(seg *wfSegment, color bool) string {
	// `oma` is a colored source tag so any host (status bar or terminal) sees at a
	// glance that the line is oma's. ":" binds the tag tightly to the workflow
	// name (compact: `oma:ralph …`); idle keeps the looser " · idle".
	tag := sgr("oma", "1;35", color)
	if seg == nil {
		return tag + " · idle"
	}
	return tag + ":" + seg.render(color)
}

func unifiedJSON(seg *wfSegment) any {
	doc := map[string]any{"schema": unifiedStatuslineSchema, "active": seg != nil}
	if seg != nil {
		doc["workflow"] = seg.Workflow
		doc["text"] = seg.Body
		doc["updated"] = seg.Updated
	}
	return doc
}

// relaySegment reuses the relay status engine (binding-scoped, deadline-bounded)
// and its rich preset rendering.
func relaySegment(rootFlag, pairOverride, preset string) *wfSegment {
	l, err := relayLedger(rootFlag, false)
	if err != nil {
		return nil
	}
	st := l.Statusline(pairOverride)
	if !st.Bound {
		return nil
	}
	return &wfSegment{
		Workflow: "relay",
		Body:     "relay " + st.Text,
		Updated:  fileMtime(filepath.Join(l.PairDir(st.Pair), "session.json")),
		render:   func(c bool) string { return "relay " + relay.RenderPreset(st, preset, c) },
	}
}

// ralphSegment shows a RUNNING loop's round/phase (terminal loops are not "what
// you're in").
func ralphSegment() *wfSegment {
	eng, err := ralphEngine()
	if err != nil {
		return nil
	}
	s, err := eng.Resolve("")
	if err != nil || s.Terminal() {
		return nil
	}
	body := fmt.Sprintf("r%d/%d %s", s.Round, s.MaxRounds, s.Phase)
	if s.KeepPolicy == ralph.KeepScoreImprovement && s.BestScore != nil {
		body += fmt.Sprintf(" best=%g", *s.BestScore)
	}
	glyph, code, loud := ralphStyle(s.Phase)
	return &wfSegment{
		Workflow: "ralph",
		Body:     "ralph " + body,
		Updated:  s.Updated,
		render:   func(c bool) string { return "ralph " + sgr(glyph, code, c) + " " + paintLoud(body, code, loud, c) },
	}
}

// interviewSegment shows a non-terminal interview's phase and the ambiguity vs
// threshold (the gate signal the agent reasons against).
func interviewSegment() *wfSegment {
	eng, err := interviewEngine()
	if err != nil {
		return nil
	}
	s, err := eng.Resolve("")
	if err != nil || s.Terminal() {
		return nil
	}
	body := fmt.Sprintf("%s amb %.2f/%.2f", s.Phase, s.CurrentAmbiguity, s.Threshold)
	glyph, code := "◆", "1;36"
	loud := false
	if s.CurrentAmbiguity > s.Threshold { // gate not yet passable
		glyph, code, loud = "◧", "1;35", false
	}
	return &wfSegment{
		Workflow: "interview",
		Body:     "interview " + body,
		Updated:  s.Updated,
		render:   func(c bool) string { return "interview " + sgr(glyph, code, c) + " " + paintLoud(body, code, loud, c) },
	}
}

// autopilotSegment reads the session-scoped autopilot phase from oma state
// (assets/skills/autopilot writes `autopilot/phase`); a missing key or a `done`
// phase is not active.
func autopilotSegment() *wfSegment {
	root := findProjectRoot()
	if root == "" {
		return nil
	}
	key, err := workflowScope().StateKey("autopilot/phase")
	if err != nil {
		return nil
	}
	phase, ok, gerr := state.New(root).Get(key, "")
	if gerr != nil || !ok || phase == "" || phase == "done" {
		return nil
	}
	ns, _, _ := strings.Cut(key, "/")
	return &wfSegment{
		Workflow: "autopilot",
		Body:     "autopilot " + phase,
		Updated:  fileMtime(filepath.Join(root, ".oma", "state", ns+".json")),
		render:   func(c bool) string { return "autopilot " + sgr("▸", "1;34", c) + " " + phase },
	}
}

func watchUnifiedStatusline(cmd *cobra.Command, rootFlag, pairOverride, preset string, noColor bool, interval time.Duration) error {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	color := !noColor && os.Getenv("NO_COLOR") == ""
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	out := cmd.OutOrStdout()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	paint := func() {
		line := renderUnified(boundedActiveSegment(rootFlag, pairOverride, preset), color)
		if color {
			// In-place repaint only when styling is on; under --no-color /
			// NO_COLOR (or a piped, non-TTY sink) emit clean newline-terminated
			// lines instead of raw erase-line escapes.
			_, _ = fmt.Fprintf(out, "\r\x1b[2K%s", line)
		} else {
			_, _ = fmt.Fprintln(out, line)
		}
	}
	paint()
	for {
		select {
		case <-ctx.Done():
			if color {
				_, _ = fmt.Fprintln(out)
			}
			return nil
		case <-ticker.C:
			paint()
		}
	}
}

// --- styling helpers (single-width BMP glyphs; column math stays correct) ---

func sgr(text, code string, color bool) string {
	if color && code != "" {
		return "\x1b[" + code + "m" + text + "\x1b[0m"
	}
	return text
}

func paintLoud(text, code string, loud, color bool) string {
	if loud {
		return sgr(text, code, color)
	}
	return text
}

func ralphStyle(phase string) (glyph, code string, loud bool) {
	switch phase {
	case ralph.PhaseRunning:
		return "▸", "1;32", false // bold green — looping
	case ralph.PhasePassed:
		return "✓", "2", false // dim — done
	case ralph.PhasePlateaued, ralph.PhaseStalled:
		return "!", "1;33", true // bold yellow — change strategy
	case ralph.PhaseExhausted, ralph.PhaseAborted:
		return "✗", "2", false
	default:
		return "·", "2", false
	}
}

func fileMtime(path string) time.Time {
	if info, err := os.Stat(path); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

func wfRank(w string) int {
	switch w {
	case "relay":
		return 0
	case "ralph":
		return 1
	case "interview":
		return 2
	case "autopilot":
		return 3
	}
	return 9
}
