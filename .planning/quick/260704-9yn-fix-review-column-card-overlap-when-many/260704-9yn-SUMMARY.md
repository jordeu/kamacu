---
quick_id: 260704-9yn
description: fix review column card overlap when many cards
date: 2026-07-04
status: complete
---

# Quick Task 260704-9yn: Fix Review column card overlap — Summary

## What changed

Added `shrink-0` to the `PRCard` root className (`web/src/components/board/PRCard.tsx`).

## Why

The Review column card list (`ReviewColumn.tsx:144`) is a
`flex flex-col gap-2 overflow-y-auto` container. Flex children default to
`flex-shrink: 1`, so when enough PR cards accumulate to overflow the column the
flex algorithm compressed each card below its natural height rather than
scrolling. Because `PRCard` has `overflow-hidden`, the compressed card clipped
its `line-clamp-2` title's second line and the meta row overlapped the next card
— the reported overlap. `shrink-0` pins each card to its content height, so the
container scrolls (`overflow-y-auto`) instead of squashing the cards.

Fix applies to both the awaiting list and the "Recently reviewed" list (both
render `PRCard`).

## Verification

- `cd web && npm run build` (tsc -b + vite build): exit 0, green.
- Change is a single className token + explanatory comment; no type/logic change.
- Scope confirmed: `PRCard` is imported only by `ReviewColumn.tsx` (`grep` — the
  kanban `TaskCard` is a separate component), so the kanban board is unaffected.

## Notes

- `TaskCard` (kanban) shares the same flex-shrink pattern but has no
  `overflow-hidden`, so it doesn't visibly clip; left unchanged (not reported, and
  changing it would alter kanban behavior). Noted as latent in the PLAN.
- `web/dist/index.html` build artifact restored to its committed state after the
  verification build (release-time embed concern, per project convention).

## Self-Check: PASSED
