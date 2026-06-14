# Phase 11: PR Review Column - Context

**Gathered:** 2026-06-13
**Status:** Ready for planning

<domain>
## Phase Boundary

On a linked project's board (GitHub integration ON), render a **collapsible right-side "Review" column** that lists the open PRs needing the user's *direct* review, fetched via the host `gh` CLI, auto-polled (paused when the tab is hidden) + manual refresh — with task-card-style PR cards and inline loading / empty / degraded states, **without any of it ever blocking the board**.

This phase is a **pure read path**: no worktree, no task mutation, no GitHub writes. It stands up the whole `gh` PR-list integration at the lowest risk.

**Out of scope (later phases):**
- Opening a PR card into a worktree-backed review workspace (agent/bash/diff) — **Phase 12** (GHREV-*).
- Auto-cleanup / reaper reconciliation of merged-closed PR worktrees — **Phase 13** (GHCLN-*).
- Richer PR cards (diffstat, fork pill, head→base, review-decision, labels, comments) — **Future GHCARD-* reqs**.
- Filter toggles (team requests, drafts) — **Future GHFILT-* reqs**. Cross-project inbox — **GHWIDE-*.**

**Requirements covered:** GHCOL-01, GHCOL-02, GHCOL-03, GHCOL-04, GHCOL-05, GHCOL-06.

</domain>

<decisions>
## Implementation Decisions

### Pre-settled by research + REQUIREMENTS.md (locked going in — not re-litigated)
- **D-00a:** Qualifier is `user-review-requested:@me`, `--state open`, **drafts excluded** (GHCOL-02). Direct requests only — `review-requested:@me` floods with all team requests (~100 PRs). GitHub auto-removes the user from the set on review submit, so the column self-empties for free — **do not over-cache against it** (PITFALLS).
- **D-00b:** Fetch via `gh pr list -R <owner/name> --search "user-review-requested:@me" --state open --json <rich set>` run with `cmd.Dir` = the project's repo path (selects the right host/account for multi-`gh`-account / enterprise). **Never** `gh search prs` (thin JSON, 30/min search budget). `gh pr list` lives on the 5000/hr core budget.
- **D-00c:** `internal/github` gains a best-effort `Service` modeled on `internal/quota`: per-repo cache with TTL/floor/backoff, typed degraded states, and an **always-200** endpoint `GET /api/projects/{id}/pull-requests[?refresh=1]`, **toggle-gated** (`github_integration`) AND **link-gated** (project has `github_repo`). `gh` is a soft dependency — degrade, don't break (GHSET-03). Do not trust `gh auth status` exit codes alone (cli/cli#8845) — prefer presence via `LookPath` + parse, mirroring the quota degrade model.
- **D-00d:** `statusCheckRollup` (raw CheckRun/StatusContext array) is reduced **server-side** to a single `pass | fail | pending | none` value. The raw array is never shipped to the browser.

