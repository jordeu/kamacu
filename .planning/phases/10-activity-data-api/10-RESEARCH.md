# Phase 10: Activity Data & API - Research

**Researched:** 2026-07-30
**Domain:** Go backend — REST endpoint, SQL aggregation, `gh` CLI integration, in-memory per-repo caching, in-Go statistics
**Confidence:** HIGH

## Summary

Phase 10 is a **backend-only data + API layer** that delivers one combined `GET /api/activity` endpoint answering "which tasks were done, which reviews were completed (merged/closed), and what are the cycle/dwell stats" for any Global / Workspace:N / Project:N scope and a Week (7d) / Month (30d) window. Phase 11 builds the page that renders this. The phase extends two existing, well-understood foundations: (1) the per-status timestamps from migration `00006` (`todo_at`/`in_progress_at`/`in_review_at`/`done_at`) that power the statistics, and (2) the `internal/github` package + per-repo cache from v1.3/v1.5 that powers the reviews-done fetch.

The phase is almost entirely a **pattern-mirror** of existing code: a new `internal/api/activity.go` handler shaped like `PullRequestRoutes` (two-gate ladder + always-200 + degrade-via-`State`), a new method+cache entry on the shared `*github.Service` (D-03, separate from the review-column hot path), and a SQL query that reuses the established `source='manual'` board-leak filter plus scope filtering on `projects.workspace_id` / `project_id`. The statistics are computed in Go (D-10) over the fetched `*_at` timestamps — no SQL `MEDIAN` gymnastics.

