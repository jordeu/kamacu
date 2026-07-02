---
phase: 23-worktree-cleanup-panel
plan: 05
subsystem: verification
tags: [verification, human-verify, worktree, cleanup, wtree-01, wtree-02, wtree-03, wtree-04, d-01]

# Dependency graph
requires:
  - phase: 23-worktree-cleanup-panel (plans 23-01/02/03/04)
    provides: the full panel surface (parser, extended removal core, endpoints, hooks, badge, section, dialogs)
provides:
  - end-of-phase verified, shippable Worktree Cleanup Panel
  - recorded human-verify checkpoint outcome (approved) for WTREE-01..04 + D-01
affects: [phase 23 completion — this is the blocking verification gate]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Human-verify gate for behavior no automated test can assert (no frontend test framework; live uid-70 ./.db blocked case)"

key-files:
  created:
    - .planning/phases/23-worktree-cleanup-panel/23-05-SUMMARY.md
  modified: []
---

# 23-05 — End-of-Phase Verification Gate — SUMMARY

**Outcome:** ✅ APPROVED. Both tasks passed; the panel is proven against a real Kamacu install.

## Task 1 — Full-phase build + vet + test gate (auto)

All green (only the STATE-noted pre-existing react-hooks lint advisories remain — zero new):

| Check | Result |
|-------|--------|
| `go build ./...` | exit 0 |
| `go vet ./...` | exit 0 |
| `go test ./...` (count=1) | exit 0 — all packages incl. `internal/api` (D-04 DELETE regression tests) and `internal/worktree` |
| `cd web && npm run build` (tsc -b + vite) | exit 0 |
| `cd web && npm run lint` | 21 problems = pre-existing baseline, **0 new** |

Note: `internal/api/TestSessionTmuxReattach` (a Phase 20/21 tmux-reattach test, untouched by Phase 23) flaked once under full-suite parallel load during the Wave 2 post-merge gate (`HasSession never became false`, 15s poll timeout); it passes in isolation (0.2s) and on full-suite re-run. Confirmed pre-existing flake, not a Phase 23 regression.

## Task 2 — Human-verify checkpoint (blocking) — APPROVED

The user exercised the live panel in Settings against the real install and confirmed all six behaviors:

1. **WTREE-01 — Listing:** every worktree listed grouped by project with classification badge + task/PR association + dirty/unpushed/stash chips; main checkouts excluded; header count present. ✓
2. **WTREE-04 — Orphans:** the `sched` project's Phase-21-leftover dirs surface as Orphan / Stale pointer with Remove / Clear-pointer controls. ✓
3. **D-10 — Refresh:** Refresh spins + updates on demand; no idle auto-refresh (no polling). ✓
4. **WTREE-02 — Normal force-remove:** confirm dialog names what's destroyed and shows "branch is kept"; type-the-dirname gate when dirty; worktree removed, branch retained, list refreshes. ✓
5. **D-01 — Blocked case (load-bearing):** the uid-70 mode-700 `./.db` worktree degrades to a `Blocked — needs manual removal` banner naming the path with a copyable `sudo rm -rf <path>` hint — not a 500, not a raw error, not a partial deregister; Copy copies the exact command and the app never runs it. ✓
6. **WTREE-03 — Bulk clean:** `Clean eligible` previews the exact server-computed safe set with why-eligible labels + "nothing forced" reassurance; confirm removes only those, leaving dirty/blocked behind. ✓

## Requirements proven

WTREE-01 (listing), WTREE-02 (force-remove keeps branch), WTREE-03 (bulk clean, never forces), WTREE-04 (orphans), and the load-bearing D-01 blocked-outcome degrade-don't-500 safety promise.

## Post-approval refinement (WTREE-01 scope reversal)

After approving the panel, the user observed it listed worktrees for still-active work (tasks not yet in Done, PRs still pending review) and decided the panel should read as a **cleanup queue, not a full inventory**. This reverses WTREE-01's original "list every worktree" wording (REQUIREMENTS.md WTREE-01 updated accordingly).

Implemented TDD as a backend-only change to `GET /api/worktrees` (`internal/api/cleanuppanel.go` `list()` + new `isCleanupCandidate`), commits:
- `459ec22` test(23): RED cleanup-candidates-only list filter
- `aed78d0` feat(23): GET /api/worktrees returns only cleanup candidates (hide active work)

New contract — a worktree appears iff it is a cleanup candidate:
- **Shown:** orphan (always), stale pointer (always — Clear-pointer housekeeping), referenced with task **Done** or PR **MERGED/CLOSED**. A done-but-dirty worktree still shows (manual force-remove candidate).
- **Hidden:** referenced in-progress task, referenced open/pending PR, and gh-unconfirmed PR state (degrade-don't-break → treated as still-active → hidden).
- Empty project groups are omitted; `counts.total`/`counts.orphaned` reflect only shown rows. `enumerate`/`computeEligible`/`remove`/`clearPointer`/JSON shape untouched. UI needs no change (renders whatever the endpoint returns).

Verification: `go build`/`go vet` exit 0; `go test ./internal/api/` green (new `TestWorktreeCleanupListShowsOnlyCandidates` + 3 updated list tests). A live re-check (rebuild + Refresh → only candidates appear) is recommended but non-blocking.

## Follow-up gaps

None. Panel is shippable.
