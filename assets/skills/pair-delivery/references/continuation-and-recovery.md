# Pair Delivery Continuation and Recovery

Load this reference only when a peer reply is not immediately available and the
host must wait or resume later, hook wiring or trust is unclear, a relay wait
exits, or status reports stale drafts/reservations. Do not load it while an
available peer artifact can be processed through the normal sequence in
`SKILL.md`.

## Continue silently

Use the host-appropriate continuation path with no progress chatter. For Codex,
the Stop hook is the main self-continuation path when it is trusted.
`oma relay wait` is the Claude/background hold path and the Codex fallback when
hook wiring or trust is unavailable:

```
oma relay wait --timeout 3600
```

`oma relay wait` blocks silently and prints nothing until it exits. The gap
between publish and peer reply is continuation time, not user time. Do not end
the turn to ask whether to keep waiting or narrate the wait.

When a relay wait is running, do not send a final answer. The wait is complete
only when it exits, the user explicitly interrupts or says not to wait, or the
pair is already terminal.

Handle its exit code as follows:

- `0` — read the new artifact path on stdout and start the next turn.
- `10` — surface that the wait window elapsed with the peer silent.
- `11` — surface that the peer created a publish intent and then went silent.
- `12` — read any final artifact and report the terminal pair state.

## Hold by host

- **Claude Code or another host with backgroundable shells** — run
  `oma relay wait` as a background shell task, emit one status line, and end the
  turn so the harness can re-invoke it. While pending, start no new relay round;
  read-only or unrelated work is fine.
- **Codex CLI / App** — use the Stop hook as the main self-continuation path.
  Before relying on it, ensure the dispatcher is wired in
  `~/.codex/hooks.json` and trusted through `/hooks`. A later peer publication
  resumes the turn with a `[relay-action]` prompt; act on that reason instead of
  re-running `oma relay status`.
- **Codex without a wired/trusted hook, or an explicit foreground-wait
  request** — run `oma relay wait --timeout 3600` with the longest per-call
  window the harness permits. On an empty wake, re-poll the same wait without
  commentary, a new draft/publish/close, or `@user:`. Esc/Ctrl-C remains the
  user's interrupt.

The Stop hook is event-driven and bounded, not a long-running waiter. Use a
held or re-polled `oma relay wait` only when hook wiring/trust is unavailable or
the user asks for it.

## Recover stale residue

Inspect residue with `oma relay status --json`. Clean leftover drafts and
reservations with `oma doctor relay --clean-stale`; never delete them by hand.
