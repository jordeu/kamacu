---
phase: 20-kamacu-rebrand-brand
plan: 04
subsystem: ui
tags: [readme, branding, kamacu, documentation, screenshot]

# Dependency graph
requires:
  - phase: 20-kamacu-rebrand-brand
    provides: "bin/kamacu binary (Plan 01), KamacuMark + favicon + tab title (Plan 02), sidebar lockup + copy rename (Plan 03)"
provides:
  - "Shareable open-source README.md at the repo root (what/why + prerequisites + build/run + workflow walkthrough)"
  - "docs/kamacu-board.png — post-rebrand branded board screenshot embedded in the README"
  - "Human confirmation that the full Kamacu rebrand renders correctly and is distinct from the amber waiting dot"
affects: [21-runtime-path-rename, documentation, onboarding]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "README references a repo-relative screenshot path (docs/kamacu-board.png), never a hotlinked remote asset (T-20-07 mitigation)"

key-files:
  created:
    - README.md
    - docs/kamacu-board.png
  modified: []

key-decisions:
  - "README documents building/using the app only; runtime data paths stay ~/.kangent until Phase 21 and are intentionally not referenced"
  - "Screenshot captured AFTER the rebrand landed (D-12) so it shows Kamacu branding, not the old kangent UI"
  - "In-checkpoint UAT: the collapsed sidebar rail no longer shows the Kamacu brand mark (fix 7b5c170), revising Plan 03's D-08 — the mark is now expanded-only"

patterns-established:
  - "Pattern: brand mark is expanded-sidebar-only; the collapsed icon rail stays uncluttered (revised D-08)"

requirements-completed: [BRAND-03]

# Metrics
duration: continuation finalize
completed: 2026-07-01
---

# Phase 20 Plan 04: README + Human-Verify Rebrand Summary

**Shareable open-source Kamacu README with a post-rebrand board screenshot, plus a human-confirmed end-to-end verification that the ember-spark branding renders correctly and is distinct from the amber waiting dot.**

## Performance

- **Duration:** continuation finalize (checkpoint approval → asset + SUMMARY commits)
- **Completed:** 2026-07-01
- **Tasks:** 2 (both complete)
- **Files modified:** 2 created

## Accomplishments
- Wrote a concise, high-signal repo-root `README.md` (BRAND-03): title + kamaq/kamacu "the one who animates" naming story, prerequisites (Claude Code CLI, git, Go 1.26, Node 20.19+/22.12+, tmux), `make build` → single `bin/kamacu` binary, project → task → agent → review workflow walkthrough, and a screenshot embed.
- Captured `docs/kamacu-board.png` from the running, fully-branded UI and confirmed it renders in the README.
- Human-verify checkpoint (blocking) APPROVED: tab title + ember-spark favicon = Kamacu; expanded sidebar lockup = mark + "Kamacu" wordmark; collapsed rail shows only the toggle after the UAT fix; no user-facing "Kangent" copy remains; ember mark visually distinct from the amber waiting dot; existing `~/.kangent` install still opens (runtime paths unchanged this phase).

## Task Commits

Each task was committed atomically:

1. **Task 1: Write the repo-root README.md** - `c426eec` (docs)
2. **UAT fix during checkpoint: hide Kamacu brand mark in collapsed sidebar rail (D-08 revised)** - `7b5c170` (fix)
3. **Task 2: Capture the README screenshot (human-verify APPROVED)** - `56fa69b` (docs — screenshot asset)

**Plan metadata:** committed separately with this SUMMARY (docs: complete plan)

## Files Created/Modified
- `README.md` - Shareable open-source README for Kamacu (created, Task 1, `c426eec`)
- `docs/kamacu-board.png` - Post-rebrand branded board + agent terminal screenshot embedded by the README (created, Task 2, `56fa69b`)

## Decisions Made
- Kept the README about building/using the app; deliberately did NOT reference `~/.kamacu` runtime paths (those are Phase 21 — runtime paths remain `~/.kangent`).
- Captured the screenshot after the rebrand landed (D-12), embedded via a repo-relative path (no remote hotlink) — satisfies threat T-20-07.
- Human-verify was a blocking checkpoint; it was executed by the user (build + run + visual verification + screenshot capture) and returned "approved".

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - UAT feedback] Removed the Kamacu brand mark from the collapsed sidebar icon rail**
- **Found during:** Task 2 (human-verify checkpoint)
- **Issue:** During visual verification the brand mark at the top of the collapsed icon rail (per Plan 03's D-08) cluttered the rail; the user requested it be removed so the collapsed rail shows only the toggle above the project avatars.
- **Fix:** Hid the `KamacuMark` in the collapsed sidebar state; the mark is now expanded-sidebar-only. This revises Plan 03's D-08 (mark-in-collapsed-rail) — the expanded lockup (mark + "Kamacu" wordmark) is unchanged.
- **Files modified:** sidebar header component (collapsed-rail branding)
- **Verification:** Human re-verified the collapsed rail post-fix and approved.
- **Committed in:** `7b5c170` (fix)

---

**Total deviations:** 1 auto-fixed (1 UAT-driven refinement)
**Impact on plan:** The change refines Plan 03's D-08 branding decision (mark is now expanded-only) with no impact on the README or rebrand correctness. No scope creep.

## Issues Encountered
None — Task 1 (README) executed as written; Task 2 was a human-run verification that surfaced the single collapsed-rail refinement above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- The Kamacu rebrand is fully landed and human-confirmed; the repo has a shareable README with a branded screenshot.
- Phase 21 can proceed with the runtime path rename (`~/.kangent` → `~/.kamacu`); this phase intentionally left runtime paths untouched and verified an existing install still opens.

## Self-Check: PASSED

- `README.md` exists at repo root and embeds `docs/kamacu-board.png` — FOUND
- `docs/kamacu-board.png` exists and committed in `56fa69b` — FOUND
- Task 1 commit `c426eec` — FOUND
- UAT fix commit `7b5c170` — FOUND
- Screenshot commit `56fa69b` — FOUND

---
*Phase: 20-kamacu-rebrand-brand*
*Completed: 2026-07-01*
