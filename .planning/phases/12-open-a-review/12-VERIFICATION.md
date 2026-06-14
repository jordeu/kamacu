---
phase: 12-open-a-review
verified: 2026-06-14T11:40:00Z
status: passed
score: 5/5 success criteria verified (24/24 plan must-have truths)
re_verification:
  note: "Phase went through a gap-closure cycle (12-06 backend + 12-07 frontend). No prior VERIFICATION.md existed; this is the first verification, run after the human-verify checkpoint was APPROVED 2026-06-14."
---

# Phase 12: Open-a-Review Verification Report

**Phase Goal:** Clicking a PR card opens it as a full task-like review workspace — a worktree on the PR's branch with the same agent, bash, and diff tabs — without ever disturbing the project's primary checkout or leaking PR reviews onto the kanban board.
**Verified:** 2026-06-14T11:40:00Z
**Status:** passed
**Re-verification:** No — initial verification, run after the gap-closure cycle (12-06/12-07) and the user's end-to-end APPROVAL.

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria — the contract)

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Clicking a PR card opens a task-like review view backed by a worktree with the PR's branch checked out (fetch `refs/pull/<n>/head`, never `gh pr checkout`, never a fresh task branch), reusing agent/bash/diff tabs (GHREV-01) | ✓ VERIFIED | `PRCard.tsx` body `role="button"` → `useOpenReview.mutate` → navigate to `/projects/{id}/tasks/{taskId}`. `pullrequests.go:256` `CheckoutPR(...HeadRefOid, HeadRefName, n)`. `worktree.go:207-251` fetches `refs/pull/%d/head` then `worktree add -b <chosen> <path> <headOID>` — no `--detach`, no `gh pr checkout`. `TaskPage.tsx` renders the same TaskTabs (Agent/Description/Diff/bash). Test: `TestCheckoutPRNamedBranch`, `TestReviewCreateThenReattach` |
| 2 | Opening the same PR again reattaches to its existing review workspace instead of creating a duplicate (GHREV-02) | ✓ VERIFIED | `pullrequests.go:191-217` find-by `project_id + pr_number + source='github_pr'`; if `worktree_path` set → instant reattach, NO CheckoutPR. Test: `TestReviewCreateThenReattach` (same id, 1 row, dir intact), `TestReviewReprovisionWhenWorktreeNull` (Retry path) |
| 3 | The review diff is computed against the PR's own base merge-base, matching GitHub Files-changed (GHREV-03) | ✓ VERIFIED | `diffs.go:82-101` branches on `source=='github_pr' && pr_base_ref` → `FetchRef(base)` then `resolvePRBase` (local `refs/heads/<base>` else `origin/<base>`) → `diff.Compute` (merge-base, renderer reused). Manual tasks keep `ResolveBase`. Base re-resolved every request. Test: `TestDiffPRReviewUsesPRBase`. (GitHub-match confirmed by user end-to-end.) |
| 4 | PR review workspaces never appear as kanban cards (GHREV-04) | ✓ VERIFIED | `tasks.go` 5 `source = 'manual'` filters (listByProject:166, create:220, move-top:364, nextPosition:470, renumberColumn:486) + `/move` 409 guard (349-352 `if source != "manual"` → StatusConflict) + by-id queries unfiltered (get:246, reattach lookup). Test: `TestPRReviewNeverLeaksToBoard` (board exclusion + move 409 + status unchanged + positioning purity), `TestPRReviewExcludedFromCreatePositioning` |
| 5 | Checkout-safety: primary checkout HEAD unchanged after opening a fork PR whose branch collides with a local branch (GHREV-05) | ✓ VERIFIED | `worktree.go:185-188` `branchExistsLocally` guard + fallback chain headRefName → `pr/<n>` → `pr/<n>-<shortOID>`: NEVER reuses/moves an existing ref. `worktree add -b` only touches the new worktree. Test: `TestCheckoutPRLeavesSourceHeadUnchanged`, `TestCheckoutPRBranchInNewWorktreeOnly`, `TestCheckoutPRCollisionFallsBackToPRBranch` (fork "master" → `pr/8`). User confirmed end-to-end. |

**Score:** 5/5 success criteria verified.

### Plan Must-Have Truths (24/24 verified across 12-01..12-07)

