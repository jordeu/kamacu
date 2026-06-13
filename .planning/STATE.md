---
gsd_state_version: 1.0
milestone: v1.3
milestone_name: GitHub PR Review
status: executing
stopped_at: Completed 10-05-PLAN.md
last_updated: "2026-06-13T15:39:11.560Z"
last_activity: 2026-06-13
progress:
  total_phases: 4
  completed_phases: 1
  total_plans: 5
  completed_plans: 5
  percent: 80
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-13)

**Core value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.
**Current focus:** Phase 10 — github-foundations

## Current Position

Phase: 11
Plan: Not started
Status: Ready to execute
Last activity: 2026-06-13 — Completed quick task 260613-ph5: mandatory (hard-block) GitHub repo-link validation

Progress: [████████░░] 80% (4/5 plans)

## Performance Metrics

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

## Accumulated Context

### Decisions

Full decision log lives in PROJECT.md (Key Decisions) and the archived milestone files:

- `.planning/milestones/v1.0-ROADMAP.md` / `v1.0-REQUIREMENTS.md`
- `.planning/milestones/v1.1-ROADMAP.md` / `v1.1-REQUIREMENTS.md`

Notable standing decisions for future work:

- D-51 reversed in v1.1 (AGENT-02): `--dangerously-skip-permissions` is the default extra-param; removable per-settings. Documented side effect: amber waiting dot rarely fires while active.
- Settings are global-only, read-at-use, absent-row-=-code-default; per-project overrides deferred (SET-FUT-01); additional shells are future data, not code (SHELL-FUT-01).

v1.3 roadmap-time decisions (from research, treat as settled going into planning):

