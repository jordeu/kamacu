# Phase 10: Activity Data & API - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-29
**Phase:** 10-activity-data-api
**Areas discussed:** Endpoint shape, Reviews-done fetch & cache, PR wire type for reviews-done, Cross-project aggregation, Stats: NULL timestamps & median

---

## Endpoint shape

| Option | Description | Selected |
|--------|-------------|----------|
| Combined (Recommended) | One GET /api/activity?scope=…&window=… returning {tasks, reviews, stats}; one TanStack query, one loading state; reviews cache-served most of the time | ✓ |
| Split endpoints | /api/activity/{tasks,reviews,stats} separately; independent loading/degradation but 3x frontend wiring | |
| Hybrid | tasks+stats combined, reviews separate; middle ground | |

**Follow-up — degradation envelope:**

| Option | Description | Selected |
|--------|-------------|----------|
| State field, always 200 (Recommended) | reviews = {state, stale, fetchedAt, prs} mirroring github.Result; whole response always HTTP 200 | ✓ |
| Reviews null on degrade | reviews: null when gh absent; loses the reason | |
| Per-repo degradation in the list | each entry carries its own state; over-engineered for REVIEWS-04 | |

**Follow-up — scope encoding:**

| Option | Description | Selected |
|--------|-------------|----------|
| Single scope param (Recommended) | ?scope=global|workspace:N|project:N — mutually exclusive, default global | ✓ |
| Separate optional params | ?workspace_id=N or ?project_id=N (absent=global); allows both-set ambiguity | |
| You decide | follow existing ?workspace_id= convention | |

**User's choice:** Combined endpoint; reviews degrade via state field (always 200); single scope param.
**Notes:** Phase 11 renders all three on one page → one query is simplest. Reviews degrade independently as a sub-object.

---

## Reviews-done fetch & cache

| Option | Description | Selected |
|--------|-------------|----------|
| Separate cache + method (Recommended) | New GetMergedClosed on *github.Service with its OWN per-repo cache + TTL; doesn't touch the review-column 60s hot path | ✓ |
| Extend existing repoEntry (third queue) | Add cachedMergedClosed to repoEntry, fetched in same cycle; 3x gh load on the 5s-poll path, wrong access pattern coupled | |
| No cache | Fresh gh per activity request; spammy on global scope, rejected by demand-driven-cache posture | |

**Follow-up — TTL:**

| Option | Description | Selected |
|--------|-------------|----------|
| 60s (same as review column) | Familiar but unnecessarily fresh for an occasional-view page | |
| 5min (Recommended) | Activity page is occasional; merged/closed don't change; 5x less gh load than 60s; ?refresh=1 bypasses | ✓ |
| 15min | Very conservative; feels stale if user merges and immediately checks | |
| You decide | | |

**Follow-up — merged vs closed distinction:**

| Option | Description | Selected |
|--------|-------------|----------|
| Treat both as 'completed' (Recommended) | One gh search (reviewed-by:@me --state closed covers both) + single completedAt; REVIEWS-02 doesn't require distinction | ✓ |
| Distinguish merged vs closed | Separate mergedAt/closedAt + state badge; needs 2 searches or extra fields; scope creep | |
| You decide | | |

**User's choice:** Separate cache + method; 5min TTL; treat merged+closed as one "completed".
**Notes:** The review column is a hot 5s-poll path — coupling a third search onto it would 3x gh load for data it never uses.

---

## PR wire type for reviews-done

| Option | Description | Selected |
|--------|-------------|----------|
| Dedicated ReviewDoneSummary (Recommended) | Focused type: number, title, completedAt, url, projectName/projectId/repo; keeps PRSummary untouched | ✓ |
| Extend PRSummary | Add CompletedAt + Repo/ProjectName to shared type; review column carries unused fields, coupling | |
| You decide | | |

**User's choice:** Dedicated ReviewDoneSummary type.
**Notes:** PRSummary has no repo/project field; provenance is annotated at aggregation time (the handler knows which project each repo belongs to).

---

## Cross-project aggregation

| Option | Description | Selected |
|--------|-------------|----------|
| Concurrent, bounded (Recommended) | errgroup (e.g. 5-wide); cache-served repos are fast; failing/slow repo doesn't block others; fastest wall-clock | ✓ |
| Sequential | One repo at a time; simpler but waits for slowest sequentially | |
| Concurrent + cap N | Hard cap on repos; prevents spam but silently drops project #11+ | |

