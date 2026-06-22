#!/usr/bin/env bash
# eval/score.sh — deterministic scorer for the oma skill-triggering eval.
#
# Given the labeled fixture (expected skill per case) and one arm's predictions
# (the skill that arm actually picked per case), compute and print:
#   - skill-triggering precision / recall / F1   (over cases that SHOULD trigger)
#   - false-trigger rate                          (over decoy cases, expected=none)
#   - a per-case correctness table
#
# Pure POSIX bash + awk (no jq, no python). The fixture is read with a tiny
# field extractor, not a real JSON parser: the fixture lines are flat objects
# with "id", "prompt", and "expected" string fields (see eval/cases/*.jsonl).
# Keep the fixture flat or this extractor will mis-read it.
#
# See eval/README.md for the metric definitions and how to interpret them.
#
# Exit codes: 0 ok, 2 usage error, 3 input/state error.
set -euo pipefail

PROG=${0##*/}

usage() {
  cat <<EOF
Usage: $PROG --cases <fixture.jsonl> --pred <predictions.tsv> [--arm <label>]

Score one arm's skill-triggering predictions against the labeled fixture.

Arguments:
  --cases <file>   Labeled fixture (JSONL/NDJSON), one flat object per line with
                   "id", "prompt", and "expected" fields. "expected":"none"
                   marks a decoy where NO skill should trigger.
  --pred  <file>   Predictions, TSV with two columns: <case-id>\\t<predicted-skill>.
                   Use the literal token "none" when the arm picked no skill.
                   Lines beginning with '#' and blank lines are ignored.
  --arm   <label>  Optional human label for the arm (printed in the header).
  -h, --help       Show this help and exit (0).

Exit codes: 0 ok, 2 usage error, 3 input/state error.
EOF
}

die_usage() { echo "$PROG: $1" >&2; echo "hint: run '$PROG --help'" >&2; exit 2; }
die_state() { echo "$PROG: $1" >&2; exit 3; }

CASES=""
PRED=""
ARM=""

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --cases) [ $# -ge 2 ] || die_usage "--cases needs a value"; CASES=$2; shift 2 ;;
    --pred)  [ $# -ge 2 ] || die_usage "--pred needs a value";  PRED=$2;  shift 2 ;;
    --arm)   [ $# -ge 2 ] || die_usage "--arm needs a value";   ARM=$2;   shift 2 ;;
    --) shift; break ;;
    -*) die_usage "unknown option: $1" ;;
    *)  die_usage "unexpected argument: $1" ;;
  esac
done

[ -n "$CASES" ] || die_usage "missing --cases <fixture.jsonl>"
[ -n "$PRED" ]  || die_usage "missing --pred <predictions.tsv>"
[ -f "$CASES" ] || die_state "fixture not found: $CASES"
[ -f "$PRED" ]  || die_state "predictions not found: $PRED"

# One awk program does extraction + scoring so we stay jq-free and single-pass.
# Pass 1 (FNR==NR): read the fixture, pull "id" and "expected" out of each line.
# Pass 2: read the TSV predictions and join on id.
awk -v arm="$ARM" '
  function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
  # Extract the string value of a flat JSON field: "key": "value"
  function jstr(line, key,    re, m) {
    re = "\"" key "\"[ \t]*:[ \t]*\"[^\"]*\""
    if (match(line, re)) {
      m = substr(line, RSTART, RLENGTH)
      sub(/^.*:[ \t]*"/, "", m)
      sub(/"$/, "", m)
      return m
    }
    return ""
  }

  # ---- Pass 1: fixture ----
  FNR == NR {
    line = $0
    if (line ~ /^[ \t]*$/) next          # blank
    if (line ~ /^[ \t]*#/) next          # comment
    id = jstr(line, "id")
    want = jstr(line, "expected")         # NB: exp is a mawk builtin, so use want
    if (id == "") next                    # not a case line
    if (want == "") want = "none"
    expected[id] = want
    order[++ncases] = id
    if (want == "none") n_decoy++; else n_trigger++
    next
  }

  # ---- Pass 2: predictions ----
  {
    pl = $0
    if (pl ~ /^[ \t]*$/) next
    if (pl ~ /^[ \t]*#/) next
    # TSV: id <TAB> skill  (tolerate extra leading/trailing whitespace)
    pid = $1
    psk = $2
    pid = trim(pid)
    psk = trim(psk)
    if (psk == "") psk = "none"
    pred[pid] = psk
    seenpred[pid] = 1
    npred++
  }

  END {
    # Validate join: every prediction must reference a known case.
    missing = 0
    for (pid in seenpred) {
      if (!(pid in expected)) {
        printf("score: prediction references unknown case id: %s\n", pid) > "/dev/stderr"
        missing++
      }
    }
    if (missing > 0) { exit 3 }

    # Counters.
    #   TP: expected!=none AND pred==expected
    #   "predicted a skill" denominator for precision = expected==none OR pred!=none, restricted to non-none preds
    tp = 0; pred_nonnone = 0; covered = 0; ft = 0; both = 0
    for (i = 1; i <= ncases; i++) {
      id = order[i]
      want = expected[id]
      p = (id in pred) ? pred[id] : "none"
      covered++
      if (p != "none") pred_nonnone++
      if (want != "none" && p == want) tp++
      if (want == "none" && p != "none") ft++   # false trigger on a decoy
    }

    # Precision = correct skill picks / all skill picks (non-none predictions).
    # Recall    = correct skill picks / all cases that should have triggered.
    prec = (pred_nonnone > 0) ? tp / pred_nonnone : 0
    rec  = (n_trigger   > 0) ? tp / n_trigger     : 0
    f1   = (prec + rec > 0) ? 2 * prec * rec / (prec + rec) : 0
    ftr  = (n_decoy     > 0) ? ft / n_decoy        : 0

    # ---- Report ----
    hdr = (arm != "") ? ("arm: " arm) : "arm: (unlabeled)"
    print hdr
    printf("cases=%d  should-trigger=%d  decoys=%d  predictions=%d\n",
           ncases, n_trigger, n_decoy, npred)
    print  "-----------------------------------------------------------------"
    printf("%-8s %-22s %-22s %s\n", "id", "expected", "predicted", "result")
    print  "-----------------------------------------------------------------"
    for (i = 1; i <= ncases; i++) {
      id = order[i]
      want = expected[id]
      p = (id in pred) ? pred[id] : "none"
      if (want == "none" && p == "none")      res = "ok (no-trigger)"
      else if (want == "none" && p != "none") res = "FALSE-TRIGGER"
      else if (p == want)                     res = "ok (hit)"
      else if (p == "none")                   res = "MISS (no-trigger)"
      else                                    res = "WRONG-SKILL"
      printf("%-8s %-22s %-22s %s\n", id, want, p, res)
    }
    print  "-----------------------------------------------------------------"
    printf("triggering precision : %6.3f   (TP %d / picks %d)\n", prec, tp, pred_nonnone)
    printf("triggering recall    : %6.3f   (TP %d / should %d)\n", rec, tp, n_trigger)
    printf("triggering F1        : %6.3f\n", f1)
    printf("false-trigger rate   : %6.3f   (FT %d / decoys %d)\n", ftr, ft, n_decoy)
    print  "-----------------------------------------------------------------"
  }
' "$CASES" "$PRED"
