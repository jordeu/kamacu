---
gsd_state_version: 1.0
milestone: v1.5
milestone_name: Sharper Review Column
status: shipped
stopped_at: v1.5 Sharper Review Column shipped 2026-06-17 — audited (passed), archived, tagged. No milestone active; run /gsd:new-milestone.
last_updated: "2026-06-17T12:30:00.000Z"
last_activity: 2026-06-17
progress:
  total_phases: 1
  completed_phases: 1
  total_plans: 2
  completed_plans: 2
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-17)

**Core value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.
**Current focus:** Planning next milestone — run `/gsd:new-milestone`

## Current Position

Milestone: v1.5 Sharper Review Column — ✅ SHIPPED 2026-06-17
Phase: none active
Status: Milestone complete — audited (passed), archived, tagged

Progress: [██████████] 100% — 1 phase (16), 2 plans, 6 tasks

**v1.5 shipped:** The Review column now carries two independent at-a-glance signals per PR — your agent's session state (a colored left rail: green working / pulsing-amber waiting / blue idle / gray exited) and the PR's CI state (a bare glyph: green check / red cross / static amber circle; none renders nothing) — split onto separate visual channels so they never confuse, plus a "Recently reviewed" section (`reviewed-by:@me`, approve or request-changes; server-deduped, quietly omitted when empty) that keeps reviewed PRs visible until they merge or close. Milestone-time redesign: the planned agent dot became a colored left rail at the human-verify gate (a dot beside the CI glyph clashed). Audit: 9/9 requirements, 4/4 integration seams, 3/3 E2E flows. Archived to `milestones/v1.5-{ROADMAP,REQUIREMENTS,MILESTONE-AUDIT}.md`.

**Next:** `/gsd:new-milestone` to scope the next cycle.

**Carried tech debt (non-blocking, build green):** ~18–20 pre-existing react-hooks eslint advisories; the v1.3 `Date.now()`-in-render advisory in `PRCard.tsx` was cleared in v1.5 (the `ReviewColumn.tsx` one may remain); plus a cosmetic stale "dot/border" comment in `ReviewColumn.tsx` / the `agentRail` JSDoc. A dedicated lint/comment-sweep is the right home.

## Performance Metrics

**Velocity (v1.3):**

- Plans completed: 19 across 4 phases (10–13); 46 tasks
- Headline: clicking a PR opens it as a full task-like review workspace on the PR's real head branch; reaper auto-cleans merged/closed worktrees

**Velocity (v1.2):**

- Plans completed: 12 across 3 phases (7, 8, 9)
- Headline: tmux sessions survive a server restart and auto-reattach invisibly; first background goroutine (Done-TTL reaper)

| Phase | Plans | Notable |
|-------|-------|---------|
| 6 | 4/4 | P01 11 min / P02 24 min / P03 8 min / P04 25 min (incl. human gate) |

