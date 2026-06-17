---
phase: 16-sharper-review-column
verified: 2026-06-17T12:05:00Z
status: passed
score: 10/10 must-haves verified
---

# Phase 16: Sharper Review Column Verification Report

**Phase Goal:** The Review column shows two independent at-a-glance signals per PR — your agent's session state and the PR's CI state — without confusing the two, and stops losing PRs from view after you review them (a "Recently reviewed" section holds reviewed PRs until merge/close).

**Verified:** 2026-06-17T12:05:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

The phase splits the two signals onto separate physical channels — agent state on the card's LEFT EDGE (a 3px colored rail), CI state on the RIGHT GLYPH (a bare lucide icon) — so the two are never confused. It adds a server-deduped "Recently reviewed" subsection that holds reviewed-but-still-open PRs until merge/close. This was implemented via a user-approved redesign at the human-verify checkpoint: the originally-planned reused `StatusDot` + separate `border-l` was dropped in favor of the unified left rail (documented in 16-02-SUMMARY.md and the revised 16-CONTEXT.md / 16-UI-SPEC.md). The redesign is the SIGNL-01/02 deliverable; the absence of a `StatusDot`/dot on the PR card is intentional and correct, not a gap.

### Observable Truths

| #   | Truth (must_have)                                                                                                   | Status     | Evidence |
| --- | ------------------------------------------------------------------------------------------------------------------- | ---------- | -------- |
| 1   | Endpoint returns a `reviewed` array beside `prs`, both riding the same state/stale/fetchedAt                        | ✓ VERIFIED | `Result.Reviewed []PRSummary json:"reviewed"` (service.go:24); both ride one `fetchedAt`/`Stale` in `resultLocked` (service.go:165-178) |
| 2   | A PR in `prs` (awaiting) is removed from `reviewed` before the response (server-side top-precedence dedup)          | ✓ VERIFIED | `dedupeReviewed` (prlist.go:311) called by `fetchLists` (prlist.go:302); `TestDedupeReviewed` 6 cases PASS |
| 3   | Both gh searches run in ONE cached Service refresh cycle (not two polls); one failure degrades both lists           | ✓ VERIFIED | `fetchLists` runs both `listPRs` sequentially, fail-fast on non-ok (prlist.go:293-304); cached under one `repoEntry` (service.go:34-43, 136-138) |
| 4   | `/api/agents/status` entries carry `prNumber` (nullable) + `source` so a PR card finds its linked session           | ✓ VERIFIED | `agentStatusEntry.PRNumber/Source` (agents.go:34-35); BOTH SELECTs widened (agents.go:89, 140); `TestAgentStatusPRLinkFields` PASS |
| 5   | Frontend `PullRequestsResponse` exposes `reviewed`; `AgentStatusEntry` exposes `prNumber`/`source`                  | ✓ VERIFIED | `reviewed: PRSummary[] | null` (pullRequests.ts:29); `prNumber: number | null` + `source: "manual" | "github_pr"` (agents.ts:12-13) |
| 6   | CI status renders as a bare lucide glyph (green Check / red X / static amber Circle); `none` renders nothing        | ✓ VERIFIED | `checksIcon` (PRCard.tsx:19-30); `size-3.5`, no `animate-spin`; gated on `pr.checks !== "none"` (PRCard.tsx:138) |
| 7   | A PR with a linked review session shows the agent state signal (rail) in BOTH sections                              | ✓ VERIFIED | `agentRail` + rail span keyed on `entry` (PRCard.tsx:40-53, 122-131); reviewed cards reuse PRCard verbatim (ReviewColumn.tsx:209-211) |
| 8   | The agent-state signal is a colored left rail: blue/idle, green/working, amber-pulse/waiting, gray/exited           | ✓ VERIFIED | rail palette mirrors StatusDot dotMeta: `bg-green-500`/`bg-amber-400`+pulse/`bg-blue-500/60`/`bg-zinc-600` (PRCard.tsx:43-52, 122-130) |
| 9   | A "Recently reviewed" subheader + reviewed PRCards render below the awaiting list, quietly omitted when empty       | ✓ VERIFIED | `reviewedSection` gated on `reviewed.length > 0` (ReviewColumn.tsx:198-213); appended to list + empty branches; no fabricated "(0)" |
| 10  | Reviewed cards reuse the exact same PRCard, opening/reattaching via the same useOpenReview                          | ✓ VERIFIED | `reviewed.map(pr => <PRCard pr={pr} projectId={projectId}/>)` (ReviewColumn.tsx:209-211); PRCard uses `useOpenReview` (PRCard.tsx:68) |

**Score:** 10/10 truths verified

### Required Artifacts

