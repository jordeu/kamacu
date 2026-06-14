---
phase: 11-pr-review-column
verified: 2026-06-14T00:00:00Z
status: passed
score: 17/17 must-haves verified
human_verification_note: "Task 3 of plan 11-04 (running-app, 7-check end-to-end checkpoint) was APPROVED by the user on 2026-06-14. All in-browser behaviors (column appears/lists/cards/collapse-persists/refresh/empty/degraded/OFF-cascade) confirmed by the human at the checkpoint."
---

# Phase 11: PR Review Column Verification Report

**Phase Goal:** On a linked project's board, a user sees a live, collapsible "Review" column of the open PRs that need their review — with the column appearing/clearing as review state changes — without any of it ever blocking the board.
**Verified:** 2026-06-14
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

Truths are the union of the `must_haves.truths` across all four plans (11-01..11-04), each mapped to verified source.

| #   | Truth (source plan)                                                                                                  | Status     | Evidence |
| --- | ------------------------------------------------------------------------------------------------------------------- | ---------- | -------- |
| 1   | A function lists open review-requested PRs in a repo, excluding drafts (11-01)                                       | ✓ VERIFIED | `prlist.go:135-140` gh arg array with `--search "user-review-requested:@me draft:false"`, `--state open`; plus belt-and-suspenders `if r.IsDraft { continue }` at L173 |
| 2   | Each PR's statusCheckRollup is reduced server-side to pass\|fail\|pending\|none (11-01)                              | ✓ VERIFIED | `reduceChecks` (`prlist.go:76-121`); 23 passing table cases incl. both __typename variants + SKIPPED/NEUTRAL/STALE→non-failing |
| 3   | Missing/unauth/failing gh degrades to typed state (no_gh\|auth_required\|error), never panics (11-01)               | ✓ VERIFIED | `runGH` returns `"no_gh"` when `!Available()` (L132-134), `"auth_required"`/`"error"` classification (L154-162); `TestServiceGet/degrade_no_gh_never_panics` passes |
| 4   | Same repo polled twice inside TTL makes only one gh spawn (11-01)                                                    | ✓ VERIFIED | `service.go:108-114` gate ladder (60s TTL on fetchedAt); `TestServiceGet/TTL_caches_within_60s` passes |
| 5   | GET /api/projects/{id}/pull-requests always returns 200 with a typed state, never an error status (11-02)           | ✓ VERIFIED | `pullrequests.go` only non-200 is the 400 from `pathID` on a non-numeric id; `TestPullRequestsNeverNon200ForValidId` passes |
| 6   | github_integration off → state=disabled (GHSET-02 backend enforcement) (11-02)                                      | ✓ VERIFIED | `pullrequests.go:35-43` Gate 1 reads `settings.Get(db, KeyGithubIntegration)`, returns disabled before any gh; `TestPullRequestsDisabledWhenToggleOff` (runner NOT called) passes |
| 7   | No github_repo link → state=disabled (11-02)                                                                        | ✓ VERIFIED | `pullrequests.go:60-63` Gate 2; `TestPullRequestsDisabledWhenUnlinked` + `TestPullRequestsUnknownProjectDisabled` pass |
| 8   | ?refresh=1 forces a fresh fetch, plain GET serves cache (11-02)                                                     | ✓ VERIFIED | `pullrequests.go:65` `force := r.URL.Query().Get("refresh") == "1"`; `TestPullRequestsRefreshForcesFetch` passes |
| 9   | usePullRequests polls every 60s, paused when tab hidden (11-03)                                                     | ✓ VERIFIED | `pullRequests.ts:39-40` `refetchInterval: 60_000` + `refetchIntervalInBackground: false` |
| 10  | useRefreshPullRequests hits ?refresh=1 and writes result back to cache (11-03)                                      | ✓ VERIFIED | `pullRequests.ts:47-51` mutation hits `?refresh=1`, `onSuccess` `qc.setQueryData(["pull-requests", projectId], data)` |
| 11  | PR card shows number, title, author, "updated X ago", checks dot (omitted when none) (11-03)                        | ✓ VERIFIED | `PRCard.tsx:35` title, L69 `#{pr.number} · @{pr.author} · updated {formatAgo(...)} ago`, L38 dot rendered only when `pr.checks !== "none"` |
| 12  | Card body is inert; only interactive element is the ↗ external-link (11-03)                                         | ✓ VERIFIED | `PRCard.tsx` has no onClick/hover/cursor-grab/useSortable; only the `<a href={pr.url} target="_blank" rel="noreferrer">` (L57-65) |
| 13  | Collapsible "Review" column appears right of Done only when integration on AND project linked (11-04)              | ✓ VERIFIED | `ReviewColumn.tsx:39-41` gate `enabled && linked` else `return null`; appended in `Board.tsx:213` after `STATUSES.map`, inside flex row, outside DragOverlay |
| 14  | Column lists PR cards (server order), shows loading/empty/degraded states inline + manual refresh (11-04)           | ✓ VERIFIED | `ReviewColumn.tsx` `ReviewStates` (L152-237): Skeleton loading, degraded amber note, empty "You're all caught up", `prs.map(PRCard)`, refresh button L129-138 |
| 15  | Collapse state persists per-project in localStorage, **default collapsed**, collapsed shows PR count (11-04)        | ✓ VERIFIED | `ReviewColumn.tsx:56` key `kangent:review-collapsed:${projectId}`; L57-58 default-collapsed (`!== "0"` → absent key collapses); collapsed rail L76-105 with count badge |
| 16  | When the gate is closed the board is byte-for-byte unchanged (no column rendered) (11-04)                           | ✓ VERIFIED | `ReviewColumn` self-gates to `return null`; `Board.tsx` props unchanged (`{ tasks, projectId }`); column reads settings/project internally |
| 17  | The list mirrors live GitHub state — cards appear/clear as review state changes (GHCOL-04 / phase goal)             | ✓ VERIFIED | 60s poll + manual refresh refetch the always-200 endpoint; no local retention of reviewed PRs (`ReviewColumn.tsx:225-236` trusts server order, self-empties); confirmed live at the approved human checkpoint |