Historical per-plan timings preserved in `.planning/milestones/` archives and git history.
| Phase 07-claude-quota-indicator P02 | 5 min | 3 tasks | 5 files |
| Phase 07 P01 | 14 min | 3 tasks | 5 files |
| Phase 07-claude-quota-indicator P03 | 1h 8m | 2 tasks | 1 files |
| Phase 08-tmux-shells-spawn-detach-lifecycle P01 | 6min | 3 tasks | 3 files |
| Phase 08-tmux-shells-spawn-detach-lifecycle P02 | 8 min | 2 tasks | 4 files |
| Phase 08-tmux-shells-spawn-detach-lifecycle P03 | 9 min | 3 tasks | 3 files |
| Phase 08-tmux-shells-spawn-detach-lifecycle P04 | 52min | 3 tasks | 4 files |
| Phase 09 P01 | 3 min | 2 tasks | 3 files |
| Phase 09-tmux-restart-resume-cleanup-integration P02 | 4min | 2 tasks | 5 files |
| Phase 09-tmux-restart-resume-cleanup-integration P03 | 14 min | 3 tasks | 11 files |
| Phase 09-tmux-restart-resume-cleanup-integration P04 | 14 min | 3 tasks | 6 files |
| Phase 09-tmux-restart-resume-cleanup-integration P05 | 12 min | 3 tasks | 5 files |
| Phase 10-github-foundations P01 | 7 min | 2 tasks | 5 files |
| Phase 10-github-foundations P02 | 9 min | 3 tasks | 5 files |
| Phase 10-github-foundations P03 | 8 min | 3 tasks | 7 files |
| Phase 10-github-foundations P04 | 4 min | 1 tasks | 3 files |
| Phase 10-github-foundations P05 | 3 min | 3 tasks | 3 files |
| Phase 11-pr-review-column P01 | 6 min | 2 tasks | 4 files |
| Phase 11-pr-review-column P02 | 5min | 2 tasks | 3 files |
| Phase 11-pr-review-column P03 | 3min | 3 tasks | 4 files |
| Phase 11-pr-review-column P04 | ~10 min active (overnight human-verify gate) | 3 tasks | 2 files |
| Phase 12-open-a-review P01 | 6 min | 2 tasks | 4 files |
| Phase 12-open-a-review P02 | 7min | 2 tasks | 3 files |
| Phase 12-open-a-review P03 | 4min | 1 tasks | 2 files |
| Phase 12-open-a-review P04 | 12min | 1 tasks | 3 files |
| Phase 12-open-a-review P05 | 18min | 2 tasks | 7 files |
| Phase 12-open-a-review P06 | 7min | 3 tasks | 9 files |
| Phase 12-open-a-review P07 | ~3h (incl. 3 human-verify rounds) | 4 tasks | 7 files |
| Phase 13-pr-worktree-auto-cleanup P01 | 9 min | 3 tasks | 11 files |
| Phase 13-pr-worktree-auto-cleanup P02 | 9min | 3 tasks | 5 files |
| Phase 13-pr-worktree-auto-cleanup P03 | 2min active (+ human-verify gate) | 3 tasks | 3 files |
| Phase 14-managed-checkout-foundations P01 | 13 min | 3 tasks | 6 files |
| Phase 14-managed-checkout-foundations P02 | 9 min | 2 tasks | 4 files |
| Phase 14-managed-checkout-foundations P03 | 7 min | 2 tasks | 4 files |
| Phase 14-managed-checkout-foundations P04 | 7 min | 2 tasks | 2 files |
| Phase 15 P01 | 5 min | 2 tasks | 7 files |
| Phase 15-repo-first-creation-flow P02 | 3 min | 2 tasks | 2 files |
| Phase 16 P01 | 10 min | 3 tasks | 9 files |
| Phase 16-sharper-review-column P02 | ~40 min | 3 tasks | 4 files |

## Accumulated Context

### Decisions

Full decision log lives in PROJECT.md (Key Decisions) and the archived milestone files:

- `.planning/milestones/v1.0-ROADMAP.md` / `v1.0-REQUIREMENTS.md`
- `.planning/milestones/v1.1-ROADMAP.md` / `v1.1-REQUIREMENTS.md`
- `.planning/milestones/v1.2-ROADMAP.md` / `v1.2-REQUIREMENTS.md`
- `.planning/milestones/v1.3-ROADMAP.md` / `v1.3-REQUIREMENTS.md`

Notable standing decisions for future work:

- D-51 reversed in v1.1 (AGENT-02): `--dangerously-skip-permissions` is the default extra-param; removable per-settings. Documented side effect: amber waiting dot rarely fires while active.
- Settings are global-only, read-at-use, absent-row-=-code-default; per-project overrides deferred (SET-FUT-01); additional shells are future data, not code (SHELL-FUT-01).

v1.4 milestone-time decisions (settled with the user before roadmapping — treat as constraints going into planning):

