package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sean2077/oh-my-agents/internal/relay"
	"github.com/spf13/cobra"
)

// relayLedger resolves root + identity and opens the ledger (every
// subcommand except init requires an initialized v2 root).
func relayLedger(ledgerRoot string, open bool) (*relay.Ledger, error) {
	root := ledgerRoot
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		if root, err = relay.DefaultRoot(cwd); err != nil {
			return nil, err
		}
	}
	id, err := relay.ResolveIdentity(os.Getenv)
	if err != nil {
		return nil, err
	}
	l := relay.NewLedger(root, id)
	if open {
		if err := l.Open(); err != nil {
			return nil, err
		}
	}
	return l, nil
}

func newRelayCmd() *cobra.Command {
	var ledgerRoot string
	cmd := &cobra.Command{Use: "relay", Short: "Pair ledger for cross-agent delivery (relay v2)"}
	cmd.PersistentFlags().StringVar(&ledgerRoot, "ledger-root", "", "override the ledger root (default: <git toplevel>/.oma/relay)")

	cmd.AddCommand(
		newRelayInitCmd(&ledgerRoot),
		newRelayPreflightCmd(&ledgerRoot),
		newRelayStatuslineCmd(&ledgerRoot),
		newRelayHookDispatchCmd(&ledgerRoot),
		newRelayPairCmd(&ledgerRoot),
		newRelayDraftCmd(&ledgerRoot),
		newRelayPublishCmd(&ledgerRoot),
		newRelayWaitCmd(&ledgerRoot),
		newRelayStatusCmd(&ledgerRoot),
		newRelayCloseCmd(&ledgerRoot),
	)
	return cmd
}

func newRelayPreflightCmd(rootFlag *string) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Diagnose identity, ledger root/sentinel, binding, and filesystem properties",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			rep := relay.Preflight(relay.PreflightInput{
				ExplicitRoot: *rootFlag,
				Cwd:          cwd,
				ProjectRoot:  findProjectRoot(),
				Getenv:       os.Getenv,
			})
			out := cmd.OutOrStdout()
			if asJSON {
				if err := printJSON(cmd, rep); err != nil {
					return err
				}
			} else {
				_, _ = fmt.Fprintf(out, "relay preflight :: %s\n\n", rep.Root)
				for _, c := range rep.Checks {
					_, _ = fmt.Fprintf(out, "  [%-4s] %-22s %s\n", c.Level, c.Name, c.Message)
				}
				_, _ = fmt.Fprintf(out, "\nsummary: %d pass, %d warn, %d fail\n", rep.Pass, rep.Warn, rep.Fail)
			}
			// 0 all pass / 1 warn / 3 fail-stop (addendum 087; usage stays 2 via cobra).
			switch rep.ExitCode() {
			case 3:
				return Errf(ExitState, "preflight found %d failing check(s)", rep.Fail)
			case 1:
				return Errf(ExitWarn, "preflight found %d warning(s)", rep.Warn)
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func newRelayInitCmd(rootFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create the v2 ledger root and sentinel (idempotent)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			l, err := relayLedger(*rootFlag, false)
			if err != nil {
				return err
			}
			if err := l.Init(DryRun()); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), l.Root)
			return nil
		}),
	}
}

