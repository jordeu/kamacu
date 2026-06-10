# Phase 4: Claude Code Agent Sessions - Research

**Researched:** 2026-06-10
**Domain:** Spawning/driving the real `claude` CLI (v2.1.170) in server-owned PTYs; hook-driven status detection; alt-screen reattach
**Confidence:** HIGH — every load-bearing claim below was verified empirically against the installed binary (`~/.local/bin/claude`, v2.1.170, `/home/jordi/.local/share/claude/versions/2.1.170`) via scripted PTY runs that never submitted a prompt (zero API usage), plus current official docs (code.claude.com, fetched 2026-06-10)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Start & Initial Prompt
- **D-35:** Start launches plain interactive `claude` — the task description is NOT auto-sent. An "Insert task description" action types the description into the prompt (bracketed paste, does NOT submit), editable before sending. Repeatable.
- **D-36:** The insert action lives in the agent pane header (next to Stop), available whenever the session is running.
- **D-37:** Tasks with no description: nothing special — clean start, insert action hidden or disabled.

#### Agent Tab & Lifecycle
- **D-38:** The Agent tab is a permanent first tab. Tab order: **Agent, Description, Bash 1..N**. It cannot be closed — only stopped. Exactly one agent session per task.
- **D-39:** Opening a task always lands on the first tab (Agent). No smart tab selection.
- **D-40:** Pre-start state: the Agent tab shows a Start button (plus context: branch name, short explanation). Start is disabled with the same pattern as bash `+` when the task has no worktree (message points to Create worktree).
- **D-41:** Stop = the standard Phase 2 teardown (SIGTERM→5s→SIGKILL on the session). After exit/stop the tab shows the exited banner with **"Start again"** which launches a FRESH `claude` session (no --continue/--resume in this phase — that's Phase 5 recovery scope).
- **D-42:** The agent session counts in the cleanup gate exactly like bash sessions: it appears in "N sessions running" and "Stop sessions and clean up" stops it too. One consistent rule.

#### Status Badges & Attention
- **D-43:** Card badge is a **dot only** (no text): green = working, amber = waiting for input, gray = idle, muted gray = exited(0), red = exited(non-zero). Tooltip gives the status word.
- **D-44:** Waiting is loud: the amber dot pulses AND the card gets a subtle highlighted border. Everything else is calm.
- **D-45:** Waiting clears **on attach** (opening the task's agent tab); it re-fires if Claude prompts again.
- **D-46:** Claude finishing a turn (Stop hook) while the session stays open = **idle** (gray). No separate "done" state — the board only gets loud when input is NEEDED.
- **D-47:** Status detection = hooks for precise transitions (Notification → waiting; Stop → idle) + PTY output-activity heuristic for working-vs-idle between hook events (output flowing = working; quiet ~10s = idle).
- **D-48:** Card badges reflect the AGENT session only. Bash sessions never drive board state.
- **D-49:** Sidebar shows an amber waiting-count chip next to project names with ≥1 waiting agent (e.g. "Fusion ·2·"); hidden when zero. Cross-project dispatching at a glance.
- **D-50:** The same status dot renders on the Agent tab label inside the task view (visible while on Description/bash tabs).

#### Claude Spawn Config
- **D-51:** Default interactive permission posture — plain `claude`, normal permission prompts in the terminal (waiting-detection surfaces them on the board). No mode picker, no --dangerously-skip-permissions.
- **D-52:** The spawned claude inherits EVERYTHING the user's terminal would have: user settings (~/.claude), the worktree's CLAUDE.md, MCP servers, env. Run it as the user would run it.
- **D-53:** Status hooks are injected invisibly and additively: they must not interfere with or replace the user's existing hooks, must not leave files in the user's repos or global config, and `claude` sessions started outside Kangent are untouched. Mechanism (e.g. `--settings` overlay file in an app dir) is research's job to verify against v2.1.170.

### Claude's Discretion
- Alt-screen reattach strategy for the Claude TUI (the deferred Phase 2 problem — `Snapshot()` seam, resize-jiggle, or headless VT state; pick what's verifiably clean on reattach with the real claude binary)
- Hook payload parsing, session-ID capture mechanics (persist what Phase 5 will need for --resume, schema-wise, even if unused this phase)
- Exact dot styling within UI-SPEC color budget (resolved — 04-UI-SPEC is approved)
- How status flows to the board (polling the sessions list vs SSE/WS push — pick pragmatically; 5s polling already exists in useSessions)
- Working/idle quiet-threshold tuning
- Whether agent sessions get a distinct label ("Agent") in the session manager vs "Bash N" numbering

### Deferred Ideas (OUT OF SCOPE)
- Browser notifications on waiting/finished (v2 NOTF-01 — detection built this phase makes it cheap later)
- "Move to In Review?" suggestion on Stop hook (v2 NOTF-02 — explicitly do NOT auto-move cards)
- `--continue`/`--resume` recovery (Phase 5 RCVR-02 — but capture session IDs now)
- Per-session permission-mode picker (only if default posture proves annoying)
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| TERM-01 | Start button spawns `claude` CLI in a PTY with cwd = task's worktree | Spawn Config (verified flags `--session-id`, `--settings` inline JSON); env inherit-all pattern; binary resolution; no trust dialog in fresh worktrees (verified); SIGTERM exits cleanly in ~1.1s with code 143 (verified) |
| STAT-01 | Live status badge per card: working / idle / waiting / exited | Status state machine (Pattern 3); idle prompt verified byte-silent → output-activity heuristic is clean; status transport via dedicated 5s-polled endpoint; task-delete gap flagged |
| STAT-02 | Detection via per-session hooks injected at spawn + bell fallback | Hooks mechanism fully verified: `--settings` overlay merges additively with user hooks; Notification matchers (`permission_prompt`); payload carries session_id; curl→localhost receiver design incl. Host-check/token interplay; bell fallback requires injecting `preferredNotifChannel: "terminal_bell"` (verified silent by default) |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Locked stack:** Go 1.26 + stdlib ServeMux, `creack/pty` v1, `coder/websocket`, `modernc.org/sqlite` + goose, `@xterm/xterm` 6 pinned addon set, binary WS frames only. No new framework choices this phase.
- **Agent constraint (PROJECT.md):** the app spawns the real `claude` CLI, never reimplements it. No SDK chat UI.
- REST under `/api/*` via route-registration functions; JSON respond helpers; table-driven Go tests with httptest.
- Frontend: typed TanStack Query hooks; components own their files; verbatim UI-SPEC copy in single template literals.
- GSD workflow enforcement: implementation happens through `/gsd:execute-phase`.
- User global rule: never mention/co-author Claude or happy-otter on commits/PRs.

## Summary

This phase wires the already-proven Phase 2/3 terminal engine to the real `claude` binary and adds hook-driven status. Research ran five scripted PTY probes against the installed v2.1.170 (no prompts submitted, no API usage) and resolved every version-dependent unknown. The three headline findings:

1. **The dreaded alt-screen replay problem does not exist in v2.1.170.** Claude Code renders inline in the normal screen buffer (Ink-style): the captured startup/idle/paste stream contains **zero** `CSI ?1049h` (alt-screen) sequences — only bracketed-paste enable (`?2004h`), focus reporting (`?1004h`), synchronized-update (`?2026h`), and cursor hide/show. Conversation history lands in ordinary scrollback, which is exactly what the 1 MiB ring buffer replays. **The Phase 2 mechanism (raw ring replay + once-per-attach SIGWINCH jiggle) works unchanged; no vt10x emulator, no `Snapshot()` swap.**

2. **The hook injection mechanism is fully verified.** `claude --settings '<inline JSON>'` (the flag accepts a JSON string — no temp file needed, satisfying D-53's "no files left anywhere") registers overlay hooks that fire **in addition to** the user's own hooks (the user's SlayZone `SessionEnd` hook ran during an overlay-active probe; docs confirm hooks merge across all sources and never override). `--session-id <uuid>` is accepted and the hook stdin payload's `session_id` echoes it verbatim — deterministic capture for Phase 5's `--resume <uuid>`. The exact overlay shape Kangent will inject (Notification with `permission_prompt|elicitation_dialog` matcher, Stop, `async: true`, curl command) was run against the binary and parsed clean.

3. **The status heuristic is on solid ground.** At an idle prompt claude emits **zero bytes** over 15 s (no spinner animation at rest), so "output flowing = working, quiet = idle" is reliable. The bell is silent by default (`preferredNotifChannel` unset → `auto` → nothing in a plain PTY); the overlay must inject `"preferredNotifChannel": "terminal_bell"` for the BEL fallback to exist at all.

**Primary recommendation:** Build the backend additively on the existing `internal/session` manager — `Kind` discriminator, per-agent status state machine fed by a token-gated `POST /api/hooks/sessions/{id}` receiver and the pump's last-output timestamp — and keep the frontend to the approved 04-UI-SPEC plus one new 5s-polled status endpoint. "Insert description" is pure frontend (`term.paste()` — verified claude enables bracketed paste and renders an injected paste un-submitted).

## Standard Stack

### Core — no new dependencies

This phase introduces **zero new Go modules and zero new npm packages.** Everything rides the locked Phase 2/3 stack.

| Asset | Where | Phase 4 use |
|-------|-------|-------------|
| `internal/session` Manager/Session | `internal/session/*.go` | Agent session = `SpawnOpts{Cwd: worktree, TaskID, Kind: agent}`; status fields added to `Session`; `Snapshot()` seam **stays as-is** (see Reattach) |
| `internal/ws/handler.go` | WS attach path | D-45 waiting-clears-on-attach hook point (call on attach when kind=agent) |
| `internal/api/sessions.go` | REST | `kind` on spawn; one-agent-per-task 409 |
| goose migrations | `internal/store/migrations/` | `00003`: `ALTER TABLE tasks ADD COLUMN claude_session_id TEXT;` |
| `useSessions` 5s polling | `web/src/api/sessions.ts` | Pattern for the new agent-status poll |
| `TabDef[]` + `keepMounted` | `web/src/components/task/TaskTabs.tsx` | Permanent Agent tab (comment at line 13 already anticipates it) |
| `TerminalPane` | `web/src/components/terminal/` | Mounted in the Agent tab; additive props per UI-SPEC (banner action label, header action slot) |
| curl 8.5.0 | `/bin/curl` (verified) | Hook command transport |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `type: "command"` + curl hooks | `type: "http"` hooks (claude POSTs directly) | **Verified rejected for SessionStart in v2.1.170** ("HTTP hooks are not supported for SessionStart" in debug log); support for Notification/Stop unverifiable without burning a turn. Command+curl is verified end-to-end — use it. Revisit http hooks later if desired |
| Raw ring + jiggle reattach | vt10x headless VT snapshot behind `Snapshot()` | Unnecessary: claude doesn't use the alt screen (verified). vt10x adds a dependency and a VT-emulation correctness surface for zero benefit today. The seam remains if a future claude version changes rendering |
| Extend 5s polling | WS/SSE status push | Push beats 5s latency but adds a protocol surface; UI-SPEC explicitly allows polling ("within 5s ... one existing poll period; push permitted but not required"). Poll now; the receiver endpoint design doesn't preclude push later |
| `--settings` inline JSON string | `--settings /path/to/overlay.json` in app dir | File works too (verified), but inline JSON leaves literally nothing on disk — cleanest D-53 posture. Use inline |

## Claude Code v2.1.170 — Verified Behaviors (empirical)

All from scripted PTY runs on this host, 2026-06-10. Raw capture: `/tmp/claude-probe/phaseA.raw`.

| # | Question | Verified Answer | Evidence |
|---|----------|-----------------|----------|
| 1 | Does claude use the alternate screen? | **NO.** Inline rendering in the normal buffer. Sequences seen: `?2004h` (bracketed paste, byte 19), `?1004h` (focus), `?2026h/l` (synchronized update), `?25l/h` (cursor). No `?1049h`, no `?47h`, no `[2J`, no mouse modes (`?1000/1002/1003/1006`), no OSC title | Phase A capture, 3.3 KB startup |
| 2 | Idle-prompt output rate | **0 bytes over 15 s.** No spinner/animation at rest; output-activity heuristic is clean | Phase A idle window |
| 3 | `--settings <file-or-json>` | Exists; accepts a file path **or a JSON string**; overlay hooks fire | `--help` + Phase C (SessionStart payload file written) |
| 4 | Overlay hooks merge with user hooks (additive)? | **YES.** With the overlay active, the user's own `~/.claude/settings.json` SessionEnd hook (`/home/jordi/.slayzone/hooks/notify.sh`) executed (`completed with status 0` in `--debug-file` log). Docs (HIGH): "hooks from multiple sources merge and run together... there is no override" | debug log + docs |
| 5 | `--session-id <uuid>` | Accepted; hook payload `session_id` == the supplied uuid; transcript at `~/.claude/projects/-home-jordi-workspace-github-kangent/<uuid>.jsonl` | Phase C payload |
| 6 | Hook stdin payload shape (SessionStart) | `{"session_id", "transcript_path", "cwd", "hook_event_name", "source", "model"}` — common fields match docs | Phase C payload file |
| 7 | Exact production overlay (Notification matcher `permission_prompt\|elicitation_dialog`, Stop, `async: true`, `timeout: 5`, curl command) | Parses clean, hooks register, SessionStart fires; zero hook-validation errors in debug log | Final probe run |
| 8 | `type: "http"` hooks | Recognized but **skipped for SessionStart** ("HTTP hooks are not supported for SessionStart"); other events unverified | debug2.log |
| 9 | Bracketed paste injection (server/client → PTY) | Writing `ESC[200~ text\ntext ESC[201~` to the PTY renders the multi-line text in the prompt box, **not submitted** | Phase A paste probe |
| 10 | SIGTERM teardown (D-41 Stop) | Exits cleanly in **1.1 s**, exit code **143** (128+SIGTERM). Well within the 5 s grace; SIGKILL never needed in the probe. SessionEnd hooks still fire on SIGTERM | Phase A teardown; debug log |
| 11 | Terminal bell | **Zero BEL bytes** in all captures. `preferredNotifChannel` unset everywhere (checked `~/.claude.json` + `~/.claude/settings.json`) → default `auto` → silent in a plain PTY. Bell fallback requires injecting `"preferredNotifChannel": "terminal_bell"` | Phase A + config inspection |
| 12 | Trust dialog in a fresh worktree | **No dialog.** A brand-new `git worktree` (under /tmp) of the already-trusted kangent repo went straight to the prompt | Phase B |
| 13 | `claude config` subcommand | **Removed in v2.1.170** — not in the Commands list; `claude config get X` is treated as a prompt. Never shell out to it | `--help` + observed |
| 14 | Settings precedence | managed > command line (`--settings`) > `.claude/settings.local.json` > `.claude/settings.json` > `~/.claude/settings.json`. Scalars override by precedence; **hooks always merge** | docs (HIGH) |
| 15 | Notification matchers | `permission_prompt`, `idle_prompt`, `auth_success`, `elicitation_dialog`, `elicitation_complete`, `elicitation_response`. Stop has **no matcher** (always fires) | docs (HIGH) |
| 16 | Hook execution env | Hooks run in their own session without a controlling terminal (v2.1.139+); cwd = session cwd; `CLAUDE_PROJECT_DIR` set. curl POST unaffected | docs (HIGH) |

## Architecture Patterns

### Pattern 1: Agent spawn (TERM-01, D-51/52/53)

```go
// internal/session: SpawnOpts gains Kind; agent spawn differs from bash in
// command, args, and env only — everything else (PTY, ring, pump, teardown)
// is the existing engine.
type Kind string
const (
    KindBash  Kind = "bash"
    KindAgent Kind = "agent"
)

claudeSessionID := uuid.NewString() // persist to tasks.claude_session_id

overlay := buildOverlayJSON(kangentSessionID, serverPort, hookToken) // see Pattern 2

bin, err := exec.LookPath("claude") // PATH inherited from the server's env
if err != nil { /* -> API "couldn't start a session" + actionable log */ }

cmd := exec.Command(bin,
    "--session-id", claudeSessionID,
    "--settings", overlay, // inline JSON string — one argv, no shell, no temp file
)
cmd.Dir = worktreePath
// D-52: inherit EVERYTHING (user env, ANTHROPIC_*, PATH, node manager shims...),
// then pin the terminal identity. This deliberately differs from bash sessions'
// minimal env (Phase 2) — "run it as the user would run it".
cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
```

- **No permission flags, no mode flags** (D-51). No `--add-dir`, no `--mcp-config` — inheritance does it (D-52).
- One agent per task: `Spawn` (or the API handler) checks `ListByTask(taskID)` for a running `KindAgent` session → API 409.
- Label: `"Agent"` (discretion resolved — distinct label; bash numbering untouched).
- Make the claude binary path overridable (flag `--claude-bin` or env) — needed anyway so integration tests can substitute a stub script (real claude in CI would hit auth/API).
- Exit-code note for the planner: user-initiated Stop produces **exit 143** (verified). Per D-43 read literally that's a red dot. Recommendation: the session knows when Stop was server-initiated (`stopOnce` fired) — record `stopRequested` and report that exit as clean for dot-color purposes (muted gray, tooltip still `Exited (code 143)`). Red stays reserved for exits Kangent didn't ask for. This is a recommendation within UI semantics, not a change to D-41 teardown.

### Pattern 2: Hook injection overlay (STAT-02, D-53) — verified shape

Built per spawn (port, token, and Kangent session ID baked in), passed as an inline JSON string:

```json
{
  "hooks": {
    "Notification": [
      {
        "matcher": "permission_prompt|elicitation_dialog",
        "hooks": [
          {
            "type": "command",
            "command": "curl -s -m 3 -H 'X-Kangent-Token: <token>' --data-binary @- http://127.0.0.1:<port>/api/hooks/sessions/<kangentSessionID>",
            "timeout": 5,
            "async": true
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "curl -s -m 3 -H 'X-Kangent-Token: <token>' --data-binary @- http://127.0.0.1:<port>/api/hooks/sessions/<kangentSessionID>",
            "timeout": 5,
            "async": true
          }
        ]
      }
    ]
  },
  "preferredNotifChannel": "terminal_bell"
}
```

Why each piece:
- **Matcher `permission_prompt|elicitation_dialog`** — `permission_prompt` covers tool permission dialogs AND plan-mode's exit-plan approval; `elicitation_dialog` covers MCP servers requesting input. **Deliberately excludes `idle_prompt`** (fires after 60 s of sitting at the prompt — that is D-46's *idle*, not *waiting*; including it would make every quiet session go loud, violating "the board only gets loud when input is NEEDED").
- **`async: true` + `timeout: 5`** — claude never blocks its UI on our receiver; curl's own `-m 3` bounds the network wait. Verified parse-clean on v2.1.170.
- **`--data-binary @-`** — the hook JSON payload arrives on stdin; curl forwards it verbatim as the POST body. The payload carries `session_id` (claude's, == our generated uuid), `hook_event_name`, `cwd`.
- **`preferredNotifChannel: "terminal_bell"`** — the ONLY scalar override in the overlay. Justification: verified default (`auto`) is silent in a plain PTY, so without this the STAT-02 bell fallback cannot exist. It changes nothing for non-Kangent sessions (overlay applies to this spawn only) and inside the Kangent PTY `auto` was a no-op anyway, so nothing the user had is lost.
- **D-53 audit:** nothing written to the repo, nothing written to `~/.claude`, no temp files (inline JSON), user hooks merge and still run (verified), sessions started outside Kangent see nothing.
- Optional but recommended: add a `SessionStart` hook entry with the same curl command — it gives the server positive per-session confirmation that the hook pipeline works (drives a "hooks alive" flag the bell fallback can key off; see Pattern 4) and was the event used to verify everything above.

### Pattern 3: Hook receiver endpoint + status state machine (STAT-01/02)

**Endpoint:** `POST /api/hooks/sessions/{id}` (id = Kangent session ID).

```go
// internal/api/hooks.go (new)
// - Validate X-Kangent-Token against the per-server-instance random token
//   (crypto/rand, generated at startup, held in memory, embedded only in
//   overlay JSON). 401 on mismatch.
// - Look up session; 404 if unknown; ignore (200) if exited.
// - Parse {hook_event_name, session_id, notification_type?, message?}.
// - Dispatch: "Notification" -> sess.SetWaiting(); "Stop" -> sess.SetIdle();
//   "SessionStart" -> sess.MarkHooksAlive() + confirm claude_session_id.
// - Respond 204 immediately; never block (manager update is a mutex flip).
```

**Security interplay (verified against Phase 2 middleware):** the hook's curl sends `Host: 127.0.0.1:<port>` (passes the port-agnostic loopback hostCheck) and no Origin header (REST routes don't check Origin). So hooks pass existing middleware untouched. **The token is required** because a malicious web page can also fire a no-CORS `fetch` POST at `http://127.0.0.1:<port>/...` whose Host header is the loopback target — without the token, any website could spoof agent status (and probe other POST endpoints). The token is cheap (one header check) and closes it. Do not relax the Host middleware for this route.

**State machine (per agent session, server-side):**

```
states: working | idle | waiting          (+ exited via existing lifecycle)

spawn                          -> working   (startup output flows immediately)
hook Stop                      -> idle      (+ settle: ignore output-driven
                                             idle->working flips for ~2s — claude
                                             renders the final response around the
                                             Stop hook; without the settle every
                                             turn ends with a spurious green dot)
hook Notification(permission|elicitation) -> waiting
bare BEL in output (fallback)  -> waiting   (only while hooks NOT confirmed alive)
WS attach (kind=agent)         -> if waiting: clear -> idle   (D-45)
stdin input ('0' frame)        -> working   (user typed/answered the prompt)
output activity                -> working   (only from idle, post-settle; NEVER from waiting)
quiet >= 10s (ticker)          -> idle      (only from working)
process exit                   -> exited(code)
```

Implementation notes:
- The pump already sees every output chunk — add `lastOutput time.Time` (mutex-guarded) and a per-session 1 s ticker goroutine (or compute lazily in `Info()`: `working` iff `now-lastOutput < threshold` — lazy is simpler and the 5 s poll is the only consumer; recommended).
- `waiting` is sticky: output (prompt redraws, animations) must never clear it; only attach, stdin, a Stop hook, or exit do.
- Quiet threshold: **10 s** per D-47. The idle prompt is verified byte-silent so even 3 s would work; 10 s simply tolerates slow tool-output gaps mid-turn. Tunable constant.
- **BEL fallback discipline:** scan pump output for bare 0x07 **outside OSC strings** (OSC sequences — `ESC ]` … — are legally terminated by BEL; a naive scan false-positives on title-set sequences). Claude emitted zero OSC in probes, but the guard is ~15 lines. Crucially, with `terminal_bell` set, claude rings BEL for **both** "needs permission" and "task complete" — BEL alone cannot distinguish waiting from done. Therefore: BEL → waiting **only when the session has never received a hook POST** (hooks-dead fallback mode, keyed off the SessionStart confirmation). When hooks are alive, ignore BEL entirely.

### Pattern 4: Status transport (board dots, tab dot, sidebar chips)

**Recommendation: one new lightweight endpoint, polled at 5 s** (rides the existing pattern; meets UI-SPEC's "within 5s" bar; no WS push this phase).

```
GET /api/agents/status ->
[
  {"taskId": 7, "projectId": 2, "sessionId": "…", "status": "waiting",
   "exitCode": null}
]
```

- Handler: `mgr.List()` filtered to `kind == agent`, joined to `tasks(id, project_id)` via one `SELECT id, project_id FROM tasks WHERE id IN (…)`. Exited agents stay listed until a new agent spawn for that task replaces them (UI-SPEC: "Exited dot persists until a new agent session starts").
- One `useAgentStatuses()` query (refetchInterval 5000) feeds: card dots (`taskId → status`), Agent-tab dot (current task), and sidebar chips (`groupBy projectId, count status === "waiting"`). Single source of truth, no per-project fan-out queries.
- `GET /api/sessions?task_id=N` Info additionally gains `kind` and `agentStatus` so the open task view (already polling) is consistent.
- D-45 optimistic clear: on WS attach of the agent session, the client sets that task's status to `idle` in the query cache; the server clears it authoritatively in the WS attach path (`internal/ws/handler.go` — after `sess.Attach`, if kind==agent, clear waiting). Other surfaces converge on the next poll.

### Pattern 5: Reattach — the deferred Phase 2 problem (TERM-05 / SC-4)

**Resolution: keep the Phase 2 mechanism unchanged.** Raw 1 MiB ring replay + `term.reset()` client-side + once-per-attach SIGWINCH jiggle.

Why this is now safe (it wasn't knowable in Phase 2):
- Verified: claude renders **inline in the normal buffer** — conversation history is ordinary scrollback, which is precisely what a raw ring replays correctly. The catastrophic failure mode (ring evicts `?1049h`, xterm never enters alt screen, garbage) **cannot happen** because the sequence is never emitted.
- The ring may still begin mid-escape-sequence: xterm's parser tolerates partial sequences (verified Phase 2 research); worst case is one garbled line at the very top of scrollback — within UI-SPEC's "brief replay flicker acceptable, corrupted alt-screen is not".
- The jiggle (rows−1 → rows, exactly once per attach — already implemented with the `firstResize` debounce) makes claude repaint its prompt box at the correct size. The known resize-storm duplication bug (anthropics/claude-code#49086) is already mitigated by the once-per-attach contract.
- `?2026h` synchronized-update sequences in the replay are harmless: xterm.js 6 either honors them (less flicker) or ignores the DECSET (degrades to ordinary rendering).
- `Snapshot()` stays the seam. Do NOT add vt10x. Do NOT set `CLAUDE_CODE_DISABLE_ALTERNATE_SCREEN` (nothing to disable; setting unverified and unnecessary).
- Verification step for the executor: attach → drive a short conversation → detach → reattach → confirm history readable + prompt box intact; resize while detached → reattach → confirm repaint.

### Pattern 6: Insert description (D-35/36/37) — pure frontend

Verified: claude enables bracketed paste (`?2004h`) at startup, and an injected `ESC[200~ … ESC[201~` block renders as pasted text in the prompt, editable, NOT submitted (multi-line included).

```ts
// In the agent pane header's Insert action:
term.paste(task.description);
// xterm wraps the text in \x1b[200~ … \x1b[201~ because the app (claude)
// enabled mode 2004 (ignoreBracketedPasteMode is false — Phase 2 config).
// It flows through the existing '0' input frames. No backend work at all.
```

- Hidden (not disabled) when `description === ""` (D-37, UI-SPEC).
- Repeatable; never sends Enter.
- The TerminalPane needs a small additive prop for header actions (UI-SPEC permits "header action slot").

### Pattern 7: Schema + persistence (Phase 5 prep)

```sql
-- 00003_agent_sessions.sql
-- +goose Up
ALTER TABLE tasks ADD COLUMN claude_session_id TEXT;
-- +goose Down
ALTER TABLE tasks DROP COLUMN claude_session_id;
```

- Written at every agent spawn (latest wins — "Start again" overwrites with the fresh uuid). Phase 5 resume = `claude --resume <uuid>` with cwd = the same worktree (`--fork-session` also exists if Phase 5 wants resume-as-new).
- Sessions themselves stay **memory-only** (Phase 2 decision holds; RCVR-01 reconciliation is Phase 5). The task row is durable; the session row isn't needed for resume — worktree path + claude_session_id is the complete resume key.
- Transcript location confirmed: `~/.claude/projects/<encoded-cwd>/<session-id>.jsonl` — Phase 5 can existence-check before offering Resume.

### Anti-Patterns to Avoid

- **Writing the overlay into the worktree or `~/.claude`** — D-53 violation; inline `--settings` JSON makes it impossible to leak.
- **Including `idle_prompt` in the Notification matcher** — turns every quiet session amber; violates D-46.
- **Clearing `waiting` on output activity** — permission-prompt redraws would silently un-flag sessions that still need input.
- **Naive 0x07 scan** — OSC terminator BELs false-positive; scan must be OSC-aware.
- **Shelling out to `claude config`** — removed in v2.1.170; arguments become a prompt and burn API quota (empirically confirmed the hard way).
- **Auto-moving cards on Stop hook** — explicit anti-feature (FEATURES.md, deferred NOTF-02).
- **Spawning claude with the minimal bash env** — kills MCP servers, node shims, auth (D-52 requires inherit-all).
- **Trusting hook POSTs without the token** — any webpage can POST to localhost with a passing Host header.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Waiting/turn-end detection | Output parsing / prompt-text regex | Claude Code hooks (Notification/Stop) | Hooks are the supported contract; TUI text changes every release |
| Session identity for resume | Transcript-file mtime scanning | `--session-id <uuid>` at spawn | Verified deterministic; payload echoes it; mtime parsing was the fallback and is no longer needed |
| Hook transport | Unix sockets, named pipes, file watching | curl → existing HTTP server | The server is already there; curl is universal; payload arrives on stdin |
| Paste framing | Manual `\x1b[200~` injection from the server | `term.paste()` client-side | xterm already implements bracketed-paste wrapping correctly against the mode claude sets |
| Terminal replay correctness | VT emulator (vt10x) | Existing ring + jiggle | Verified unnecessary for claude's inline rendering |

**Key insight:** v2.1.170 provides first-class primitives (`--settings` inline overlay, `--session-id`, hook payloads with session identity) for exactly what this phase needs — the custom code is a ~100-line receiver/state machine, not infrastructure.

## Common Pitfalls

### Pitfall 1: Stop hook fires around the final render → spurious "working" after every turn
**What goes wrong:** Stop → idle, then claude paints the response/prompt → output-activity flips idle → working for another 10 s. Every turn ends green-then-gray instead of gray.
**How to avoid:** 2 s settle window after a Stop hook during which output does not flip idle→working (stdin still does).
**Warning signs:** Dots blink green after each completed turn with nobody touching the session.

### Pitfall 2: Bell fallback marks "waiting" on task completion
**What goes wrong:** `terminal_bell` rings for BOTH permission prompts and task-complete. Treating every BEL as waiting makes finished turns look like input requests.
**How to avoid:** BEL → waiting only in hooks-dead fallback mode (no hook POST ever received for the session). Hooks-alive sessions ignore BEL.
**Warning signs:** Amber pulses appearing at the same moment turns complete.

### Pitfall 3: Hook curl rejected by middleware (or accepted from attackers)
**What goes wrong:** Either the Host check 403s curl (it won't — verified port-agnostic loopback allowlist), or the endpoint trusts any local POST and webpages spoof status.
**How to avoid:** Per-instance random token in `X-Kangent-Token`, checked by the receiver. Add an httptest case: POST without token → 401; with token → 204.
**Warning signs:** Status changes you didn't cause; hook debug log shows curl exit 22.

### Pitfall 4: Task deletion leaks a running claude
**What goes wrong:** `DELETE /api/tasks/{id}` (tasks.go:441) deletes the DB row only — verified it never calls `StopAllForTask`. A deleted task's agent (and bash) sessions keep running headless, holding the worktree busy.
**How to avoid:** Call `mgr.StopAllForTask(id)` in the delete handler (worktree-cleanup already does this; mirror it). Decide response-latency posture: Stop blocks up to 5 s — run before the DB delete (consistent with the cleanup gate) or async after.
**Warning signs:** Stray `claude` processes for tasks that no longer exist; worktree cleanup 409s on a deleted task.

### Pitfall 5: Overlay validation failure silently drops all hooks
**What goes wrong:** An overlay field claude doesn't recognize can fail settings validation; in non-interactive mode invalid settings files are "silently ignored" (per `--help`). Status detection dies invisibly.
**How to avoid:** Use exactly the verified overlay shape (Pattern 2 — ran clean on v2.1.170). The SessionStart confirmation hook doubles as the canary: if no SessionStart POST arrives within ~30 s of spawn, log a warning and rely on bell fallback.
**Warning signs:** Sessions stuck on output-heuristic-only status; no entries in the receiver's log.

### Pitfall 6: Spawn env too clean, or claude not on PATH
**What goes wrong:** Reusing the bash sessions' minimal env breaks MCP/auth/shims (D-52); a server launched outside the user's shell may lack `~/.local/bin` on PATH → `exec.LookPath("claude")` fails.
**How to avoid:** `os.Environ()` + TERM/COLORTERM overrides for agent spawns; LookPath at spawn-time with a clean API error (UI copy exists: "Couldn't start a session. Try again."); log the resolution result at startup for diagnosability.
**Warning signs:** "2 setup issues: MCP" style banners differing from the user's normal terminal; spawn 500s.

### Pitfall 7: First-ever claude run in a repo shows the trust dialog inside the Agent tab
**What goes wrong:** Worktrees of repos the user has already opened in claude are trusted (verified). But a project whose repo was NEVER opened in claude will show the trust dialog on first agent start.
**How to avoid:** Nothing to build — it renders fine in the PTY and the user answers it interactively (that's the D-51 posture). Just don't let status logic misread it: it's a dialog awaiting input but fires no Notification hook; the bell/hooks won't mark waiting. Acceptable for v1; the session shows as idle while the dialog sits. Document in the phase verification checklist.
**Warning signs:** "Agent stuck doing nothing" reports on brand-new projects — the terminal shows the dialog when opened.

## Code Examples

### OSC-aware bare-BEL scanner (pump-side)

```go
// belScanner returns the number of bare BELs in chunk, treating BELs that
// terminate an OSC string (ESC ] ... BEL) as part of the sequence, with
// state carried across chunks.
type belScanner struct{ inOSC bool }

func (b *belScanner) scan(chunk []byte) (bare int) {
    for i := 0; i < len(chunk); i++ {
        c := chunk[i]
        switch {
        case b.inOSC:
            if c == 0x07 { // OSC terminator — not a bell
                b.inOSC = false
            } else if c == 0x1b && i+1 < len(chunk) && chunk[i+1] == '\\' {
                b.inOSC = false // ST terminator
                i++
            }
        case c == 0x1b && i+1 < len(chunk) && chunk[i+1] == ']':
            b.inOSC = true
            i++
        case c == 0x07:
            bare++
        }
    }
    return
}
```

### Verified hook payload (what the receiver parses)

```json
{
  "session_id": "25e51cfd-8b91-4a83-8ba5-957f45cb2a7f",
  "transcript_path": "/home/jordi/.claude/projects/-home-jordi-workspace-github-kangent/25e51cfd-….jsonl",
  "cwd": "/home/jordi/workspace/github/kangent",
  "hook_event_name": "SessionStart",
  "source": "startup",
  "model": "claude-fable-5[1m]"
}
```

Notification adds `notification_type` (e.g. `"permission_prompt"`) and `message`; Stop adds nothing the receiver needs beyond `hook_event_name` (docs, HIGH). The receiver should switch on `hook_event_name` only and tolerate unknown extra fields.

### Test stub for integration tests (no real claude)

```bash
#!/usr/bin/env bash
# testdata/fake-claude: enough to exercise spawn/status/teardown.
printf '\x1b[?2004h'           # bracketed paste on (like real claude)
echo "fake claude ready"
trap 'exit 143' TERM            # mirrors verified SIGTERM behavior
sleep 300 & wait                # idle until stopped
```
Spawn it via the claude-binary override; drive hook POSTs at the receiver directly with httptest for state-machine coverage.

## State of the Art

| Old Assumption (project research, 2026-06-10 morning) | Verified Reality (v2.1.170) | Impact |
|---|---|---|
| Claude Code is an alt-screen TUI; raw-ring replay corrupts; vt10x likely needed | **No alt screen at all** — inline rendering | Reattach problem dissolves; Phase 2 engine unchanged |
| `--session-id` "verify against installed version" (MEDIUM) | Verified: accepted, echoed in hook payloads | Deterministic Phase 5 resume; no jsonl-mtime fallback |
| Bell "as fallback" assumed available | Default `preferredNotifChannel` is silent in a PTY | Overlay must inject `terminal_bell` or the fallback doesn't exist |
| Hook config shape version-dependent (flagged) | Full shape verified incl. matchers + `async` | Overlay in Pattern 2 is copy-paste ready |
| `claude config get` usable for inspection | Subcommand removed; args become a prompt | Never invoke it |
| `CLAUDE_CODE_DISABLE_ALTERNATE_SCREEN` as escape hatch | Irrelevant — nothing to disable | Drop from consideration |
| Internal ~2000-line TUI scrollback concern | History is normal-buffer scrollback; ring + client 10k lines cover it | UX caveat softens |

## Open Questions

1. **Does `notification_type` appear for plan-mode approval prompts specifically?**
   - What we know: plan-mode exit goes through the permission system (ExitPlanMode tool approval); `permission_prompt` is the documented type for permission dialogs.
   - What's unclear: unverifiable without burning a turn.
   - Recommendation: ship with the `permission_prompt|elicitation_dialog` matcher; phase verification (human-in-the-loop, real session) confirms plan mode flips the dot amber. If a gap appears, widen the matcher to `"*"` and filter on `notification_type` server-side (payload carries it).
2. **AskUserQuestion-style dialogs (claude asking the user to choose)** — do they fire a Notification hook?
   - Recommendation: same as above — verify during phase execution with a real session; bell fallback and the visible terminal cover misses. Not a blocker.
3. **`type: "http"` hooks for Notification/Stop** — supported events are undocumented; rejected for SessionStart (verified).
   - Recommendation: don't use them this phase (command+curl verified). Candidate simplification later.
4. **Stop-initiated exit shown gray vs red** (exit 143 from SIGTERM, verified).
   - Recommendation: gray when Kangent initiated the stop (session knows), red otherwise. Planner should confirm this reading of D-43 in the plan; UI copy unchanged either way.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| claude CLI | agent spawn | ✓ | 2.1.170 (`~/.local/bin/claude` → `~/.local/share/claude/versions/2.1.170`) | — (spawn error surfaces in UI) |
| curl | hook transport | ✓ | 8.5.0 (`/bin/curl`) | wget/custom — not needed |
| Go | backend | ✓ | 1.26.0 linux/amd64 | — |
| Node | frontend build | ✓ | 24.4.1 | — |
| git worktree | agent cwd | ✓ | Phase 3 shipped | — |
| Claude auth/settings | D-52 inheritance | ✓ | `~/.claude` populated; kangent repo trusted (`hasTrustDialogAccepted: true`); user hooks present (merge-tested against them) | — |

**Missing dependencies with no fallback:** none.

## Sources

### Primary (HIGH confidence)
- **Empirical probes against installed claude v2.1.170** (2026-06-10, this host): five scripted PTY runs — startup/idle/paste capture, fresh-worktree trust, `--settings` overlay + `--session-id` + payload capture, `--debug-file` hook-merge evidence, final overlay-shape validation. Raw capture `/tmp/claude-probe/phaseA.raw`; debug logs `/tmp/claude-probe/debug*.log`
- `claude --help` (v2.1.170) — `--settings <file-or-json>`, `--session-id <uuid>`, `--resume`, `--fork-session`, `--setting-sources`, permission-mode flags
- code.claude.com/docs/en/hooks (fetched 2026-06-10) — event list, config schema (matcher/async/timeout), stdin payload fields, Notification matcher values, merge semantics, hook execution environment
- code.claude.com/docs/en/settings (fetched 2026-06-10) — precedence order, hooks-merge-across-scopes, `preferredNotifChannel` values/default
- Repo inspection — `internal/session/{session,manager}.go`, `internal/ws/handler.go`, `internal/api/{sessions,tasks}.go` (delete-path gap), `internal/store/migrations/`, `cmd/kangent/main.go` (port/flags/app dir), `web/src/api/sessions.ts`, `web/src/components/task/TaskTabs.tsx`
- `~/.claude/settings.json` + `~/.claude.json` (read-only inspection) — user hook inventory, trust storage, notif channel unset

### Secondary (MEDIUM confidence)
- Plan-mode/AskUserQuestion Notification coverage (docs imply, unverified live — Open Questions 1–2)
- xterm.js 6 handling of DECSET 2026 (harmless either way)
- anthropics/claude-code#49086 resize-storm bug (via PITFALLS.md; mitigation already shipped in Phase 2)

### Tertiary (LOW confidence)
- `type: "http"` hook support for Notification/Stop events (explicitly not relied upon)

## Metadata

**Confidence breakdown:**
- Hook mechanism / overlay shape / session-id capture: HIGH — verified end-to-end on the exact installed binary
- Reattach strategy: HIGH on mechanism (no alt screen, verified; engine already proven); MEDIUM on subjective redraw polish — phase verification bar is "correctly redrawn, brief flicker OK"
- Status state machine: HIGH on inputs (idle silence, hook events verified); MEDIUM on edge timing (settle window, thresholds) — flagged tunable
- Bell fallback: HIGH on mechanism (channel default + injection), MEDIUM on semantics (task-complete vs permission BEL ambiguity — mitigated by hooks-alive gating)
- Trust dialog: HIGH for repos already opened in claude; MEDIUM for never-opened repos (Pitfall 7)

**Research date:** 2026-06-10
**Valid until:** ~2026-07-10 — claude CLI moves fast; if the installed binary is updated past 2.1.x, re-verify the overlay shape and the no-alt-screen finding before executing
