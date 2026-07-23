# Phase 08: Sessions & Terminal Read Access - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-23
**Phase:** 08-sessions-terminal-read-access
**Areas discussed:** Live-tail delivery model, Terminal-output encoding, list_sessions scope, Dead-session behavior

---

## Live-tail delivery model

### Delivery model

| Option | Description | Selected |
|--------|-------------|----------|
| Collect-and-return | Handler ranges over attach channel for bounded duration (or ctx.Done on cancel), accumulates chunks, returns ONE TextContent blob. Matches SC3 "returns the partial stream collected so far". | ✓ |
| Progress-streaming | Each chunk pushed via req.Session.NotifyProgress (Message=chunk). Agent watches live. Cost: progress notifications aren't meant for binary terminal streams. | |
| Hybrid (collect + heartbeat) | Collect-and-return primary; periodic progress notifications as liveness heartbeat (not carrying raw bytes). | |

**User's choice:** Collect-and-return
**Notes:** SC3's exact words ("returns the partial stream collected so far") drove this. Progress notifications are designed for progress bars, not high-volume binary PTY output — rejected as a misuse.

### Replay handling

| Option | Description | Selected |
|--------|-------------|----------|
| Live-only | Attach, drain+discard first (replay) message, collect only new output. Clean separation from get_session_output. | |
| Replay + live | Attach as-is — full ring history then live. Self-contained but duplicates snapshot; risks large returns. | |
| Configurable via arg | Add include_history?:boolean (default false=live-only). Agent opts into replay+live. | ✓ |

**User's choice:** Configurable via arg (include_history?, default false = live-only)
**Notes:** Gives a one-shot self-contained option (include_history=true) while keeping the default bounded and non-duplicative of the snapshot tool.

### Exit mid-tail behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Return early + exit marker | Session.Done() fires → stop, return bytes gathered + structured note (exit code, stopRequested). | ✓ |
| Return bytes only, no marker | Stop early, return just bytes. Agent can't distinguish exit from duration-elapsed. | |
| Keep tailing until duration | Ignore exit; PTY is dead so no new bytes — wastes the window. | |

**User's choice:** Return early + exit marker
**Notes:** Highest-value: agent learns the sibling finished/crashed and what it last said. Requires a structured (JSON envelope) result.

### Accumulated-size cap

| Option | Description | Selected |
|--------|-------------|----------|
| Cap, keep most recent + note | Cap ~1 MiB; drop from front keeping most recent N; append 'truncated' note. | ✓ |
| Cap, stop early | Return as soon as N bytes collected (first burst wins). | |
| You decide / trust the duration | No explicit cap beyond 300s. Risky (OOM). | |

**User's choice:** Cap, keep most recent + note

---

## Terminal-output encoding

### Encoding

| Option | Description | Selected |
|--------|-------------|----------|
| Raw bytes, base64 | Raw PTY bytes base64-encoded in TextContent. Faithful, safe over JSON, no lossy transform. | ✓ |
| UTF-8 text, ANSI kept | Inline UTF-8-best-effort, ANSI left in. Agent reads directly but escape noise + invalid-UTF-8 risk. | |
| ANSI-stripped UTF-8 text | Server-side strip ANSI → clean text. Lossy; TUI redraw bytes stay messy even after stripping. | |

**User's choice:** Raw bytes, base64
**Notes:** Matches the project's honest-no-transform posture (cf. D-M001-2). Agent CLI is told the encoding.

### Snapshot max + clamp

| Option | Description | Selected |
|--------|-------------|----------|
| Cap ~512 KiB, clamp + note | Default 4096; hard cap ~512 KiB raw so base64 fits bridge 1 MiB; clamp + note at bound. | ✓ |
| Full ring (up to 1 MiB), bypass cap | Allow whole ring; bypass/raise maxBodyBytes for this tool. Weakens OOM guard. | |
| You decide the cap | Pick a sensible cap during planning. | |

**User's choice:** Cap ~512 KiB, clamp + note

### Slice semantics

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — last N bytes | bytes? slices the TAIL of the ring (most recent). Matches MCPSESS-03 "last N bytes". | ✓ |
| Head N bytes | Slice the front (oldest retained). Rarely what's needed. | |

**User's choice:** Yes — last N bytes (most recent)

### Result shape