func newRelayPairCmd(rootFlag *string) *cobra.Command {
	pair := &cobra.Command{Use: "pair", Short: "Create, bind and inspect pairs"}

	var newPeer string
	var newJSON bool
	pairNew := &cobra.Command{
		Use:   "new <topic-slug>",
		Short: "Create a pair (creator becomes lead) and bind to it",
		Args:  cobra.ExactArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			l, err := relayLedger(*rootFlag, true)
			if err != nil {
				return err
			}
			s, err := l.NewPair(args[0], newPeer, projectName(), DryRun())
			if err != nil {
				return err
			}
			if newJSON {
				return printJSON(cmd, s)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\npeer joins with: oma relay pair join %s\n", s.Pair, s.Pair)
			return nil
		}),
	}
	pairNew.Flags().StringVar(&newPeer, "peer", "", "peer participant (default: the claude/codex counterpart)")
	pairNew.Flags().BoolVar(&newJSON, "json", false, "machine-readable output")

	var ensureJSON bool
	ensure := &cobra.Command{
		Use:   "ensure",
		Short: "Resolve this session's pair binding (auto-bind a single active pair)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			l, err := relayLedger(*rootFlag, true)
			if err != nil {
				return err
			}
			s, err := l.ResolvePair("", !DryRun())
			if err != nil {
				return err
			}
			if ensureJSON {
				return printJSON(cmd, map[string]any{"action": "use", "pair": s.Pair, "session": s})
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), s.Pair)
			return nil
		}),
	}
	ensure.Flags().BoolVar(&ensureJSON, "json", false, "machine-readable output")

	var joinJSON bool
	join := &cobra.Command{
		Use:   "join <slug>",
		Short: "Bind this session to an existing active pair",
		Args:  cobra.ExactArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			l, err := relayLedger(*rootFlag, true)
			if err != nil {
				return err
			}
			s, err := l.Join(args[0], DryRun())
			if err != nil {
				return err
			}
			if joinJSON {
				return printJSON(cmd, s)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), s.Pair)
			return nil
		}),
	}
	join.Flags().BoolVar(&joinJSON, "json", false, "machine-readable output")

	var showPair string
	var showJSON bool
	show := &cobra.Command{
		Use:   "show",
		Short: "Show the resolved pair, peer and the peer's join command",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			l, err := relayLedger(*rootFlag, true)
			if err != nil {
				return err
			}
			s, err := l.ResolvePair(showPair, false)
			if err != nil {
				return err
			}
			if showJSON {
				return printJSON(cmd, s)
			}
			peer, _ := s.Peer(l.Identity.Author)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "pair: %s\nlead: %s\npeer: %s (joins with: oma relay pair join %s)\n", s.Pair, s.Roles["lead"], peer, s.Pair)
			return nil
		}),
	}
	show.Flags().StringVar(&showPair, "pair", "", "pair slug (default: resolved binding)")
	show.Flags().BoolVar(&showJSON, "json", false, "machine-readable output")

	var listJSON bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List pairs in the active root",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			l, err := relayLedger(*rootFlag, true)
			if err != nil {
				return err
			}
			slugs, err := l.AllPairs()
			if err != nil {
				return err
			}
			type row struct {
				Pair, Status, Lead string
				Participants       []string
			}
			var rows []row
			for _, slug := range slugs {
				s, err := l.LoadSession(slug)
				if err != nil {
					rows = append(rows, row{Pair: slug, Status: "corrupt: " + err.Error()})
					continue
				}
				rows = append(rows, row{Pair: s.Pair, Status: s.Status, Lead: s.Roles["lead"], Participants: s.Participants})
			}
			if listJSON {
				return printJSON(cmd, rows)
			}
			for _, r := range rows {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\tlead=%s\t%s\n", r.Pair, r.Status, r.Lead, strings.Join(r.Participants, "+"))
			}
			return nil
		}),
	}
	list.Flags().BoolVar(&listJSON, "json", false, "machine-readable output")

	var leadPair string
	setLead := &cobra.Command{
		Use:   "set-lead <participant>",
		Short: "Persist a user-confirmed lead swap into session.json.roles.lead",
		Args:  cobra.ExactArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			l, err := relayLedger(*rootFlag, true)
			if err != nil {
				return err
			}
			s, err := l.ResolvePair(leadPair, false)
			if err != nil {
				return err
			}
			s, err = l.SetLead(s.Pair, args[0], DryRun())
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s lead=%s\n", s.Pair, s.Roles["lead"])
			return nil
		}),
	}
	setLead.Flags().StringVar(&leadPair, "pair", "", "pair slug (default: resolved binding)")

	pair.AddCommand(pairNew, ensure, join, show, list, setLead)
	return pair
}

func newRelayDraftCmd(rootFlag *string) *cobra.Command {
	var kind, pairSlug string
	var inReplyTo, corrects int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "draft",
		Short: "Reserve a sequence number and create the draft (durable publish intent)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			l, err := relayLedger(*rootFlag, true)
			if err != nil {
				return err
			}
			var irt, cor *int
			if cmd.Flags().Changed("in-reply-to") {
				irt = &inReplyTo
			}
			if cmd.Flags().Changed("corrects") {
				cor = &corrects
			}
			path, err := l.CreateDraft(pairSlug, kind, irt, cor, DryRun())
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd, map[string]string{"draft": path})
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		}),
	}
	cmd.Flags().StringVar(&kind, "kind", "", "artifact kind: "+strings.Join(relay.Kinds, "|"))
	cmd.Flags().IntVar(&inReplyTo, "in-reply-to", 0, "seq this artifact replies to")
	cmd.Flags().IntVar(&corrects, "corrects", 0, "seq this correction corrects (required for kind=correction)")
	cmd.Flags().StringVar(&pairSlug, "pair", "", "pair slug (default: resolved binding)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	_ = cmd.MarkFlagRequired("kind")
	return cmd
}

