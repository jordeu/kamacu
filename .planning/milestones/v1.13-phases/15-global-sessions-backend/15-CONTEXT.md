# Phase 15: Global sessions backend - Context

**Gathered:** 2026-08-26
**Status:** Ready for planning

<domain>
## Phase Boundary

The API-driven global scratchpad session surface: the session engine gains an additive global scope (`SpawnOpts.Global`/`Info.Global` + counters + `ListGlobal`/`StopAllForScope`), `POST /api/sessions {scope:"global"}` spawns agent + bash tabs (plain bash + invisible tmux `kamacu-global-<n>`, engine-branched resume, restart reconcile, opencode capture re-target) in the configured global root, and `/api/agents/status` widens so a global session is visible live AND post-restart (both passes). Curl/API-testable only — no frontend (Phase 16), no config surface (Phase 14, done).

Co-phasing mandate (roadmap): the status-feed widening ships in the SAME phase as the spawn path — otherwise the global session is invisible and the phase cannot be human-verified.

</domain>

<decisions>
## Implementation Decisions

### Carry-Forward Locked (research + Phases 13–14 — do not reopen)

- **Representation:** additive scope discriminator on the session engine — NEVER `TaskID = 0` (already the dev-session overload, three load-bearing checks), never a sentinel row (research P2/P4).
- **Status feed:** synthesized server-side entries with non-nullable labels (`projectName:"Global"`, `taskTitle:"Scratchpad"` — D-09/D-10; strings start emitting in THIS phase); bar rows key by `sessionId`, not taskId (P6). Co-phased with the spawn path.
- **tmux mint:** `kamacu-global-<n>` (D-11/D-02) — stays inside the sweep's `kamacu-` prefix and `defaultTmuxLabel`'s last-numeric-segment parsing.
- **One-agent gate:** a second CONCURRENT global agent spawn → 409 (GVIEW-02); an exited agent never blocks — fresh spawn replaces (task parity, agents.go:31).
- **No auto-reap:** no Done-TTL analog, no PR reconcile pass — immortal until explicitly stopped (GSESS-04); Stop is scope-targeted, NEVER `StopAllForTask(0)` (GSESS-01).
- **Agent resolution:** read-at-use — spawn reads the singleton's `agent_id` FK (D-24); a live session keeps the agent it was spawned with.
- **Resume ids:** internal to the server (D-18 — never on any wire); persisted engine-keyed in the `global_task` singleton (00017 columns); cleared only via D-16 (root change).
- **GET /api/global count fields become real** in this phase (D-19 forward-wired them as zeros; `ListGlobal()` lands the truth behind the Phase-14 409 gate).
- **Activity exclusion (GINT-03):** the v1.12 task-keyed surface is unchanged by construction — verify, don't add filters (research component 8: reaper/activity unchanged-by-design).
- **MCP (GINT-02):** session tools handle task-less global sessions with honest synthesized labels — never 404 or garbled; read-only contract unchanged.

### Spawn-gate error posture (new — D-28..D-34)

