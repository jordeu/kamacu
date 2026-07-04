---
quick_id: 260704-a6q
description: fix board drag-and-drop overshooting to the next-next column
date: 2026-07-04
status: complete
---

# Quick Task 260704-a6q: Fix board drag-and-drop column overshoot — Summary

## What changed

`web/src/components/board/Board.tsx`: replaced `collisionDetection={closestCorners}`
with a pointer-first custom strategy `boardCollisionDetection`.

```ts
const boardCollisionDetection: CollisionDetection = (args) => {
  const pointerHits = pointerWithin(args);
  if (pointerHits.length === 0) return closestCorners(args);
  const cardHit = pointerHits.find((c) => typeof c.id === "number");
  return cardHit ? [cardHit] : pointerHits;
};
```

## Why

`closestCorners` ranks droppables by the dragged card's RECTANGLE-corner distance.
A card is a full column-width box, so dragged sideways its leading corners reach
into the far column and corner-distance minimization selected the next-next column
instead of the one under the cursor. `handleDragOver`'s live reflow (moving the
card into the hovered column grows it and shifts rects) made the corner-closest
target oscillate, so the adjacent column was effectively unreachable — the card
"jumped to Done" when dragging In Progress → In Review.

`pointerWithin` picks the droppable the cursor is literally inside; a column's
horizontal position is stable across the vertical reflow, so the target no longer
overshoots. Preferring a card over its enclosing column keeps the within-column
insert index precise; the `closestCorners` fallback preserves gap-hover and
keyboard dragging (no pointer coordinates).

## Verification

- `cd web && npm run build` (tsc -b + vite build): exit 0, green.
- Handler logic unchanged — only the collision-detection input to `DndContext`.
- No automated frontend tests exist for the board (dnd-kit collision is a
  browser-interaction behaviour); needs human visual re-check:
  - Drag In Progress → In Review lands in In Review (no overshoot to Done).
  - Reorder within a column still works.
  - Keyboard drag (Tab to a card, Space, arrows) still works.

## Notes

- `web/dist/index.html` build artifact restored to its committed state after the
  verification build (release-time embed concern, per project convention).

## Self-Check: PASSED