### Card content (GHCOL-03) — **Strict minimal**
- **D-01:** PR cards show exactly the GHCOL-03 set and nothing more: **PR number, title, author, relative "updated X ago" time, and the checks pill** (D-04). The card is a presentational variant of `TaskCard` (number/title row, dot for status).
- **D-02:** Diffstat (+/−), fork-PR indicator, and head→base branch line are **NOT** shown in Phase 11 — they remain Future Requirements (GHCARD-01/02/03), even though their JSON fields (`additions`/`deletions`, `isCrossRepository`, `headRefName`/`baseRefName`) are already fetched. **Fetch the rich `--json` set anyway** (it's one call and Phase 12/13 consume `headRefName`/`headRefOid`/`baseRefName`/`isCrossRepository`); just don't render the deferred fields.

### Checks pill (GHCOL-03) — **Small colored dot, "none" omitted**
- **D-03:** Render the CI rollup as a **small colored dot** using the card's existing dot visual language (consistent with `StatusDot` on task cards): **green = pass, red = fail, amber = pending**.
- **D-04:** When a PR has **no checks configured** (`none`), render **nothing** — no dot, no reserved gutter (matches the dotless-task-card rule: dotless cards must not shift layout).

### Collapse + count persistence (GHCOL-01/06) — **localStorage, per-project, default collapsed**
- **D-05:** Collapse state is persisted in **`localStorage`, keyed per-project** (project id). No SQLite, **no new migration, no new endpoint** — Phase 10 banked "no further migration needed through Phase 12," and single-user-localhost doesn't need cross-browser persistence. **Default is collapsed** — this supersedes the original D-05/D-06 "default expanded" per user request on 2026-06-14.
- **D-06:** The column **defaults to collapsed** (supersedes the original "defaults to expanded" per user request 2026-06-14) — the board opens with the slim Review rail; the column only stays expanded once the user has explicitly expanded it.
- **D-07:** The **collapsed** column shows a **PR count badge**; the count reflects current GitHub state (so it changes as cards appear/disappear). Count source is the same query data that fills the expanded list.

### Click behavior (Phase 11 has no worktree yet) — **Inert body + ↗ to GitHub**
- **D-08:** The **card body is inert** in Phase 11 — clicking it does nothing. The whole-card click is **reserved for Phase 12** (open the worktree-backed review view), so there is no behavior to unlearn when Phase 12 lands.
- **D-09:** A small **external-link ↗ icon** on the card opens the PR on **github.com** in a new tab via the fetched `url` field — gives the shipped/UAT'd Phase 11 immediate utility without colliding with Phase 12's body click.

### Auto-poll & refresh (GHCOL-04) — Claude's discretion, mirroring Phase 7 quota
- **D-10:** Auto-poll mirrors the **proven Phase 7 quota pattern verbatim**: a TanStack `useQuery(['pull-requests', projectId])` with `refetchInterval: 60_000` and `refetchIntervalInBackground: false` (visibility-paused, GHCOL-04), plus a manual refresh that hits `?refresh=1` and writes the result back to the cache (the `useRefreshQuota` precedent).
- **D-11:** Only the **active/visible project's** Review column polls (the query is scoped to that board). Combined with the server-side per-repo cache (D-00c) and backoff, this keeps the steady-state `gh` call rate bounded — the mitigation for the multi-project secondary-rate-limit concern (STATE.md blocker). No explicit rate-limit polling needed; the quota-style "show stale, back off on failure" degrade is sufficient.

### Degraded / empty / loading states (GHCOL-05) — Claude's discretion
- **D-12:** All states render **inline in the column, never as a modal, never blocking the board**: a loading affordance on first fetch; an empty state with **"you're all caught up"** (ROADMAP copy) when the linked repo has zero review-requested PRs; and distinct degraded states for `gh` missing / unauthenticated / error, surfaced as a quiet inline note (typed states copied from the quota model: `ok`/`no_gh`/`auth_required`/`disabled`/`error`). Exact degraded copy is planner/UI-SPEC discretion, consistent with how the quota indicator degrades.

### Column placement & layout — Claude's discretion
- **D-13:** The Review column is a **sibling of the existing status columns**, rendered to the **right** of Done in the board's flex row (the `Column` component already exposes a `topSlot`; the board row is `flex … overflow-x-auto`). It is **not** a `dnd-kit` droppable — PR cards are never dragged and never enter To Do/In Progress/In Review/Done. Min-width / exact styling is planner discretion, visually echoing the task columns.

### PR list ordering — Claude's discretion
- **D-14:** List PRs **most-recently-updated first** (`updatedAt` desc) — the default that best matches a review queue. Stale-aging / custom sort stays deferred (GHCARD/GHFILT Future).

### Claude's Discretion (summary)
- Auto-poll interval value (defaulted to 60s per D-10) and backoff/TTL constants (mirror quota).
- Exact degraded/empty/loading copy and placement (D-12).
- Column min-width, header styling, count-badge styling (D-13).
- The `internal/github.Service` cache shape and the `PRSummary` struct field set (fetch rich, render minimal).
- Whether the checks dot reuses `StatusDot` directly or a small sibling component (D-03) — same visual language either way.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Milestone research (v1.3) — the authoritative source for this phase
- `.planning/research/SUMMARY.md` §"Phase B — `internal/github` package + read-only PR list + Review column" — Phase 11's exact deliverables, the "reuse not new machinery" thesis, the qualifier decision, and the "don't fight the self-emptying column" caution.
- `.planning/research/STACK.md` §"List PRs" — the **exact** `gh pr list … --search "…@me" --json <field set>` command, the full `--json` field-by-field table (incl. `statusCheckRollup` rollup-server-side note), why `gh pr list` not `gh search prs`, and the core(5000/hr) vs search(30/min) rate budgets. Floor `gh 2.82.0`.
- `.planning/research/ARCHITECTURE.md` — the component map: the `GET /api/projects/{id}/pull-requests[?refresh=1]` endpoint pair, the `internal/github` leaf shape, and the integration points against real v1.2 files.
- `.planning/research/PITFALLS.md` — degrade-don't-break (always-200, typed states), the qualifier-flood trap, the over-caching-against-self-empty trap, and unreliable `gh auth status` exit codes (cli/cli#8845).

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` §"PR Review Column" — GHCOL-01..06 (this phase) + §"Future Requirements" (GHCARD-*, GHFILT-*, GHWIDE-* — what's explicitly deferred) + §"Out of Scope".
- `.planning/ROADMAP.md` §"Phase 11: PR Review Column" — goal + 5 success criteria (the column-matches-GitHub's-filtered-page test, the self-emptying test, the inline-degraded-never-modal test).

### Prior phase (foundations this phase consumes)
- `.planning/phases/10-github-foundations/10-CONTEXT.md` — the `github_integration` toggle KV (default on), the shared `useSettings()` OFF-cascade gate the column must respect, the `internal/github` leaf (`Available()`/`ParseRepoRef`/`ValidateRepo`), and `projects.github_repo` (the link the column gates on).

### Reuse precedents in-repo (read for pattern, not requirement)
- `internal/quota/quota.go` + `web/src/api/usage.ts` + `web/src/components/quota/QuotaIndicator.tsx` — the best-effort server proxy (cache/TTL/backoff/typed-state) and the 60s/visibility-paused poll + manual-refresh the column clones (D-00c, D-10).
- `web/src/components/board/{Board,Column,TaskCard}.tsx` — the board flex row, the `Column` `topSlot`, and the task card the PR card mirrors (D-01, D-13).

No external (non-`.planning`) specs — requirements fully captured above and in the research files.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/github/github.go` — the existing leaf (`Available()` via `LookPath`, `ParseRepoRef`, `ValidateRepo`, arg-array shell-out, soft-degrade philosophy). Extend with `ListReviewRequested(ctx, repo) ([]PRSummary, error)` + a quota-shaped best-effort `Service` (cache/TTL/backoff/typed states). Don't reimplement degrade — copy the quota model.
- `internal/quota/quota.go` — the template for the per-repo cache + backoff + typed degraded states + always-200 behavior (`ok`/`no_credentials`/`auth_expired`/`error` → adapt to `ok`/`no_gh`/`auth_required`/`disabled`/`error`).
- `web/src/api/usage.ts` (`useQuota`/`useRefreshQuota`) — copy verbatim shape for `usePullRequests(projectId)`/`useRefreshPullRequests(projectId)`: `refetchInterval: 60_000`, `refetchIntervalInBackground: false`, refresh writes back via `qc.setQueryData`.
- `web/src/components/board/Column.tsx` — already has a `topSlot` prop and the column shell (header label + count); the Review column is a sibling with its own header (collapse toggle + count badge) and a non-droppable card list.
- `web/src/components/board/{TaskCard,Board}.tsx` — `TaskCard`/`CardRow` is the presentational base for the PR card; `Board.tsx` renders `STATUSES.map(Column)` in a `flex … overflow-x-auto` row — append the Review column after Done (D-13).
- `web/src/components/StatusDot.tsx` — the dot visual language to echo for the checks pill (D-03/D-04).
- `web/src/api/settings.ts` (`useSettings`) — the shared query whose `github_integration === 'on'` value gates whether the column renders at all (Phase 10 OFF-cascade, D-00c).

### Established Patterns
- Best-effort `gh`/external proxy: per-key cache, TTL/floor, backoff on failure, typed degraded states, endpoint always returns 200 with a state discriminator (quota precedent). PR list follows this exactly.
- Auto-poll: TanStack `useQuery` with `refetchInterval` + `refetchIntervalInBackground:false` (visible-only) + a manual-refresh mutation that writes the cache (Phase 7).
- Shell-out: arg-array `exec.CommandContext`, never `sh -c`; `gh … --json` parsed as machine output (never human text); run with `cmd.Dir` = repo for host/account selection.
- Dotless cards must not reserve a gutter / shift layout (TaskCard UI-SPEC rule) — applies to the "no checks" case (D-04).
- Frontend GitHub UI is render-gated on `useSettings().github_integration === 'on'` (OFF = byte-for-byte pre-v1.3).

### Integration Points
- **New endpoint:** `GET /api/projects/{id}/pull-requests[?refresh=1]` — toggle-gated + link-gated, always 200, returns `{ state, stale, fetchedAt, prs }`.
- **New `internal/github` surface:** `ListReviewRequested` + `Service` (cache/poll/degrade), wired in `main.go` like `quota`.
- **Board:** Review column appended to the right of Done in `Board.tsx`; renders only when integration is on AND the project has a `github_repo`.
- **Frontend API:** new `web/src/api/pullRequests.ts` (clone of `usage.ts`) + types.
- **No migration, no new settings KV** — collapse state lives in `localStorage` (D-05).

</code_context>

<specifics>
## Specific Ideas

- The whole feature should feel like a sibling of the quota indicator: best-effort, degrade-don't-break, "show stale, back off on failure" — never a blocking spinner or modal.
- Cards should be *lean* — deliberately matching the spare task card, not a GitHub-style PR row. Resist info-cramming; the richness is parked in GHCARD on purpose.
- The card body click is sacred ground for Phase 12 — keep it inert now; the ↗ icon is the only Phase-11 affordance.
- "you're all caught up" is the intended empty-state voice.

</specifics>

<deferred>
## Deferred Ideas

- **Diffstat (+/−), fork-PR pill, head→base branch line** on cards — GHCARD-01/02/03 (Future Requirements). JSON is already fetched; rendering is deferred by user decision (D-01/D-02).
- **`reviewDecision` pill, labels, comment-count, stale-aging sort** — GHCARD-04 / GHFILT (Future).
- **Include team review requests (`review-requested:@me`) / show drafts** — GHFILT-01/02 (Future); Phase 11 is direct-requests-only, drafts excluded.
- **Cross-project "all repos" review inbox / author-side PR view** — GHWIDE-01/02 (Future).
- **SQLite/cross-browser persistence of collapse state** — chose localStorage per-project for single-user-localhost (D-05); a backend-backed pref can come later if multi-device ever matters.

None of the discussion was scope creep — all four areas clarified HOW to implement the fixed Phase 11 scope.

</deferred>

---

*Phase: 11-pr-review-column*
*Context gathered: 2026-06-13*