**Score:** 17/17 truths verified

### Required Artifacts

All four plans' `must_haves.artifacts` pass gsd-tools `verify artifacts` (12/12 artifacts, all_passed:true per plan). Each verified at Levels 1-3 (exists, substantive, wired); dynamic-data artifacts verified at Level 4.

| Artifact                                          | Expected                                                       | Status     | Details |
| ------------------------------------------------- | ------------------------------------------------------------- | ---------- | ------- |
| `internal/github/prlist.go`                       | PRSummary, reduceChecks, runGH gh shell-out + classification  | ✓ VERIFIED | 195 LOC; `reduceChecks`, exact gh arg array, no `sh -c`, cmd.Dir set; consumed by service.go |
| `internal/github/service.go`                      | per-repo cache Service (quota clone) with Result + Get ladder | ✓ VERIFIED | 165 LOC; `Service.Get`, `map[string]*repoEntry`, Runner seam, 60s/10s/maxFailures constants |
| `internal/github/prlist_test.go`                  | reduceChecks table tests + SKIPPED-heavy guard                | ✓ VERIFIED | `TestReduceChecks` (23 cases) + `TestReduceChecksSkippedHeavy`, both pass |
| `internal/github/service_test.go`                 | TTL/floor/force/isolation/degrade/drop tests                  | ✓ VERIFIED | `TestServiceGet` (7 subtests), all pass |
| `internal/api/pullrequests.go`                    | always-200, toggle+link-gated handler                         | ✓ VERIFIED | `GET /api/projects/{id}/pull-requests`, two gates → disabled, `svc.Get`, refresh param |
| `internal/api/pullrequests_test.go`               | always-200 + disabled-gate + refresh contract tests           | ✓ VERIFIED | 7 `TestPullRequests*` functions, all pass |
| `cmd/kangent/main.go`                             | github.Service construction + PullRequestRoutes wiring         | ✓ VERIFIED | L21 import, L131-132 `github.New(github.Config{})` + `PullRequestRoutes(mux, db, ghSvc)` after UsageRoutes |
| `web/src/api/pullRequests.ts`                     | types + usePullRequests + useRefreshPullRequests              | ✓ VERIFIED | 60s poll, bg-paused, per-project key, ?refresh=1 write-back |
| `web/src/lib/time.ts`                             | shared formatAgo lifted from QuotaIndicator                   | ✓ VERIFIED | `export function formatAgo`; QuotaIndicator local def count = 0, imports it |
| `web/src/components/board/PRCard.tsx`             | presentational PR card with checks dot + ↗ link               | ✓ VERIFIED | inert body, conditional dot (green/red/amber), ↗ anchor only interactive |
| `web/src/components/board/ReviewColumn.tsx`       | self-gating collapsible column + states + localStorage        | ✓ VERIFIED | gate→null, default-collapsed localStorage, all state branches, refresh, w-10 rail |
| `web/src/components/board/Board.tsx`              | ReviewColumn appended after STATUSES.map, outside dnd         | ✓ VERIFIED | L213 sibling inside flex row, no droppable wrapper |