| Artifact                                          | Expected                                                | Status     | Details |
| ------------------------------------------------- | ------------------------------------------------------- | ---------- | ------- |
| `internal/github/prlist.go`                       | Second reviewed-by:@me search + two-list dedup          | ✓ VERIFIED | `searchReviewed`, `listPRs`, `fetchLists`, `dedupeReviewed` all present; `reviewed-by:@me draft:false` const |
| `internal/github/service.go`                      | `Result.Reviewed` + Service wiring both lists in one cycle | ✓ VERIFIED | `Reviewed` field, two-slice `repoEntry` cache, `fetchLists` default runner |
| `internal/api/agents.go`                          | `agentStatusEntry` gains `PRNumber`+`Source`, joined     | ✓ VERIFIED | struct widened; both SELECTs include `pr_number, source` (count=2); `prNumberOf` helper |
| `web/src/api/pullRequests.ts`                     | `PullRequestsResponse.reviewed` type                     | ✓ VERIFIED | `reviewed: PRSummary[] | null` present |
| `web/src/api/agents.ts`                           | `AgentStatusEntry.prNumber` + `source` types             | ✓ VERIFIED | both fields present |
| `web/src/components/board/PRCard.tsx`             | CI icon swap + agent rail (redesign) on PR cards         | ✓ VERIFIED | `checksIcon`, `agentRail`, rail span; StatusDot import/render absent by design |
| `web/src/components/board/ReviewColumn.tsx`       | "Recently reviewed" subheader + list, quiet-omit         | ✓ VERIFIED | `reviewedSection`, `reviewed.map`, quiet-omit gate, "You're all caught up" retained |

gsd-tools `verify artifacts`: 16-01 → 5/5 passed; 16-02 → 2/2 passed. All exist, all substantive, all wired, all data flowing.

### Key Link Verification

| From                              | To                                              | Via                                        | Status     | Details |
| --------------------------------- | ----------------------------------------------- | ------------------------------------------ | ---------- | ------- |
| `service.go Service.Get`          | `prlist.go fetchLists` (both searches)          | runner returns `(PRLists, state, err)`     | ✓ WIRED    | `lists, state := s.runner(...)` (service.go:129); `e.cachedReviewed = lists.Reviewed` (service.go:137); default runner `fetchLists` (service.go:77) |
| `agents.go status handler`        | `tasks.pr_number / tasks.source` columns        | both SELECTs widened to `pr_number, source`| ✓ WIRED    | agents.go:89 + 140 both contain `pr_number, source FROM tasks` |
| `pullrequests.go GET handler`     | `github.Result` (incl. Reviewed)                | `writeJSON(w, 200, svc.Get(...))`          | ✓ WIRED    | full Result serialized (pullrequests.go:80) — no field dropped |
| `PRCard.tsx`                      | `useAgentStatuses()` entry for this PR           | `agents.find(projectId && prNumber===pr.number)` | ✓ WIRED | drives rail color + aria-label (PRCard.tsx:73-77) |
| `ReviewColumn.tsx`                | `data.reviewed` from `usePullRequests`           | `reviewed.map(pr => <PRCard/>)` gated       | ✓ WIRED    | `const reviewed = data?.reviewed ?? []` (line 73) → `reviewed.map` (line 209) |

_Note on tooling:_ gsd-tools `verify key-links` reported 2 false-negatives — 16-01 links ("Source file not found") because the `from`/`to` fields contain multi-word phrases the path-parser mis-reads, and the 16-02 ReviewColumn link ("data.reviewed not found") because the source uses optional chaining `data?.reviewed` rather than the literal `data.reviewed` pattern string. Both were confirmed WIRED by direct grep (evidence above).

### Data-Flow Trace (Level 4)

| Artifact            | Data Variable          | Source                                   | Produces Real Data | Status     |
| ------------------- | ---------------------- | ---------------------------------------- | ------------------ | ---------- |
| `PRCard.tsx` (rail) | `entry` / `rail`       | `useAgentStatuses()` → `GET /api/agents/status` → live `mgr.List()` + DB tasks join | ✓ Yes (real session manager + DB) | ✓ FLOWING |
| `PRCard.tsx` (CI)   | `pr.checks`            | `PRSummary` from `usePullRequests` → `reduceChecks(statusCheckRollup)` from live `gh pr list` | ✓ Yes (real gh fetch) | ✓ FLOWING |
| `ReviewColumn.tsx`  | `reviewed`             | `data?.reviewed` ← `usePullRequests` ← `GET /pull-requests` ← `svc.Get().Reviewed` ← `fetchLists` (real `gh pr list reviewed-by:@me`) | ✓ Yes (real gh fetch, server-deduped) | ✓ FLOWING |

No hollow props: PRCard is invoked with live `pr` and `projectId` at both call sites (ReviewColumn.tsx:209-211 reviewed, :277-279 awaiting); rail data flows from the real shared 5s agent-status poll. No static `[]`/`{}` returns on the data path.

### Behavioral Spot-Checks

