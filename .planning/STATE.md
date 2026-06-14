---
gsd_state_version: 1.0
milestone: v1.3
milestone_name: GitHub PR Review
status: verifying
stopped_at: Completed 12-07-PLAN.md — all 5 12-05 follow-ups closed (12-06 backend + 12-07 frontend); human-verify APPROVED 2026-06-14; phase 12 plans all complete
last_updated: "2026-06-14T09:28:12.658Z"
last_activity: 2026-06-14
progress:
  total_phases: 4
  completed_phases: 3
  total_plans: 16
  completed_plans: 16
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-13)

**Core value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.
**Current focus:** Phase 12 — open-a-review

## Current Position

Phase: 13
Plan: Not started
Status: all 5 12-05 follow-ups closed (12-06 backend + 12-07 frontend); orchestrator re-runs phase verification next
Last activity: 2026-06-14

Progress: [██████████] 100% (12-01..12-07 implemented; phase re-verification remains)

**12-05 human-verify outcome — 5 follow-ups → gap plans 12-06/12-07 — ALL CLOSED + APPROVED 2026-06-14:**

1. PR worktree is detached HEAD; want the PR's real head branch checked out (fallback `pr/<n>` on collision). [12-06 ✓ DONE — CheckoutPR named-branch + collision fallback, GHREV-05 preserved]
2. Bash terminal in the PR review opens empty with excessive height forcing scroll (layout/fit regression). [12-07 ✓ DONE — confirmed the body chain was never forked; merge line kept inside the shrink-0 header block]
3. Seed prompt re-injected on every open (per-mount ref) — inject only once, at first agent Start. [12-07 ✓ DONE — module-level seededSessionIds Set keyed by agent session id]
4. Make the review prompt configurable via a Settings field (default = current template, `<n>`/`<title>` placeholders). [12-06 key ✓ + 12-07 textarea field ✓ DONE — pr_review_seed, frontend-interpolated, blank = no injection]
5. Show a GitHub-style merge line: "<author> wants to merge <N> commits into <base> from <head>". [12-06 commits/head/base ✓ + 12-07 render ✓ DONE — single-line clickable header #<num> @<author> wants to merge <N> commits into <base> from <head>]

**Bonus checkpoint fixes (12-07):** single-line clickable PR header (merge the two rows; #<num> is the GitHub link); F5 re-hydration via a read-only GET /api/projects/{id}/pull-requests/{n} + usePullRequestDetail (the cache-only detail blanked on hard reload).

GHREV-01 / GHREV-05 are now COMPLETE in REQUIREMENTS.md (gap fixes landed + human-verified). The orchestrator re-runs phase verification (gsd-verifier) next.

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
| Phase 12-open-a-review P07 | ~3h (incl. 3 human-verify rounds) | 4 tasks | 7 files |

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
- [Phase 11-pr-review-column]: internal/github.Service is a quota.Service clone re-keyed to map[owner/name]*repoEntry (per-repo 60s TTL on fetchedAt, 10s floor on lastAttempt binding even force, in-flight dedup, drop-cached-after-3-failures); no 429/backoff field — gh rate-limit surfaces as 'error' with serve-stale per D-11
- [Phase 11-pr-review-column]: reduceChecks maps SKIPPED/NEUTRAL/STALE to non-failing (Pitfall 1, else ~30% of real PRs go red); auth classification combines exit-code-4 OR stderr substrings (gh auth login/401/Bad credentials) since exit 4 alone is unreliable (cli/cli#9338); PRSummary fetches the rich --json set for Phase 12/13 but renders minimal in Phase 11 (D-01/D-02)
- [Phase 11-pr-review-column]: PR endpoint GET /api/projects/{id}/pull-requests is always-200 with a two-gate ladder: GATE 1 reads settings.KeyGithubIntegration (val != 'on' -> disabled, GHSET-02 backend enforcement), GATE 2 returns disabled for unlinked OR unknown projects (200, never 404); both gates short-circuit before any gh spawn
- [Phase 11-pr-review-column]: Handler SELECTs github_repo + repo_path; repo_path = cmd.Dir so gh resolves the right host/account (Pitfall 6); settings/DB errors map to state=error (200), the only non-200 is pathID's 400 on a non-numeric id
- [Phase 11-pr-review-column]: formatAgo lifted to web/src/lib/time.ts (RESEARCH Open Q2 = LIFT): one tier-logic shared by the quota footer + PR card; QuotaIndicator imports it, behavior byte-identical (zero local defs)
- [Phase 11-pr-review-column]: PRCard checks dot uses a local 3-way switch typed Exclude<checks,'none'> (D-03), not StatusDot.dotMeta; none renders nothing (no gutter, no layout shift, D-04); body inert, only the ↗ anchor is interactive (D-08/D-09)
- [Phase 11-pr-review-column]: Review column defaults to COLLAPSED (supersedes D-05/D-06 'default expanded') per user request 2026-06-14: collapsed unless localStorage holds explicit '0'; absent key reads as collapsed (getItem !== '0')
- [Phase 12-open-a-review]: CheckoutPR pins the detached worktree to gh's headRefOid (never FETCH_HEAD, clobbered by 12-03's base fetch); remote hard-coded origin for v1.3; ViewPR does a fresh gh pr view (Phase 11 PRSummary lacks body and reattach can fire with no list mounted)
- [Phase 12-open-a-review]: GHREV-04 board-leak guard at the data layer: AND source = 'manual' on the 5 board/position queries (listByProject, create top-of-ToDo, move drop-at-top, nextPosition, renumberColumn) + a /move 409 guard rejecting source != 'manual'; by-id get/update/delete/afterPosition left unfiltered so the review view deep-links its own PR row. source/pr_number/pr_base_ref added to the Task wire shape (scanTask + loadTaskRepo column-aligned)
- [Phase 12-open-a-review]: Diff base for a github_pr review comes from its own pr_base_ref (D-12/GHREV-03): diffs.go SELECT reads source+pr_base_ref, FetchRef(base) runs before merge-base (RESEARCH Pitfall 3), then resolvePRBase prefers local refs/heads/<base> (show-ref in the worktree dir) else origin/<base> (never FETCH_HEAD); diff.Compute reused unchanged, internal/diff untouched. Manual tasks keep ResolveBase verbatim.
- [Phase 12-open-a-review]: Open-or-reattach endpoint POST .../{n}/review: find-or-create source='github_pr' keyed by (project_id, pr_number); full row -> instant reattach (no CheckoutPR), worktree_path NULL -> re-provision in place, not found -> INSERT (position=0 sentinel, board filters exclude it). off/unlinked/unknown -> 409 BEFORE any gh/worktree spawn. Live gh pr view on EVERY open (no title/body drift) returned as {task, pr}; ViewPR fetched before INSERT so a gh failure 502s with no half-row. viewPR package seam keeps create/reattach tests hermetic (real refs/pull/<n>/head + stubbed gh).
- [Phase 12-open-a-review]: 12-05 frontend wiring shipped (open-or-reattach at the query cache: useOpenReview.onSuccess stashes {task, pr}; TaskPage branches every delta on source==='github_pr'; one-shot seed via pasteApiRef). Human-verify NOT approved -> 5 follow-ups become gap plans 12-06/12-07; phase NOT verified, GHREV-01..05 Pending.
- [Phase 12-open-a-review]: 12-06 gap backend: CheckoutPR DECISION-OVERRIDES D-01 detached -> NAMED branch (headRefName) via worktree add -b, with a never-reuse selection (headRefName -> pr/<n> -> pr/<n>-<shortOID> -> error). Realigns with literal GHREV-01 while PRESERVING GHREV-05 (an existing local ref is never chosen/moved; the pr/<n> fallback protects the fork same-name 'master' trap). PRDetail.Commits (len of gh commits array) + prWire.headRefName/commits feed the 12-07 merge line; pr_review_seed settings key (free-text, length-capped, migration-free) feeds the 12-07 configurable seed.
- [Phase 12-open-a-review]: 12-07 gap frontend (APPROVED 2026-06-14): once-per-session seed via a module-level seededSessionIds Set keyed by agent session id (replaces the per-mount ref re-injection); seed from pr_review_seed with <n>/<title> frontend-interpolated (blank = no injection); single-line clickable PR header #<num> @<author> wants to merge <N> commits into <base> from <head> kept inside the shrink-0 block (fix-#6 height chain preserved, never forked); F5 re-hydration via a read-only GET /pull-requests/{n} + usePullRequestDetail re-fetching under the same prDetailKey (no schema/migration). All 5 follow-ups verified live.

### Pending Todos

- Quota poll while idle (no connected browsers) is acceptable for the first iteration with jitter + backoff; revisit before milestone close (research tech-debt note).
- Phase 12 and Phase 13 are research-flagged in ROADMAP.md — `/gsd:plan-phase` should decide whether to run `/gsd:research-phase` (PR-branch checkout mechanics + per-query board-leak audit for 12; gated-cleanup refactor + unpushed-work gate for 13).

### Blockers/Concerns

- Plan-mode exit-plan approval → amber dot (v1.0 research OQ1): still unobserved — carry to v1.3 UAT.
- `gh` is a soft dependency; the integration must degrade-don't-break on every failure mode (missing/unauthenticated/rate-limited). Secondary rate limits (403/`Retry-After`) are a real risk under multi-project auto-poll — bounded steady-state call rate, paused-when-hidden, backoff (Phase 11).
- The board-leak regression (a forgotten `source='manual'` filter on any `tasks` SELECT) is the milestone's highest-severity risk — treat as a per-query checklist item in Phase 12.
- Phase 12 gap closure DONE + human-verify APPROVED (2026-06-14): all 5 12-05 follow-ups closed across 12-06 (backend: named-branch PR checkout, pr_review_seed key, commits+head/base) and 12-07 (frontend: once-per-session seed, single-line clickable merge-line header, terminal-height chain confirmed, Settings prompt field, + F5 detail re-hydration via GET /pull-requests/{n}). GHREV-01/05 now Complete. Remaining: the orchestrator re-runs the phase verification (gsd-verifier) before the phase is closed.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260613-osu | Warn when a linked GitHub repo cannot be verified (surface verify_state) — completes GHPRJ-03 soft-save-with-warning | 2026-06-13 | 41f3d50 | [260613-osu-warn-when-a-linked-github-repo-cannot-be](./quick/260613-osu-warn-when-a-linked-github-repo-cannot-be/) |
| 260613-ph5 | Make GitHub repo-link validation MANDATORY (hard-block invalid repos with highlighted error) — supersedes 260613-osu's soft verify_state advisory; reverses D-11 for the repo-link UX per user decision | 2026-06-13 | 9ea0df6 | [260613-ph5-make-github-repo-link-validation-mandato](./quick/260613-ph5-make-github-repo-link-validation-mandato/) |

## Session Continuity

Last session: 2026-06-14T10:20:00.000Z
Stopped at: Completed 12-07-PLAN.md — all 5 12-05 follow-ups closed (12-06 backend + 12-07 frontend); human-verify APPROVED 2026-06-14; phase 12 plans all complete
Resume file: None
Next: orchestrator re-runs phase 12 verification (gsd-verifier); GHREV-01/05 marked Complete
