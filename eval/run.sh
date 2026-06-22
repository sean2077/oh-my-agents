#!/usr/bin/env bash
# eval/run.sh — run the oma skill-triggering eval for one arm.
#
# This is a thin front-end over eval/score.sh: it resolves the fixture, sanity-
# checks the predictions file, and prints the scored table. It does NOT run any
# live agent — producing a predictions file is a human/CI step described below
# and in eval/README.md. The deterministic parts (joining + scoring) are the
# only things automated here, on purpose: see eval/README.md "Automated vs.
# manual".
#
# Exit codes: 0 ok, 2 usage error, 3 input/state error (delegated from score.sh).
set -euo pipefail

PROG=${0##*/}
# Resolve this script's own directory (CDPATH unset so 'cd' never echoes a path).
HERE=$(unset CDPATH; cd -- "$(dirname -- "$0")" && pwd)
DEFAULT_CASES="$HERE/cases/triggering.jsonl"
SCORE="$HERE/score.sh"

usage() {
  cat <<EOF
Usage: $PROG <predictions.tsv> [--cases <fixture.jsonl>] [--arm <label>]

Score one arm's skill-triggering predictions against the labeled fixture.

Arguments:
  <predictions.tsv>   Required. TSV with two columns per line:
                          <case-id><TAB><predicted-skill>
                      Use the literal token "none" when the arm picked no skill.
                      '#'-comment and blank lines are ignored.
  --cases <file>      Fixture to score against
                      (default: $DEFAULT_CASES).
  --arm <label>       Human label for the arm, printed in the header
                      (default: inferred from the predictions filename).
  -h, --help          Show this help and exit (0).

Producing a predictions file (the manual step):
  1. Pick an arm (A: plain agent, no oma / B: OMC-style always-resident skills /
     C: oma on-demand). See eval/README.md for what each arm means.
  2. For each prompt in the fixture, present it to that arm's agent and record
     which skill it actually triggered (or "none").
  3. Write one '<id><TAB><skill>' line per case. A worked example lives at
     eval/results/example-arm-c.tsv.

Examples:
  bash $PROG eval/results/example-arm-c.tsv
  bash $PROG eval/results/example-arm-c.tsv --arm "C: oma on-demand"

Exit codes: 0 ok, 2 usage error, 3 input/state error.
EOF
}

die_usage() { echo "$PROG: $1" >&2; echo "hint: run '$PROG --help'" >&2; exit 2; }
die_state() { echo "$PROG: $1" >&2; exit 3; }

PRED=""
CASES="$DEFAULT_CASES"
ARM=""

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --cases) [ $# -ge 2 ] || die_usage "--cases needs a value"; CASES=$2; shift 2 ;;
    --arm)   [ $# -ge 2 ] || die_usage "--arm needs a value";   ARM=$2;   shift 2 ;;
    --) shift; break ;;
    -*) die_usage "unknown option: $1" ;;
    *)
      [ -z "$PRED" ] || die_usage "unexpected extra argument: $1"
      PRED=$1; shift ;;
  esac
done

[ -n "$PRED" ] || { usage >&2; exit 2; }
[ -f "$PRED" ]  || die_state "predictions file not found: $PRED"
[ -f "$CASES" ] || die_state "fixture not found: $CASES"
[ -f "$SCORE" ] || die_state "scorer not found: $SCORE"

# Infer an arm label from the filename when none was given (cosmetic only).
if [ -z "$ARM" ]; then
  base=${PRED##*/}
  ARM=${base%.tsv}
fi

exec bash "$SCORE" --cases "$CASES" --pred "$PRED" --arm "$ARM"
