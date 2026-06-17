---
phase: 16-sharper-review-column
plan: 02
subsystem: ui
tags: [react, typescript, lucide, tailwind, pr-review, xterm-adjacent, tanstack-query]

# Dependency graph
requires:
  - phase: 16-sharper-review-column
    provides: "16-01 wire contracts — PullRequestsResponse.reviewed (server-deduped), AgentStatusEntry.prNumber/source, the shared useAgentStatuses 5s poll"
  - phase: 12-open-a-review
    provides: "useOpenReview (open/reattach a PR review), source='github_pr' task↔PR link, /api/agents/status"
  - phase: 11-pr-review-column
    provides: "PRCard + ReviewColumn (awaiting list, count, collapse rail, visibility-paused poll, loading/degraded/empty states)"
provides:
  - "PRCard agent-state LEFT RAIL (3px colored span; working green / waiting amber-pulse / idle blue / exited gray) — keyed on the linked review session's agent-status entry, in BOTH sections (SIGNL-01/02/03)"
  - "PRCard CI as a bare lucide glyph (green Check / red X / static amber Circle), none renders nothing (CHECK-01/02)"
  - "ReviewColumn 'Recently reviewed' subsection — hairline-divided uppercase label + sub-count + reviewed PRCards, quiet-omitted when empty, server-deduped against awaiting (REVWD-01/03/04)"
  - "The locked card-row contract [title]·[CI icon if checks≠none]·[↗] with agent state on the separate left-edge channel"
affects: [verification, ReviewColumn, PRCard, future-card-richness]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Two-channel card signal: agent state on the left EDGE (rail), the PR's CI state on the right GLYPH — never side-by-side, so the two are never confused (the redesign's core principle)"
    - "agentRail(status) → {barClass, pulse, label}: a switch mirroring StatusDot's dotMeta palette but rendered as a positioned span, not a dot"
    - "Quiet-omit subsection: reviewed.length > 0 gates an entire <>fragment</> (label + cards); undefined/empty data yields null automatically across loading/degraded/empty branches"
    - "Reviewed cards reuse PRCard verbatim — rail/CI-icon/open-reattach come for free, zero second card"

key-files:
  created: []
  modified:
    - web/src/components/board/PRCard.tsx
    - web/src/components/board/ReviewColumn.tsx
    - .planning/phases/16-sharper-review-column/16-CONTEXT.md
    - .planning/phases/16-sharper-review-column/16-UI-SPEC.md
    - web/dist/index.html

key-decisions:
  - "Agent dot → left rail (checkpoint redesign): the planned reused StatusDot was REMOVED at the human-verify gate per user feedback — a filled dot beside the line-art CI glyph read as two clashing marks; agent state is now shown ONLY by a 3px colored left rail, freeing the right-glyph channel entirely for CI"
  - "Rail palette mirrors StatusDot's dotMeta verbatim (working bg-green-500 / waiting bg-amber-400 + animate-pulse / idle bg-blue-500/60 / exited bg-zinc-600); the waiting pulse (the attention cue) moved from the dot onto the rail"
  - "Rail is a single positioned span on a relative overflow-hidden card root (not a border-l override) so the 3px edge clips to the rounded corners and never fights the 1px border-border on the other 3 edges"
  - "Rail is aria-hidden; agent state is exposed instead via the card's aria-label ('Open review for PR #N — agent waiting for input') and title, keeping the visual a pure decoration"
  - "CI amber kept at text-amber-500 (a half-step deeper than the rail's amber-400) so the two ambers are never pixel-identical (UI-SPEC §Color guardrail)"
  - "Both carried Date.now()-in-render advisories (PRCard, ReviewColumn) cleared by hoisting to a const now = Date.now() at the top of each component body (D-50)"

patterns-established:
  - "Pattern: agent-state-as-edge — open-session state lives on the card's left edge (rail), never inline, so it cannot collide or misalign with content marks"
  - "Pattern: signal separation by channel — when two independent at-a-glance signals share a card, give each its own physical region (left edge vs right glyph) rather than two adjacent marks"

requirements-completed: [CHECK-01, CHECK-02, SIGNL-01, SIGNL-02, SIGNL-03, REVWD-01, REVWD-03, REVWD-04]

# Metrics
duration: ~40 min (incl. checkpoint redesign + human-verify gate)
completed: 2026-06-17
---

# Phase 16 Plan 02: Sharper Review Column — Rendering Summary

**The Review column's two at-a-glance signals split onto separate channels: your agent's state as a 3px colored LEFT RAIL (working green / waiting amber-pulse / idle blue / exited gray) and the PR's CI as a bare lucide glyph (green Check / red X / static amber Circle, `none` renders nothing) — plus a quiet-omitted "Recently reviewed" subsection reusing the same PRCard, all server-deduped against the awaiting list.**

## Performance