**The one genuinely open technical question (now RESOLVED):** the exact `gh` invocation for the merged/closed reviews search. Verified against the official cli/cli issue tracker + GitHub search docs + the locally-installed `gh 2.82.0`:
- `gh pr list --state closed` **currently INCLUDES merged PRs** (behavior reversed in ~2023; cli/cli #8102 filed as a bug, still open/unfixed as of gh 2.29+). Relying on this flag behavior is **fragile** — it is a documented bug a future gh release could "fix."
- The **STABLE, documented** filter is the `is:closed` **search qualifier**, which includes merged (GitHub discussion #5599 + search docs). **Use `is:closed` in the `--search` string as the authoritative filter** (belt-and-suspenders with `--state closed`).
- `completedAt` ← the `closedAt` `--json` field (set for BOTH merged and closed PRs; `mergedAt` is merged-only).

**Primary recommendation:** Implement `GET /api/activity` with a new `GetMergedClosed` method on the shared `*github.Service` (own 5min-TTL cache, own `--search "reviewed-by:@me draft:false is:closed"` + `--state closed` + `--limit 100` + `--json number,title,closedAt,url`), concurrent bounded aggregation across in-scope linked repos, in-Go median/cycle/dwell over the tasks-done rows, and the established two-gate ladder + always-200 + `State`-field degradation.

**Two NEW pitfalls discovered that CONTEXT.md does not mention** (both HIGH impact):
1. **The `--limit` default-30 truncation** — the existing `listPRs` passes no `--limit` (defaults to 30, fine for the open review queue). A reviews-done history over weeks/months WILL exceed 30 for an active reviewer. The new search MUST pass `--limit` (recommend 100). Left at the default, reviews-done silently truncates.
2. **The `--state closed` flag is a filed bug** — using it alone (D-05's literal wording) couples correctness to an unfixed gh bug. Add `is:closed` to the search string as the authoritative filter.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Tasks-done list (SQL) | API / Backend (Go) | Database / Storage | Pure SQL read over `tasks JOIN projects` with `done_at` window + scope filter; no client involvement |
| Reviews-done list (`gh`) | API / Backend (Go) | External (GitHub via `gh`) | Server-side `gh pr list` per repo, cached in `*github.Service`; browser never calls `gh` directly |
| Cross-project aggregation | API / Backend (Go) | — | Handler fans out `GetMergedClosed` across in-scope linked repos concurrently; merges per-repo `Result`s into one aggregate `state` |
| Statistics (cycle/dwell/median) | API / Backend (Go) | — | Computed in Go over fetched `*_at` timestamps; SQLite has no `MEDIAN` |
| Scope/window param parsing | API / Backend (Go) | — | `?scope=` + `?window=` are server-validated, then drive the SQL `WHERE` and the in-Go window cutoff |
| Degradation orchestration | API / Backend (Go) | — | Two-gate ladder (GATE 1 global toggle) + per-repo degrade-within-errgroup; `State` field carries status, HTTP always 200 |
| Caching | API / Backend (Go) | — | Per-repo in-memory cache on the shared `*github.Service` (5min TTL), distinct from the review-column 60s cache |

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** One combined `GET /api/activity` endpoint. Query params: `?scope=global|workspace:N|project:N` (single mutually-exclusive scope param; `global` default) and `?window=week|month` (default `week`; 7d / 30d rolling from now). Returns `{tasks:[...], reviews:{state, stale, fetchedAt, prs:[...]}, stats:{...}}`.
- **D-02:** Reviews sub-object degrades via a `state` field; the WHOLE response is always HTTP 200. `reviews` mirrors `github.Result`: `{state, stale, fetchedAt, prs}`. Tasks + stats always populate regardless of gh state. Global toggle off → `reviews.state="disabled"`, no `gh` spawns.
- **D-03:** A NEW separate cache + method on `*github.Service` (e.g. `GetMergedClosed`) with its OWN per-repo cache entry + TTL. Does NOT touch the review-column 60s-TTL hot path. Reuses existing Service infrastructure (demand-driven, in-flight dedup, drop-cache-after-N, no_gh/auth_required/error classification).
- **D-04:** 5min TTL for the new merged/closed reviews cache. `?refresh=1` bypasses the TTL.
- **D-05:** Treat merged and closed as one "completed" state. One `gh` search covers both, with a single `completedAt` date field. *(Researcher note: D-05's literal "reviewed-by:@me --state closed" is verified-but-fragile — see Open Question 1; add `is:closed` to the search string.)*
- **D-06:** A dedicated `ReviewDoneSummary` type carrying only what the activity list needs: `number, title, completedAt, url`, and project provenance (`projectName`, `projectId`, and/or `repo`). Keeps `PRSummary` untouched.
- **D-07:** Concurrent, bounded aggregation across in-scope repos via a bounded `errgroup` (e.g. 5-wide). Each repo's call is cache-served most of the time; a failing/slow repo returns its own degraded state and does NOT block the others (per-repo context timeout/degrade).
- **D-08:** Aggregate `reviews.state` = `ok` / `partial` / degraded. `ok` if ALL in-scope repos succeeded; `partial` if some succeeded and some degraded; `no_gh`/`disabled`/`error` only if ALL repos degraded (or the global GATE 1 blocks).
- **D-09:** The rolling week/month window naturally filters pre-00006 rows. No special NULL-handling beyond the window filter on `done_at`; if a row lands in-window without timestamps it contributes to `taskCount` but is excluded from the time-stat slice.
- **D-10:** Compute cycle/dwell in Go, not SQL. Median = sort durations ascending; middle element (odd N) or average of two middle (even N). Durations surface as seconds.
- **D-11:** Stats object shape: `stats: {taskCount, reviewCount, cycle: {n, min, max, median}, dwellInProgress: {n, min, max, median}, dwellInReview: {n, min, max, median}}`. Each time-stat carries its own `n`. `reviewCount` is a plain number. Durations in seconds.
- **D-12:** Tasks-done list = `source='manual'` only. Query: `WHERE status='done' AND source='manual' AND done_at IS NOT NULL AND done_at >= <window-start>` joined to `projects` with the scope filter, ordered by `done_at DESC`.

### the agent's Discretion
- The exact `gh` flag/date verification for the merged/closed search (confirm `--state closed` returns both, and which JSON date field maps to `completedAt`). **→ RESOLVED by this research (see Code Examples + Open Questions).**
- The new method/cache-entry naming in `internal/github`; the `ReviewDoneSummary` struct field names/order; whether the new cache entry reuses `repoEntry` (with a new field) or a separate map.
- The errgroup bound value (5-wide is a suggestion); the per-repo timeout within the aggregate; whether `?refresh=1` propagates to every repo's `force` flag.
- Exact stats struct Go types (int64 seconds vs float64 vs `time.Duration` on the wire).
- File organization — new `internal/api/activity.go` (handler) + extend `internal/github` (new method/cache); new `ActivityRoutes(mux, db, ghSvc)` in `serve.go` alongside `PullRequestRoutes`.
- Whether the tasks-done query reuses `taskColumns`/`scanTask` or a dedicated scan (`taskColumns` does NOT include the `*_at` columns).

### Deferred Ideas (OUT OF SCOPE)
- Distinguishing merged vs closed in the reviews-done data (D-05 treats both as "completed" with one `completedAt`).
- Custom date ranges / "All time" preset (ACTFUT-01).
- Throughput trend charts / bar graphs (ACTFUT-02).
- CSV/JSON export of activity data (ACTFUT-03).
- Per-agent or per-workspace breakdown statistics (ACTFUT-04).
- Activity for events beyond done/merged (ACTFUT-05).
- Time stats for reviews (STATS-04 — permanently out of scope).
- The exact `?refresh=1`-equivalent force flag param name/propagation.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| TASKS-01 | List of tasks completed (moved to Done) within the window and scope | SQL query pattern (D-12): `tasks JOIN projects WHERE status='done' AND source='manual' AND done_at >= ? + scope filter`, ordered `done_at DESC`. `done_at` from migration 00006, millisecond ISO-8601 (lexically-sortable). Dedicated scan needed (`taskColumns` lacks `*_at`). See Code Examples. |
| REVIEWS-01 | List of PRs reviewed-by:@me that MERGED or CLOSED within window+scope, with merge/close date | New `GetMergedClosed` on `*github.Service` (D-03): `gh pr list --search "reviewed-by:@me draft:false is:closed" --state closed --json number,title,closedAt,url --limit 100`. `is:closed` qualifier = closed+merged (verified). `completedAt` ← `closedAt`. Concurrent cross-repo aggregation (D-07). See Pitfall 1 (--limit) + Pitfall 2 (--state flag bug). |
| REVIEWS-04 | Reviews-done degrades gracefully when gh unavailable / repo fetch fails (never blocks the page) | D-02/D-08: `reviews.state` = `ok`/`partial`/degraded; per-repo degrade-within-errgroup; tasks+stats always populate. Mirrors `github.Result` + two-gate ladder (GHSET-02). See Architecture Patterns / Anti-Patterns. |
| STATS-01 | Count of tasks done and count of reviews done within window+scope | `taskCount` = len(tasks-done rows); `reviewCount` = len(aggregated reviews `prs`. Plain numbers in the `stats` object (D-11). |
| STATS-02 | min / max / median cycle time (In Progress → Done) for tasks done in window | In-Go compute (D-10): for each in-window done row with both `in_progress_at` + `done_at`, duration = `done_at − in_progress_at`; min/max/median over the slice. `cycle.n` = count of rows it was computed over. See Code Examples. |
| STATS-03 | min / max / median dwell per column (In Progress, In Review) for tasks done in window | In-Go compute (D-10): dwellInProgress = `done_at − in_progress_at` OR `(in_review_at − in_progress_at)` — see Open Question 3 for the exact dwell definition; dwellInReview = `done_at − in_review_at`. Each carries its own `n`. |
| STATS-04 | Cycle/dwell cover tasks only; reviews contribute a count, not time stats | Enforced by shape: `reviewCount` is a plain number; no `cycle`/`dwell` object under reviews. GitHub merge signal has no Kamacu in-progress/in-review timestamps. |
</phase_requirements>

## Standard Stack

**This phase installs NO new dependencies.** It is pure Go against the existing module + a shell-out to the already-installed `gh` CLI. The stack below is the existing tooling this phase composes; all versions are from `go.mod` (verified) and the local environment.

### Core
| Library / Tool | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib `net/http` ServeMux | stdlib (go 1.26) | Route `GET /api/activity` | Project pattern: stdlib ServeMux with method+path matching, no router framework (`routes.go`, `pullrequests.go`) [VERIFIED: go.mod `go 1.26`, codebase] |
| `database/sql` + `modernc.org/sqlite` | v1.52.0 | Tasks-done SQL + scope filter | Single-writer SQLite, parameterized `?` SQL. `done_at`/`*_at` columns from migration 00006 [VERIFIED: go.mod, migration 00006] |
| `kamacu/internal/github` (in-repo) | existing | New `GetMergedClosed` method + cache on shared `*github.Service` | D-03 extends the existing per-repo cache (service.go). Reuses `github.Result` wire contract [VERIFIED: internal/github/service.go] |
| `gh` CLI (system) | 2.82.0 | `gh pr list --search ... --state closed --json ...` for reviews-done | The app's whole premise is `gh` on the host; `Available()` gates spawns [VERIFIED: `gh --version` local] |
| `log/slog` | stdlib | Structured logging to stderr | Project pattern [VERIFIED: codebase] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `golang.org/x/sync/errgroup` | v0.20.0 (currently **indirect**) | Bounded concurrent cross-repo aggregation (D-07) | Promote to **direct** dep in go.mod; idiomatic Go for fan-out-with-errors. Alternative: plain goroutines + semaphore channel + `sync.WaitGroup` (zero new dep). See Don't Hand-Roll. [VERIFIED: go.mod `// indirect`] |
| `sort` (stdlib) | stdlib | Median computation (sort duration slice) | D-10 in-Go stats [VERIFIED: stdlib] |
| `time` (stdlib) | stdlib | Parse `*_at` timestamps, compute durations, window cutoff | `time.Parse` layout `2006-01-02T15:04:05.000Z` (the reaper's format) [VERIFIED: internal/reaper/reaper.go] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `errgroup` for concurrent aggregation | goroutines + semaphore channel + `sync.WaitGroup` | errgroup is slightly less code and is the idiomatic 2026 choice; but the project is stdlib-first and errgroup requires promoting an indirect dep. **Either is fine** — errgroup is recommended only because the dep is already transitively present. |
| In-Go median/cycle/dwell (D-10) | SQLite `julianday` math + `percentile_cont` workaround | SQLite has no native `MEDIAN`; the workaround is fiddly and N is tiny (a week/month of done tasks). Go is cleaner. D-10 locks this. |
| Client-side window filter on `closedAt` | Server-side `closed:>YYYY-MM-DD` search qualifier | Client-side matches the existing fetch-everything-then-reduce pattern AND makes the per-repo cache window-agnostic (one entry serves week OR month). **Recommend client-side** for cache coherence. See Open Question 2. |

**Installation:** None. No `go get` strictly required. If `errgroup` is chosen, run `go mod tidy` after importing `golang.org/x/sync/errgroup` (it flips `// indirect` → direct). No frontend work in this phase.

## Package Legitimacy Audit

> This phase installs **no external packages**. The only candidate is `golang.org/x/sync/errgroup`, which is **already present** in go.mod as an indirect dependency (`v0.20.0`) — it is the official Go team's supplementary sync library, transitively pulled by existing deps. Promoting it to a direct import requires no registry verification beyond what go.mod already asserts.

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| `golang.org/x/sync` | Go modules (go.dev) | ~10 yrs | (Go stdlib-adjacent; ubiquitous) | github.com/golang/sync | OK | Approved — promote indirect→direct if errgroup used |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

*No packages discovered via WebSearch or training data are recommended. The only external symbol used (`errgroup`) is verified present in go.mod and is optional (a stdlib-only WaitGroup alternative exists).*

## Architecture Patterns

### System Architecture Diagram

```
                         GET /api/activity?scope=...&window=...&refresh=1
                                      │
                                      ▼
                        ┌─────────────────────────────┐
                        │  internal/api/activity.go    │   ← handler (always HTTP 200)
                        │  ActivityRoutes(mux,db,ghSvc)│
                        └──────────────┬──────────────┘
                                       │
          ┌────────────────────────────┼─────────────────────────────┐
          │ (1) parse + validate        │ (2) GATE 1: github toggle    │ (3) SQL: tasks-done
          │     scope + window          │     off → reviews.state=     │     (source='manual',
          │                             │     "disabled", no gh spawn  │      done_at>=cutoff,
          ▼                             ▼                              │      scope filter)
   window cutoff =             settings.Get(KeyGithubIntegration)      ▼
   now - 7d/30d,                      │                    ┌──────────────────────┐
   formatted ISO-8601            on? ──┴── off ──► skip     │ tasks JOIN projects  │
                                       │     gh entirely    │ ORDER BY done_at DESC│
                                       ▼                    └──────────┬───────────┘
                          ┌────────────────────────┐                   │
                          │ SQL: in-scope LINKED   │                   │ (6) in-Go stats:
                          │ projects (github_repo, │                   │ cycle / dwell /
                          │ repo_path, ws filter)  │                   │ min/max/median
                          └───────────┬────────────┘                   │ over *_at durations
                                      │                                │
                                      ▼                                │
                          ┌────────────────────────┐                   │
                          │ (4) CONCURRENT bounded  │                   │
                          │   errgroup per repo     │                   │
                          │   (D-07, ~5-wide)       │                   │
                          └───────────┬────────────┘                   │
                                      │                                │
                    ┌─────────────────┼─────────────────┐              │
                    ▼                 ▼                 ▼              │
              ┌──────────┐      ┌──────────┐      ┌──────────┐         │
              │ repo A   │      │ repo B   │      │ repo C   │         │
              │GetMerged │      │GetMerged │      │GetMerged │         │
              │Closed()  │      │Closed()  │      │Closed()  │         │
              └────┬─────┘      └────┬─────┘      └────┬─────┘         │
            cache? │                 │ degrade?        │ cache?        │
                   ▼                 ▼                 ▼               │
            ┌─────────────┐   (no_gh/error →   ┌─────────────┐         │
            │ gh pr list  │    empty prs,      │ gh pr list  │         │
            │ --search    │    repo skipped)   │ --search    │         │
            │ reviewed-by │                     │ reviewed-by │        │
            │ :@me draft  │                     │ :@me draft  │        │
            │ :false      │                     │ :false      │        │
            │ is:closed   │                     │ is:closed   │        │
            │ --state     │                     │ --state     │        │
            │  closed     │                     │  closed     │        │
            │ --limit 100 │                     │ --limit 100 │        │
            │ --json num, │                     │ --json ...  │        │
            │  title,     │                     └──────┬──────┘        │
            │  closedAt,  │                            │               │
            │  url        │                            │               │
            └──────┬──────┘                            │               │
                   │ 5min TTL cache (D-03/D-04)        │               │
                   └──────────────┬────────────────────┘               │
                                  ▼                                    │
                    ┌──────────────────────────────┐                   │
                    │(5) aggregate: map each PR →   │                   │
                    │  ReviewDoneSummary (+proj     │                   │
                    │  provenance); roll up state:  │                   │
                    │  all ok → "ok"; some ok →     │                   │
                    │  "partial"; all degraded →    │                   │
                    │  no_gh/disabled/error (D-08)  │                   │
                    └──────────────┬────────────────┘                   │
                                   │                                    │
                                   └────────────────┬───────────────────┘
                                                    ▼
                                    ┌─────────────────────────────┐
                                    │ {tasks:[...],               │
                                    │  reviews:{state,stale,      │
                                    │   fetchedAt,prs:[...]},     │
                                    │  stats:{taskCount,reviewCnt,│
                                    │   cycle{...},               │
                                    │   dwellInProgress{...},     │
                                    │   dwellInReview{...}}}      │
                                    │   ← always HTTP 200         │
                                    └─────────────────────────────┘
```

**Trace the primary use case:** a user opens the (Phase 11) Activity page → one `GET /api/activity?scope=global&window=week` → handler validates params → GATE 1 check → SQL pulls in-scope linked projects + tasks-done concurrently with the errgroup firing `GetMergedClosed` per repo (cache-served most of the time) → in-Go stats computed over the tasks-done rows → one combined 200 JSON response.

### Recommended Project Structure
```
internal/api/
├── activity.go         # NEW: GET /api/activity handler + ActivityRoutes + stats compute
├── pullrequests.go     # existing: the degrade + two-gate template (read, don't change)
├── tasks.go            # existing: statusAtCol + source='manual' pattern (read, don't change)
├── respond.go          # existing: writeJSON/writeError (reuse)
└── routes.go           # existing: core Routes (ActivityRoutes is a sibling, like PullRequestRoutes)
internal/github/
├── service.go          # EXTEND: add mergedClosed cache entry + GetMergedClosed method
├── prlist.go           # EXTEND: new listCompletedReviews fn (own --json/--state/--limit) + shared classify helper
└── state.go            # existing: PRState (read-only reference)
cmd/kamacu/
└── serve.go            # EXTEND: api.ActivityRoutes(mux, db, ghSvc) alongside PullRequestRoutes
```

### Pattern 1: The degrade-via-State handler (mirror PullRequestRoutes)
**What:** Always HTTP 200; degradation rides in a `state` field, never in the HTTP status.
**When to use:** Any endpoint that shells out to `gh` (REVIEWS-04).
**Source:** `internal/api/pullrequests.go:47-88` [VERIFIED: codebase]
```go
// GATE 1 (GHSET-02): integration toggle off -> disabled, never spawn gh.
val, err := settings.Get(db, settings.KeyGithubIntegration)
if err != nil { writeJSON(w, http.StatusOK, github.Result{State: "error"}); return }
if val != "on" { writeJSON(w, http.StatusOK, github.Result{State: "disabled"}); return }
// ... GATE 2 implicit (only linked projects are queried) ...
// Activity endpoint: tasks+stats ALWAYS populate; reviews.state carries gh status.
writeJSON(w, http.StatusOK, activityResponse{Tasks: tasks, Reviews: revResult, Stats: stats})
```

### Pattern 2: Demand-driven per-repo cache (mirror Service.Get)
**What:** No ticker goroutine; the browser/request is the only trigger. In-flight dedup + drop-cache-after-N + attempt floor.
**When to use:** The new `GetMergedClosed` (D-03) — its OWN cache entry + 5min TTL, NOT coupled to the review-column 60s entry.
**Source:** `internal/github/service.go:109-146` [VERIFIED: codebase]. The new method mirrors `Get`'s gate ladder with `mergedClosedTTL = 5 * time.Minute` instead of `cacheTTL = 60 * time.Second`.

### Pattern 3: listPRs classification (extract + reuse)
**What:** `gh pr list` shells out via an **arg array** (never shell-interpolated), `cmd.Dir` selects the right gh host/account, and the outcome is classified into `ok`/`no_gh`/`auth_required`/`error` via exit-code-4 + stderr-substring sniffing (exit code 4 alone is unreliable — cli/cli#9338).
**When to use:** The new `listCompletedReviews` reuses this classification. **Recommend extracting a `classifyGhListError(runErr, stderr) string` helper** so both `listPRs` and the new fn share the exact same sniffing (no duplication drift).
**Source:** `internal/github/prlist.go:212-244` [VERIFIED: codebase]

### Pattern 4: ISO-8601 lexical sort + window cutoff (mirror reaper)
**What:** `done_at` is stored as millisecond ISO-8601 (`strftime('%Y-%m-%dT%H:%M:%fZ','now')`), which sorts lexically == chronologically. A lexical `>=` against a cutoff formatted the same way is a correct time comparison — no per-row parsing needed for the SQL filter.
**When to use:** The tasks-done window filter + ordering.
**Source:** `internal/reaper/reaper.go:158-166` [VERIFIED: codebase]. Cutoff format: `now.UTC().Add(-window).Format("2006-01-02T15:04:05.000Z")`.

### Anti-Patterns to Avoid
- **Don't couple the reviews-done search onto `fetchLists` / the review-column `repoEntry`.** D-03 is explicit: the review column is a 5s-poll hot path with 2 `--state open` searches; adding a third merged/closed search would 3x the gh load on that hot path for data it never uses. The new method has its OWN cache entry + 5min TTL.
- **Don't rely on the `--state closed` FLAG alone.** It currently includes merged (cli/cli #8102) but that is a **filed, unfixed bug** — a future gh could "fix" it to exclude merged, silently dropping reviews. **Always add `is:closed` to the `--search` string** (stable, documented to include merged). See Pitfall 2.
- **Don't omit `--limit`.** `gh pr list` defaults to 30. The open review queue fits in 30; a reviews-done history over weeks/months does NOT. Pass `--limit 100` (or higher). See Pitfall 1.
- **Don't compute median in SQL.** SQLite has no `MEDIAN`; the `percentile_cont` workaround is fiddly. Compute in Go (D-10).
- **Don't interpolate the `scope` param into SQL.** Parse+validate it (`global` | `workspace:N` | `project:N`) and use parameterized `?` for the numeric parts; branch on the parsed shape for the structural SQL difference. See Security Domain.
- **Don't let one slow/failing repo block the aggregate.** A per-repo context timeout inside the errgroup + degrade-on-error (D-07/D-08). Never propagate a single repo's error to fail the whole endpoint.
- **Don't double-count review work.** Tasks-done is `source='manual'` (D-12); reviews-done is the `gh`-sourced list. A `source='github_pr'` row moved to Done does NOT appear in tasks-done.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Concurrent fan-out with per-call error handling | goroutine + channel + WaitGroup + manual error slice | `golang.org/x/sync/errgroup` with `SetLimit(5)` | Bounded concurrency + first-error semantics in ~3 lines; dep already indirect in go.mod. (A WaitGroup alternative is acceptable if avoiding the dep.) |
| gh list classification (auth vs error vs no_gh) | Copy the exit-code+stderr sniffing | Extract `classifyGhListError` from `listPRs` | The sniffing is subtle (exit 4 alone is unreliable, cli/cli#9338); sharing one helper prevents drift between the two fetch functions |
| Per-repo cache state machine | A new ad-hoc cache | Mirror `Service.Get`'s `repoEntry` ladder | The demand-driven + in-flight-dedup + drop-after-N + attempt-floor pattern is battle-tested in this codebase; the new entry reuses the shape with a different TTL |
| Median / percentile | A hand-rolled selection algorithm over the SQL result | `sort.Slice` + middle/avg-of-two-middle (D-10) | N is tiny (a week/month of done tasks); the textbook median is correct and trivial |
| Scope param parsing | Regex on the raw query string | `strconv.ParseInt` after splitting on `:` | `scope=workspace:42` → split once on `:`, validate the prefix is in `{global,workspace,project}`, `ParseInt` the suffix; reject anything else as 400 |

**Key insight:** This phase's risk is NOT novel algorithms — it is correctly mirroring three existing patterns (the degrade handler, the per-repo cache, the listPRs classification) while adding exactly one new gh search dimension. The temptation to "improve" or refactor the shared `repoEntry` / `fetchLists` must be resisted (D-03): the separation is load-bearing.

> **Runtime State Inventory:** SKIPPED — this is a greenfield backend feature (new endpoint + new method), not a rename/refactor/migration. No stored data, live service config, OS-registered state, secrets, or build artifacts carry a renamed string. The only existing state touched is read-only (SQLite `tasks`/`projects` rows; the `gh`-sourced PR list). No data migration.

## Common Pitfalls

### Pitfall 1: `--limit` default-30 silently truncates reviews-done [NEW — not in CONTEXT.md]
**What goes wrong:** `gh pr list` defaults to `--limit 30`. The existing `listPRs` passes no `--limit` (fine for the ~handful of open PRs awaiting your review). A reviews-done history over a week/month for an active reviewer easily exceeds 30 — the list returns the 30 most-recently-updated and silently drops the rest.
**Why it happens:** Copy-pasting the existing `listPRs` command shape (which omits `--limit`) into the new fetch.
**How to avoid:** The new `listCompletedReviews` MUST pass `--limit 100` (or higher). Recommend 100 as a balance (covers ~weeks/months of per-repo reviewed-then-closed PRs for a single user); note it as a knob.
**Warning signs:** reviews-done count looks low; week vs month windows return nearly identical lists (both capped at 30).
[VERIFIED: `gh pr list --help` shows `-L, --limit int  Maximum number of items to fetch (default 30)`; `internal/github/prlist.go:216-221` omits the flag]

### Pitfall 2: `--state closed` flag is a filed, unfixed gh bug [NEW — refines D-05]
**What goes wrong:** D-05's literal wording ("`reviewed-by:@me --state closed` (`--state closed` includes merged in `gh`)") relies on the *current* gh behavior. That behavior — `--state closed` INCLUDING merged — is reported as a **bug** in cli/cli #8102 (gh 2.29.0+, still OPEN as of 2.82.0). The behavior also reversed historically: pre-2020 it EXCLUDED merged (#475, fixed via #513). A future gh "fix" to #8102 would silently drop all merged PRs from reviews-done.
**Why it happens:** The `--state closed` flag semantics conflict with the GitHub SEARCH `is:closed`/`state:closed` semantics, and gh's own maintainers haven't settled which is "correct."
**How to avoid:** **Add `is:closed` to the `--search` string** (`reviewed-by:@me draft:false is:closed`). The `is:closed` SEARCH QUALIFIER is the stable, documented contract that includes merged (GitHub discussion #5599 + search docs). Keep `--state closed` too for belt-and-suspenders (consistent with the existing `--state open` pattern), but the search qualifier is the authoritative filter.
**Warning signs:** merged PRs missing from reviews-done after a gh upgrade.
[VERIFIED: cli/cli #475, #8102, #1809 (official GitHub repo); GitHub search docs / discussion #5599]

### Pitfall 3: Coupling onto the review-column hot path (D-03 violation)
**What goes wrong:** Adding the merged/closed search to `fetchLists` or the existing `repoEntry.cachedAwaiting/cachedReviewed` 3x's the gh load on the 5s-poll review column for data it never renders.
**How to avoid:** Separate cache entry + separate method + separate runner (D-03). The new `GetMergedClosed` has its own map/field + 5min TTL.

### Pitfall 4: One slow/failing repo blocks the whole aggregate
**What goes wrong:** A sequential per-repo loop, or an errgroup without a per-repo timeout, makes the endpoint latency = slowest repo, or fails entirely on one repo's error.
**How to avoid:** `errgroup` with `SetLimit` + a per-repo `context.WithTimeout` (e.g. 10-15s). Each repo's `GetMergedClosed` degrades to its own `state`; the aggregate rolls up (D-08). A degrade is NOT a `errgroup.Go` error that cancels the group — return the degraded result, not an error.

### Pitfall 5: Scope param SQL injection
**What goes wrong:** Interpolating the raw `?scope=` value into the SQL `WHERE` clause.
**How to avoid:** Parse + validate the scope (`global` | `workspace:N` | `project:N`); branch structurally on the parsed shape (which `WHERE` clause to build); pass the numeric `N` as a parameterized `?`. Never string-concatenate user input. See Security Domain.

### Pitfall 6: Median of an even-length slice
**What goes wrong:** Off-by-one on the median (taking the upper-middle only, or averaging the wrong pair).
**How to avoid:** After `sort.Slice(durations, ...)`: if `len` is odd → `durations[len/2]`; if even → `(durations[len/2 - 1] + durations[len/2]) / 2`. Standard textbook. Add a unit test for both parities.

### Pitfall 7: In-window row missing timestamps (D-09 defensive)
**What goes wrong:** A row lands in-window (`done_at` set) but `in_progress_at`/`in_review_at` is NULL (e.g. a status-transition bug, or a pre-00006 row that slipped in). `time.Parse("")` fails.
**How to avoid:** Skip the row from the time-stat slice on any `time.Parse` failure (or NULL `*_at`); still count it in `taskCount`. `cycle.n` / `dwell*.n` may be `< taskCount`. This is the natural exclude (D-09), not a separate code path — just don't panic/abort on the parse failure.

## Code Examples

### The verified gh search for reviews-done (load-bearing)
```bash
# Source: cli/cli issues #475/#8102/#1809 + GitHub search docs (discussion #5599) + local gh 2.82.0 --help
# is:closed (search qualifier) = closed + merged — STABLE, documented.
# --state closed (flag) = currently also includes merged, but it's a filed bug (#8102) — belt-and-suspenders only.
# --limit 100 avoids the default-30 truncation (Pitfall 1).
# closedAt = set for BOTH merged and closed PRs → maps to completedAt.
gh pr list -R owner/name \
  --search "reviewed-by:@me draft:false is:closed" \
  --state closed \
  --limit 100 \
  --json number,title,closedAt,url
# -> [{"number":42,"title":"...","closedAt":"2026-07-25T14:30:00Z","url":"https://..."}]
```
[VERIFIED: local `gh pr list --help` confirms `--state {open|closed|merged|all}`, `--limit` default 30, and `closedAt`/`mergedAt` JSON fields]

### Window cutoff format (mirror reaper)
```go
// Source: internal/reaper/reaper.go:158-166 [VERIFIED: codebase]
// done_at is millisecond ISO-8601 (strftime '%Y-%m-%dT%H:%M:%fZ'); sorts lexically == chrono.
window := 7 * 24 * time.Hour // week; 30*24*time.Hour for month
cutoff := time.Now().UTC().Add(-window).Format("2006-01-02T15:04:05.000Z")
rows, err := db.Query(`
   SELECT t.id, t.title, t.done_at, t.in_progress_at, t.in_review_at, p.name, p.id
   FROM tasks t JOIN projects p ON p.id = t.project_id
   WHERE t.status = 'done' AND t.source = 'manual'
     AND t.done_at IS NOT NULL AND t.done_at >= ?
     AND <scope-clause>   -- project_id=? / p.workspace_id=? / (nothing for global)
   ORDER BY t.done_at DESC`, cutoff, /* scope args... */)
```

### In-Go stats (cycle/dwell/median — D-10/D-11)
```go
// Source: D-10 (locked) + stdlib sort/time. time.Parse layout matches the stored format.
const tsLayout = "2006-01-02T15:04:05.000Z"

type timeStat struct {
    N       int   `json:"n"`
    Min     int64 `json:"min"`     // seconds
    Max     int64 `json:"max"`     // seconds
    Median  int64 `json:"median"`  // seconds
}

func computeTimeStat(durations []time.Duration) timeStat {
    if len(durations) == 0 { return timeStat{} }
    sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
    var med time.Duration
    n := len(durations)
    if n%2 == 1 {
        med = durations[n/2]
    } else {
        med = (durations[n/2-1] + durations[n/2]) / 2
    }
    return timeStat{
        N: n,
        Min: int64(durations[0].Seconds()),
        Max: int64(durations[n-1].Seconds()),
        Median: int64(med.Seconds()),
    }
}

// Build the duration slices, skipping any row whose *_at is NULL or unparseable (Pitfall 7).
//   cycle         = done_at - in_progress_at   (needs both)
//   dwellInReview = done_at - in_review_at     (needs both)
// Note: see Open Question 3 for the dwellInProgress definition.
```

### Concurrent bounded aggregation (D-07) — errgroup variant
```go
// Source: golang.org/x/sync/errgroup (already indirect in go.mod). SetLimit bounds concurrency.
// Each repo's call returns a (ReviewDoneSummary-slice, state) pair; a degrade is NOT a group error.
func aggregateReviews(ctx context.Context, ghSvc *github.Service, repos []repoRow, force bool) ([]github.ReviewDoneSummary, string) {
    type perRepo struct{ prs []github.ReviewDoneSummary; state string; proj projectRow }
    results := make([]perRepo, len(repos))
    g, gctx := errgroup.WithContext(ctx)
    g.SetLimit(5)
    for i, r := range repos {
        i, r := i, r
        g.Go(func() error {
            rc, cancel := context.WithTimeout(gctx, 12*time.Second)
            defer cancel()
            prs, st := ghSvc.GetMergedClosed(rc, r.githubRepo, r.repoPath, force)
            // annotate provenance here (PRSummary has no project field — D-06)
            results[i] = perRepo{prs: annotate(prs, r.project), state: st, proj: r.project}
            return nil // never return an error — degrade rides in `state`
        })
    }
    _ = g.Wait()
    // roll up state (D-08): all ok → "ok"; some ok → "partial"; all degraded → worst
    return flatten(results), rollupState(results)
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `gh pr list --state closed` excludes merged (pre-2020) | `--state closed` INCLUDES merged (2.29+), but it's a filed bug | ~2023 (gh 2.29) | Don't rely on the flag; use `is:closed` search qualifier (stable) |
| Two `gh` searches for merged + closed | One search (`is:closed` covers both) | GitHub search model | D-05 is correct — one call, one `completedAt` |
| gorilla/websocket / separate cache libs | `*github.Service` demand-driven in-memory cache | v1.3/v1.5 | The new method reuses this exact pattern with a 5min TTL |
| Hand-rolled concurrency | `errgroup.SetLimit` | Go 1.21+ (SetLimit) | Bounded fan-out in stdlib-adjacent idiom |

**Deprecated/outdated:**
- **`--state closed` flag as the SOLE merged+closed filter** — fragile (filed bug #8102). Superseded by `is:closed` search qualifier.
- **`@xterm/addon-attach` / anything client-side** — irrelevant here (backend phase), but note this is a backend-only phase; no terminal/xterm work.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `is:closed` search qualifier includes merged PRs (stable, documented) | REVIEWS-01 / Code Examples | LOW — cross-verified against GitHub docs + discussion #5599 + 3 cli/cli issues. If wrong, merged PRs would be missing → two searches (`is:merged` + `is:closed`) needed. |
| A2 | `closedAt` is set for both merged and closed PRs | REVIEWS-01 / Code Examples | LOW — `closedAt` is "when the PR was closed"; a merge is a close. `mergedAt` is the merged-only field. Verified via gh `--help` JSON fields list. |
| A3 | Client-side window filtering on `closedAt` (vs server-side `closed:>=`) for cache coherence | Architecture / Open Q2 | MEDIUM — if the per-repo closed/merged set is huge for an active reviewer, fetching everything + `--limit 100` could still truncate deep history. Server-side date filter is the mitigation. See Open Question 2. |
| A4 | errgroup is the right concurrency primitive (vs WaitGroup) | Standard Stack | LOW — both work; errgroup is idiomatic and the dep is already indirect. |

**No `[ASSUMED]`-only package names are recommended** — the only external symbol (`errgroup`) is verified present in go.mod.

## Open Questions

1. **`--state closed` flag vs `is:closed` qualifier — RESOLVED (recommend both, qualifier authoritative).**
   - What we know: `--state closed` currently includes merged (filed bug #8102); `is:closed` search qualifier stably includes merged (docs). `closedAt` is the right date field.
   - Recommendation: `--search "reviewed-by:@me draft:false is:closed"` + `--state closed` + `completedAt ← closedAt`. This resolves D-05's "verify the exact flag" discretion item definitively.

2. **Server-side vs client-side window filter for reviews.**
   - What we know: existing `listPRs` fetches everything then reduces (no server-side date filter). The new 5min cache would be window-agnostic if filtering is client-side (one entry serves week OR month).
   - What's unclear: whether `--limit 100` fetches enough deep history for very active reviewers (server-side `closed:>=` would bound it precisely).
   - Recommendation: **client-side** for cache coherence (matches existing pattern + window-agnostic cache), with `--limit 100` to bound truncation risk. Add a server-side `closed:>=YYYY-MM-DD` qualifier ONLY if profiling shows the full-set fetch is too large — keep it as a documented optimization knob. Planner treats the exact choice as low-risk either way.

3. **Exact definition of `dwellInProgress` (STATS-03).**
   - What we know: cycle = `done_at − in_progress_at` (clear). dwellInReview = `done_at − in_review_at` (clear).
   - What's unclear: does `dwellInProgress` mean "total time in In Progress" = `done_at − in_progress_at` (same as cycle, redundant), OR "time in In Progress before moving to In Review" = `in_review_at − in_progress_at` (only meaningful for rows that entered In Review)?
   - Recommendation: STATS-03 says "time in In Progress AND time in In Review" — the natural per-column dwell. For rows that went In Progress → In Review → Done: dwellInProgress = `in_review_at − in_progress_at`, dwellInReview = `done_at − in_review_at`. For rows that skipped In Review (In Progress → Done directly): dwellInProgress = `done_at − in_progress_at`, dwellInReview excluded. **The planner should confirm this interpretation with the user** since it affects the numbers Phase 11 shows; it's a semantic choice, not a technical one. (cycle stays `done_at − in_progress_at` regardless.)

4. **`?refresh=1` propagation.**
   - What we know: the review-column endpoint uses `?refresh=1` → `force=true` (bypasses TTL, bound by `attemptFloor`).
   - What's unclear: should the activity `?refresh=1` propagate `force=true` to EVERY repo's `GetMergedClosed` in the aggregate?
   - Recommendation: yes — mirror the existing precedent; `force` fans out to all in-scope repos. Low-risk; planner can confirm.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build | ✓ | 1.26 (go.mod) | — |
| `gh` CLI | reviews-done fetch | ✓ | 2.82.0 | `Available()` gates spawns; `reviews.state="no_gh"` when absent |
| `modernc.org/sqlite` | tasks-done + stats SQL | ✓ | v1.52.0 (go.mod) | — |
| SQLite DB (existing) | all task/project reads | ✓ | (app's existing DB) | — |
| `golang.org/x/sync` (errgroup) | concurrent aggregation | ✓ (indirect) | v0.20.0 (go.mod) | stdlib WaitGroup + semaphore channel (zero new dep) |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none — all present. `errgroup` is optional (WaitGroup alternative documented).

## Security Domain

> `security_enforcement` is absent in `.planning/config.json` → treated as enabled. This phase is local-only, single-user (per PROJECT.md), read-only backend (no writes, no new auth surface). The security surface is narrow: parsing two query params and feeding them to parameterized SQL + an arg-array `gh` spawn.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | local-only single-user; no auth in this endpoint |
| V3 Session Management | no | read-only endpoint; no session mutation |
| V4 Access Control | no (weak) | local single-user; no per-resource authz. The scope param only narrows the single user's own data. |
| V5 Input Validation | yes | `scope` + `window` parsed/validated server-side; numeric scope parts via `strconv.ParseInt`; parameterized `?` SQL (never interpolate scope into SQL). Invalid scope/window → 400. |
| V6 Cryptography | no | — |

### Known Threat Patterns for the Go + SQLite + gh-shell-out stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| SQL injection via `scope` param | Tampering | Parse+validate scope; build the structural `WHERE` clause by branching on the parsed shape (not string concat); pass `N` as `?` placeholder. Established single-writer SQLite pattern (parameterized `?`). |
| Command injection via gh args | Tampering | `gh` is spawned as an **arg array** (`exec.CommandContext(ctx, "gh", "pr", "list", "-R", repo, ...)`), never via a shell string. The repo name comes from the DB (`projects.github_repo`), not the request. Consistent with `listPRs` (prlist.go:216). |
| Information disclosure via gh stderr | Information Disclosure | `listPRs` classifies errors but **never logs the stderr body** (prlist.go:234-243). The new fetch mirrors this — degrade to `auth_required`/`error` with no stderr leak. |
| SSRF / external service abuse | (n/a) | `gh` talks to GitHub only; repo/dir from DB; two-gate ladder (GATE 1 toggle) prevents spawns when integration off. |

## Sources

### Primary (HIGH confidence)
- **`internal/github/service.go`** (read in-session) — `Result`/`repoEntry`/`Service.Get` cache ladder; the exact pattern `GetMergedClosed` mirrors [codebase]
- **`internal/github/prlist.go`** (read in-session) — `listPRs` classification + arg-array spawn + `searchAwaiting`/`searchReviewed`; the template for the new fetch [codebase]
- **`internal/api/pullrequests.go`** (read in-session) — two-gate ladder + always-200 + `State`-field degrade template [codebase]
- **`internal/store/migrations/00006_task_status_timestamps.sql`** (read in-session) — the `*_at` schema (nullable TEXT, ms ISO-8601) [codebase]
- **`internal/reaper/reaper.go:158-166`** (read in-session) — `done_at` lexical-sort window-cutoff pattern + format [codebase]
- **`internal/api/tasks.go:486-495`** (read in-session) — `statusAtCol` + move-time stamping (last-entry-wins) [codebase]
- **Local `gh pr list --help`** (run in-session, gh 2.82.0) — confirms `--state {open|closed|merged|all}`, `--limit` default 30, `closedAt`/`mergedAt` JSON fields [VERIFIED: local gh]
- **cli/cli issue #8102** (webfetch) — `gh pr list --state closed` ALSO returns merged (gh 2.29.0+, filed as bug, still open) [VERIFIED: official cli/cli repo]
- **cli/cli issue #475** (webfetch) — historical: pre-2020 `--state closed` EXCLUDED merged; closed via PR #513 [VERIFIED: official cli/cli repo]
- **GitHub discussion #5599 + GitHub search docs** (websearch) — `is:closed` search qualifier includes merged; `is:merged`/`is:unmerged` disambiguate [VERIFIED: GitHub docs]

### Secondary (MEDIUM confidence)
- `go.mod` (read in-session) — `golang.org/x/sync v0.20.0 // indirect`, `go 1.26`, `modernc.org/sqlite v1.52.0` [codebase]
- `internal/api/routes.go`, `cmd/kamacu/serve.go` (read in-session) — route registration + `ghSvc` shared instance wiring [codebase]

### Tertiary (LOW confidence)
- None — all load-bearing claims are verified against the codebase or authoritative GitHub sources.

## Project Constraints (from CLAUDE.md / project instructions)

- **`not mention or co-author claude on a commit or a pull request`** — commit message hygiene (applies at commit time, not this research).
- **GSD workflow enforcement** — work proceeds through GSD commands; this research is part of that flow.
- **Tech stack lock** (from CLAUDE.md STACK section): Go backend (stdlib ServeMux, `modernc.org/sqlite`, `coder/websocket`, `creack/pty`), React frontend. This phase is Go-backend-only and uses only locked-stack components.
- **Single-writer SQLite** (`SetMaxOpenConns(1)`, parameterized `?`) — the tasks-done + scope queries follow this.
- **Two-gate ladder** (GHSET-02) — settings toggle + project linked before any `gh` spawn.
- **Degrade-don't-break** — GitHub always returns 200 with `State` field; never blocks the rest of the page.
- **`slog` to stderr**; no stdout writes from handlers.
- No AGENTS.md present in the working directory.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new deps; all versions verified in go.mod + local gh.
- Architecture: HIGH — pure pattern-mirror of existing, read-in-session code.
- Pitfalls: HIGH — both NEW pitfalls (limit-30, state-flag-bug) verified against official cli/cli issues + local gh help; load-bearing gh question fully resolved.
- gh search semantics: HIGH — cross-verified across 3 official cli/cli issues + GitHub search docs + local gh `--help`.

**Research date:** 2026-07-30
**Valid until:** 2026-08-29 (30 days — stable; the only fast-moving element is the `--state closed` flag behavior, which is mitigated by the `is:closed` qualifier recommendation)