| Option | Description | Selected |
|--------|-------------|----------|
| JSON envelope | {encoding, output (base64), bytes, truncated?, exited?, exitCode?, stopRequested?} as TextContent. | ✓ |
| Plain base64 + appended note | base64 string + human-readable notes appended. Agent must parse prose. | |
| Raw base64 only | Just base64, no metadata. Discards exit/truncation signals. | |

**User's choice:** JSON envelope
**Notes:** Consistent with Kamacu's other JSON endpoints; machine-parseable for the exit/truncation metadata.

---

## list_sessions scope

### Enumeration scope

| Option | Description | Selected |
|--------|-------------|----------|
| All live terminal sessions | Every in-memory PTY (bash + agent) via /api/sessions. Coherent session-id space for all 4 tools. | ✓ |
| Agent-only, mirror the bar | /api/agents/status. BUT bash excluded + post-restart entries have sessionId="" (un-operable). | |
| All sessions + bar-style JOIN | All sessions (like opt 1) + every entry carries task/project/agent JOIN. | |

**User's choice:** All live terminal sessions
**Notes:** The 4 tools share one session-id space; every listed id must be operable by get_session/get_session_output/subscribe. The bar is a separate UI view.

### Context JOIN

| Option | Description | Selected |
|--------|-------------|----------|
| JOIN the context in | list_sessions AND get_session carry taskTitle/projectName/agentName. Satisfies SC1 directly. | ✓ |
| Raw session.Info; agent calls get_task | Only taskID. Lighter but extra round-trips; doesn't satisfy SC1 literally. | |
| get_session JOINs, list stays raw | Cheap list, rich detail-on-demand. Middle ground. | |

**User's choice:** JOIN the context in
**Notes:** SC1 requires "task, project, agent". Reuses the JOIN agents/status already does.

---

## Dead-session behavior

### Exited-session operability

| Option | Description | Selected |
|--------|-------------|----------|
| Operate — read final output | Exited sessions readable: get_session=exited+exitCode; get_session_output=final ring; subscribe=immediate (empty live-only or replay if include_history) + exit marker. | ✓ |
| Reject exited sessions | get/get_output/subscribe on exited → error. Cleaner contract but loses final-output reading. | |

**User's choice:** Operate — read final output
**Notes:** Ring survives exit; "what did the sibling leave on screen" is high-value.

### Orphan tmux survivors + unknown id

| Option | Description | Selected |
|--------|-------------|----------|
| Filter orphans; unknown id = 404 | list_sessions excludes orphaned tmux-survivor entries (no ring until reattach). Unknown id → 404 → D-05 wrap. | ✓ |
| Include orphans; get_output 404s on them | Surface them but can't read — confusing. | |
| You decide the orphan handling | Defer; unknown-id stays 404. | |

**User's choice:** Filter orphans; unknown id = 404
**Notes:** Every listed id is operable. Orphans are a SPA-reattach concern, not terminal-read targets.

---

## the agent's Discretion

- Live-tail HTTP transport shape (chunked HTTP streaming vs SSE-shaped vs read-only WS variant) — constraints locked in D-11, wire shape open.
- Exactly where base64 is applied (Kamacu octet-stream + bridge encodes, vs Kamacu returns full envelope inline).
- `?project_id=N` JOIN implementation (in-memory filter vs single SQL pass).
- Exact ~512 KiB / ~1 MiB cap constants.
- `internal/mcp/sessions.go` handler + test layout (mirror Phase 07 D-07; subscribe needs an in-memory-transport cancellation/leak test for SC3).
- Exact InputSchema map shapes + description copy.
- SDK panic-recovery wrapping breadth (every session handler vs just streaming); SC2 stream-desync invariant must hold.

## Deferred Ideas

- MCP progress-notification streaming / liveness heartbeats (D-01 chose collect-and-return).
- Headless-VT-emulator clean-text snapshots (raw bytes today; future swap behind same signature).
- Full-ring (1 MiB) snapshots (D-06 caps at ~512 KiB to fit bridge ceiling).
- `include_history` defaulting to true (currently false).
- MCPAUTO-02 (`get_my_session` auto-scoping) — v1.12.
- MCPHARD-01..03 (typed error taxonomy, stdout guards, real-binary e2e) — v1.12.
- Subscribe over WebSocket (D-11 leaves transport open; HTTP chunked likely simpler).
- Agent-CLI auto-registration (MCPREG) — v1.12.
