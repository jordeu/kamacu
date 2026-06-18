---
phase: quick-260618-mlu
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - web/src/components/layout/ActiveSessionsBar.tsx
autonomous: false
requirements:
  - SBAR-06
must_haves:
  truths:
    - "Clicking a session row in the EXPANDED bar navigates to that task AND collapses the bar in the same gesture"
    - "Pressing Enter/Space on a focused session row also collapses the bar (keyboard parity with click)"
    - "The collapse persists: after a row-click the bar stays collapsed across reloads (localStorage key written to '1')"
    - "Toggling the bar via the bar/chevron still works exactly as before (no regression to the existing expand/collapse)"
  artifacts:
    - path: "web/src/components/layout/ActiveSessionsBar.tsx"
      provides: "Active Sessions Bar that auto-collapses on row open"
      contains: "setCollapsed"
  key_links:
    - from: "SessionRow.onOpen"
      to: "ActiveSessionsBar collapse state + localStorage"
      via: "collapse() helper invoked alongside navigate()"
      pattern: "collapse\\(\\)"
---

<objective>
When the Global Active Sessions Bar is EXPANDED and the user clicks (or keyboard-activates) a session row, the bar must automatically collapse after navigating to that task's agent view. Today it navigates but leaves the bar expanded, floating over the freshly-opened content.

Purpose: A row click is a "go look at this session" intent; the expanded list has served its purpose and should get out of the way (overlay floats up over content, D-03). Auto-collapsing on open mirrors how a menu/popover dismisses after a selection.

Output: A one-file behavioral change to `web/src/components/layout/ActiveSessionsBar.tsx` that collapses (and persists collapsed) the bar whenever a row's `onOpen` fires.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md

@web/src/components/layout/ActiveSessionsBar.tsx

<interfaces>
<!-- Current state mechanics in ActiveSessionsBar.tsx the executor must work with.
     Extracted from the file — no further codebase exploration needed. -->

Collapse state + persistence (lines 33-42):
```typescript
const storageKey = "kangent:sessions-bar-collapsed";
const [collapsed, setCollapsed] = useState<boolean>(
  () => localStorage.getItem(storageKey) !== "0",   // absent/"1" => collapsed
);
const toggle = () =>
  setCollapsed((c) => {
    const next = !c;
    localStorage.setItem(storageKey, next ? "1" : "0");  // "1"=collapsed, "0"=expanded
    return next;
  });
```

Row rendering + open handler (lines 95-105) — the ONLY call site to change:
```typescript
<SessionRow
  key={entry.taskId}
  entry={entry}
  isCurrent={String(entry.taskId) === openTaskId}
  onOpen={() =>
    navigate(`/projects/${entry.projectId}/tasks/${entry.taskId}`)
  }
/>
```

`SessionRow` already wires `onOpen` to BOTH the row `onClick` and the Enter/Space
`onKeyDown` (lines 217-223), so collapsing inside `onOpen` covers click AND
keyboard with one change — do NOT touch SessionRow.

