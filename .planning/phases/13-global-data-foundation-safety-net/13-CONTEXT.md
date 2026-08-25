# Phase 13: Global data foundation & safety net - Context

**Gathered:** 2026-08-25
**Status:** Ready for planning

<domain>
## Phase Boundary

The DB + startup invariants that let task-free global sessions exist, landed so an existing v1.12 install upgrades byte-for-byte: the `global_task` singleton migration + idempotent boot backfill (GDATA-01), the `tmux_sessions` scope rebuild making `task_id` nullable with an explicit scope discriminator + XOR CHECK (GDATA-02), and the scope-aware `sweepOrphanTmux` fix shipped in the same phase as the migration (GDATA-03 — the roadmap co-phasing mandate), plus the agents delete-guard extension via FK `ON DELETE RESTRICT`.

This phase ALSO locks the v1.13 decisions that bake into schema/path formats and later-phase contracts (namespace, validation, naming, gate semantics) even though their user-facing delivery lands in Phases 14–16. No API routes, no frontend, no spawn paths in this phase.

</domain>

<decisions>
## Implementation Decisions

### Carry-Forward Locked (research-locked — do not reopen)

- **Representation:** `global_task` singleton table + task-free "global scope" sessions — NEVER a sentinel project/task row, NEVER `TaskID = 0` (REQUIREMENTS Out-of-Scope; research P2/P4).
- **Co-phasing mandate:** the sweep fix ships WITH the tmux migration in this phase — migration alone kills every live global tmux tab at next startup.
- **Storage:** singleton table only — settings-KV rejected (loses FK integrity, mixes config with state).
- **tmux rebuild technique:** CREATE-copy-drop-rename under `PRAGMA foreign_keys=OFF` / `NO TRANSACTION` (00012/00013 discipline, one notch heavier). Whether this is one migration or two (e.g., 00017+00018) is a PLAN-PHASE detail — explicitly deferred, do not re-ask the user.
- **Zero new dependencies** (verified against `go.mod` / `web/package.json`).

### Managed-root namespace (bakes into the stored path format now; clone itself is Phase 14)

- **D-01: Separate namespace.** The gh-cloned global root lives in its own namespace — structurally unrepresentable collision with a project's gated `os.RemoveAll` delete (research P8). No cross-entity dedup 409 guards anywhere.
- **D-02: Path format `~/.kamacu/repos/global/<owner>/<name>`.** Sits inside the existing `repos/` tree; `global` reads as a reserved pseudo-owner. Minimal new surface — no second top-level data dir.
- **D-03: Same repo as project + global is allowed.** The same `owner/name` may exist as a managed project clone AND the global root simultaneously — separate entities by design, zero extra guard code, disk cost is the user's explicit choice.
- **D-04: Reconfiguring never deletes the old clone.** When the root moves away from a managed clone, the clone stays on disk — so re-configuring the same repo later reattaches for free (Phase-14 SC2). No gated-removal code in the global path.

### Folder-root validation (validator lands in Phase 14; strictness locked now)

- **D-05: Git repo required.** Folder-root validation reuses `validateRepoPath` semantics (current project behavior) — a git repo is the recovery story (checkout/reset) for a skip-permissions agent with no worktree isolation. Matches STACK + PITFALLS leans.
- **D-06: Persistent un-isolation banner.** When the root is a folder (vs managed clone), the global view shows a one-line persistent notice ("agent runs directly in \<root\> — no worktree isolation"). This is the P3 safety element — deliberately DISTINCT from the deselected GT-FUT-01 (root-path legibility line) and GT-FUT-02 (git-status summary).
- **D-07: Block obvious footguns.** `$HOME` / `~` / `/` (and equivalents) are rejected with a clear 400 at configure time — one equality check against the P3 worst case.
- **D-08: Spawn-time honesty for a vanished root.** Config validates once at PUT; if the dir disappears later, spawn fails with an honest error (mirrors project folder-path behavior — validated at creation, failures surface at use). No boot-time revalidation.

### UI naming (label strings start emitting in Phase 15; decided once, early)

- **D-09: User-facing name is "Scratchpad".** Avoids the collision with v1.12 Activity's "Global" scope-selector label. All user-facing copy (Settings section, view title, "Open Scratchpad" affordance) says Scratchpad.
- **D-10: Synthesized label strings: `projectName:"Global"`, `taskTitle:"Scratchpad"`.** The bar/status row reads "Global · Scratchpad" — scope + name, no duplication; keeps the TS wire contract non-nullable.
- **D-11: Internal naming stays `global` everywhere.** `scope:"global"`, `/global` route, `global_task` table, `kamacu-global-*` tmux mint, `repos/global/` namespace — the label is presentation-only; zero churn against the research architecture.
- **D-12: Bar-row visual treatment settles at UAT (Phase 17).** Research says cosmetic; strings are locked now, visuals (globe badge vs text-only) deferred.

### "Live" definition for the reconfigure gate (gate enforces in Phase 14; semantics locked now)

- **D-13: Any live global PTY blocks.** Agent + plain-bash + tmux tabs all count as live for the GCONF-04 409 gate (one `ListGlobal()`-nonempty check). A cwd change under a running shell is a lie waiting to happen.
- **D-14: Exited sessions never block.** Exited PTYs and persisted resume ids do not gate — only live processes. On success the resume ids are cleared (they point at conversations in the OLD root).
- **D-15: 409 shape is a reasons list.** `{error, reasons:["agent session running", "2 bash tabs running"]}` — mirrors the existing gated-delete 409 grammar (managed project delete, worktree cleanup) app-wide.
- **D-16: Resume ids clear on ANY successful root change.** Folder↔repo, or clearing — uniform GCONF-04 contract; no no-op-re-PUT exception branch.