All `must_haves.truths` in each plan's frontmatter verified against source. The 12-01 "DETACHED" truths are intentionally SUPERSEDED by 12-06's user-approved named-branch decision (not a deviation — see Notes). 12-06's named-branch truths verified in their place.

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `internal/worktree/worktree.go` | CheckoutPR (named branch, pr/<n> fallback, fetch refs/pull/<n>/head, worktree add -b) + FetchRef + branchExistsLocally | ✓ VERIFIED | Exact shape; pinned to headOID; GHREV-05 guard present |
| `internal/github/github.go` | ViewPR + PRDetail incl. Commits | ✓ VERIFIED | `gh pr view -R --json …,commits`; `Commits = len(raw.Commits)`; HeadRefName/Oid/BaseRefName/Body/Author |
| `internal/api/pullrequests.go` | POST review open-or-reattach + GET {n} live detail + shared prWireFrom | ✓ VERIFIED | Both gates (integration/link) before gh; find-or-create+reattach+reprovision; prWireFrom single builder |
| `internal/api/tasks.go` | 5 source='manual' filters + /move 409 + source/pr_number/pr_base_ref wire | ✓ VERIFIED | All present; by-id unfiltered |
| `internal/api/diffs.go` | PR diff vs pr_base_ref (FetchRef + merge-base origin/<base>, renderer reused) | ✓ VERIFIED | Source-branched; ResolveBase unchanged for manual |
| `internal/settings/settings.go` + `validate.go` | KeyPRReviewSeed + default + validate | ✓ VERIFIED | Const, default with <n>/<title>, length cap 2000 |
| `cmd/kangent/main.go` | PullRequestRoutes wired with wtSvc | ✓ VERIFIED | `PullRequestRoutes(mux, db, ghSvc, wtSvc)` line 132 |
| `web/src/api/pullRequests.ts` | useOpenReview + usePullRequestDetail (F5) + PRDetailWire(headRefName,commits) | ✓ VERIFIED | All present; shared prDetailKey |
| `web/src/components/board/PRCard.tsx` | clickable body open (↗ stopPropagation) | ✓ VERIFIED | role=button, in-flight guard, error feedback |
| `web/src/components/task/AgentTab.tsx` | durable once-per-session seed guard (seededSessionIds) | ✓ VERIFIED | Module-scope Set keyed by agentSession.id; one-shot paste |
| `web/src/pages/TaskPage.tsx` | source branches, merge line, seed-from-settings, F5 re-hydration | ✓ VERIFIED | All present; read-only title, omitted ⋯, single-line clickable header |
| `web/src/pages/SettingsPage.tsx` | pr_review_seed field in GitHub section | ✓ VERIFIED | Bound to pr_review_seed, gated by effectiveEnabled |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| PRCard body click | useOpenReview → navigate | openReview.mutate(pr.number,{onSuccess}) | ✓ WIRED | PRCard.tsx:47-51 |
| AgentTab Start | pasteApiRef.current.paste(seed) | one-shot per agentSession.id | ✓ WIRED | AgentTab.tsx:173-176 (gsd-tools confirmed) |
| pullrequests.go review | existing github_pr task | WHERE pr_number=? AND source='github_pr' | ✓ WIRED | pullrequests.go:192 |
| pullrequests.go review | CheckoutPR with HeadRefName | wtSvc.CheckoutPR(...HeadRefOid, HeadRefName, n) | ✓ WIRED | pullrequests.go:256 (gsd-tools reported "invalid regex" on its own pattern; verified manually) |
| tasks.go listByProject | board | WHERE project_id=? AND source='manual' | ✓ WIRED | tasks.go:166 |
| tasks.go move | 409 non-manual | source != 'manual' → StatusConflict | ✓ WIRED | tasks.go:349-352 |
| diffs.go get | merge-base origin/<pr_base_ref> | FetchRef(base) → resolvePRBase → diff.Compute | ✓ WIRED | diffs.go:91-103 |
| worktree CheckoutPR | git worktree add -b | named branch + collision pre-check | ✓ WIRED | worktree.go:249 (gsd-tools confirmed) |
| TaskPage | useSettings pr_review_seed | interpolate <n>/<title> | ✓ WIRED | TaskPage.tsx:273-279 (gsd-tools confirmed) |