- **Duration:** ~40 min (Task 1+2 implementation, the checkpoint redesign, and the human-verify gate)
- **Started:** 2026-06-17T09:10:48Z (first task commit `0dfb421`)
- **Completed:** 2026-06-17T09:58:57Z (closeout)
- **Tasks:** 3 (2 auto, 1 human-verify checkpoint — APPROVED)
- **Files modified:** 4 source/doc + 1 build artifact (`web/dist/index.html`)

## Accomplishments

- **CI as a bare glyph (CHECK-01/02).** Replaced the colored CI dot with a `checksIcon()` mapping to a lucide glyph: `Check`/`text-green-500` (pass), `X`/`text-red-500` (fail), `Circle`/`text-amber-500` (running — STATIC, no `animate-spin`), all at `size-3.5`. The render stays gated on `pr.checks !== "none"`, so a PR with no checks shows no icon, no reserved gutter, and no layout shift.
- **Agent state as a left rail (SIGNL-01/02 — REDESIGNED).** A linked review session (matched via `agents.find(e => e.projectId === projectId && e.prNumber === pr.number)`) renders a 3px colored left rail whose color is the agent state. The card root became `relative overflow-hidden`; the rail is an `absolute inset-y-0 left-0 w-[3px]` span carrying `rail.barClass` and `rail.pulse && "animate-pulse motion-reduce:animate-none"`. No session → no rail, no gutter — byte-identical to a plain card.
- **Both signals on separate channels.** Agent state lives on the left EDGE, CI on the right GLYPH; nothing sits side-by-side. The locked row is now `[title flex-1]·[CI icon if checks≠none]·[↗]`, with the agent signal off-row on the left edge.
- **"Recently reviewed" subsection (REVWD-01/03/04).** `ReviewColumn` derives `const reviewed = data?.reviewed ?? []` and threads it into `ReviewStates`; a `reviewedSection` (a hairline `border-t border-border` divider + an uppercase `text-muted-foreground` label + a `reviewed.length` sub-count + `reviewed.map(pr => <PRCard …>)`) is appended below the awaiting list and below the "You're all caught up" empty block, inside the same scroll area. It's gated on `reviewed.length > 0`, so it's silently absent (no blank section, no "(0)") across the loading/degraded/empty branches. The column header count stays the awaiting count. Reviewed cards reuse `PRCard` verbatim, so they inherit the rail + CI glyph + open/reattach for free (SIGNL-03).
- **Lint cleanup (D-50).** Both carried `Date.now()`-in-render advisories cleared by hoisting `const now = Date.now()` in `PRCard` and `ReviewColumn`.

## Task Commits

Each task was committed atomically:

1. **Task 1: CI icons + agent dot + session left-border on PRCard** — `0dfb421` (feat) — _initial implementation per the as-written plan (StatusDot + border-l)_
2. **Task 2: Recently reviewed section in ReviewColumn** — `d6aabbd` (feat)
3. **Checkpoint redesign (agent dot → left rail):** code `c98472a` (feat — drop the StatusDot, introduce `agentRail()` + the positioned rail span, `web/src/components/board/PRCard.tsx` + `web/dist/index.html`); contract docs `53568fd` (docs — revise `16-CONTEXT.md` + `16-UI-SPEC.md` to the rail design)
4. **Task 3: human-verify** — APPROVED by the user (no code; the gate over Tasks 1–2 + the redesign)

**Plan metadata:** this commit (docs: complete plan)

## Files Created/Modified

- `web/src/components/board/PRCard.tsx` — `checksIcon()` (bare lucide glyph, 3 colors, no spinner); `agentRail(status)` → `{barClass, pulse, label}` mirroring StatusDot's `dotMeta`; card root → `relative overflow-hidden border border-border` via `cn()`; the rail span (`absolute inset-y-0 left-0 w-[3px]`, `aria-hidden`, pulse on waiting); agent state surfaced in `aria-label`/`title`; `const now` hoist. The `StatusDot` import/render was removed.
- `web/src/components/board/ReviewColumn.tsx` — `reviewed` derived + threaded into `ReviewStates`; `reviewedSection` (border-t divider + uppercase muted label + sub-count + `reviewed.map` of `PRCard`) appended to the list and empty branches; quiet-omit gate; `const now` hoist.
- `.planning/phases/16-sharper-review-column/16-CONTEXT.md` — revised to the rail design (commit `53568fd`).
- `.planning/phases/16-sharper-review-column/16-UI-SPEC.md` — §Card Row Contract / §Session-border matrix / §Color revised from dot+border to the left-rail model (commit `53568fd`).
- `web/dist/index.html` — rebuilt embed (hashed assets gitignored; only `index.html` committed, the established Phase-15 pattern).

## Decisions Made