### the agent's Discretion

- Migration split shape (one combined migration vs 00017+00018) — explicitly a plan-phase detail per research.
- The exact scope-discriminator column shape (`scope TEXT` vs `global INTEGER`) and column names/granularity for resume ids in the singleton — GDATA-01/02 lock the behavior, the researcher/planner picks the schema spelling.
- The sweep known-set query rewrite (LEFT JOIN vs UNION of global rows) — implementation detail of GDATA-03.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### v1.13 milestone research (architecture-locked — the source of every carry-forward decision)
- `.planning/research/SUMMARY.md` — recommended architecture (Option C: singleton + scope flag), the 3 killer risks, phase-1 decision list, resolved/researcher-disagreement gaps
- `.planning/research/ARCHITECTURE.md` — the three-representation evaluation, the four patterns to follow, component map
- `.planning/research/PITFALLS.md` — P2 (sentinel leak), P3 (no worktree safety net), P5 (restart amnesia — this phase's core risk), P7 (reconfigure gate), P8 (managed-clone path collision)
- `.planning/research/STACK.md` — zero-new-deps verification; tmux rebuild technique; settings-KV rejection
- `.planning/research/FEATURES.md` — parity principle (scratch = first-class citizen, not degraded task)

### Milestone planning artifacts
- `.planning/REQUIREMENTS.md` — v1.13 requirements GDATA-01..03 (this phase), GCONF/GVIEW/GSESS/GINT (context), Out-of-Scope table, GT-FUT deferrals
- `.planning/ROADMAP.md` §Phase 13 — goal, success criteria, co-phasing mandates, research flag (tmux table-rebuild rehearsal)
- `.planning/PROJECT.md` — milestone scope + settled-decision histories for v1.4 (managed clone), v1.9 (migration+backfill pattern), v1.10 (agent FK pattern)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/store/migrations/00012_workspaces.sql` + `00013_agents.sql` — the column-add + in-SQL backfill precedent this phase's singleton migration mirrors (seeded default row, FK RESTRICT, protected flag)
- `internal/store/migrations/00005_tmux_sessions.sql` — the table being rebuilt (`task_id INTEGER NOT NULL REFERENCES tasks(id)`, `UNIQUE(task_id, n)`, `label`, `name UNIQUE` — all must survive byte-for-byte)
- `internal/api/workspaces.go:29` (`BackfillWorkspaces`) and `internal/api/agents_backfill.go` (`BackfillAgents`) — the idempotent boot-backfill hook pattern `BackfillGlobalTask` mirrors; wired once right after `store.Migrate(db)`
- `cmd/kamacu/serve.go:351` (`sweepOrphanTmux`) — the sweep being made scope-aware; its known-set query is `SELECT ts.name FROM tmux_sessions ts JOIN tasks t ON t.id = ts.task_id` (INNER JOIN — the exact line that would kill global tabs)
- `internal/api/projects.go` `validateRepoPath` — the folder-root validator whose semantics D-05 reuses (Phase 14 wiring)
- v1.4 managed-clone sequence (`internal/github` `ParseRepoRef`/`ValidateRepo`/`Clone` + createByRepo ordering) — what Phase 14 adapts into `repos/global/`; this phase only bakes in the path format

### Established Patterns
- Migration-first phase shape (v1.9 Phase 25 / v1.10 Phase 01): schema + backfill + invariants, curl/test-verifiable, no UI
- Idempotent startup backfill guaranteeing an invariant on every boot (never rely on migration-only seeding)
- Gated-delete 409 grammar `{error, reasons:[...]}` — D-15's template
- Degrade-don't-break for optional machinery (sweep is already best-effort warn-only)
- Goose embedded migrations; the `PRAGMA foreign_keys=OFF` + `NO TRANSACTION` rebuild discipline (proven at 00012/00013 — this phase goes one notch heavier per the research flag)

### Integration Points
- `store.Migrate(db)` → new migration(s) 00017+; boot wiring in `cmd/kamacu/serve.go` gains `BackfillGlobalTask`
- The sweep's known-set query (serve.go:373) — must become scope-aware (global rows known WITHOUT a task JOIN)
- Agents delete path — extend the in-use check to include the `global_task.agent_id` FK (FK RESTRICT is the DB backstop)
- `~/.kamacu/repos/global/` — new on-disk namespace (Phase 14 writes it; no code in this phase)

</code_context>

<specifics>
## Specific Ideas

- Bar/status row renders exactly `Global · Scratchpad` (scope slot + name slot — not duplicated).
- The folder-mode banner copy should say the agent runs directly in the root with no worktree isolation; it may include the root path even though the GT-FUT-01 legibility line was deselected (the banner is a safety warning, not decoration).
- Blocking set for footgun paths: `$HOME`, `~`, `/` (and path-equivalents at validation time).

</specifics>

<deferred>
## Deferred Ideas

- Bar-row visual treatment (globe badge vs text-only) — settles at UAT in Phase 17 (D-12).
- GT-FUT-01..07 remain parked in REQUIREMENTS.md (promotion, multiple scratchpads, per-workspace scratchpads, MCP write parity, sidebar entry, root-path line, git-status line).

</deferred>

---

*Phase: 13-Global data foundation & safety net*
*Context gathered: 2026-08-25*
