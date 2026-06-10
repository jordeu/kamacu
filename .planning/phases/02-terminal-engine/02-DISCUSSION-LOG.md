# Phase 2: Terminal Engine - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-10
**Phase:** 2-terminal-engine
**Areas discussed:** Where the terminal lives, Session lifecycle UX, Terminal UX details, Security posture

---

## Area Selection

| Option | Selected |
|--------|----------|
| Where the terminal lives | ✓ |
| Session lifecycle UX | ✓ |
| Terminal UX details | ✓ |
| Security posture | ✓ |

---

## Where the Terminal Lives

| Option | Description | Selected |
|--------|-------------|----------|
| Dev route, promote later (Recommended) | /terminal route for Phase 2; Phase 3 moves component into task tabs | ✓ |
| Task Terminal tab now | Generic Terminal tab in task view immediately | |
| Both | Dev route + task tab preview | |

| Option | Description | Selected |
|--------|-------------|----------|
| Simple: one-click spawn (Recommended) | New terminal action, session ids, list/attach/stop | ✓ |
| Single global session | One bash session total | |
| You decide | Claude picks | |

---

## Session Lifecycle UX

| Option | Description | Selected |
|--------|-------------|----------|
| Graceful then force (Recommended) | SIGTERM to process group → ~5s → SIGKILL | ✓ |
| Two-step UI | Separate Stop and Force kill actions | |

| Option | Description | Selected |
|--------|-------------|----------|
| Frozen output + banner (Recommended) | "Session exited (code N)" overlay, history readable | ✓ |
| Auto-close | Pane disappears on exit | |

| Option | Description | Selected |
|--------|-------------|----------|
| Never (Recommended) | No idle timeout; sessions live until stopped | ✓ |
| Idle timeout | Auto-kill after N idle hours | |

---

## Terminal UX Details

| Option | Description | Selected |
|--------|-------------|----------|
| Copy-on-select + Ctrl+Shift+V (Recommended) | Classic terminal behavior + bracketed paste | ✓ |
| Explicit only | No copy-on-select | |

| Option | Description | Selected |
|--------|-------------|----------|
| ~10k lines (Recommended) | Deep history under xterm.js perf limits | ✓ |
| ~50k lines | Heavier memory | |

| Option | Description | Selected |
|--------|-------------|----------|
| System mono, 13-14px (Recommended) | ui-monospace stack, no webfont | ✓ |
| Bundled coding font | JetBrains Mono etc. — breaks no-webfont rule | |

---

## Security Posture

| Option | Description | Selected |
|--------|-------------|----------|
| Origin+Host only (Recommended) | Strict loopback allowlist on WS upgrades + API | ✓ |
| Origin+Host + token | Jupyter-style per-instance token | |

| Option | Description | Selected |
|--------|-------------|----------|
| Hard-block non-local (Recommended) | Refuse to start on non-loopback --addr | ✓ |
| Warn but allow | Loud warning on 0.0.0.0 | |

---

## Claude's Discretion

- Ring buffer size and replay strategy for reattach
- WS wire protocol details (ttyd-style per research)
- WS reconnect behavior
- Session naming on dev route
- Memory-only vs DB session records for this phase

## Deferred Ideas

- Per-instance auth token (if shared machines ever matter)
- Whether /terminal survives as a permanent debug surface after Phase 3