Note: several key-link checks returned "Source file not found" from gsd-tools because the `from` field embeds a prose suffix (e.g. "internal/api/tasks.go listByProject") that the tool treats as part of the path. All were verified manually against source and are WIRED.

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| --- | --- | --- | --- | --- |
| TaskPage merge line | prDetail | usePullRequestDetail → GET .../pull-requests/{n} → live gh pr view | Yes (live gh) | ✓ FLOWING |
| TaskPage seed | settings.pr_review_seed | useSettings → GET /api/settings (Defaults-backed) | Yes | ✓ FLOWING |
| TaskPage Description (PR) | prDetail.body | live gh pr view body | Yes | ✓ FLOWING |
| DiffTab (PR) | server diff vs pr_base_ref | diffs.go merge-base origin/<base> | Yes (git) | ✓ FLOWING |
| AgentTab seed paste | seed prop | TaskPage interpolated setting | Yes | ✓ FLOWING |

The PR header degrades gracefully (task-field fallback) when prDetail is in flight or errors — intentional, not a hollow prop.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Go packages build | `go build ./...` | exit 0 | ✓ PASS |
| Full Go suite | `go test ./...` | all ok, exit 0 | ✓ PASS |
| Frontend typecheck+build | `npm run build` (tsc -b && vite build) | exit 0 | ✓ PASS |
| GHREV-05 HEAD-unchanged | `TestCheckoutPRLeavesSourceHeadUnchanged` | pass | ✓ PASS |
| GHREV-05 collision fallback | `TestCheckoutPRCollisionFallsBackToPRBranch` | pass | ✓ PASS |
| GHREV-01/02 create+reattach | `TestReviewCreateThenReattach` | pass | ✓ PASS |
| GHREV-03 PR-base diff | `TestDiffPRReviewUsesPRBase` | pass | ✓ PASS |
| GHREV-04 board exclusion + move 409 | `TestPRReviewNeverLeaksToBoard` | pass | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| GHREV-01 | 12-01, 12-04, 12-05, 12-06, 12-07 | PR card opens task-like review on a worktree with the PR branch checked out (fetch PR head ref, not a fresh task branch); reuse agent/bash/diff | ✓ SATISFIED | Truth 1; REQUIREMENTS.md marks Complete |
| GHREV-02 | 12-04, 12-05 | Reopen reattaches, no duplicate | ✓ SATISFIED | Truth 2 |
| GHREV-03 | 12-03, 12-05 | Diff vs PR base merge-base, matches GitHub | ✓ SATISFIED | Truth 3 |
| GHREV-04 | 12-02, 12-05 | PR reviews never appear as board cards | ✓ SATISFIED | Truth 4 |
| GHREV-05 | 12-01, 12-05, 12-06, 12-07 | Fork / colliding-branch PRs open without disturbing primary checkout HEAD | ✓ SATISFIED | Truth 5 |

All five GHREV requirements are declared across plan frontmatter, mapped to Phase 12 in REQUIREMENTS.md, and marked **Complete**. No orphaned requirements.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| internal/api/pullrequests.go | 203 | grep matched "create" in "half-created state" (comment) | ℹ️ Info | False positive — explanatory comment, no stub |
| web/src/api/settings.ts | 13 | comment "All four settings" now stale (five) | ℹ️ Info | Cosmetic; `Settings` is `Record<string, SettingEntry>` so pr_review_seed resolves dynamically — no functional impact |

No TODO/FIXME/PLACEHOLDER, no stub returns, no empty handlers, no hardcoded empty render data in any phase-12 source file.

### Human Verification Required

None outstanding. The phase's human-verify checkpoint was already APPROVED by the user on 2026-06-14 with end-to-end confirmation of: named branch checked out, primary checkout HEAD unchanged, diff matches GitHub, PR reviews not on the board, reattach works, seed injected once, configurable prompt, F5 re-hydration, and the bash-tab height fix. No NEW unverified must-haves were found, so `human_needed` does not apply.

### Gaps Summary

No gaps. All 5 ROADMAP success criteria, all 24 plan must-have truths, all artifacts (exist + substantive + wired + data flowing), and all key links are verified. Full Go suite, frontend typecheck/build, and the targeted GHREV-01..05 tests all pass. The named-branch checkout is the user-approved design (collision fallback preserves GHREV-05), and the configurable seed + single-line merge header are in-scope checkpoint additions — neither is a deviation.

---

_Verified: 2026-06-14T11:40:00Z_
_Verifier: Claude (gsd-verifier)_