### Key Link Verification

gsd-tools `verify key-links` produced false negatives for several links because the PLAN `from` fields carry a descriptive suffix (e.g. `internal/github/service.go Get()`) that the tool treated as a file path ("Source file not found") or a literal regex (`href=\{pr.url\}` escaping). All links were re-verified manually against the actual source and are WIRED.

| From                              | To                                  | Via                                            | Status  | Details |
| --------------------------------- | ----------------------------------- | ---------------------------------------------- | ------- | ------- |
| `service.go Get()`                | `prlist.go` runner                  | calls runner, classifies no_gh/auth_required   | ✓ WIRED | `service.go:122` `s.runner(...)`; `prlist.go:133/159` return typed states |
| `pullrequests.go` handler         | `github.Service.Get`                | `svc.Get(ctx, repo, repoDir, refresh=="1")`    | ✓ WIRED | `pullrequests.go:66` |
| `pullrequests.go` handler         | `settings.Get(KeyGithubIntegration)`| backend toggle gate (GHSET-02)                 | ✓ WIRED | `pullrequests.go:35` |
| `pullRequests.ts`                 | `GET .../pull-requests`             | useQuery queryFn get(...)                       | ✓ WIRED | `pullRequests.ts:38,49` |
| `PRCard.tsx`                      | `pr.url`                            | `<a href={pr.url} target="_blank">`            | ✓ WIRED | `PRCard.tsx:58` |
| `ReviewColumn.tsx`                | `usePullRequests(projectId)`        | poll + render state branch                      | ✓ WIRED | `ReviewColumn.tsx:68` |
| `ReviewColumn.tsx`                | `useSettings + useProjects` gate    | renders null unless on AND linked               | ✓ WIRED | `ReviewColumn.tsx:36-41` |
| `Board.tsx`                       | `ReviewColumn`                      | sibling after STATUSES.map, no useDroppable     | ✓ WIRED | `Board.tsx:19,213` |

### Data-Flow Trace (Level 4)

| Artifact                | Data Variable | Source                                              | Produces Real Data | Status     |
| ----------------------- | ------------- | --------------------------------------------------- | ------------------ | ---------- |
| `ReviewColumn.tsx`      | `data.prs`    | `usePullRequests` → GET endpoint → `svc.Get` → `runGH` (`gh pr list`) | Yes — real gh shell-out, no static fallback | ✓ FLOWING |
| `PRCard.tsx`            | `pr` prop     | `prs.map((pr) => <PRCard pr={pr} />)` — populated from `data.prs` | Yes — not hardcoded at call site | ✓ FLOWING |
| `pullrequests.go`       | response body | `svc.Get(...)` (real Service, real runGH in prod via `github.New(github.Config{})`) | Yes — `disabled`/`error` are intentional typed states, not empty stubs | ✓ FLOWING |

No HOLLOW or DISCONNECTED data paths. The `[]` and `null` defaults present (`prs = data?.prs ?? []`, `count = ... ? prs.length : null`) are correct loading/degraded fallbacks overwritten by the fetch, not stubs.

### Behavioral Spot-Checks

| Behavior                                          | Command                                                        | Result | Status |
| ------------------------------------------------- | ------------------------------------------------------------- | ------ | ------ |
| Backend compiles                                  | `go build ./...`                                               | BUILD OK | ✓ PASS |
| github + api tests pass                           | `go test ./internal/github/ ./internal/api/`                  | ok (cached), all subtests pass | ✓ PASS |
| reduceChecks + Service state machine              | `go test ./internal/github/ -run 'TestReduceChecks\|TestServiceGet' -v` | 23 + 7 subtests PASS | ✓ PASS |
| Endpoint contract (always-200, gates, refresh)    | `go test ./internal/api/ -run 'TestPullRequests' -v`          | 7 functions PASS | ✓ PASS |
| Frontend typechecks                               | `npx tsc --noEmit -p tsconfig.app.json`                       | TSC OK | ✓ PASS |
| gh search term excludes drafts (GHCOL-02)         | grep `user-review-requested:@me draft:false`                  | PRESENT | ✓ PASS |
| No shell interpolation                            | grep `sh -c` in prlist.go                                     | 0 matches | ✓ PASS |
| Poll + visibility-pause (GHCOL-04)                | grep `refetchInterval: 60_000` + `refetchIntervalInBackground: false` | both PRESENT | ✓ PASS |
| formatAgo lifted, no dup                          | `grep -c 'function formatAgo' QuotaIndicator.tsx`             | 0 (import only) | ✓ PASS |
| Column never a dnd droppable (D-13)               | grep `useDroppable\|SortableContext\|useSortable` in ReviewColumn | 0 matches | ✓ PASS |