- **Model a PR review as a `tasks` row with `source='github_pr'` + `pr_number` + `pr_base_ref`, NOT a separate `pr_reviews` table** (forced by the `taskID`-keyed session manager). Board excludes them via `WHERE source='manual'`. This is the milestone's single highest-risk regression: audit EVERY `SELECT ... FROM tasks` for the `source` filter (Phase 12).
- **Never use `gh pr checkout`** — it mutates the current checkout, isn't worktree-aware, and breaks on fork PRs / `/`-branches. Use `git fetch <remote> refs/pull/<n>/head:<localBranch>` + `git worktree add` (fork-safe via the server-side pull ref). The fetch is a deliberate, scoped exception to worktree's "never fetch" invariant (D-24).
- **PR diff base comes from the PR's own `pr_base_ref`, not the project default** — `diff.Compute(wt, base)` is already parameterized, so only the diff handler branches.
- **Review qualifier = `user-review-requested:@me`** (direct requests only; the unprefixed `review-requested:@me` floods with all team requests — ~100 PRs seen). Exclude drafts by default. GitHub auto-removes the user from the set on review submit, so the column self-empties for free — don't over-cache against it.
- **`internal/github` is a best-effort leaf package modeled on `internal/quota`**: per-repo cache with TTL/floor/backoff, typed degraded states (`ok`/`no_gh`/`auth_required`/`disabled`/`error`), always-200 endpoints. `gh` is a soft dependency — degrade, don't break. Don't trust `gh auth status` exit codes alone (cli/cli#8845).
- **Migration 00007** adds `projects.description`, `projects.github_repo` (nullable = not linked), `tasks.source` (default `'manual'`), `tasks.pr_number`, `tasks.pr_base_ref`, and the `github_integration` settings KV (code-default `'on'`). Store the canonical `owner/name` (validated/canonicalized via `gh repo view --json nameWithOwner`).
- **Merge/close detection is server-side in the always-on reaper**, never the browser poll (the poll pauses when hidden and only sees open PRs). Add a `reconcilePRsOnce` pass via a `PRStateGetter` interface (mirror the existing `SessionStopper` seam). `gh pr view --json state` returns UPPERCASE `OPEN`/`CLOSED`/`MERGED`; there is no `merged` field — derive it from state.
- **Gated auto-removal**: only remove a merged/closed PR worktree when the dirty gate AND the sessions gate pass clean (research recommends also gating on unpushed commits `git rev-list <headRefOid>..HEAD` + `git stash list`); deferred-and-re-checked, never immediate. The branch is always kept (D-34). Extract `cleanupWorktreeGated` once — one path, two callers (HTTP + reaper).
- **Global toggle** is a `github_integration` settings KV (default `on`), read-at-use in API gating and via the shared `useSettings()` query in the frontend so all GitHub UI flips atomically. When off, the app is byte-for-byte unchanged.

Open product decisions to resolve in Phase planning (from research, mostly defaulted above):

- Named-branch (`kangent-pr/<n>`) vs `--detach` for the PR worktree — STACK prefers a namespaced branch with collision pre-check, PITFALLS prefers `--detach`. Reconcile in Phase 12 (research-flagged).
- Collapse/count persistence scope (per-project vs global; SQLite vs localStorage) — Phase 11.
- Unpushed-work gate scope beyond `git status --porcelain` — Phase 13 (research-flagged).
- [Phase 10-github-foundations]: github_integration default 'on' (D-01): absent settings row reads as 'on' via existing Get fallback — no Get/GetAll/Set change needed
- [Phase 10-github-foundations]: All five v1.3 columns land in migration 00007 so Phase 12 needs no further migration; tasks.source CHECK(manual/github_pr) default 'manual' gates the board
- [Phase 10-github-foundations]: internal/github leaf (ParseRepoRef/ValidateRepo/Available): pure canonicalization of owner/name + URL/ssh, gh soft-validation that degrades (syntactic save, no error) on absence or unverifiable refs — only syntactic invalidity hard-blocks (GHSET-03/D-11)
- [Phase 10-github-foundations]: PATCH /api/projects/{id} is a partial update (D-13): pointer fields so omitted != clear; description "" clears, github_repo "" unlinks to NULL; canonical hard-error copy 'Not a valid repository — use owner/name or a GitHub URL.'; GET .../github-origin prefills from git origin on dialog open only (D-08)
- [Phase 10-github-foundations]: shadcn ui/* primitives use the radix-ui umbrella import (Switch as SwitchPrimitive from 'radix-ui'), not @radix-ui/react-*; hand-authored switch.tsx to match select.tsx/label.tsx — no new dep
- [Phase 10-github-foundations]: OFF cascade is a render gate on the shared useSettings() github_integration==='on' value; when off the ProjectSettingsDialog re-sends the existing github_repo so the hidden link is never clobbered
- [Phase 10-github-foundations]: Soft-verify/gh-degraded muted advisories are wired but dormant (keyed on a future updated.verify_state); Plan 02's server returns plain 200, so success closes the dialog (UI-SPEC-sanctioned non-blocking)
- [Phase 10-github-foundations]: Gap 1 backend half: GET /api/github/status is an always-200 endpoint returning {gh_available: bool} from github.Available() (call-time LookPath, reflects install/uninstall without restart); package-level handler (no DB), snake_case field Plan 10-05 reads; contract test compares to github.Available() so it is host-independent
- [Phase 10-github-foundations]: Gap 1 frontend half + Gap 2: /settings GitHub toggle is gh-aware via useGithubStatus() (GET /api/github/status); effectiveEnabled = enabled && ghAvailable forces OFF when gh is missing, an enable attempt is blocked with install-gh guidance (Switch stays interactive, no disabled), and the over-claiming help copy is dropped

### Pending Todos

- Quota poll while idle (no connected browsers) is acceptable for the first iteration with jitter + backoff; revisit before milestone close (research tech-debt note).
- Phase 12 and Phase 13 are research-flagged in ROADMAP.md — `/gsd:plan-phase` should decide whether to run `/gsd:research-phase` (PR-branch checkout mechanics + per-query board-leak audit for 12; gated-cleanup refactor + unpushed-work gate for 13).

### Blockers/Concerns

- Plan-mode exit-plan approval → amber dot (v1.0 research OQ1): still unobserved — carry to v1.3 UAT.
- `gh` is a soft dependency; the integration must degrade-don't-break on every failure mode (missing/unauthenticated/rate-limited). Secondary rate limits (403/`Retry-After`) are a real risk under multi-project auto-poll — bounded steady-state call rate, paused-when-hidden, backoff (Phase 11).
- The board-leak regression (a forgotten `source='manual'` filter on any `tasks` SELECT) is the milestone's highest-severity risk — treat as a per-query checklist item in Phase 12.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260613-osu | Warn when a linked GitHub repo cannot be verified (surface verify_state) — completes GHPRJ-03 soft-save-with-warning | 2026-06-13 | 41f3d50 | [260613-osu-warn-when-a-linked-github-repo-cannot-be](./quick/260613-osu-warn-when-a-linked-github-repo-cannot-be/) |
| 260613-ph5 | Make GitHub repo-link validation MANDATORY (hard-block invalid repos with highlighted error) — supersedes 260613-osu's soft verify_state advisory; reverses D-11 for the repo-link UX per user decision | 2026-06-13 | 9ea0df6 | [260613-ph5-make-github-repo-link-validation-mandato](./quick/260613-ph5-make-github-repo-link-validation-mandato/) |

## Session Continuity

Last session: 2026-06-13T15:33:00.370Z
Stopped at: Quick task 260613-ph5 (mandatory repo-link validation) complete
Resume file: None
Next: `/gsd:discuss-phase 11`