- **Repo cloning uses `gh repo clone`** (host auth, so private/org repos work) into an `owner/name`-namespaced dir under `~/.kangent/repos/` — collision-safe. GitHub-only for the repo path (consistent with v1.3); arbitrary git URLs / other forges stay out of scope — the folder is the non-GitHub escape hatch.
- **The managed clone IS the repo root** that task/PR-review worktrees branch off (the user just never picks a folder). A **new schema marker** distinguishes Kangent-managed checkouts from user-pointed folders so delete/cleanup knows what it owns — this needs a migration (**00008**, next after v1.3's 00007), landed in Phase 14 so Phase 15 needs no further migration.
- **Repo-first when on, folder-only when off:** with `github_integration` on, "Add project" defaults to `owner/name`; folder becomes the optional alternative. With the toggle off, creation is folder-only exactly as before v1.4. Existing folder-based projects are untouched (backward compatible).
- **Freshness:** before each new task worktree on a managed checkout, fetch the latest default branch so new work starts from latest.
- **Gated delete:** deleting a managed-checkout project removes the clone, gated like worktree cleanup (dirty / unpushed / stash / running-session) via the v1.3 shared `CleanupWorktreeGated`; folder-based dirs are never removed (cleanup is local-directory-only; the app never mutates the remote).
- **Degrade-don't-break provisioning:** clone failures (auth/network/bad repo) surface inline with no half-created project (no orphan row, no partial dir); re-adding an existing managed dir reattaches/reuses rather than re-cloning.
- **Builds directly on v1.3's `internal/github`** (gh auth, `ParseRepoRef`/`ValidateRepo`, `Available`, the project `github_repo` link) and the existing worktree provisioning + gated cleanup — an internally-focused milestone, no new external surface beyond `gh repo clone`.

Standing v1.3 decisions still relevant to v1.4 (managed-checkout worktrees ride the same machinery):

- **Model a PR review as a `tasks` row with `source='github_pr'`**; the board excludes them via `WHERE source='manual'`. v1.4's CKOUT-02 freshness/branching must keep this guard intact for PR-review worktrees off the managed clone.
- **Never use `gh pr checkout`** — use `git fetch <remote> refs/pull/<n>/head:<localBranch>` + `git worktree add`; the fetch is a scoped exception to worktree's "never fetch" invariant. v1.4's per-new-task default-branch fetch is a second deliberate, scoped fetch.
- **`CleanupWorktreeGated`** (force=false) is the shared gated-removal path (dirty + unpushed via `rev-list` + stash + sessions); v1.4's managed-clone delete reuses these gates.
- **Global `github_integration` toggle** (settings KV, default `on`), read-at-use in API gating and via `useSettings()` in the frontend; when off the app is byte-for-byte unchanged. v1.4 gates the repo-first Add-project UI on this same value (RPROJ-04).

(Earlier v1.0–v1.3 per-phase decisions are preserved in the archived milestone files and PROJECT.md Key Decisions.)

- [Phase 14-managed-checkout-foundations]: Schema marker is a single boolean-as-INTEGER managed column (D-06), not an enum or path-prefix derivation — the column is the single source of truth for dir ownership; path derivation is a data-loss hazard.
- [Phase 14-managed-checkout-foundations]: github.Clone uses a package-level cloneRunner var as its test seam (Clone is a package function like ValidateRepo); exit-0-only success + os.RemoveAll on failure + trimmed stderr.
- [Phase 14-managed-checkout-foundations]: Phase 14 sets github_repo=canonical at create-by-repo INSERT (research OQ1) so Phase 15's form relies on it.
- [Phase 14-managed-checkout-foundations]: create-by-repo tests use github package-level validateRunner/availableRunner seams + exported SetXForTest setters so they are deterministic regardless of host gh.
- [Phase 14-managed-checkout-foundations]: CKOUT-02: provisionWorktree does a managed-only, best-effort (error-discarded, D-05) default-branch fetch via worktree.DefaultBranch+FetchRef immediately before ResolveBase; folder projects skip it (D-24 preserved). The git read lives in worktree.DefaultBranch, not the api layer.
- [Phase 14-managed-checkout-foundations]: CKOUT-03 gated delete: projectHandlers.delete branches on the managed marker — folder (managed=0) delete is byte-for-byte unchanged (dir never touched, D-09); managed (managed=1) runs a two-pass all-or-nothing gate (dirty/unpushed/stash/sessions over every task+PR worktree AND the clone root), 409 {reasons:[{kind,target}]} on any blocker (removes nothing), and on all-clear removes linked worktrees first then os.RemoveAll(clone) then FK-ordered rows (204).
- [Phase 14-managed-checkout-foundations]: Managed-delete unpushed gate base is origin/<default>..HEAD with NO fetch (network-free, conservative); one base shared by the clone root + all task worktrees (they branch off it); an unresolvable default branch is a conservative blocker. Clone root removed only via os.RemoveAll, never git worktree remove (refuses the main worktree, exit 128).
- [Phase 15]: RPROJ-02 description capture rides a DEDICATED github.RepoDescription(ctx, canonical) string (best-effort, error-free public signature: gh-absent/empty/nonzero/parse-fail all map to ""), NOT a widening of the shared ValidateRepo — its (ctx, ref) (canonical, verified, err) signature stays so the PATCH update handler is untouched.
- [Phase 15]: createByRepo persists the captured description into projects.description at the repo-first INSERT (folder-create INSERT unchanged); description is server-side-only at create (D-05), surfaced editable later in Project settings, never a field in the Add dialog. Frontend: useCreateProject body widened to { name?; repo_path?; repo? }; Project type gains managed: boolean.
- [Phase 15-repo-first-creation-flow]: RPROJ-01/03/04: Add-project dialog gains an integration-gated 'GitHub repo' | 'Local folder' segmented toggle (repo default) reusing ui/tabs.tsx as a controlled segmented control; folder-only byte-for-byte when github_integration is off (repo branch never mounts).
- [Phase 15-repo-first-creation-flow]: RPROJ-02/CKOUT-04: repo mode prefills the editable Name from a pure local owner/name parse (nameEdited guard, no gh call, D-09), submit POSTs { repo } with a blocking 'Cloning <owner/name>…' Loader2 spinner; failures surface in the mirrored destructive-alert box, dialog open, values+mode preserved, no half-created-project copy (Phase-14 atomicity). web/dist rebuilt; only index.html committed (hashed assets gitignored).
- [Phase 16]: Refactored runGH into listPRs(ctx, repo, repoDir, search): one gh-list primitive both review queues share; fetchLists runs both searches in one cache cycle and degrades both together (D-12); dedupeReviewed drops reviewed PRs whose number is in awaiting (top precedence, D-14)
- [Phase 16]: Result widened with reviewed array sharing state/stale/fetchedAt; repoEntry caches both lists (cachedAwaiting/cachedReviewed + hasCache) under one TTL/floor; /api/agents/status entries gain prNumber/source via SELECT widening (no migration — columns from 00007), D-15
- [Phase 16-sharper-review-column]: 16-02 checkpoint redesign (APPROVED): agent state on PR cards is shown by a 3px colored LEFT RAIL via agentRail() (working green / waiting amber-pulse / idle blue / exited gray), NOT a StatusDot — the dot was dropped at the human-verify gate because it clashed with the line-art CI glyph beside it. Rail = open session; color = agent state; the waiting pulse moved onto the rail. CI is a bare lucide glyph (green Check / red X / static amber Circle; none renders nothing). The two signals split by channel: left edge = your agent, right glyph = the PR's CI. This unifies SIGNL-01+SIGNL-02 onto one control.

### Pending Todos

- Quota poll while idle (no connected browsers) is acceptable for the first iteration with jitter + backoff; revisit before milestone close (research tech-debt note).
- Lint-cleanup pass: ~18–20 pre-existing react-hooks eslint errors + two v1.3 `Date.now()`-in-render advisories (`PRCard.tsx`/`ReviewColumn.tsx`) — gating build green, but a dedicated pass is the right home.
- Phase 14 open product questions to resolve at plan time: exact schema-marker shape (a `managed`/`source` flag on `projects` vs a separate column distinguishing clone-owned from user-pointed paths); whether reattach (CKOUT-05) validates the existing dir's remote matches `owner/name` before reusing; how a gated-blocked managed-clone delete surfaces to the user (mirror the worktree CleanupWorktreeDialog variants).

### Blockers/Concerns

- `gh` is a soft dependency; provisioning must degrade-don't-break on every failure mode (missing/unauthenticated/network/bad-repo). CKOUT-04's "no half-created project" guarantee (no orphan row, no partial dir) is the milestone's highest-severity correctness item — treat clone as a transaction: validate (RPROJ-05) → clone to a staging/namespaced dir → only then commit the project row; roll back the dir on any failure.
- The CKOUT-03 gated delete must never remove a folder-based (user-pointed) directory — the schema marker is the single source of truth for "Kangent owns this dir." A missing/incorrect marker check is a data-loss risk (deleting a user's working checkout).
- Plan-mode exit-plan approval → amber dot (v1.0 research OQ1): still unobserved — carry to v1.4 UAT.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260613-osu | Warn when a linked GitHub repo cannot be verified (surface verify_state) — completes GHPRJ-03 soft-save-with-warning | 2026-06-13 | 41f3d50 | [260613-osu-warn-when-a-linked-github-repo-cannot-be](./quick/260613-osu-warn-when-a-linked-github-repo-cannot-be/) |
| 260613-ph5 | Make GitHub repo-link validation MANDATORY (hard-block invalid repos with highlighted error) — supersedes 260613-osu's soft verify_state advisory; reverses D-11 for the repo-link UX per user decision | 2026-06-13 | 9ea0df6 | [260613-ph5-make-github-repo-link-validation-mandato](./quick/260613-ph5-make-github-repo-link-validation-mandato/) |
| 260616-8l7 | Fix Review-column refresh showing stale RED dots: `reduceChecks` now dedupes superseded check runs (keeps the latest run per check name, matching GitHub's rollup state) so a re-run/concurrency-cancelled FAILURE no longer paints a green PR red. Initial cache attempt-floor diagnosis was wrong and discarded (service.go unchanged). Code + tests done; Task 3 human-verify pending (user verifies against live instance). | 2026-06-16 | ccdb2d5 | [260616-8l7-the-refresh-button-at-review-column-seem](./quick/260616-8l7-the-refresh-button-at-review-column-seem/) |

## Session Continuity

Last session: 2026-06-17T10:00:32.703Z
Stopped at: Completed 16-02-PLAN.md — phase 16 plans complete (2/2), ready for phase verification
Resume file: None
Next: `/gsd:plan-phase 14` — Managed Checkout Foundations (CKOUT-01, RPROJ-05, CKOUT-02, CKOUT-05, CKOUT-03); migration 00008 schema marker is the foundation
