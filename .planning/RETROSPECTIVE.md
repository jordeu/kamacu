# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.0 — MVP

**Shipped:** 2026-06-11
**Phases:** 5 | **Plans:** 28 | **Tasks:** 74

### What Was Built
- Single Go binary serving an embedded React SPA at localhost: projects sidebar, four-column kanban board, task CRUD, SQLite persistence (Phase 1)
- Server-owned PTY session engine with ring-buffer replay, binary WebSocket bridge, xterm.js terminal, detach/reattach, and loopback-only security (Phase 2)
- Worktree-per-task isolation (`task/<slug>-<id>` under `~/.kangent/worktrees/`), bash tabs inside the worktree, gated confirmed cleanup that always keeps the branch (Phase 3)
- Real `claude` CLI in the worktree PTY via Start button, invisible additive status hooks, dispatcher board with status dots and sidebar waiting chips (Phase 4)
- Restart reconciliation (zero schema change), `claude --resume <uuid>` recovery, and a read-only merge-base diff tab (Phase 5)

### What Worked
- **Risk-front-loaded roadmap.** Phase 2 (the PTY/WebSocket session manager — the only subsystem with no off-the-shelf solution) was built and proven against plain bash before anything depended on it. Every later phase reused that proven stack instead of discovering its risk late.
- **Empirical phase research over training-data assumptions.** Each phase verified version-specific behavior against the actually-installed binaries (git 2.43, claude v2.1.170→173). This caught real surprises before planning: claude does NOT use the alt-screen buffer (the feared reattach problem evaporated), `--settings` overlays merge additively, `--resume` reuses the same session ID (no DB mutation needed), the worktree branch-leak footgun, and the `git diff --no-index` exit-1-as-success quirk.
- **Interface-first wave parallelism.** Plans that published exact Go signatures / REST contracts in their `<interfaces>` blocks let backend and frontend tracks run fully parallel with disjoint file ownership — both waves of most phases ran two agents at once with no merge conflicts.
- **Headless-Chrome pre-checks before every human checkpoint.** The orchestrator drove the real app in headless Chrome before handing each checkpoint to the user — this caught a tree-crashing `TooltipProvider` bug (blank page) and the collapsed-sidebar overlap in Phase 1 before the user wasted time, and pre-verified the diff tab in Phase 5.
- **fake-claude stub for CI.** Agent-session tests never touched the real claude binary (no API quota, deterministic), while the real binary was reserved for the human checkpoint.

