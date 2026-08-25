# Phase 14: Global config API - Context

**Gathered:** 2026-08-25
**Status:** Ready for planning

<domain>
## Phase Boundary

The configuration API for the global scratchpad — `GET/PUT /api/global` in a new `internal/api/global.go`: reading the config plus derived live state, configuring the root as a validated local folder (GCONF-01) or a gh-validated managed clone into `~/.kamacu/repos/global/<owner>/<name>` (GCONF-02), setting the default agent (GCONF-03), and the live-session 409 gate on root change/clear with resume-id clearing on success (GCONF-04). Curl-verifiable backend only — no spawn path (Phase 15), no frontend (Phase 16).

This phase exercises the Phase-13 deferred decisions (13-CONTEXT D-01..D-16: namespace, reattach, validation strictness, gate semantics) — they are locked inputs, not open questions.

</domain>

<decisions>
## Implementation Decisions

### Carry-Forward Locked (from 13-CONTEXT — do not reopen)

- **Namespace:** managed global root = `~/.kamacu/repos/global/<owner>/<name>` (D-01/D-02); same repo as project + global allowed, no cross-entity guards (D-03); reconfiguring never deletes the old clone, same-repo re-PUT reattaches (D-04).
- **Folder validation:** `validateRepoPath` semantics — git repo required (D-05); `$HOME`/`~`/`/` rejected 400 (D-07); no boot revalidation — a vanished root fails honestly at spawn time (D-08).
- **Gate semantics (GCONF-04):** any live global PTY blocks — agent + plain-bash + tmux (D-13); exited sessions and persisted resume ids never block (D-14); 409 shape `{error, reasons:[...]}` mirroring the app-wide gated-delete grammar (D-15); resume ids cleared on ANY successful root change, no no-op-re-PUT exception (D-16).
- **Atomicity (SC2):** v1.4 `createByRepo` 8-step ordering with UPDATE swapped for INSERT — validate before clone, clone before row; a failed clone leaves no half-configured row or directory.
- **Agent delete-guard:** already landed in Phase 13 (`agents_crud.go:274` 409 + FK RESTRICT backstop) — SC3's "cannot be deleted" half is done; this phase ships the SETTABLE half.

### GET wire shape (new)