Persistence idiom: collapsed persists as "1", expanded as "0"; an absent key reads
as collapsed (the `!== "0"` check). Any new collapse path MUST write "1" to keep
the reload-persisted state honest (must_have truth #3).
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Collapse (and persist) the bar when a session row is opened</name>
  <files>web/src/components/layout/ActiveSessionsBar.tsx</files>
  <action>
Implement SBAR-06's "open dismisses the list" behavior by collapsing the bar inside the row-open path.

1. Add a dedicated `collapse` helper next to `toggle` (after line 42). It must set collapsed to true unconditionally AND persist "1" to localStorage under the existing `storageKey`, so the collapsed state survives a reload (do NOT reuse `toggle`, which flips relative to current state). Mirror `toggle`'s persistence exactly — write "1" for collapsed. Name it `collapse`.

2. In the `<SessionRow ... onOpen={...}>` call site (lines 100-104), change `onOpen` to BOTH navigate and collapse: keep the existing `navigate(\`/projects/${entry.projectId}/tasks/${entry.taskId}\`)` call, then call `collapse()`. Wrap the two statements in a block-body arrow function. Order: navigate first, then collapse (navigation kicks off the route change; collapse then dismisses the now-redundant overlay).

Do NOT modify `SessionRow` itself — it already routes both `onClick` and the Enter/Space `onKeyDown` through `onOpen`, so this single call-site change gives click + keyboard parity (must_have truth #2).

Do NOT change `toggle`, the collapsed-bar/chevron handlers, the `storageKey`, or the persistence idiom — the manual expand/collapse must remain untouched (must_have truth #4).

Constraint: this is the whole change. No new state, no new props, no new effects, no refactor of the persistence model.
  </action>
  <verify>
    <automated>cd web && npm run build && npm run lint</automated>
  </verify>
  <done>
`web/src/components/layout/ActiveSessionsBar.tsx` contains a `collapse` helper that writes "1" to localStorage; the `SessionRow` `onOpen` invokes both `navigate(...)` and `collapse()`. `npm run build` (tsc -b + vite build) passes with no new type errors and `npm run lint` introduces no new advisories. SessionRow, `toggle`, and the chevron/bar toggle handlers are unchanged.
  </done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <what-built>
The Active Sessions Bar now auto-collapses when you open a session from the expanded list. Clicking (or pressing Enter/Space on) a row navigates to that task's agent view AND collapses the bar in one gesture; the collapse persists across reloads. Manual expand/collapse via the bar/chevron is unchanged.
  </what-built>
  <how-to-verify>
1. Run the app (`make dev` or your usual dev flow) and ensure at least one LIVE agent session exists so the bar shows entries.
2. Click the bottom bar (or the up-chevron) to EXPAND it — the session list floats up.
3. Click a session row.
   - Expected: the app navigates to that task's agent view AND the bar collapses to the thin counts strip (it does NOT stay expanded over the content).
4. Expand again, focus a row with Tab, press Enter (and separately Space).
   - Expected: same behavior — navigates and the bar collapses.
5. After a row-click reload the page.
   - Expected: the bar comes back COLLAPSED (the collapse persisted).
6. Regression check: expand and collapse the bar using the bar itself and the chevron a few times.
   - Expected: still works exactly as before; clicking the chevron does not navigate, and `e.stopPropagation()` on the chevron still prevents a double-toggle.
  </how-to-verify>
  <resume-signal>Type "approved" or describe what you saw (e.g., "bar stayed open" / "chevron broke").</resume-signal>
</task>

</tasks>

<verification>
- `cd web && npm run build` passes (tsc -b typecheck + vite build) with no new errors.
- `cd web && npm run lint` introduces no new advisories beyond the carried ~18-20 pre-existing react-hooks ones noted in STATE.md.
- Human-verify confirms: row open collapses + navigates (click and keyboard), collapse persists across reload, and the manual bar/chevron toggle is unregressed.

Note on automated unit test: this project ships no frontend test framework (no vitest/testing-library, no `test` script in `web/package.json`) — consistent with how the v1.6 phase-17 plans were verified (typecheck/lint + human-verify gate). The build+lint command is the automated gate; the human-verify checkpoint covers the runtime behavior. Standing up a test harness is out of scope for this one-line quick fix.
</verification>

<success_criteria>
- Clicking a session row in the expanded bar navigates to the task AND collapses the bar.
- Enter/Space on a focused row does the same (keyboard parity).
- The collapse persists across reloads (localStorage written to "1").
- The existing manual expand/collapse (bar body + chevron, with `stopPropagation`) is unchanged.
- Only `web/src/components/layout/ActiveSessionsBar.tsx` is modified; build and lint are green.
</success_criteria>

<output>
Create `.planning/quick/260618-mlu-when-the-status-bottom-bar-is-expanded-a/260618-mlu-SUMMARY.md` when done.
</output>
