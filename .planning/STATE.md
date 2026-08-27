---
gsd_state_version: 1.0
milestone: v1.13
milestone_name: Global Task
current_phase: 17
current_phase_name: Hardening & E2E
status: planning
stopped_at: Phase 16 complete, ready to plan Phase 17
last_updated: "2026-08-27T15:05:00.000Z"
last_activity: 2026-08-27
last_activity_desc: Phase 16 complete, transitioned to Phase 17
progress:
  total_phases: 5
  completed_phases: 4
  total_plans: 9
  completed_plans: 9
  percent: 80
---

# Project State

**Current focus:** Phase 17 — Hardening & E2E

See: .planning/PROJECT.md (updated 2026-08-27)

**Core value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent agent session you can open, leave, and reattach to from the browser.

## Deferred Items

Items acknowledged and deferred at milestone close on 2026-07-29 (carried over from prior milestones — none are v1.11 work):

| Category | Item | Status | Deferred At |
|----------|------|--------|-------------|
| quick_task | 260613-osu-warn-when-a-linked-github-repo-cannot-be | unknown |
| quick_task | 260613-ph5-make-github-repo-link-validation-mandato | unknown |
| quick_task | 260616-8l7-the-refresh-button-at-review-column-seem | awaiting-human-verify |
| quick_task | 260618-mlu-when-the-status-bottom-bar-is-expanded-a | unknown |
| quick_task | 260625-9db-the-bottom-status-bar-is-hidding-the-bot | unknown |
| quick_task | 260626-hwd-add-an-opt-in-insecure-allow-remote-flag | unknown |

Known verification overrides: 6 (all prior-milestone quick tasks — see table above)

## Current Position

Phase: 17 — Hardening & E2E
Plan: Not started
Total Plans in Phase: Not yet planned
Status: Ready to plan
Last activity: 2026-08-27 — Phase 16 complete (UAT 4/4, verification passed), transitioned to Phase 17

Progress: [████████████████████] 9/9 plans (100%)

### v1.13 Roadmap Snapshot

| Phase | Focus | Requirements |
|-------|-------|--------------|
| 13 | Data foundation & safety net (singleton + tmux rebuild + sweep fix, co-phased) | GDATA-01..03 |
| 14 | Global config API (folder + managed-clone root, agent, 409 gate) | GCONF-01..04 |
| 15 | Sessions backend (scope, spawn, status widening — risk center) | GSESS-01..04, GVIEW-02/03, GINT-02/03 |
| 16 | View, Settings & bar (UI) | GCONF-05, GVIEW-01/04, GINT-01 |
| 17 | Hardening & E2E (restart, interlock, sentinel-leak, UAT) | gate-closer (no owned reqs) |