- **D-17: Config + live truths.** `GET /api/global` returns the stored singleton (root_path, github_repo, agent_id, updated_at) PLUS derived state: `root_exists` (cheap `os.Stat` at GET time — honest about a vanished root without violating D-08's no-boot-revalidation), live global session counts, and an embedded agent summary (id, name, engine). One round-trip serves Phase 16's Settings section and the `/global` view's honest unconfigured state (GVIEW-04).
- **D-18: Resume ids internal only.** `claude_session_id` / `opencode_session_id` NEVER appear on the GET wire — they are Phase-15 spawn-path internals (the server constructs `--resume`; the frontend never needs the ids).
- **D-19: Ship the count fields now as zeros.** The live-session count fields ship in the Phase 14 wire shape (`{agent:0, bash:0, tmux:0}` until Phase 15's spawn path exists) — the wire contract is stable from day one, and the 409 gate check is forward-wired the same way (trivially passes until global sessions can exist).

### PUT grammar & clear (new)

- **D-20: Single partial PUT.** One `PUT /api/global` with pointer fields `root_path` (*string), `repo` (*string), `agent_id` (*int64) — omitted = untouched (the app-wide partial-PATCH pointer idiom). Root dispatch mirrors `create()`: non-empty `repo` → managed-clone variant; non-empty `root_path` → folder variant; both supplied → 400; `root_path:""` is the clear (not a folder variant).
- **D-21: `root_path:""` clears the root.** Explicit empty string resets `root_path` → `''` AND `github_repo` → `NULL`, behind the same live-session 409 gate as any root change, resume ids cleared on success (D-16 uniform).
- **D-22: Field names `root_path` + `repo` + `agent_id`.** `root_path` matches the 00017 column (GET and PUT share one name); `repo` matches the v1.4 create dispatch field exactly; `agent_id` as everywhere.
- **D-23: PUT returns the GET shape.** Updated config + root_exists + counts + agent summary — one wire type for the endpoint; curl shows the clone/reattach outcome immediately.

### Default-agent change (new)

- **D-24: Agent change allowed anytime — no 409.** A live global session keeps the agent it was spawned with; the change applies at the next Start. Exactly the per-project pattern (`projects.go:616` — existence-validated 400, no session gate, read-at-use at spawn). Root gating exists because a cwd change under a running shell is a lie; an agent-default change mutates nothing under a live session.
- **D-25: Agent change never touches resume ids.** D-16 (clear on root change) stays the ONLY clearing path — ids are engine-keyed, not agent-keyed; a claude id sitting unused while opencode is the default is harmless, and switching back restores the resumable conversation.

### Folder-overlap policy (new)

- **D-26: Any valid git folder allowed.** No project-overlap guard — the folder branch validates git-repo + D-07 footguns, nothing else. Mirrors D-03's no-cross-entity-guards stance: the global root claims no ownership (reconfigure never deletes; folder-project deletes never touch the dir), a shared checkout is the user's explicit choice.
- **D-27: Block paths inside `~/.kamacu` (400).** One HasPrefix check on the expanded data-dir path with clear copy — everything under it is Kamacu-managed machinery (a task worktree gets gated-removed by cleanup; a managed clone belongs to a project), so a global root there is always a trap, never a choice. Extends the D-07 footgun block in the same spirit.

### the agent's Discretion

- Clone-failure status code (v1.4 `createByRepo` uses 500 + one inline error — mirroring it is the default).
- Empty-body `PUT {}` behavior (the workspaces "nothing to update" 400 is the precedent).
- Same-value re-PUT of `agent_id` (no-op success is fine — no gate applies).
- Exact JSON spelling of the agent summary and count fields (D-17/D-19 fix the content, not the key names).
- Whether the reattach check reuses `reattachManaged` verbatim or a thin wrapper (it is exactly the same semantics).
- How the forward-wired live check queries (in-memory manager surface vs tmux `has-session`) — Phase 15's `ListGlobal()` lands the real thing; Phase 14 wires the gate so Phase 15's spawn path makes it bite.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` — GCONF-01..04 (this phase), the Out-of-Scope table, GT-FUT deferrals; mapping note: GCONF-01..04 are API-level (curl-verifiable), the Settings UX is Phase 16's GCONF-05
- `.planning/ROADMAP.md` §Phase 14 — goal, success criteria 1–4, dependency on Phase 13's singleton

### Milestone research (architecture-locked)
- `.planning/research/SUMMARY.md` — §"Phase 2: Global config API" (v1.4 sequence with INSERT swapped for UPDATE), §Gaps (namespace + validation strictness — both resolved), P7/P8 carry-over
- `.planning/research/ARCHITECTURE.md` — component 1 (`internal/api/global.go` NEW — config GET/PUT, folder validation, managed-clone lifecycle, gated root switch/clear)
- `.planning/research/PITFALLS.md` — P3 (no worktree safety net — validation/warning posture), P7 (reconfigure gate), P8 (managed-clone path collision — solved by the namespace)

### Phase-13 decisions this phase exercises
- `.planning/phases/13-global-data-foundation-safety-net/13-CONTEXT.md` — D-01..D-16, every one a locked input here (namespace, reattach, validation strictness, gate semantics, clearing)

### Code-level precedents (read in-repo)
- `internal/api/projects.go` — `validateRepoPath` (:78), `createByRepo` 8-step atomic ordering (:357), `reattachManaged` (:456), partial-PATCH pointer idiom + agent_id existence validation (:495/:616), hard-block messages (:482)
- `internal/store/migrations/00017_global_task.sql` — the singleton's storage contract (root_path/github_repo semantics documented in-line)
- `.planning/PROJECT.md` — v1.4 settled decisions (clone-then-create atomicity, reattach-on-origin-match, degrade-don't-break clone failures)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/api/projects.go:78` `validateRepoPath` — the folder validator D-05 reuses verbatim (~ expansion, abs check, dir check, `git rev-parse --git-dir`); D-07/D-27 footgun blocks wrap it
- `internal/api/projects.go:361` `createByRepo` — the 8-step ordering SC2 adapts (ParseRepoRef → ValidateRepo → dest → reattach-or-409 → clone → row); swap INSERT for UPDATE on the singleton
- `internal/api/projects.go:456` `reattachManaged` — reattach-on-origin-match semantics, exactly what D-04's "re-configuring the same repo reattaches" needs (git repo check → origin read → case-insensitive canonical compare → reuse, never clobber)
- `internal/github` — `ParseRepoRef`/`ValidateRepo`/`Clone`/`Available`/`RepoDescription` + the `msgRepoNotFound`/`msgGHUnavailable` degrade copy (:482)
- `internal/api/agents_crud.go:274` — the global_task agent delete-guard (409 "reassign the Scratchpad agent first") already shipped in Phase 13
- `settings.ExpandHome(reposBase)` — data-dir expansion for both the `repos/global/` namespace and the D-27 HasPrefix block
- The gated-delete 409 grammar (`{error, reasons:[...]}` — `deleteManaged`, worktree cleanup) — D-15's template

### Established Patterns
- Clone-then-create atomicity: validate BEFORE any clone, clone BEFORE any row — failed clone → belt-and-braces remove, ONE inline error, no row
- Partial-PATCH pointer decode: nil = omitted (untouched), "" = explicit clear — D-20/D-21 ride this idiom
- Handler-file-per-resource (`internal/api/global.go` NEW per research) + route registration in `routes.go`
- Degrade-don't-break gh messaging (not-found vs no-gh copy chosen by `github.Available()`)
- Read-at-use agent resolution — spawn reads the FK, so config changes need no restart (D-24's foundation)

### Integration Points
- `routes.go` — register `GET /api/global` + `PUT /api/global`
- `global_task` singleton (00017) — UPDATE root_path/github_repo/agent_id; `root_path=''` + `github_repo=NULL` = unconfigured
- Session manager — the forward-wired live-global check behind the 409 gate (trivially empty until Phase 15's `ListGlobal()`); tmux side reads `scope='global'` rows / `kamacu-global-*` names
- `~/.kamacu/repos/global/<owner>/<name>` — the on-disk namespace this phase starts writing

</code_context>

<specifics>
## Specific Ideas

- A curl PUT→GET round-trip should tell the whole story: `PUT {"repo":"owner/name"}` → response shows the cloned root + agent summary; `PUT {"root_path":""}` while a session is live → 409 with reasons.
- Agent-change semantics copy for Phase 16: "applies at the next Start" — a running session is never affected.
- The `~/.kamacu` block message should say why (Kamacu-managed machinery gets cleaned up), not just "invalid path".

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope. GT-FUT-01..07 remain parked in REQUIREMENTS.md (promotion, multiple scratchpads, per-workspace scratchpads, MCP write parity, sidebar entry, root-path line, git-status line).

</deferred>

---

*Phase: 14-Global config API*
*Context gathered: 2026-08-25*