**Follow-up — aggregate state:**

| Option | Description | Selected |
|--------|-------------|----------|
| ok / partial / degraded (Recommended) | ok if all succeeded; partial if some degraded (prs carries successful); degraded only if all failed/gate-blocked | ✓ |
| ok if any succeeded | simpler enum but hides that some repos were unavailable | |
| Per-repo status array | reviews.repos:[{repo, state}]; over-engineered for REVIEWS-04 | |

**User's choice:** Concurrent bounded; ok/partial/degraded aggregate state.
**Notes:** REVIEWS-04 satisfied — successful repos always surface; a failing repo never blocks the rest, nor the tasks list, nor the stats.

---

## Stats: NULL timestamps & median

| Option | Description | Selected |
|--------|-------------|----------|
| Exclude from time stats, count in total (Recommended) | count in STATS-01; exclude from STATS-02/03; carry n per stat | |
| Exclude entirely | drop from count AND stats; undercounts STATS-01 | ✓ (then clarified) |
| Approximate missing timestamps | noise; migration comment called banked timestamps "noise" if approximated | |

**Clarification — what "exclude entirely" means:**

| Option | Description | Selected |
|--------|-------------|----------|
| Window filters them anyway (Recommended) | rolling week/month from now excludes pre-00006 rows (months old); every in-window row has both timestamps; non-issue | ✓ |
| Drop from count AND stats | undercounts STATS-01 (only matters if a row lands in-window without timestamps) | |
| Reconsider: count them, exclude from time stats | | |

**Follow-up — computation location:**

| Option | Description | Selected |
|--------|-------------|----------|
| Go-side compute, standard median (Recommended) | fetch rows + timestamps, time.Parse + Sub, min/max/median in Go; SQLite julianday fiddly, no native MEDIAN; N is small | ✓ |
| SQL-side compute | julianday subtraction; verbose median via window funcs; no win | |
| You decide | | |

**Follow-up — stats object shape:**

| Option | Description | Selected |
|--------|-------------|----------|
| Counts + per-stat n/min/max/median (Recommended) | {taskCount, reviewCount, cycle:{n,min,max,median}, dwellInProgress:{...}, dwellInReview:{...}}; each carries n; seconds | ✓ |
| Counts + min/max/median (no n) | simpler but Phase 11 can't show "over N tasks" | |
| Add mean + percentiles | richer but STATS-02/03 only ask min/max/median; scope creep | |

**User's choice:** Window filters pre-00006 rows anyway (no special NULL handling); Go-side compute with standard median; counts + per-stat n/min/max/median shape.
**Notes:** Initial "exclude entirely" answer was clarified — the rolling window naturally excludes old pre-migration rows, so it's a non-issue in practice.

---

## Wrap-up: tasks-done scope

| Option | Description | Selected |
|--------|-------------|----------|
| Confirm: tasks-done = manual only (Recommended) | source='manual' only; github_pr rows are PR reviews covered by REVIEWS-01; no double-counting | ✓ |
| Explore more gray areas | | |
| I'm ready for context | | |

**User's choice:** tasks-done = source='manual' only (D-12).
**Notes:** A PR-review workspace moved to Done does NOT appear in tasks-done (would double-count review work).

## the agent's Discretion

- Exact `gh` flag/date verification for the merged/closed search (the one genuinely open technical question — researcher verifies live).
- New method/cache-entry naming in internal/github; ReviewDoneSummary struct field names/order.
- Errgroup bound value (5 suggested); per-repo timeout; ?refresh=1 propagation.
- Exact stats struct Go types (seconds as int64 vs float64); file organization; whether tasks-done reuses taskColumns or a dedicated scan.

## Deferred Ideas

- Distinguishing merged vs closed in reviews-done data (D-05 treats both as "completed").
- ACTFUT-01 custom date ranges / "All time" preset.
- ACTFUT-02 throughput trend charts / bar graphs.
- ACTFUT-03 CSV/JSON export.
- ACTFUT-04 per-agent/workspace breakdown stats.
- ACTFUT-05 events beyond done/merged.
- Time stats for reviews (permanently out of scope — STATS-04).
- Exact ?refresh param name + propagation (agent's discretion).

None pulled into Phase 10 — discussion stayed within the Activity Data & API boundary.