### Requirements Coverage

All 6 phase requirement IDs (GHCOL-01..06) are declared across the plans' `requirements` frontmatter and map to verified implementation. No orphaned requirements — every ID REQUIREMENTS.md assigns to Phase 11 is claimed by at least one plan. GHSET-02 (Phase 10-owned) is the backend-enforcement contract validated here as part of the OFF cascade.

| Requirement | Source Plan(s)        | Description                                                        | Status      | Evidence |
| ----------- | --------------------- | ----------------------------------------------------------------- | ----------- | -------- |
| GHCOL-01    | 11-04                 | Collapsible "Review" column appears when linked + integration on  | ✓ SATISFIED | Truths 13, 16; ReviewColumn gate + Board wiring |
| GHCOL-02    | 11-01, 11-02          | Lists `user-review-requested:@me` open PRs, excluding drafts       | ✓ SATISFIED | Truth 1; gh search term + IsDraft skip; human checkpoint confirmed list matches GitHub |
| GHCOL-03    | 11-01, 11-03          | Card with number/title/author/"updated X ago" + checks indicator  | ✓ SATISFIED | Truths 2, 11; reduceChecks + PRCard layout |
| GHCOL-04    | 11-02, 11-03, 11-04   | Auto-refresh (tab-paused) + manual refresh, list mirrors GitHub   | ✓ SATISFIED | Truths 8, 9, 10, 14, 17; poll config + refresh + self-empty |
| GHCOL-05    | 11-01, 11-02, 11-04   | Loading/empty/degraded states inline, never blocking the board    | ✓ SATISFIED | Truths 3, 5, 14; degrade typed states + ReviewStates branches |
| GHCOL-06    | 11-04                 | Collapse/expand, collapsed shows count, state remembered          | ✓ SATISFIED | Truth 15; localStorage per-project key + count badge |
| GHSET-02    | (Phase 10) 11-02, 11-04 | OFF → all GitHub UI disappears, board pre-v1.3 (enforced here)   | ✓ SATISFIED | Truths 6, 7, 16; backend disabled gate + frontend self-gate to null |

### Anti-Patterns Found

None. The scan of all phase files found:
- No TODO/FIXME/XXX/HACK/PLACEHOLDER/"not implemented" comments.
- The only `return null` (`ReviewColumn.tsx:41`) is the intentional, architecturally-required self-gate (OFF cascade), not a stub — it is the mechanism that keeps the board byte-for-byte unchanged.
- The `?? []` / `?? null` defaults in ReviewColumn are loading/degraded fallbacks overwritten by the fetch (per the stub-classification rule, not stubs).
- No `sh -c` shell interpolation in the gh shell-out; the command is a verified arg array with `cmd.Dir`.

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | — | — | — |

### Human Verification Required

None outstanding. The phase's running-app checkpoint (Task 3 of plan 11-04) was a blocking human-verify gate covering the seven behaviors that cannot be checked programmatically (column appears on a real linked repo, list matches github.com, ↗ opens PR, collapse persists across reload, manual refresh + self-emptying, empty state, degraded non-blocking note, OFF cascade). This checkpoint was **APPROVED by the user on 2026-06-14** (recorded `status="complete" approved="2026-06-14"` in 11-04-PLAN.md), including explicit confirmation of the default-collapsed behavior.

### Gaps Summary

No gaps. All 17 observable truths are verified, all 12 declared artifacts pass Levels 1-4, all 8 key links are wired (manually confirmed where the gsd-tool produced parse-driven false negatives), all 6 requirement IDs are satisfied with no orphans, and no blocker anti-patterns exist. Backend (`go build`, `go test`) and frontend (`tsc`, prior `npm run build`) are green, and the blocking human-verify checkpoint was approved.

Note on the design change: the Review column defaults to COLLAPSED, which supersedes the original CONTEXT decision D-05/D-06 ("default expanded") per the user's request on 2026-06-14. The PLAN, UI-SPEC, and CONTEXT carry dated supersede notes, and the code (`ReviewColumn.tsx:57-58`, `localStorage.getItem(storageKey) !== "0"` → absent key collapses) implements default-collapsed correctly. This is the intended behavior, not a deviation.

---

_Verified: 2026-06-14_
_Verifier: Claude (gsd-verifier)_