| Behavior                                            | Command                                         | Result                          | Status |
| --------------------------------------------------- | ----------------------------------------------- | ------------------------------- | ------ |
| Backend compiles through all callers                | `go build ./...`                                | exit 0                          | ✓ PASS |
| Dedup logic correct (6 cases)                       | `go test -run TestDedupeReviewed`               | 6/6 PASS                        | ✓ PASS |
| Agent-status PR-link fields (manual null / pr=42)   | `go test -run TestAgentStatusPRLinkFields`      | PASS                            | ✓ PASS |
| Full backend suite (gate ladder, cache, dedup)      | `go test ./...`                                 | 12/12 packages ok               | ✓ PASS |
| Frontend types compile                              | `npx tsc -b`                                     | exit 0                          | ✓ PASS |
| Frontend production build                           | `npx vite build`                                | exit 0, dist emitted            | ✓ PASS |
| Live rendering (rail colors, CI icons, sections)    | human-verify checkpoint (16-02 Task 3)          | APPROVED by user                | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan(s)   | Description                                                            | Status       | Evidence |
| ----------- | ---------------- | --------------------------------------------------------------------- | ------------ | -------- |
| SIGNL-01    | 16-01, 16-02     | Open review session → card shows agent status signal                  | ✓ SATISFIED  | Rail color = agent state (PRCard.tsx:40-53); data from `prNumber`/`source` join (agents.go). Served by the approved rail redesign instead of the dot. |
| SIGNL-02    | 16-02            | Open-session PR highlighted with a colored left edge                  | ✓ SATISFIED  | The 3px left rail IS the colored left highlight (PRCard.tsx:122-131). REQUIREMENTS.md text says "border" — the approved redesign unified border+dot into one rail (16-02-SUMMARY.md, user-approved). |
| SIGNL-03    | 16-02            | Agent signal applies in BOTH awaiting + Recently Reviewed sections    | ✓ SATISFIED  | Reviewed cards reuse PRCard verbatim → inherit rail (ReviewColumn.tsx:209-211) |
| CHECK-01    | 16-02            | CI status renders as an icon (check/cross/circle), not a dot          | ✓ SATISFIED  | `checksIcon` lucide glyph, `text-green/red/amber-500` (PRCard.tsx:19-30) |
| CHECK-02    | 16-02            | `none` renders no CI indicator, no gutter, no layout shift            | ✓ SATISFIED  | `pr.checks !== "none"` guard (PRCard.tsx:138); icon only inside the guard |
| REVWD-01    | 16-01, 16-02     | "Recently Reviewed" section lists open reviewed PRs as same cards     | ✓ SATISFIED  | `reviewed-by:@me draft:false` search (prlist.go:196) + `reviewedSection` (ReviewColumn.tsx:198-213) |
| REVWD-02    | 16-01            | Reviewed PR stays until merged/closed, then leaves                    | ✓ SATISFIED  | `--state open` bounds the search (prlist.go:219); merged/closed PRs drop naturally, no reaper needed |
| REVWD-03    | 16-01, 16-02     | A PR appears in exactly one section, top takes precedence             | ✓ SATISFIED  | `dedupeReviewed` server-side top-precedence drop (prlist.go:311); `TestDedupeReviewed` proves it |
| REVWD-04    | 16-01, 16-02     | Reviewed section shares poll/refresh/states; quietly omitted when empty| ✓ SATISFIED | Both lists ride one `usePullRequests` query (one cache cycle); `reviewed.length > 0` quiet-omit gate (ReviewColumn.tsx:199) |

All 9 requirement IDs declared in the plan frontmatters; all 9 mapped to Phase 16 in REQUIREMENTS.md. **No orphaned requirements** — every Phase-16 ID is claimed by at least one plan.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | No TODO/FIXME/placeholder/stub markers in any of the 7 modified source files | — | — |

Scan of `PRCard.tsx`, `ReviewColumn.tsx`, `pullRequests.ts`, `agents.ts`, `prlist.go`, `service.go`, `agents.go` found no TODO/FIXME/placeholder/not-implemented markers, no `animate-spin` on the CI icon, no leftover `size-2 rounded-full` CI dot, no fabricated "no recently reviewed" empty copy, and no hollow empty-array returns on the data path. The `PRLists{}` empty returns in `fetchLists` degrade-paths are correct degraded-state behavior (both lists empty together), not stubs.

### Human Verification Required

None outstanding. The live-rendering human-verify checkpoint (16-02 Task 3) covering all 8 visual requirements (CI icons, agent rail in both sections, blue/green/amber-pulse/gray rail states, Recently reviewed subsection, server-side dedup, quiet-omit, drop-on-merge/close) was completed and the user typed "approved" after the rail redesign was applied and re-verified against a live instance (documented in 16-02-SUMMARY.md).

### Gaps Summary

No gaps. All 10 must-have truths are verified, all 7 artifacts pass levels 1-4 (exist, substantive, wired, data flowing), all 5 key links are wired, all 9 requirements are satisfied, and every automated gate is green (`go build`, `go test ./...` 12/12, `tsc -b`, `vite build`). The one notable design point — SIGNL-01/02's REQUIREMENTS.md text still references a reused `StatusDot` and a separate left "border" — is a documentation lag, not an implementation gap: the user-approved checkpoint redesign deliberately unified both signals into a single colored left rail (right-glyph channel freed entirely for CI), which fully delivers the phase goal of "two independent at-a-glance signals without confusing the two." The revised 16-CONTEXT.md / 16-UI-SPEC.md and 16-02-SUMMARY.md document the shipped rail design.

---

_Verified: 2026-06-17T12:05:00Z_
_Verifier: Claude (gsd-verifier)_