### What Was Inefficient
- **Stray verification servers blocked the user's port.** Twice (Phases 2 and 5 setup) an orchestrator/executor verification instance was left holding port 7333, blocking the user's own `./bin/kangent`. Fixed by an explicit "kill any server before returning" instruction to checkpoint executors — should be a standing rule from the start.
- **Smoke tests gave false confidence.** Phase 1's restart-persistence smoke test passed while the UI was a blank page (the Tooltip crash) because it only asserted HTTP responses, not browser rendering. Pure-HTTP verification is necessary but not sufficient for UI phases.
- **Parallel-executor commit interleaving.** Wave-parallel agents occasionally raced on commits (an `--amend` collided with a sibling's commit; `go.mod` ownership was ambiguous between two plans). Recovered each time, but tightening `files_modified` ownership and avoiding `--amend` under parallelism would prevent it.
- **Shell cwd drift.** The orchestrator's shell repeatedly reset to `web/` or a temp dir, causing several `gsd-tools` calls to fail with "not found" until re-run from the repo root. Absolute paths or explicit `cd` guards would have avoided the churn.

### Patterns Established
- **The `TabDef[]` seam.** Phase 1 shipped the task-view tab strip as a data-driven array with just a Description tab; Phases 2–5 appended Agent, Bash, and Diff tabs to the same seam with zero rework. Designing the extensibility point in the first phase paid off four times.
- **Reserved color budget.** Phases 1–3 deliberately kept the palette neutral so status color (Phase 4's amber/green/gray dots) and diff color (Phase 5's green/red) could own their meaning. The UI-SPEC inheritance chain enforced this across phases.
- **Checkpoint-feedback loop is real design input.** Phase 4's exited-agent UX was reworked from user feedback (dimmed terminal, code-free banner, "Reset session" not "Start again"), and the CONTEXT/UI-SPEC docs were updated to match so the change persisted — user feedback overrode the previously-approved spec cleanly.
- **Decisions persisted by ID (D-NN).** Every discuss-phase decision got a stable ID referenced verbatim in plan actions and grep-able acceptance criteria, making decision fidelity checkable rather than aspirational.

### Key Lessons
1. **Verify external-tool behavior empirically, per version, before planning** — training-data assumptions about claude/git cost nothing to check and the checks repeatedly changed the plan (alt-screen, --resume, --settings merge, diff exit codes).
2. **Build the riskiest, least-off-the-shelf subsystem first** against a stand-in (bash before claude) so the rest of the project composes proven parts.
3. **For UI phases, drive a real browser before declaring done** — HTTP/test-level green does not mean the page renders.
4. **Make checkpoint executors clean up their own servers/processes** — a standing teardown rule, not a per-checkpoint reminder.
5. **A data-driven extensibility seam in phase 1 compounds** — the tab strip absorbed four later phases without rework.

### Cost Observations
- Model mix: phases planned/executed predominantly on Sonnet (inherit profile); orchestration spanned Fable and later Opus 4.8.
- Notable: wave-parallel execution (2 agents/wave for most phases) roughly halved wall-clock per wave; the fake-claude stub kept agent-session testing at zero API cost.

---

## Milestone: v1.1 — Settings & Polish

**Shipped:** 2026-06-11
**Phases:** 1 | **Plans:** 4

### What Was Built
- SQLite-backed settings KV store (migration 00004) with code defaults (absent row = default), per-key validation carrying the UI-SPEC canonical error copy, branch-template expansion with git ref validation, quote-aware extra-params tokenizer, and the GET/PUT `/api/settings` surface (06-01)
- All four settings wired read-at-use into their existing v1.0 call sites — claude extra-params on every spawn (fresh + resume), LookPath-resolved bash-tab shell, template-driven branches under the settings worktree base — managers stayed DB-free, so "applies at next spawn, no restart" is structural (06-02)
- Full `/settings` page with per-field commit (blur/Enter, Esc revert, 2s Saved flash, Reset to default, verbatim inline server errors), sidebar gear, API-driven shell select, and the UI-01 full-width task header (06-03)
- Phase gate: clean release binary, full suite green across 8 packages, 8/8 verbatim copy audit, restart-persistence smoke, all seven success criteria approved live (06-04)

### What Worked
- **Coarse single-phase milestone.** Twelve requirements in one phase with 4 plans kept planning overhead proportionate to integration-only work — no artificial phase boundaries inside what was really one feature.
- **Integration-over-net-new framing held.** Every setting landed in an existing v1.0 call site; zero new Go modules and zero new npm deps vs the v1.0 tag, verified by diffing manifests against the tag in the gate.
- **Grep-able copy contract.** The UI-SPEC's canonical error strings were asserted verbatim by tests and re-audited at the gate (8/8) — copy fidelity stayed checkable, not aspirational.
- **Backend/frontend wave parallelism again.** Disjoint file ownership let 06-02 and 06-03 run concurrently in the same checkout with interleaved commits and no conflicts.
- **Checkpoint → continuation-agent flow.** The 06-04 gate ran its automated half, returned structured state, and a fresh continuation agent closed the plan after approval — no resume fragility.

### What Was Inefficient
- **One cross-plan test-harness clash.** 06-01's `TestSettingsGetAllDefaults` collided with 06-02's planned seed row; auto-fixed in-flight, but explicit harness/seed ownership in plan frontmatter would have prevented it.
- **gsd-tools key-link checker false negatives.** 5 link checks failed on path-suffix parsing and a regex-escaping bug; each had to be manually re-verified with grep during verification.
- **Checkpoint asked for two things, got one.** The gate requested approval *plus* the carried plan-mode-amber observation; the user replied only "approved", so the carried v1.0 UAT item (research OQ1) remains open. Observations should be separate, explicit questions.

### Patterns Established
- **Settings KV with absent-row-as-default.** No seeded rows; defaults live in code, so new settings need no migration and `Reset to default` is a row delete.
- **Read-at-use settings reads in handlers.** Managers take values via `SpawnOpts`/locals and never import the store — next-spawn semantics by construction.

### Key Lessons
1. **Make checkpoint questions atomic** — a combined "approve + report observation" prompt yields partial answers; carried UAT items need their own explicit question.
2. **Declare test-harness/seed ownership across plans** — parallel plans sharing a test DB schema need the same `files_modified`-style discipline for fixtures.
3. **Diff dependency manifests against the previous tag at the gate** — cheap, mechanical proof of the zero-new-deps discipline.

### Cost Observations
- Single execution session (orchestrated on Fable 5, inherit profile): 6 subagents (~830k subagent tokens) across 3 waves + verifier; 19 commits.
- Notable: wave 2 ran backend and frontend executors concurrently in the shared checkout (harness worktree isolation unavailable); zero conflicts thanks to disjoint ownership.

---

## Milestone: v1.2 — Quota & Resumable Shells

**Shipped:** 2026-06-13
**Phases:** 3 | **Plans:** 12

### What Was Built
- (Phase 7) Claude quota indicator: a DB-free `internal/quota` server proxy of the OAuth usage endpoint behind always-200 `GET /api/usage` (token-keyed cache, 60s TTL, in-flight dedup, 429 backoff, never writes the token), plus a QuotaIndicator + hover popup in both page headers polling every 60s while visible — six-state degradation matrix, zero new deps.
- (Phase 8) Invisible tmux shells: a socket-isolated `internal/tmux` leaf package (`new-session -A`/`has-session`/`kill-session`, `=name` exact-match, 5s timeouts), the identity-only `tmux_sessions` table (migration 00005), a call-time `LookPath` shell dropdown, killer-first `Stop()` with `has-session` exit-vs-detach, and end-to-end spawn wiring with honest 409 errors — tabs indistinguishable from plain bash (× kills).
- (Phase 9) Restart durability: `GET /api/sessions` reconciles surviving tmux rows into auto-reattaching ghost tabs (no Resume button, D-88), cleanup/delete kill tmux before worktree removal + a once-at-startup orphan sweep (`ListSessions`), and the codebase's first background goroutine — a Done-TTL reaper keyed on new per-status timestamps (migration 00006) that kills bash/tmux/agent sessions of long-Done tasks while keeping the agent resumable.

### What Worked
- **Empirical per-version tool research, finally re-tested on a novel-risk subsystem.** `08-RESEARCH.md` verified tmux behavior on the host binary before planning and caught the load-bearing traps: SIGTERM kills only the attach client (the "Stop doesn't stop" trap), bare names prefix-match (`kangent-99` matched `kangent-99-1`), and `has-session` is the only exit-vs-detach discriminator. Building the riskiest subsystem against the real binary first paid off.
- **The phase split isolated risk from payoff.** Phase 8 shipped the durable-session plumbing; Phase 9 added the user-visible reattach. The risky daemonizing-tmux lifecycle landed and was verified before the headline feature rode on top.
- **A mid-stream user reversal propagated cleanly.** The "invisible tmux / × kills" pivot during Phase 8 discussion was reversed across CONTEXT + REQUIREMENTS + ROADMAP in one commit, so researcher/planner never saw the stale "close = detach" wording.
- **DB-derived ghost reconcile generalized.** The Phase 5 agent-reconcile (DB row + liveness probe → ghost entry) carried straight over to tmux with `has-session` swapped for `transcriptExists`.
- **Wave parallelism with disjoint ownership, again.** Phase 9 wave 1 ran 3 executors concurrently in the shared checkout with zero content conflicts.

### What Was Inefficient
- **The gsd-planner truncated ROADMAP.md on commit** (131→44 lines), deleting the milestone structure and Phase 1–8 details — required manual reconstruction mid-milestone. Code was never affected, but it's a recurring planner foot-gun.
- **`phase complete` doesn't maintain the summary markers.** It returns `roadmap_updated: true` but never flips the milestone checklist or the Progress-table rows; Phases 7, 8, and 9 all showed "Not started" until corrected by hand each phase.
- **summary-extract field misfires.** Two plans' "accomplishments" came through as a deviation line and a "Task 1 — reconcile branch" line, needing manual cleanup from MILESTONES.md.
- **Shared-doc commit churn under parallelism.** Concurrent executors swept STATE/ROADMAP/REQUIREMENTS into sibling commits — harmless but muddies attribution.

### Patterns Established
- **Invisible-subsystem principle.** A durability mechanism can hide entirely behind existing UI when reattach is cheap/lossless; diverging from an existing pattern (the agent's explicit Resume) is justified by *cost*, not consistency.
- **Leaf-package-per-external-tool, extended not scattered.** Every tmux verb lives in `internal/tmux`; the orphan sweep added `ListSessions` to it rather than scattering raw `tmux` calls.
- **Decision-locked-then-amended docs in one commit.** A user reversal updates CONTEXT, REQUIREMENTS, and ROADMAP together so no downstream agent reads a contradiction.

### Key Lessons
1. **Verify planning-doc integrity after the planner commits** — a post-commit line-count/structure check on ROADMAP.md catches the truncation before it compounds.
2. **`phase complete` leaves summary markers stale** — manually flip the milestone checklist + Progress-table rows each phase, or they drift silently (they drifted from Phase 7 onward this milestone).
3. **Name the user-visible payoff's phase explicitly across a split** — Phase 8's checkpoint genuinely couldn't demonstrate restart-survival (Phase 9's deliverable); stating that up front turns a "looks unfinished" surprise into an understood boundary.

### Cost Observations
- Orchestrated across ~3 execution sessions (Fable 5 inherit, then Opus 4.8). Phase 9 wave 1 ran 3 executors concurrently; checkpoints used the fresh-continuation-agent flow.
- Notable: real-tmux integration tests (`internal/api` ~80–90s, `internal/ws` ~60s) dominate suite wall-clock; every other package is sub-10s.

---

## Milestone: v1.3 — GitHub PR Review

**Shipped:** 2026-06-14
**Phases:** 4 | **Plans:** 19 | **Tasks:** 46

### What Was Built
- (Phase 10) GitHub Foundations: migration 00007 (all five v1.3 columns + the `github_integration` KV); the degrade-don't-break `internal/github` leaf (`ParseRepoRef`/`ValidateRepo`/`Available`); a gh-gated `/settings` toggle (OFF + un-enableable when `gh` absent, via always-200 `GET /api/github/status`) with a full OFF cascade; a Project settings dialog editing an origin-prefilled, soft-validated `owner/name` link + description.
- (Phase 11) PR Review Column: a self-gating, default-collapsed per-project column listing `user-review-requested:@me draft:false` PRs via ONE cached `gh pr list` call (server-side `statusCheckRollup` reduction, no N+1), behind always-200 `GET /api/projects/{id}/pull-requests`; 60s visibility-paused poll + manual refresh; appended outside the dnd machinery.
- (Phase 12) Open-a-Review — the headline: clicking a PR find-or-creates a `source='github_pr'` task in the same TaskPage/agent/bash/diff shell, on a worktree on the PR's REAL head branch (`fetch refs/pull/<n>/head` + `worktree add -b`, `pr/<n>` collision fallback — never `gh pr checkout`); reattach-not-duplicate; diff vs the PR's own base merge-base; a 5-query `source='manual'` board-leak guard + `/move` 409; fork/colliding-branch PRs open with primary HEAD provably unchanged.
- (Phase 13) PR Worktree Auto-Cleanup: the Phase-9 reaper gains a `reconcilePRsOnce` pass reading `gh pr view --json state`; merged/closed worktrees auto-remove ONLY when pristine + idle (dirty/unpushed/stash/session each skip), branch always kept; the gated logic extracted byte-equivalent into a shared `CleanupWorktreeGated` (HTTP DELETE + reaper, `force=false`); manual cleanup `⋯` item + merged/closed banner.

### What Worked
- **Research-flagged the integration risk center and spiked it before planning.** Phases 12 and 13 carried explicit research flags; `12-RESEARCH.md` proved `gh pr checkout` is NOT worktree-aware (cli/cli#972), fails on `/`-branches (#3231), and fast-forwards fork same-name branches (#8383) — so it was replaced with `git fetch refs/pull/<n>/head` + `worktree add`, verified end-to-end. The riskiest subsystem (PR-branch checkout) was proven against the real `gh`/git before anything rode on it — the v1.0 "build the riskiest part first" lesson, re-exercised on a genuine novel-risk surface.
- **One shared `github.Service` across three phases.** Constructed once in `main.go` and injected into both `PullRequestRoutes` (list cache + PR detail/checkout) and `reaper.NewWithPR` (PRState) via a compiler-enforced `PRStateGetter` seam — one cache, one auth context, no divergent instances. The leaf-package-per-external-tool pattern, extended not scattered (as with `internal/tmux` in v1.2).
- **Byte-equivalent extraction with a regression guard.** `CleanupWorktreeGated` was lifted out of the live HTTP DELETE handler into one helper with two callers, proven unchanged by the existing DELETE tests *before* the reaper became the second caller — the 409/500/204 contract never moved.
- **Human checkpoints as load-bearing design input, again.** Phase 12's checkpoint surfaced 5 follow-ups → gap plans 12-06/12-07 (named-branch over detached HEAD, configurable seed, single-line header, F5 re-hydration); Phase 10's UAT surfaced gh-gated enablement → 10-04/10-05. Each reversal updated PROJECT.md Key Decisions with a dated supersede note.
- **Independent integration audit before completion.** The milestone audit + integration checker re-verified all 6 cross-phase seams against source (`go build` proves the type seams) rather than trusting the per-phase VERIFICATION.md — confirming the board-leak guard, the shared service, and the branch-never-deleted invariant hold across phase boundaries.

### What Was Inefficient
- **The summary-extract one-liner misfire is now a three-milestone recurring bug.** 13-02-SUMMARY's `one_liner` came through as `"1. [Rule 1 - Bug] Unpushed-gate fetch must run in the worktree..."` — a numbered deviation list, not a one-liner — and the milestone-complete CLI dumped all 19 raw plan one-liners (including that garbage) straight into MILESTONES.md, needing manual curation to the intended 4–6 accomplishments. Same class of bug flagged in v1.1 and v1.2.
- **Mid-stream design reversals left stale wording in settled artifacts.** Detached→named-branch (supersedes D-01) and default-expanded→default-collapsed (supersedes D-05/D-06) meant the original plans/UI-SPECs/CONTEXT carried wording the code no longer matched; reconciled with dated supersede notes, but PROJECT.md's "Current Milestone" target list still described `gh pr checkout` after the research had ruled it out.
- **Gap-closure churn.** Phases 10 and 12 each needed unplanned gap plans (10-04/05, 12-06/07) after their human-verify gates — 4 of the milestone's 19 plans were gap closure. Front-loading the gh-gate (10) and the named-branch decision (12) at discuss-time could have folded them into the original plans.
- **Pre-existing react-hooks lint debt kept surfacing, never addressed.** Logged to `deferred-items.md` in both Phase 10 and Phase 12; the green gating build (`tsc -b && vite build`) masks ~18–20 eslint errors, and v1.3 added two more (`Date.now()` in render). Carried as audited tech debt.
- **Phase 10 Progress row drifted to "4/5"** (gap-plan accounting) until corrected at completion — the `phase complete` summary-marker staleness flagged in v1.2 recurred.

### Patterns Established
- **Shared-service-across-phases with an interface seam.** A single external-tool service constructed once and injected into multiple consumers, with a narrow interface (`PRStateGetter`) for the lower-layer consumer (reaper → no import cycle) — integration is compiler-enforced, not asserted.
- **Source-discriminated view shell.** One `TaskPage`/`TaskTabs` branches every PR delta on `task.source==='github_pr'`, leaving the manual-task path byte-for-byte unchanged — the v1.0 `TabDef[]`/source-discrimination seam paying off a fifth time.
- **Conservative multi-gate auto-removal.** Auto-cleanup skips on ANY of dirty / unpushed (`rev-list FETCH_HEAD..HEAD` after a per-worktree re-fetch) / stash / running-session, runs `force=false`, and never deletes the branch — a destructive action defaults to "leave it for the human".

### Key Lessons
1. **Research-flag and spike the integration risk center before planning** — `gh pr checkout`'s worktree-unawareness would have been a late, expensive discovery; proving the `fetch refs/pull/<n>/head` path first made GHREV-05 (primary-checkout safety) a verified property, not a hope.
2. **A single shared service + interface seam beats per-phase re-construction** — one `github.Service` across three phases gave one cache, one auth context, and a compiler-proven seam the audit could confirm had no divergent instances.
3. **Treat the milestone-complete accomplishments list as a DRAFT** — the summary-extract one-liner misfire has now polluted MILESTONES.md three milestones running; always hand-curate (or fix the extractor to reject numbered-list one_liners).
4. **Fold checkpoint-likely reversals into the first plan** — the named-branch and gh-gate decisions were both foreseeable at discuss-time; deferring them cost 4 gap plans across the milestone.

### Cost Observations
- Orchestrated on Opus 4.8 (inherit profile) across ~3 execution sessions over 2 days (2026-06-13 → 06-14); 125 commits (42 `feat`), +5,410/−159 LOC code (51 files), zero new Go modules or npm deps.
- Notable: the milestone audit spawned a dedicated integration checker that re-verified all 6 seams independently against source; gap-closure cycles in Phases 10 and 12 added 4 unplanned plans.

---

## Milestone: v1.4 — Repo-First Projects

**Shipped:** 2026-06-15
**Phases:** 2 | **Plans:** 7 | **Tasks:** 13

### What Was Built
- (Phase 14) Managed Checkout Foundations: migration 00008 `managed` marker (existing folder rows backfill to never-touch); `github.Clone` (`gh repo clone`, exit-0-only, remove-on-failure, faked-runner seam); a second `POST /api/projects` path that gh-validates `owner/name`, clones into `~/.kangent/repos/<owner>/<name>`, and INSERTs `managed=1`+`github_repo` ONLY after exit 0 (atomic — no orphan row/dir), with origin-matched reattach; managed worktrees best-effort-fetch the default branch before `ResolveBase` (folder projects keep the no-network path); an all-or-nothing two-pass gated managed-delete (worktrees + clone root) that removes nothing on any blocker, and never touches folder-project dirs.
- (Phase 15) Repo-First Creation Flow: a dedicated best-effort `github.RepoDescription` (`gh repo view --json description`) captured into `projects.description` at create (degrade-don't-break, `ValidateRepo` signature untouched); the `AddProjectDialog` becomes integration-gated repo-first — a "GitHub repo | Local folder" segmented toggle (repo default, reusing `ui/tabs.tsx`), `owner/name` name-prefill, blocking "Cloning…" spinner, inline destructive-alert failure (dialog open, values preserved, no half-created project); folder mode byte-for-byte and the repo-first UI vanishes when integration is off.

### What Worked
- **Backend-primitive-first, UI-on-top phase split paid off cleanly.** Phase 14 stood up the entire managed-checkout capability (clone, marker, fetch, gated delete) and proved it with 24 real-git tests *before* any UI existed; Phase 15 then drove it through a thin dialog + one small backend touch. The integration audit confirmed the create contract matched with zero field drift — the seam was designed, not discovered.
- **Reuse-before-invent kept the surface tiny.** v1.4 added *zero* new dependencies and *no new `ui/` file*: the segmented toggle reused the existing `tabs.tsx`, the error box reused `ProjectSettingsDialog`'s destructive-alert, the spinner reused the `DiffTab` `animate-spin` idiom, and the gated-delete reused `CleanupWorktreeGated`'s primitives. The UI-SPEC + UI-checker enforced "no new tokens" before a line was written.
- **A small backend touch slotted into an existing atomic path without regression.** The Phase-15 description capture rode *inside* Phase-14's single clone-then-INSERT — the planner deliberately added a *separate* `RepoDescription` rather than widening the shared `ValidateRepo` (which the PATCH handler also calls), so the change couldn't ripple. The audit verified atomicity and the `ValidateRepo` signature both held.
- **The discuss → ui-phase → plan → execute → verify → audit loop ran frictionlessly on a 2-phase milestone.** Coarse granularity (2 phases, 7 plans) matched the scope; the single human-verify gate (15-03) was the only checkpoint and passed first time with no gap-closure cycles — a contrast with v1.3's 4 gap plans.
- **Migration numbering + degrade-don't-break carried over verbatim.** 00008 mirrored 00007's goose up/down; every new `gh` shell (clone, description) degrades to a safe default and never crashes — the v1.3 GitHub-integration discipline transferred directly.

### What Was Inefficient
- **The summary-extract one-liner misfire struck a FOURTH straight milestone.** 15-03's (a human-verify checkpoint plan) `one_liner` extracted as the literal `"Type:"`, and the milestone-complete CLI dumped it straight into MILESTONES.md as a garbage bullet — hand-curated out again. This is now a confirmed, unfixed recurring bug across v1.1–v1.4; the extractor should reject checkpoint-plan / malformed one-liners, or the curation should be built into the CLI.
- **`milestone complete` reported `tasks: 13` while the audit/roadmap had counted 17** — the per-plan task tally the CLI derives differs from the must-have task counts; cosmetic, but worth reconciling so the shipped stats line is consistent.
- **No new lessons surfaced — which is itself a mild signal.** v1.4 was almost pure application of established patterns (good for delivery speed), so the retrospective is thin on novel insight; the interesting risk (clone-then-create atomicity) was already de-risked by Phase 14's tests before the milestone-level audit even ran.

### Patterns Established
- **Backend-capability phase → UI-driver phase**, with the create/IPC contract verified by an integration audit rather than assumed — a repeatable shape for "add a user-facing entry point to an existing engine."
- **Separate-function-over-widen for a shared helper:** when a new caller needs *more* from an existing shared function, add a sibling (`RepoDescription`) rather than widening the shared one (`ValidateRepo`) — keeps the blast radius to the new path.
- **UI-SPEC "registry safety / no new tokens" as a hard gate** on a brownfield design system: the checker verifying `components.json registries: {}` and that the cited primitive already exists kept the dialog dependency-free.

### Key Lessons
1. **Split a user-facing feature into "prove the engine" then "wire the UI"** — Phase 14's 24 real-git tests made Phase 15 a low-risk thin layer, and the audit could confirm the seam rather than hope for it.
2. **Add a sibling, don't widen a shared function** — the `RepoDescription`-vs-`ValidateRepo` call kept a cross-cutting backend touch from rippling into an unrelated handler.
3. **The milestone-complete one-liner list is unreliable enough to treat as always-draft** — four milestones of the same misfire; hand-curation is now a standing step, not a fix-it-later.

### Cost Observations
- Orchestrated on Opus 4.8 (inherit profile) across ~1 day (2026-06-14 → 06-15); 42 commits (9 `feat`), +2,045/−62 LOC code (17 files, excl. embedded dist), zero new Go modules or npm deps.
- Notable: smallest milestone since v1.1 (2 phases, 7 plans, 1 human gate, 0 gap-closure cycles); the independent integration checker again re-verified all seams against source before completion.

---

## Milestone: v1.5 — Sharper Review Column

**Shipped:** 2026-06-17
**Phases:** 1 | **Plans:** 2 | **Tasks:** 6

### What Was Built
- (Phase 16, plan 16-01) Data layer: a second `reviewed-by:@me draft:false` `gh` search riding the existing per-repo TTL cache in ONE cycle (a `PRLists` combined runner, degrade-together), server-side top-precedence `dedupeReviewed`, a widened `{prs, reviewed, state, stale, fetchedAt}` Result, and `pr_number`/`source` joined onto `/api/agents/status` entries (no migration — columns pre-existed from 00007) — plus the matching frontend wire types. Zero visual change in this plan.
- (Phase 16, plan 16-02) Rendering: CI status as a bare lucide glyph (green `Check` / red `X` / static amber `Circle`; `none` renders nothing), agent state as a 3px colored **left rail** (working green / waiting amber-pulse / idle blue / exited gray), and a quiet-omitted "Recently reviewed" subsection reusing the same `PRCard`, server-deduped against the awaiting list.

### What Worked
- **The data→rendering plan split made the wire contract explicit and verifiable.** 16-01 shipped the `reviewed` array + `prNumber`/`source` fields with tests and zero UI; 16-02 consumed them. The integration checker traced all four seams to source (endpoint serializes the whole `Result`, the agent-status SELECT is a real join, `dedupeReviewed` is invoked on the production path) — designed, not hoped.
- **Reuse kept the milestone tiny and low-risk.** The reviewed search cloned the existing `user-review-requested:@me` path (same reducer, same cache `Service`); no new endpoint, no second poll, no new dependency, no migration. The whole milestone concentrated in two frontend files + the `internal/github`/`internal/api` list paths.
- **The human-verify gate did its job — it caught a design problem the contract missed.** The planned reused `StatusDot` dot beside the line-art CI glyph looked wrong on sight; the gate turned that into a clean, user-chosen redesign (agent state → left rail) rather than shipping a clash. The redesign *reduced* element count (one mark per card, two separate channels) and introduced no new token.
- **The checkpoint redesign was absorbed without derailing the flow.** Code, the design contract (16-CONTEXT.md D-04/06/07, 16-UI-SPEC.md), the SUMMARY, and PROJECT.md were all revised to match; re-verification (verifier 10/10 + integration 4/4) ran against the *shipped* rail, not the stale dot — the docs never lied about what shipped.

### What Was Inefficient
- **The clash was foreseeable before implementation.** The UI-SPEC + UI-checker approved a dot-next-to-glyph layout that the user rejected the moment they saw it rendered. A 30-second visual mock (even ASCII, as was used to *choose* the redesign) during ui-phase would have surfaced "filled dot vs line glyph" before a line of code — the design-contract review validated tokens/spacing but not the *gestalt* of two adjacent marks.
- **Post-redesign doc churn touched five files** (CONTEXT, UI-SPEC, PRCard, SUMMARY, PROJECT) to keep the record honest. Worth it, but a sign that locking visual specifics pre-prototype front-loads rework when the gate flips them.
- **Minor record inconsistencies persisted:** 16-02's SUMMARY used `provides:` rather than a `requirements_completed` frontmatter field (the audit had to read the IDs from prose), and stale "dot/border" comments remain in `ReviewColumn.tsx`/`agentRail` JSDoc (flagged non-blocking by the integration checker).

### Patterns Established
- **Agent-state-as-edge-rail:** encode a per-card status as a colored left rail (color = state, pulse for the attention state) when an inline mark would compete with another inline signal. A reusable "two independent signals, two physical channels (edge vs glyph)" card pattern.
- **Revise-the-contract-on-checkpoint-redesign:** when a human-verify gate drives a design change, update CONTEXT.md/UI-SPEC.md (not just the code) and re-run the verifier against the shipped design, so the planning record matches reality.

### Key Lessons
1. **Prototype the *look* of adjacent signals before locking the spec** — token/spacing review passes a layout the eye rejects; a cheap visual mock during ui-phase catches gestalt clashes the checker can't.
2. **A human-verify gate is worth most when it can change the design, not just bless it** — here it converted a flawed spec into a better-shipped result; budget for the redesign + doc revision rather than treating the gate as a rubber stamp.
3. **Keep the design contract authoritative after a redesign** — revise CONTEXT/UI-SPEC/SUMMARY in lockstep with the code so downstream audits verify what actually shipped.

### Cost Observations
- Orchestrated on Opus 4.8 (inherit profile) in ~1 day (2026-06-17); single phase, 2 plans, 1 human-verify gate (which drove the rail redesign), 0 gap-closure cycles. No new Go modules or npm deps; no migration.
- Notable: the smallest milestone to date (1 phase), yet the human gate produced the milestone's defining decision (dot → rail) — the value was in the gate, not the volume.

---

## Milestone: v1.6 — Global Active Sessions Bar

**Shipped:** 2026-06-18
**Phases:** 1 | **Plans:** 2 | **Tasks:** 5

### What Was Built
- (Phase 17, plan 17-01) Backend: the existing `GET /api/agents/status` query widened with a `JOIN projects` on BOTH passes (manager-derived + DB-derived/post-restart) to carry `tasks.title` + `projects.name` on every entry; the Go `agentStatusEntry` struct + the TS `AgentStatusEntry` type gained `taskTitle`/`projectName`. The sole backend change (SBAR-10) — no new endpoint, no migration (columns pre-existed; mirrors v1.5's `prNumber`/`source` widening).
- (Phase 17, plan 17-02) Frontend: `ActiveSessionsBar.tsx` mounted once in `AppLayout` outside `<Outlet/>` (every route). Collapsed = per-state colored counts (reused `dotMeta()`) + total, waiting amber-pulsing only when > 0, quiet "No active sessions" zero state; expanded = a fixed overlay floating UP over content (terminals never reflow) listing live sessions attention-first, each row `project · title` (+ `#n` PR badge) click-through to that task's agent view cross-project, current-row highlighted; collapse persisted in localStorage (default collapsed); live-only filter.

### What Worked
- **Reuse made it a ~300-LOC, two-file milestone.** The bar became the 4th consumer of the one existing 5s `useAgentStatuses()` poll (alongside card dots, agent tab, sidebar chips), pulled status colors from `dotMeta()`, and mirrored `ReviewColumn`'s collapse/overlay/scroll idioms. The only backend change was a JOIN — no endpoint, no migration, no new dependency, no second poll.
- **The data→UI plan split made the wire contract explicit.** 17-01 shipped `taskTitle`/`projectName` with tests on BOTH query passes and zero UI; 17-02 consumed a typed feed. The integration checker traced all 6 seams (json-tag↔TS parity, JOIN on both passes, single poll, shell mount, dotMeta reuse, cross-project nav with the entry's own ids) to source.
- **Front-loaded decisions made research skippable and planning concrete.** discuss-phase locked all four interaction areas (D-01..D-14) and ui-phase produced an approved spec before planning; the planner wrote exact tokens/dimensions into task actions, so the executor made no ad-hoc styling calls and the plan-checker passed on the first iteration.
- **The human-verify gate confirmed the one thing tests can't.** The critical D-03 check (xterm terminal does NOT reflow when the overlay expands/collapses) passed against a live instance — the exact risk the overlay-not-docked decision was made to avoid.

### What Was Inefficient
- **A `--help` probe to a gsd-tools subcommand silently EXECUTED an archive.** `milestone complete --help` has no help handler — the first positional is the version, so it archived a `--help` milestone (garbage `--help-*` files, a bogus MILESTONES entry, a STATE mutation). Caught immediately and fully reverted (tracked files via `git checkout`, untracked via `rm`) because nothing had been committed, but it was avoidable churn.
- **The research gate fired for a milestone with nothing to research.** With CONTEXT.md + UI-SPEC fully pinning the build on existing infra, both the milestone-level and phase-level research prompts were ceremony — answered "skip" correctly, but they're friction for purely-internal reuse work.

### Patterns Established
- **Extend-the-shared-feed, don't add a surface:** when a new view needs existing server state, widen the one cached/polled endpoint and become an Nth consumer (TanStack Query dedupes the shared key) rather than adding an endpoint or a second poll. Keeps cost flat and behavior consistent across consumers.
- **Overlay-not-dock for terminal-adjacent chrome:** UI that can expand near a height-sensitive embed (xterm) should float over content (`fixed`, absolute panel) instead of resizing the layout, so the embed never reflows.

### Key Lessons
1. **Never probe a mutating CLI with `--help` unless help is known to exist** — gsd-tools subcommands treat the first token as a positional arg; verify with a dry-run/echo or read the tool source before invoking, especially for archive/delete/complete verbs.
2. **For internal reuse milestones, the value is in discuss + ui-phase, not research** — locking interaction + visual contracts up front let planning and execution run clean on the first pass; research can be skipped without cost when the build is fully determined by existing assets.
3. **Put the human gate on the property that automation can't prove** — here, "the terminal doesn't reflow" was the whole reason for the overlay decision, and only a live check could confirm it.

### Cost Observations
- Orchestrated on Opus 4.8 (inherit profile), same-day (~1.5h wall, 2026-06-18). 1 phase, 2 plans, 5 tasks, 1 human-verify gate, 0 gap-closure cycles. No new Go modules or npm deps; no migration; ~298 source insertions across 5 files.
- Notable: tied for the smallest milestone (1 phase) and the leanest by surface area — a pure extend-and-reuse milestone whose only self-inflicted cost was a reverted tooling misstep, not implementation rework.

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Phases | Plans | Key Change |
|-----------|--------|-------|------------|
| v1.0 | 5 | 28 | Established the discuss → ui-spec → research → plan → execute → verify loop with human checkpoints; risk-front-loaded roadmap; interface-first wave parallelism |
| v1.1 | 1 | 4 | Coarse single-phase milestone for integration-only scope; checkpoint → fresh continuation-agent flow; manifest-diff-vs-tag as the zero-dep gate |
| v1.2 | 3 | 12 | Empirical tmux research verified on the host binary pre-planning (first novel-risk subsystem since v1.0); invisible-subsystem principle; DB-derived ghost reconcile generalized from Phase 5; post-planner ROADMAP integrity checks |
| v1.3 | 4 | 19 | Research-flagged the integration risk center (`gh pr checkout` worktree-unawareness) and spiked it pre-planning; one shared `github.Service` across 3 phases via a compiler-enforced interface seam; byte-equivalent helper extraction with regression guard; independent integration audit before completion |
| v1.4 | 2 | 7 | Backend-capability phase → UI-driver phase, with the create contract verified by an integration audit; reuse-before-invent kept it zero-new-dep / no-new-`ui/`-file; separate-function-over-widen for a shared helper; smallest milestone with no gap-closure cycles |
| v1.5 | 1 | 2 | Data→rendering plan split with the wire contract verified by an integration audit; reuse-before-invent (cloned the existing `gh` search + cache — no new endpoint/poll/dep/migration); the human-verify gate DROVE a design redesign (agent dot → left rail), not just blessed it; revise-the-contract-on-redesign keeps CONTEXT/UI-SPEC honest |
| v1.6 | 1 | 2 | Extend-the-shared-feed (bar = 4th consumer of the one 5s `/api/agents/status` poll; sole backend change a JOIN — no endpoint/poll/dep/migration); data→UI plan split with all 6 seams integration-audited; overlay-not-dock to protect terminal layout, confirmed by the human gate's no-reflow check; research correctly skipped — discuss + ui-phase fully determined the build |

### Cumulative Quality

| Milestone | Go packages tested | Frontend | Zero-Dep Discipline |
|-----------|--------------------|----------|--------------------|
| v1.0 | 6 (api, diff, session, store, worktree, ws) | tsc + vite build green | Held throughout — only sanctioned deps added (xterm set, dnd-kit, radix collapsible); no go.mod surprises |
| v1.1 | 7 (+settings) | tsc + vite build green | Held — zero new Go modules and zero new npm deps, proven by manifest diff against the v1.0 tag |
| v1.2 | 11 (+quota, tmux, reaper) | tsc + vite build green | Held — zero new Go modules and zero new npm deps across all 3 phases |
| v1.3 | 12 (+github) | tsc + vite build green | Held — zero new Go modules and zero new npm deps across all 4 phases; carried ~18–20 pre-existing react-hooks lint advisories (build green) as audited tech debt |
| v1.4 | 12 (github extended) | tsc + vite build green | Held — zero new Go modules and zero new npm deps; no new `ui/` file (reused `tabs.tsx`); v1.4 added NO new lint debt (dialog eslint-clean, 0 useEffect); pre-existing advisories still carried |
| v1.5 | 12 (github + api list paths extended) | tsc + vite build green | Held — zero new Go modules and zero new npm deps; no migration; cleared the `PRCard` `Date.now()`-in-render advisory (the `ReviewColumn` one may remain); pre-existing react-hooks advisories still carried |
| v1.6 | 11 (api `agents` JOIN extended; full suite green) | tsc + vite build green | Held — zero new Go modules and zero new npm deps; no migration (JOIN onto existing columns); no new `ui/` primitive (reused `dotMeta()`/`ReviewColumn` idioms); pre-existing react-hooks advisories still carried |

### Top Lessons (Verified Across Milestones)

1. **Empirical per-version tool research before planning** — re-confirmed strongly in v1.3 (`gh pr checkout` worktree-unawareness) and again in v1.4 (host-verified `gh repo clone` behavior contradicted the brief's "gh absent" assumption, strengthening the plan). Held across v1.0, v1.2, v1.3, v1.4.
2. **Build the riskiest, least-off-the-shelf subsystem first** — v1.3 (PR-branch checkout) and v1.4 (managed-checkout clone/atomicity proven by Phase 14's tests before the Phase 15 UI rode on it) both confirmed it. Held.
3. **Interface-first wave parallelism with disjoint file ownership** — confirmed across all milestones; zero merge conflicts in any parallel wave. (v1.4's tightly-coupled `projects.go` plans were deliberately run sequentially across waves instead — the same disjoint-ownership discipline, applied by serializing rather than parallelizing.)
4. **A single shared service / leaf package + narrow seam for a cross-phase external tool** — `internal/tmux` (v1.2), `internal/github` (v1.3), and the v1.4 managed-checkout verbs all stayed one leaf package extended-not-scattered; v1.4 added the "separate-function-over-widen" corollary (add `RepoDescription`, don't widen the shared `ValidateRepo`).
5. **The milestone-complete accomplishments list needs hand-curation** — the summary-extract one-liner misfire has now polluted MILESTONES.md in v1.1, v1.2, v1.3, AND v1.4 (15-03 → `"Type:"`); treat the CLI output as a draft every time.
6. **Backend-capability phase → UI-driver phase** (new in v1.4) — proving the engine with tests first makes the UI a thin, low-risk layer the audit can confirm rather than hope for; expect to reuse this shape whenever adding a user-facing entry point to an existing subsystem.
7. **A human-verify gate should be able to change the design, not just approve it** (new in v1.5) — token/spacing review can pass a layout the eye rejects (v1.5's dot-next-to-glyph clash); when the gate flips a visual decision, revise the design contract (CONTEXT/UI-SPEC) alongside the code and re-verify against what actually shipped. A cheap visual mock during ui-phase would catch such gestalt clashes pre-build. (Note: the summary-extract one-liner misfire did NOT recur in v1.5 — both plans produced clean one-liners.)