- **D-28: Unconfigured root → 409.** `scope:"global"` spawn with `root_path=''` + `github_repo=NULL` returns 409 "global root not configured" — the task-gate family ("task has no worktree", sessions.go:269), state conflict not malformed request. Never a silent `$HOME`/cwd fallback (research pattern 4).
- **D-29: Vanished root → distinct 409 naming the path.** Root configured but directory gone since PUT → 409 "global root no longer exists on disk: \<path\>" — deliberately SHARPER than task parity (tasks fall through to Spawn's stat pre-check → generic 500). The user can tell misconfiguration (fix in Settings) from disk rot (fix on disk). No boot revalidation (D-08 unchanged).
- **D-30: Gate applies to ALL kinds uniformly.** Agent, plain bash, tmux bash, and `resume:true` all hit the unconfigured/vanished gate — one gate before kind dispatch (mirrors how the worktree query feeds every kind today). No kind can silently land in `$HOME`.
- **D-31: Resume with no persisted id → 409.** `scope:"global"` + `resume:true` with no singleton id for the requested engine → 409 "no global \<engine\> session to resume". Never a silent fresh spawn — the user asked for a conversation back, not a blank terminal.
- **D-32: Cross-scope tmux reattach → 404.** `scope:"global"` + `reattach_tmux_name` pointing at a task-scoped tab (or vice versa) → 404 not-found-in-scope. The lookup runs scoped (`WHERE scope='global' AND name=?`); a foreign row simply doesn't exist in this scope. No scope-mismatch 409 branch, no information leak.
- **D-33: Gate order — root gates first.** Unconfigured → vanished → one-agent limit. Config-level checks are cheapest and most actionable; mirrors the task path (worktree resolution precedes the one-agent gate).
- **D-34: One-agent 409 copy mirrors task-gate voice.** e.g. "global agent already running" — same voice as "task has no worktree" and the Scratchpad 409s from Phases 13–14. Only RUNNING blocks (SC1 "second concurrent"); exited never blocks, fresh spawn replaces.

### the agent's Discretion

- `SpawnOpts.Global`/`Info.Global` field spelling, `ListGlobal()`/`StopAllForScope` signatures, counter wiring (research locks the shape, not the names).
- `agentStatusEntry` wire mechanics for the global entry — the FLAGGED SPIKE for planning research (`--research-phase`): nullable-vs-synthesized is settled (synthesized), the TaskID/ProjectID/discriminator field spelling ripples into 4+ frontend consumers.
- opencode capture re-target mechanics (`discoverOpenCodeSession` PWD/directory filter matching the global root; UPDATE `global_task` instead of `tasks`) — flagged for the planning-research spike, worth one host-gated e2e.
- Bash tab label spelling (task-parity "Bash N" is the default; not user-locked).
- csid persist-failure posture (tasks do `slog.Warn` non-fatal — mirroring is the default).
- Wave split if the phase feels heavy at planning time (engine+bash, then agent+status — waves, NOT separate phases; roadmap research flag).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` — GSESS-01..04, GVIEW-02/03, GINT-02/03 (this phase); Out-of-Scope table (no TTL/reaping); the Phase-15 mapping note (GVIEW-02/03 are API-testable backend behaviors here, tab UI is Phase 16's GVIEW-01)
- `.planning/ROADMAP.md` §Phase 15 — goal, success criteria 1–5, the co-phasing mandate (status feed WITH spawn), the research flag (two spikes)

### Milestone research (architecture-locked)
- `.planning/research/SUMMARY.md` — §"Phase 3: Session engine scope + spawn/status backend" (delivers list, two-wave note), P1 (invisibility — the three filters), P4 (TaskID=0 overload), P5 runtime half (restart reconcile), P6 (bar contract), P9 (scope-targeted stop), Research Flags §Phase 3 (the two spikes)
- `.planning/research/ARCHITECTURE.md` — components 3 (`internal/session` additive), 4 (`internal/api/sessions.go` spawn branch), 5 (`internal/api/agents.go` third synthesized pass); "synthesized JOIN entries instead of LEFT-JOIN surgery" pattern
- `.planning/research/PITFALLS.md` — P1 (invisible global session — widen BOTH passes), P4 (`StopAllForTask(0)` landmine), P5 (restart amnesia runtime half), P9 (non-reaping + stop affordance)

### Prior-phase decisions this phase exercises
- `.planning/phases/13-global-data-foundation-safety-net/13-CONTEXT.md` — D-08 (spawn-time honesty), D-09/D-10/D-11 (naming: Scratchpad label, synthesized strings, internal `global`), D-13/D-14 (live-definition for the gate Phase 14 wired)
- `.planning/phases/14-global-config-api/14-CONTEXT.md` — D-17/D-18/D-19 (GET wire, resume-ids internal, count fields forward-wired as zeros — THIS phase makes them real), D-24 (read-at-use agent), D-27 (`~/.kamacu` block)

### Code-level precedents (read in-repo)
- `internal/api/sessions.go` — `create` (:223, the spawn path being widened: worktree query → one-agent gate → tmux mint/reattach → csid persist → opencode capture), `stop` (:502, per-session surface), `defaultTmuxLabel` (:615), `captureOpencodeSessionAsync` (:793)
- `internal/api/agents.go` — `agentStatusEntry` (:26, the exact wire contract), `status` (:44, manager-derived pass `TaskID <= 0` filter at :50 + DB-derived restart pass at :156)
- `internal/session/manager.go` — `SpawnOpts` (:54), `Spawn` (:114), `ListByTask`/`listWhere` (:421/:427), `StopAllForTask` (:456)
- `internal/store/migrations/00017_global_task.sql` + `00018` — the singleton (resume-id columns) and the `tmux_sessions` scope rebuild (XOR CHECK) this phase writes against
- `internal/api/global.go` — the Phase-14 surface whose forward-wired 409 gate + count fields this phase makes real

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/session/manager.go` `Spawn` — already cwd-driven, not worktree-driven; an additive `Global` flag rides the zero-value back-compat pattern `Shell`/`TmuxName` established
- `sessions.go:282` worktree-resolution block — the template for the global root-resolution + gate block (singleton SELECT instead of task JOIN; D-28..D-33 slot in exactly where "task has no worktree" sits)
- `sessions.go:418` tmux `COALESCE(MAX(n),0)+1` minting + `00018` scope-scoped rows — the global variant needs the scope-scoped counter and `kamacu-global-<n>` format
- `agents.go` dual-pass status feed — both passes widen by appending ONE synthesized entry each (research: synthesized entries, never LEFT-JOIN surgery on the hot queries)
- `captureOpencodeSessionAsync` (:793) — the polling/capture shape; global re-target = directory filter + UPDATE target swap
- fake-claude/fake-opencode test harnesses (v1.10) — extend with global cases; task-path regression must stay byte-for-byte green

### Established Patterns
- One-gate-before-kind-dispatch with honest 409s (D-28..D-33 extend the "task has no worktree" family)
- Partial scope discriminators: zero value = current behavior (`Kind`, `Shell`, `TmuxName` precedents)
- Read-at-use FK resolution — spawn reads config at spawn time, no restart needed (D-24)
- Engine-branched resume: claude `--resume` keyed on csid + transcript glob; opencode keyed on ocsid alone (M002/S03 — the singleton columns mirror `tasks.claude_session_id`/`opencode_session_id`)
- Degrade-don't-break on the unconfigured state (honest 409, never silent fallback)

### Integration Points
- `internal/api/routes.go` — `POST /api/sessions` body gains `scope` (no new side-channel endpoint — research: it would fork the engine-branched spawn logic)
- `GET /api/global` — the Phase-14 count fields flip from zeros to `ListGlobal()`-derived; the 409 reconfigure gate starts biting
- `tmux_sessions` (00018 shape) — task-less global rows; reattach lookup goes scope-scoped (D-32)
- `/api/agents/status` — both passes + the frontend consumers (bar, board dot) ride the widened feed
- `internal/mcp/sessions.go` bridge — consumes GET /api/sessions; synthesized labels flow through `sessionDetail` JOIN

</code_context>

<specifics>
## Specific Ideas

- The vanished-root 409 should include the configured path verbatim — copy shape: `global root no longer exists on disk: <path>`.
- The unconfigured 409 should point at the fix path (Settings once Phase 16 lands; until then it's PUT /api/global) — wording at implementer's discretion.
- Curl round-trip tells the story: unconfigured spawn → 409; PUT a root; spawn {scope:"global", kind:"bash"} → 200 + session in GET /api/sessions with synthesized labels; second {kind:"agent"} while one runs → 409 "global agent already running".

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope. Not selected for discussion (left to planning/discretion): stop surface shape, bash tab label spelling, resume-id capture timing details. GT-FUT-01..07 remain parked in REQUIREMENTS.md.

</deferred>

---

*Phase: 15-Global sessions backend*
*Context gathered: 2026-08-26*
