#!/usr/bin/env bash
# Claude Code status line (single line):
#   model · ctx% (tok) · rate↺reset  |  cwd  branch ●dirty  (wt:name) ← from orig · +/-
#
# Reference example shipped with oma. Copy to ~/.claude/statusline-command.sh
# and tweak to taste. Everything above the "oma relay segment" block is generic
# Claude Code status-line rendering; the relay segment at the bottom is the only
# oma-specific part — it calls `oma relay statusline --json` and shows the bound
# pair's "whose turn" state, staying silent (fail-quiet, exit 0) when oma isn't
# installed or no pair is bound. `oma` is resolved from PATH, so there is no
# absolute path to edit.
input=$(cat)

# Never leak to stderr — Claude Code hides the whole status line if the script
# writes anything to stderr. Keep this script fail-silent.
exec 2>/dev/null

# ---- colors ----
C_PATH='\033[0;34m'; C_BRANCH='\033[0;32m'; C_DIRTY='\033[0;31m'
C_WT='\033[0;33m';   C_DIM='\033[0;90m';    C_MODEL='\033[1;36m'
C_ADD='\033[0;32m';  C_DEL='\033[0;31m';    C_RATE='\033[0;35m'; R='\033[0m'
C_SKILL='\033[0;36m'
SEP=" ${C_DIM}\xc2\xb7${R} "          # " · "  (minor)
BAR=" ${C_DIM}|${R} "                  # " | "  (major)

# ---- extract ----
model=$(echo "$input" | jq -r '.model.display_name // "unknown"')
# compact model label: "Opus 4.8 (1M context)" -> "Opus 4.8(1M)"
model="${model/ (/(}"; model="${model/ context)/)}"
effort=$(echo "$input" | jq -r '.effort.level // empty')   # low|medium|high|xhigh|max
cwd=$(echo "$input" | jq -r '.workspace.current_dir // .cwd // empty')
git_worktree=$(echo "$input" | jq -r '.workspace.git_worktree // empty')
wt_branch=$(echo "$input" | jq -r '.worktree.branch // empty')
wt_orig_cwd=$(echo "$input" | jq -r '.worktree.original_cwd // empty')
used_pct=$(echo "$input" | jq -r '.context_window.used_percentage // empty')
total_input=$(echo "$input" | jq -r '.context_window.total_input_tokens // empty')
ctx_size=$(echo "$input" | jq -r '.context_window.context_window_size // empty')
lines_added=$(echo "$input" | jq -r '.cost.total_lines_added // empty')
lines_removed=$(echo "$input" | jq -r '.cost.total_lines_removed // empty')
five_pct=$(echo "$input" | jq -r '.rate_limits.five_hour.used_percentage // empty')
week_pct=$(echo "$input" | jq -r '.rate_limits.seven_day.used_percentage // empty')
five_reset=$(echo "$input" | jq -r '.rate_limits.five_hour.resets_at // empty')
week_reset=$(echo "$input" | jq -r '.rate_limits.seven_day.resets_at // empty')

# ---- helpers ----
humanize() {  # tokens -> 24k  (integer-safe)
    local n="${1%%.*}"
    case "$n" in ''|*[!0-9]*) return;; esac
    if [ "$n" -ge 1000 ]; then printf '%dk' "$(( (n + 500) / 1000 ))"; else printf '%d' "$n"; fi
}
fmt_reset() { [ -n "$1" ] && date -d "@$1" "+$2"; }

# ---- session: model + effort (most important — first) ----
printf "${C_MODEL}%s${R}" "$model"
[ -n "$effort" ] && printf " ${C_SKILL}%s${R}" "$effort"

# ---- context: % + token counts (second) ----
if [ -n "$used_pct" ]; then
    used_int=$(printf '%.0f' "$used_pct")
    if   [ "$used_int" -le 50 ]; then c='\033[0;32m'
    elif [ "$used_int" -le 80 ]; then c='\033[0;33m'
    else                              c='\033[0;31m'; fi
    printf "${SEP}${c}ctx %d%%${R}" "$used_int"
    if [ -n "$total_input" ] && [ -n "$ctx_size" ]; then
        printf "${C_DIM}(%s/%s)${R}" "$(humanize "$total_input")" "$(humanize "$ctx_size")"
    fi
fi

# ---- rate limits: % + reset (5h clock, 7d weekday) ----
rate=""
if [ -n "$five_pct" ]; then
    rate="5h $(printf '%.0f' "$five_pct")%"
    r=$(fmt_reset "$five_reset" '%H:%M'); [ -n "$r" ] && rate="$rate\xe2\x86\xba$r"
fi
if [ -n "$week_pct" ]; then
    [ -n "$rate" ] && rate="$rate "
    rate="${rate}7d $(printf '%.0f' "$week_pct")%"
    r=$(fmt_reset "$week_reset" '%a %H:%M'); [ -n "$r" ] && rate="$rate\xe2\x86\xba$r"
fi
[ -n "$rate" ] && printf "${SEP}${C_RATE}%b${R}" "$rate"

# ---- location ----
if [ -n "$cwd" ]; then
    printf "${BAR}${C_PATH}%s${R}" "$cwd"

    branch="$wt_branch"
    [ -z "$branch" ] && branch=$(cd "$cwd" && git rev-parse --abbrev-ref HEAD)
    if [ -n "$branch" ]; then
        printf "  ${C_BRANCH}\xee\x82\xa0 %s${R}" "$branch"
        dirty=$(cd "$cwd" && git status --porcelain | wc -l | tr -d ' ')
        [ -n "$dirty" ] && [ "$dirty" -gt 0 ] && printf " ${C_DIRTY}\xe2\x97\x8f%s${R}" "$dirty"
    fi
    [ -n "$git_worktree" ] && printf "  ${C_WT}(wt:%s)${R}" "$git_worktree"
    [ -n "$wt_orig_cwd" ] && [ "$wt_orig_cwd" != "$cwd" ] && printf "  ${C_DIM}\xe2\x86\x90 from %s${R}" "$wt_orig_cwd"
fi

# ---- lines changed ----
if [ -n "$lines_added" ] || [ -n "$lines_removed" ]; then
    printf "${SEP}${C_ADD}+%s${R}/${C_DEL}-%s${R}" "${lines_added:-0}" "${lines_removed:-0}"
fi

# ---- oma relay segment (fail-quiet, hard-bounded) ----
#   resolve binding from the project cwd (oma reads process cwd, not stdin);
#   gate on .bound so non-pair windows stay clean (oma prints "relay: —" otherwise)
#   `oma` resolved from PATH — the existence guard keeps the line fail-quiet when
#   it is not installed (no command-not-found spam, segment simply omitted)
oma_bin=$(command -v oma 2>/dev/null)
if [ -n "$oma_bin" ] && [ -x "$oma_bin" ]; then
    relay_json=$( { [ -n "$cwd" ] && cd "$cwd"; } 2>/dev/null; timeout 1 "$oma_bin" relay statusline --json 2>/dev/null )
    if [ "$(printf '%s' "$relay_json" | jq -r '.bound // false')" = "true" ]; then
        relay_seg=$(printf '%s' "$relay_json" | jq -r '.text // empty')
        [ -n "$relay_seg" ] && printf "${SEP}%s" "$relay_seg"
    fi
fi

# Always succeed: Claude Code hides the status line on a non-zero exit code.
exit 0