- **Agent dot → left rail (the checkpoint redesign).** At the human-verify gate the user judged the planned reused `StatusDot` to be visually wrong: a filled dot sitting beside the line-art CI glyph read as two clashing marks, and the dot+border pair carried the same color redundantly. The fix was to drop the dot entirely and show agent state ONLY as the colored left rail. This is strictly better signal separation — the left edge is "your agent," the right glyph is "the PR's CI," and the two never sit side-by-side or misalign. SIGNL-01 ("show the agent's working/waiting/idle/exited state on a PR card with an open review session") is fully satisfied by the rail's color; SIGNL-02 ("colored left highlight for open-session cards") is now the SAME element rather than a separate border, collapsing two requirements onto one cleaner control.
- **Rail palette mirrors StatusDot verbatim** (`bg-green-500` / `bg-amber-400`+pulse / `bg-blue-500/60` / `bg-zinc-600`) so the column stays consistent with task-card semantics; the waiting `animate-pulse` (the attention cue) moved from the old dot onto the rail.
- **Rail as a positioned span, not a `border-l` override** — on a `relative overflow-hidden` root the 3px span clips to the rounded corners and never fights the 1px `border-border` on the other three edges (a `border-l-2` would have thickened one edge and fought the radius).
- **Rail is `aria-hidden`; state is exposed via `aria-label`/`title`** so the visual is pure decoration and screen-reader users get the agent state in words.
- **CI amber stays `text-amber-500`** (a half-step deeper than the rail's `amber-400`) so the two ambers are never pixel-identical (UI-SPEC §Color guardrail).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug / Design] Agent dot replaced by a left rail at the human-verify gate**
- **Found during:** Task 3 (human-verify checkpoint)
- **Issue:** The plan (and 16-01's contract) specified reusing the `StatusDot` component on PR cards plus a separate blue/amber `border-l-2`. On a live instance the user found the dot+border pair visually wrong: the filled dot clashed with the bare line-art CI glyph beside it, and the dot and border duplicated the same state color redundantly within a single card.
- **Fix:** Removed the `StatusDot` import and render; introduced `agentRail(status)` returning `{barClass, pulse, label}` (the StatusDot palette) and rendered a single 3px `absolute inset-y-0 left-0 w-[3px]` rail on a `relative overflow-hidden` card root. The rail color IS the agent state; waiting pulses; agent state is exposed via the card `aria-label`/`title`. This unifies SIGNL-01 and SIGNL-02 onto one control and cleanly separates the two card signals by channel (left edge vs right glyph).
- **Files modified:** `web/src/components/board/PRCard.tsx`, `web/dist/index.html` (code `c98472a`); `16-CONTEXT.md`, `16-UI-SPEC.md` (contract revision `53568fd`)
- **Verification:** `tsc -b` + `vite build` + `make build` green; `go vet ./...` clean; binary rebuilt at `bin/kangent`; user typed "approved" after re-verifying the rail design against a live instance.
- **Committed in:** `c98472a` (code) + `53568fd` (contract docs)

**Note on classification:** this is a visual-design correction made at a verification gate (Rule 1 — the as-written visual was wrong on a live instance), not an architectural change (Rule 4) — no data shape, API, schema, or component contract beyond the two touched files changed, and the 16-01 wire contracts (`AgentStatusEntry`, `PullRequestsResponse.reviewed`) were consumed verbatim.

---

**Total deviations:** 1 auto-fixed (1 design correction at the human-verify gate).
**Impact on plan:** Improved the deliverable — the two card signals are now unmistakably separate. All eight requirements still satisfied; SIGNL-01/02 are now served by one rail rather than a dot + a border. No scope creep, no contract change to 16-01.

## Issues Encountered

None beyond the documented checkpoint redesign. Tasks 1 and 2 built and verified green as written; the redesign was a user-driven refinement at the human-verify gate, applied and re-verified before approval.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- Phase 16 is complete: both plans (16-01 data layer, 16-02 rendering) have SUMMARYs; all 9 v1.5 requirements are satisfied (SIGNL-01/02/03, CHECK-01/02, REVWD-01/02/03/04 — REVWD-02 delivered by 16-01's open-state bounding + server dedup).
- Ready for **phase verification** (`/gsd:verify-work 16`) and then milestone close — the orchestrator runs phase verification next; this closeout did NOT run the phase verifier or mark the phase complete.
- The UI-SPEC and CONTEXT docs now describe the SHIPPED rail design, so any future card-richness work (deferred GHCARD-*/REVWD-FUT-*) starts from accurate contracts.

## Self-Check: PASSED

- `web/src/components/board/PRCard.tsx` — present; contains `agentRail`, `bg-green-500`/`bg-amber-400`/`bg-blue-500/60`/`bg-zinc-600`, `w-[3px]`, `checksIcon` (no `StatusDot`).
- `web/src/components/board/ReviewColumn.tsx` — present; contains `Recently reviewed`, `reviewedSection`, `reviewed.map`, `border-t border-border`.
- Task commits found in git log: `0dfb421`, `d6aabbd`, `c98472a`, `53568fd`.
- Gates (per the executing session): `tsc -b`, `vite build`, `make build`, `go vet ./...` green; `bin/kangent` rebuilt.

---
*Phase: 16-sharper-review-column*
*Completed: 2026-06-17*