func newRelayPublishCmd(rootFlag *string) *cobra.Command {
	var bodyFile, promptFile, status, pairSlug, verdict string
	var touched []string
	var reviewTarget int
	cmd := &cobra.Command{
		Use:   "publish <draft>",
		Short: "Fill the draft and run the publish transaction (render → sidecars → ready)",
		Args:  cobra.ExactArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			l, err := relayLedger(*rootFlag, true)
			if err != nil {
				return err
			}
			_ = pairSlug // the draft path itself names the pair; flag accepted for symmetry
			in := relay.PublishInput{Touched: touched, Status: status, Verdict: verdict}
			if cmd.Flags().Changed("review-target") {
				in.ReviewTarget = &reviewTarget
			}
			if bodyFile != "" {
				raw, err := os.ReadFile(bodyFile)
				if err != nil {
					return fmt.Errorf("read body file: %w", err)
				}
				in.Body = string(raw)
			}
			if promptFile != "" {
				raw, err := os.ReadFile(promptFile)
				if err != nil {
					return fmt.Errorf("read prompt file: %w", err)
				}
				in.Prompt = string(raw)
			}
			formal, err := l.Publish(args[0], in, DryRun())
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), formal)
			return nil
		}),
	}
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "file holding the artifact body")
	cmd.Flags().StringVar(&promptFile, "prompt-file", "", "file holding prompt_for_next")
	cmd.Flags().StringArrayVar(&touched, "touched", nil, "repo-relative path changed by this turn (repeatable)")
	cmd.Flags().StringVar(&status, "status", "", "artifact status (default ready; timed_out pauses for @user)")
	cmd.Flags().StringVar(&pairSlug, "pair", "", "pair slug (informational; the draft path names the pair)")
	cmd.Flags().StringVar(&verdict, "verdict", "", "kind:review verdict: "+strings.Join(relay.Verdicts, "|"))
	cmd.Flags().IntVar(&reviewTarget, "review-target", 0, "kind:review: seq this review judges (default: the draft's --in-reply-to)")
	return cmd
}

func newRelayWaitCmd(rootFlag *string) *cobra.Command {
	var timeoutSec int
	var pairSlug string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "wait",
		Short: "Block until the peer publishes, the pair ends, or the timeout elapses",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			l, err := relayLedger(*rootFlag, true)
			if err != nil {
				return err
			}
			res, err := l.Wait(pairSlug, time.Duration(timeoutSec)*time.Second)
			if err != nil {
				return err
			}
			if asJSON {
				if err := printJSON(cmd, res); err != nil {
					return err
				}
			} else if res.ArtifactPath != "" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), res.ArtifactPath)
			}
			if res.Code != relay.WaitNewArtifact {
				return Errf(res.Code, "%s", res.Reason)
			}
			return nil
		}),
	}
	cmd.Flags().IntVar(&timeoutSec, "timeout", 3600, "wait window in seconds")
	cmd.Flags().StringVar(&pairSlug, "pair", "", "pair slug (default: resolved binding)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func newRelayStatusCmd(rootFlag *string) *cobra.Command {
	var last int
	var pairSlug string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Diagnostic view: latest artifact, heartbeats, drafts, residue",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			l, err := relayLedger(*rootFlag, true)
			if err != nil {
				return err
			}
			st, err := l.Status(pairSlug, last)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd, st)
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "pair: %s (%s)\n", st.Pair, st.Session.Status)
			if st.Latest != nil {
				_, _ = fmt.Fprintf(out, "latest: %03d %s %s (%s)\n", st.Latest.Seq, st.Latest.Author, st.Latest.Kind, st.Latest.Status)
			}
			_, _ = fmt.Fprintf(out, "next seq: %03d\n", st.NextSeq)
			for _, w := range st.Residue {
				_, _ = fmt.Fprintf(out, "residue: %s\n", w)
			}
			if st.LegacyV1 != "" {
				_, _ = fmt.Fprintf(out, "legacy v1 ledger at %s (archival; oma never reads or writes it)\n", st.LegacyV1)
			}
			return nil
		}),
	}
	cmd.Flags().IntVar(&last, "last", 5, "number of recent artifacts in the report")
	cmd.Flags().StringVar(&pairSlug, "pair", "", "pair slug (default: resolved binding)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func newRelayCloseCmd(rootFlag *string) *cobra.Command {
	var outcome, reason, pairSlug string
	cmd := &cobra.Command{
		Use:   "close",
		Short: "End the pair (writes terminal state, archives the directory)",
		Args:  cobra.NoArgs,
		RunE: run(func(cmd *cobra.Command, _ []string) error {
			l, err := relayLedger(*rootFlag, true)
			if err != nil {
				return err
			}
			s, err := l.ResolvePair(pairSlug, false)
			if err != nil {
				return err
			}
			// R4: an unsatisfied quality gate is a gate miss (exit 4);
			// corrupt/tampered relay state stays exit 3.
			if cerr := l.Close(s.Pair, outcome, reason, DryRun()); cerr != nil {
				if errors.Is(cerr, relay.ErrGate) {
					return Errf(ExitGate, "%v", cerr)
				}
				return cerr
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&outcome, "outcome", "", "approve|reject|abandon")
	cmd.Flags().StringVar(&reason, "reason", "", "what concluded")
	cmd.Flags().StringVar(&pairSlug, "pair", "", "pair slug (default: resolved binding)")
	_ = cmd.MarkFlagRequired("outcome")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

// projectName labels session.json.project from the checkout directory.
func projectName() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	if root, err := relay.DefaultRoot(cwd); err == nil {
		// DefaultRoot = <top>/.oma/relay
		return filepath.Base(filepath.Dir(filepath.Dir(root)))
	}
	return filepath.Base(cwd)
}

func printJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