Research flags: Phase 13 (tmux table-rebuild rehearsal) and Phase 15 (status-wire widening + opencode capture spikes) should run with `--research-phase`.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260728-q5k | Add MCP tool to start an agent session for a task (MCP-only — reverses no-auto-start for the agent delegate surface; browser Start button unchanged). Bridges to POST /api/sessions {kind:agent, task_id:N, resume?:bool}. | 2026-07-28 | cf64c5a | [260728-q5k-add-mcp-tool-to-start-an-agent-session-f](./quick/260728-q5k-add-mcp-tool-to-start-an-agent-session-f/) |
| 260728-qsf | Add MCP tool send_session_message(session_id, message) — reverses v1.11 D-14 read-only-terminal contract for the agent delegate surface (browser WS stays the authoritative interactive surface). Adds a NEW Kamacu endpoint POST /api/sessions/{id}/input calling session.Session.WriteInput, then a thin MCP bridge tool in internal/mcp/sessions.go. | 2026-07-28 | 264170e | [260728-qsf-add-mcp-tool-send-session-message-sessio](./quick/260728-qsf-add-mcp-tool-send-session-message-sessio/) |
| 260728-r86 | Add MCP tool post_pr_review(project_id, pr_number, verdict, body, inline_comments?) that posts a PR review to GitHub via gh. Reverses v1.3 Out-of-Scope "no GitHub writes" for the agent delegate surface. The MCP tool is the write primitive only — multi-agent orchestration (subagent double-checking, verdict synthesis) and publish-checkpoint UX live in the agent workflow prompt, NOT in this tool. | 2026-07-28 | 6003e9b | [260728-r86-add-mcp-tool-post-pr-review-project-id-p](./quick/260728-r86-add-mcp-tool-post-pr-review-project-id-p/) |
| 260728-s5a | Fix send_session_message terminator — \r (CR) not \n (LF). Raw-mode TUI agents (Claude Code, opencode) read \r as the Enter key; \n does not submit the prompt. The browser WS path worked because xterm.js sends \r; the existing integration test passed because it drives bash (cooked mode). | 2026-07-28 | 9c3bf0d | [260728-s5a-fix-send-session-message-terminator-must](./quick/260728-s5a-fix-send-session-message-terminator-must/) |
| 260728-sm5 | Fix send_session_message — split the PTY write into two WriteInput calls (body, then \r). The CR-terminator fix (260728-s5a) was necessary but not sufficient: a single combined write of (text + \r) triggers paste-detection in raw-mode TUIs (Claude Code via Ink), which treats the embedded CR as paste content rather than the Enter key. Two writes mirror how human typing reaches the PTY. | 2026-07-28 | e6cfb1c | [260728-sm5-fix-send-session-message-split-the-pty-w](./quick/260728-sm5-fix-send-session-message-split-the-pty-w/) |
| 260728-t4c | Fix send_session_message (3rd attempt, deterministic) — wrap body in bracketed paste markers (ESC[2004 ... ESC[2014) so raw-mode TUIs capture the chunk as a single paste event, then send \r OUTSIDE the closing bracket as a separate WriteInput call so it is interpreted as the Enter key (submit). VERIFIED on live Claude Code v2.1.22 / Opus 5 — agent went idle→working. The split-write fix (260728-sm5) was necessary but not sufficient; with brackets + outside-\r, submission is deterministic. | 2026-07-28 | 563e19f | [260728-t4c-fix-send-session-message-3rd-attempt-wra](./quick/260728-t4c-fix-send-session-message-3rd-attempt-wra/) |

## Session

**Last session:** 2026-08-27T15:05:00.000Z
**Stopped at:** Phase 16 complete, ready to plan Phase 17
**Resume file:** None

## Performance Metrics

| Phase | Plan | Duration | Notes |
|-------|------|----------|-------|
| Phase 06 P01 | 12 min | 2 tasks | 7 files |
| Phase 06 P03 | 8 min | 2 tasks | 7 files |
| Phase 06 P02 | 22 min | 2 tasks | 8 files |
| Phase Phase 07 P01 | 14 min | 2 tasks | 11 files |
| Phase 07 P02 | 7 min | 2 tasks | 3 files |
| Phase Phase 07 P03 | 7 min | 2 tasks tasks | 2 files files |
| Phase 07 P04 | 7min | 2 tasks | 2 files |
| Phase 08 P01 | 21 min | 2 tasks | 2 files |
| Phase 08 P02 | 10 min | 2 tasks | 3 files |
| Phase 08 P03 | 25min | 2 tasks | 2 files |
| Phase 10 P01 | 12 min | 2 tasks | 4 files |
| Phase 10 P03 | 15 min | 1 tasks | 2 files |
| Phase 10 P02 | 9 min | 2 tasks | 4 files |
| Phase Phase 11 P01 | 5 min | 2 tasks | 4 files |
| Phase 11 P02 | 5 min | 2 tasks | 8 files |
| Phase 11 P03 | 10 min | 3 tasks (resume) | 5 source + 3 docs |
| Phase 12 P01 | 5 min | 3 tasks | 5 files |
| Phase Phase 12 P02 | 6 min | 2 tasks tasks | 4 files files |
| Phase 12.1 P01 | 4 min | 3 tasks tasks | 4 files files |
| Phase 13 P01 | 5 min | 2 tasks | 3 files |
| Phase 13 P02 | 24 min | 3 tasks | 10 files |
| Phase 14 P01 | 23 min | 3 tasks tasks | 3 source + 2 docs files |
| Phase 14 P02 | 8 min | 2 tasks | 2 files |
| Phase 15 P1 | 95 min | - tasks | - files |
| Phase 15 P2 | 42 min | - tasks | - files |
| Phase 16 P01 | 10 min | 3 tasks | 6 files |
| Phase 16 P02 | 38 min | 2 tasks tasks | 5 files files |
| Phase 16 P03 | 9 min | 2 tasks tasks | 3 files files |

## Decisions

- [Roadmap]: v1.13 phase order is dependency-locked (data → config API → sessions backend → UI → hardening) with two research-mandated co-phasing rules: sweepOrphanTmux fix ships WITH the tmux migration (Phase 13); status-feed widening ships WITH the spawn path (Phase 15).
- [Roadmap]: GVIEW-02/GVIEW-03 map to Phase 15 (backend) not 16 — their observable behaviors (agent runs in root cwd, 409 on concurrent spawn, bash shell options) are API-testable and land with the spawn path; the tab UI is covered by GVIEW-01 in Phase 16.
- [Roadmap]: Phase 17 owns no requirements — it closes E2E verification gates for GCONF-04, GSESS-02/03, GINT-02/03 and the sentinel-leak invariant (research P2/P5 verification halves).
- [Phase ?]: Phase 07 / Plan 01: Extracted bridge.call as the shared response-handling helper (chosen YES over inline — halves per-tool line count for the 13 Phase 07 tools; structurally enforces the every-tool-is-the-same-shape invariant)
- [Phase ?]: Phase 07 / Plan 01: list_projects InputSchema declared workspace_id (integer, optional) as the ONLY property; stale project_id no-op arg REMOVED per 07-RESEARCH Pitfall 6
- [Phase ?]: Phase 07 / Plan 02: Widened bridge.call from HTTP 200-only to full 2xx range — Kamacu POST returns 201 (Created), DELETE returns 204 (No Content); 200-only would have surfaced every successful create_task / delete_task as an error to the agent
- [Phase ?]: Phase 07 / Plan 02: move_task InputSchema exposes ONLY task_id + status (D-06); handler marshals {status, after_id: nil} unconditionally so Kamacu's MIN(position)-1.0 top-of-column path is the only one taken
- [Phase ?]: Phase 07 / Plan 02: update_task uses *string pointer fields (Title, Description) per D-03 — nil = omitted (leave untouched), "" = explicit clear (legal for description); body map built conditionally so omitted keys never reach Kamacu's partial-PATCH
- [Phase ?]: Phase 07 / Plan 03: create_project body marshals {name, repo_path, repo} unconditionally and workspace_id only when non-nil (D-04) - Kamacu's strings.TrimSpace(req.Repo) != empty check is the canonical dispatch; the bridge sends every field as-is with ZERO type detection
- [Phase ?]: Phase 07 / Plan 03: update_project args struct has NO WorkspaceID and NO AgentID fields (D-03 / 07-RESEARCH Pitfall 3) - workspace transfer has its own tool in a later plan; agent reassignment is Out of Scope (MCPMORE-01). Double-lock ensures excluded fields never reach Kamacu
- [Phase 07]: Phase 07 / Plan 04: registerWorkspaceTools owns move_project_to_workspace even though the handler PATCHes /api/projects/{id} (NOT a /api/workspaces route). D-07 per-resource ownership tracks the resource being acted on (transferring a project INTO a workspace), not the route being called. — Cross-route resource ownership — the D-07 split tracks the resource acted on. move_project_to_workspace is conceptually a workspace-management action even though it touches the project route.
- [Phase 07]: Phase 07 / Plan 04: update_workspace uses *string pointer for the lone Name field (D-03 single-field rename). InputSchema declares name as optional (only workspace_id is required). When name is omitted the body is {} and Kamacu returns 400 "nothing to update" verbatim; when supplied (incl. explicit "") the rename triggers. — D-03 single-field rename pattern — pointer field distinguishes omitted (nil) from supplied (incl. ""), matching Kamacu's PATCH *string decode.
- [Phase ?]: [Phase 08 / Plan 01]: sessionDetail embeds session.Info so every existing JSON tag flows through unchanged; taskTitle/projectName/agentName ride as additional top-level fields (omitempty for dev sessions) — D-10 JOIN wired into list + get_session
- [Phase ?]: [Phase 08 / Plan 01]: subscribe drains the Attach replay (D-02) when include_history is absent — one <-q read atomically removes the replay; streams application/octet-stream (not text); duration clamped to [1s, 300s] server-side (T-08-03); defer Detach on every return path (SC3)
- [Phase ?]: [Phase 08 / Plan 01]: Type-level read-only contract (D-14) enforced — handlers consume ONLY Info/Snapshot/Attach/Detach/Done; scoped grep gate proves zero references to the PTY-write primitive and zero references to WS FrameData
- [Phase ?]: [Phase 08 / Plan 02]: withRecover(name, h) wraps every session tool handler closure so a panic becomes a returned non-nil error rather than unwinding through the SDK's tools/call dispatch (server.go:753 has no recover — Phase 06 Open Q1 / Pitfall 2 closed). Named returns (result, err) are REQUIRED so the deferred recover can overwrite them. — Top-level helper (not a method) so Plan 03 subscribe_session_output reuses it unchanged
- [Phase ?]: [Phase 08 / Plan 02]: listSessions D-13 orphan filter degrades to passthrough on ANY shape surprise (not a JSON array, orphaned not bool, marshal failure) — never errors. The orphan filter is best-effort shape cleanup; a malformed Kamacu body still reaches the agent verbatim rather than breaking the tool. — Preserves Kamacu's SPA behavior intact while guaranteeing the MCP contract (every listed id is operable)
- [Phase ?]: [Phase 08 / Plan 02]: Type-level read-only contract (D-14) enforced — internal/mcp/sessions.go imports ONLY stdlib + SDK; performs only HTTP GETs; scoped grep gate (grep -c 'internal/session' sessions.go == 0) is green; no PTY-write primitive and no WS FrameData type are in scope. — The bridge speaks HTTP only; the session engine package never appears in the import graph
- [Phase ?]: [Phase 08 / Plan 03]: subscribeClient is a package-level *http.Client{} with NO Timeout — the SDK handler ctx is the cancellation mechanism (D-11 / Pitfall 1). Reusing b.client (10s timeout at bridge.go:53) would silently kill every >10s tail. Subscribe diverges from the Phase 06/07 bridge pattern for the first time.
- [Phase ?]: [Phase 08 / Plan 03]: Exit marker via follow-up GET (08-RESEARCH Open Q2 option b) over a trailing JSON line on the stream (option a). Option (b) avoids sentinel-byte/length-prefix framing of the raw application/octet-stream (which would complicate the D-08 envelope contract), reuses the existing Plan 01 GET /api/sessions/{id}, and costs one extra loopback GET per subscribe — acceptable for v1.11 single-user localhost.
- [Phase ?]: [Phase 08 / Plan 03]: D-01 cancel-before-attach returns emptySubscribeEnvelope + nil (NOT a transport error). D-01's 'returns ONE CallToolResult' holds even when zero bytes were streamed because the ctx was cancelled before subscribeClient.Do succeeded. The envelope is a valid zero-byte D-08 result.
- [Phase ?]: [Phase 08 / Plan 03]: SC3 contract is on the HANDLER's return value, not what CallTool surfaces to the client. go-sdk@v1.6.1's own Example_cancellation shows CallTool returns (nil, context.Canceled) to the client when the client's ctx is cancelled — the handler still runs to completion and returns its partial result internally. TestSubscribe_CancelledViaContext wraps subscribeSessionOutput in a recorder to observe the handler's actual return — the load-bearing SC3 assertion.
- [Phase 10]: [Phase 10/Plan 01]: 5min TTL (mergedClosedTTL) for the merged/closed reviews cache, fully decoupled from the 60s review-column cacheTTL (D-03/D-04). Coupling onto the 5s-poll hot path would 3x the gh load for data the column never renders. — Mirrors Service.Get/cacheTTL=60s with a separate map+TTL; the gate-ladder (in-flight dedup + attemptFloor + drop-after-N) is reused verbatim.
- [Phase 10]: [Phase 10/Plan 01]: is:closed SEARCH QUALIFIER is the authoritative merged+closed filter (GitHub docs #5599); --state closed is belt-and-suspenders only since cli/cli #8102 is a filed unfixed bug. — Verified against local gh 2.82.0 + cli/cli #475/#8102. A future gh fix to #8102 would silently drop merged PRs if we relied on the flag alone.
- [Phase 10]: [Phase 10/Plan 01]: classifyGhListError extracted as a PURE helper shared by listPRs + listCompletedReviews (Pattern 3) — one classification path, no drift on exit-code-4 + stderr-substring sniff (cli/cli#9338). — Pure (no I/O, no logging) so unit-testable directly; callers slog.Debug the state only (T-10-03: never log stderr body).
- [Phase ?]: [Phase 10 / Plan 03]: parseActivityTime is the shared mismatched-precision ISO parser (tries ms layout '2006-01-02T15:04:05.000Z' then bare-Z) — consumed by buildDurationSlices here AND plan 10-02's reviews-window cutoff; never string-compare timestamps across precisions. The .000Z layout requires the fraction, so the bare-Z fallback is what makes one comparison correct for reaper ms timestamps vs gh second-precision closedAt.
- [Phase ?]: [Phase 10 / Plan 03]: buildDurationSlices implements the Open Q3 dwellInProgress semantic (USER-CONFIRMED) — in_review_at set -> dwellInProgress = inReview - inProgress; absent -> fallback done - inProgress. cycle is always done - inProgress; dwellInReview is always done - inReview.
- [Phase ?]: [Phase 10 / Plan 03]: activity_helpers.go is stdlib-only (fmt/sort/strconv/strings/time) — no *sql.DB, no internal/github — so it ran as a wave-1 sibling parallel to plan 10-01. Negative-grep gate (internal/github==0, database/sql==0) is the purity contract; all gh/DB coupling lives in plan 10-02's handler.
- [Phase 10]: [Phase 10 / Plan 02]: GET /api/activity is ONE combined endpoint ({tasks,reviews,stats}) so Phase 11 issues ONE TanStack query with ONE loading state; degradation rides in reviews.state and the whole response is always 200 (D-01, REVIEWS-04). GATE 1 (github_integration != 'on') short-circuits to reviews.state='disabled' with zero gh spawns before any aggregateReviews call.
- [Phase 10]: [Phase 10 / Plan 02]: aggregateReviews uses errgroup (SetLimit 5, promoted indirect→direct v0.20.0) with per-repo 12s timeout; closures return nil on EVERY outcome so a degrade rides in the per-repo state and NEVER cancels the group (Pitfall 4). The reviews WINDOW FILTER compares time.Time-vs-time.Time via parseActivityTime — never a string compare across the ms cutoff vs second-precision gh closedAt (REVIEWS-01/STATS-01). Two cutoff representations (cutoffStr ms-ISO for lexical SQL, cutoffTime time.Time for reviews) derive from one now.Add(-window).
- [Phase Phase 11]: Split Task 1 (tdd=true) into 3 commits: non-TDD parts (types+query) first, then RED→GREEN for formatDuration — keeps TDD discipline clean for the behavior-specified function — Only formatDuration has a behavior block (TDD candidate); wire types and query are declarations/glue code (Skip TDD per tdd.md guidance). Splitting avoids forcing type declarations into a test commit.
- [Phase ?]: [Phase 11 / Plan 02]: Per-child collapsed-rail visibility — removed SidebarFooter's wholesale group-data-[collapsible=icon]:hidden and re-applied it per-child (Add project + Settings gear keep hiding; Activity stays reachable) rather than lifting Activity into a separate sidebar menu. Generalizes the project-row dual-surface idiom to footer utility buttons and satisfies D-02 (Activity always reachable in both states).
- [Phase 11]: [Phase 11 / Plan 03]: Non-nil-slice wire contract — Go nil slices marshal to JSON null, violating the ActivityTask[]/ReviewDoneSummary[] contract the frontend iterates without null guards (empty-instance black page). Canonical fix is backend `make([]T, 0)` / `[]T{}` on every path (happy-empty, query-error, gate-disabled, settings-error, empty-repos, all-filtered); frontend `?? []` guards are defense-in-depth. The regression test asserts on RAW body bytes (`"tasks":[]` / `"prs":[]`) because json.Unmarshal accepts null for a slice — a decoded `len==0` check cannot catch a nil-slice regression.
- [Phase 12]: shadcn add chart resolved recharts to ^3.8.0 (not ^3.10.1 as RESEARCH projected) — same v3 major, peer-declares react ^19.0.0; the D-02 sole-new-dep contract is satisfied (the before/after package.json diff showed recharts as the ONLY new entry). The specific patch version within v3 is not load-bearing.
- [Phase 12]: recharts SUS verdict from package-legitimacy seam overridden (false positive — keyed on latest-version publish date 2026-07-25, not the package's 2015-08-07 creation date; 49M weekly downloads, 11-year history, shadcn's own chart block depends on it). No checkpoint:human-verify needed; documented in the plan's T-12-SC threat entry.
- [Phase 12]: Pre-existing npm run lint errors (29 errors, primarily react-hooks/set-state-in-effect in TaskPage.tsx) are OUT OF SCOPE for this frontend chart-tooltips phase — chart.tsx and StatsStrip.tsx both lint clean in isolation. Logged to deferred-items.md for a dedicated cleanup task.
- [Phase Phase 12]: [Phase 12 / Plan 02]: ZeroDayStub passed as a function reference (shape={ZeroDayStub}) rather than an element — RESEARCH documents both as equivalent; the function form is type-safe against recharts BarShapeProps (the element form would require Partial weakening). Same D-05 zero-day-stub behavior.
- [Phase Phase 12]: [Phase 12 / Plan 02]: bucketByDay(now) defaults to Date.now() internally; ActivityChart does not pass now — buckets recomputed every render from useActivity data, so scope/window change rebuckets without a new fetch (Phase 10 D-01).
- [Phase ?]: [Phase 12.1 / Plan 01]: scopeResolved computed via useMemo over [activeWorkspaceId] (not the audit state+effect snippet) — avoids the react-hooks/set-state-in-effect lint-debt class; effective Activity scope computed via useMemo over [savedScope, workspaces, projects] mirroring useActiveWorkspace.tsx:57-66 so D-06 (do not persist the demotion) is structurally satisfied (the useMemo returns the literal global without calling any setter)
- [Phase ?]: [Phase 13 / Plan 01]: Two-migration split (00017 plain transaction + 00018 NO TRANSACTION rebuild) — keeps the FK-off discipline scoped to the one migration that needs it; the exact split the research rehearsal validated
- [Phase ?]: [Phase 13 / Plan 01]: global_task seed via SELECT 1, id FROM agents WHERE is_default = 1 (never a literal id) — the default flag is movable on real installs; FK ON DELETE RESTRICT is the DB backstop behind the 13-02 handler guard
- [Phase ?]: [Phase 13 / Plan 01]: tmux scope XOR CHECK spelling (task_id IS NULL) = (scope = 'global') with both rejection directions test-asserted — inverted or OR spellings fail loudly or protect nothing
- [Phase 13 / Plan 02]: Sweep fix shipped as the research-verified subquery form (WHERE scope='global' OR task_id IN (SELECT id FROM tasks)); the LEFT-JOIN equivalent deliberately not used (planner resolution) — Both shapes verified equivalent on real data; the subquery form is what the research validated
- [Phase 13 / Plan 02]: The agents delete-guard 409 message is count-free ('reassign the Scratchpad agent first') — the singleton references exactly one agent, always; D-09 locked string, the 00017 FK RESTRICT stays the backstop
- [Phase 13 / Plan 02]: TestBackfillAgents wipe simulation updated to drop the global_task singleton first — under 00017 the raw agent DELETE is correctly refused by ON DELETE RESTRICT; BackfillGlobalTask re-arms the row next boot
- [Phase ?]: Phase 14/02: real-binary curl smoke isolates via env (HOME=sandbox + temp --db + free 127.0.0.1 port) rather than containers — every ~-relative resolution follows HOME, which is the exact fresh-install production contract
- [Phase ?]: Phase 14/02: the managed/clone PUT variant is excluded from the curl smoke (seam-covered by 14-01 fakes; a live gh clone would be network-flaky) — reattach/no-reclone is instead proven in-package by TestPutGlobalConfigHistory
- [Phase ?]: Phase 14/02: SC3's settable half demonstrated with agent_id=2 (opencode seed) so the agent summary observably changes engine — a same-id no-op PUT could not prove the JOIN reflects the write
- [Phase ?]: [Phase 15 / Plan 01]: wrapInputForWrite delimiters fixed to the xterm spec (ESC[200~/ESC[201~) — the original ESC[2004~/ESC[2014~ confused the ?2004 MODE number with the paste DELIMITERS, leaking a stray '~' into readline command lines (every plain-bash input-endpoint submission parsed as '~<cmd>'); exposed by the cwd-proof test, TestInput_Happy was latently red
- [Phase ?]: [Phase 15 / Plan 01]: deriveGlobalState/globalLiveBlockers disjoint-by-name resolution of OQ1 — KindAgent counts only under live.agent, TmuxName-empty bash only under live.bash, minted tabs only under live.tmux; blockers dedupe engine sessions against tmux rows by name so each live surface appears once
- [Phase ?]: [Phase 16 / Plan 01]: ApiErrorReason exported as a named interface on client.ts so 16-03's Change-root dialog imports the exact {kind, target} shape it renders verbatim (D-45 mechanics — additive field, single-message callers untouched)
- [Phase ?]: [Phase 16 / Plan 01]: GlobalConfig.agent.engine typed as plain string mirroring the Go wire (claude|custom|opencode) — the stale types.ts Agent.engine union deliberately NOT widened (Pitfall 2; deferred to Phase 17)
- [Phase ?]: [Phase 16 / Plan 01]: GINT-01/GVIEW-01 left unchecked in REQUIREMENTS.md — this plan ships the wire/data halves per its success criteria; the requirements close when 16-02 lands the /global route + Phase 17 UAT
- [Phase 16]: 16-02: AgentTab task-branch worktree metadata reads the shared ["task", id] cache via useTask (NaN-disabled on global) — keeps the locked AgentTabScope shape and scope/description/seed prop surface; global branch symmetrically reads useGlobal() (research Pattern 4) — The plan locks the scope type and documents the prop surface without naming a worktree-prop source; the cached-query read preserves byte-for-byte task behavior (TaskPage resolved the query before mounting the tab) with zero new fetches
- [Phase 16]: 16-02: keepExited accumulator uses adjust-state-during-render (AddProjectDialog prevOpen tracker, research Pitfall 8) instead of TaskPage's effect idiom — new files must lint clean in isolation — The copied wholesale effect trips react-hooks/set-state-in-effect, violating the plan's own eslint-clean acceptance gate; the guarded render-phase union is behaviorally identical and converges
- [Phase 16]: 16-03: Clear root placed in the DialogFooter with sm:mr-auto (destructive variant, left of Cancel/Save) — resolves the UI-SPEC planner-discretion placement to the footer-left slot
- [Phase 16]: 16-03: the 409 error box is one element (message + reasons ul) rendered below the active input for both Save-root and Clear-root failures — keeps the exactly-one-of help/error idiom while guaranteeing clear-root 409s surface since the dialog stays open on failure

## Operator Next Steps

- Plan Phase 13: `/gsd-plan-phase 13` (research flagged: `--research-phase` for the tmux table-rebuild rehearsal)

## Accumulated Context

### Roadmap Evolution

- Phase 12.1 inserted after Phase 12: Address Activity tech debt (WR-01..03) (URGENT)
- v1.13 roadmap created 2026-08-25: Phases 13–17, 19/19 requirements mapped
