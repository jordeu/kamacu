---
phase: 11-activity-page-controls
reviewed: 2026-07-31T06:11:53Z
depth: standard
files_reviewed: 12
files_reviewed_list:
  - web/src/api/queries.ts
  - web/src/api/types.ts
  - web/src/App.tsx
  - web/src/components/activity/ActivityList.tsx
  - web/src/components/activity/ReviewsList.tsx
  - web/src/components/activity/ScopeSelector.tsx
  - web/src/components/activity/StatsStrip.tsx
  - web/src/components/activity/WindowToggle.tsx
  - web/src/components/sidebar/ProjectSidebar.tsx
  - web/src/lib/time.ts
  - web/src/lib/useActivityView.ts
  - web/src/pages/ActivityPage.tsx
findings:
  critical: 0
  warning: 3
  info: 3
  total: 6
status: issues_found
---

# Phase 11: Code Review Report

**Reviewed:** 2026-07-31T06:11:53Z
**Depth:** standard
**Files Reviewed:** 12
**Status:** issues_found

## Summary

The Phase 11 Activity page renders the Phase 10 `GET /api/activity` contract correctly across the happy paths: the wire types (`types.ts`) faithfully mirror the Go structs, the degrade-state branching in `ReviewsList` matches the D-12 "quietly empty" intent, the `n===0` em-dash guard runs before any `formatDuration` call, and the `completedAt DESC` client-side sort addresses the cross-repo unsorted-merge pitfall. No security vulnerabilities, crashes, or data-loss risks were found. The route registration (bare `/activity`, not wrapped in `BoardWorkspaceSync`) and the sidebar rail-reachability restructuring are sound.

The defects found are quality/robustness issues. The most consequential is **WR-01**: the `useActivity` `enabled` gate — the documented Pitfall 6 mitigation, designed specifically so the page can defer the fetch until the persisted scope resolves — is never wired by `ActivityPage`. The parameter exists with a default of `true`, the only caller takes the default, so first-time users (no saved scope) get a wasted `global` fetch that is immediately discarded when the default-promotion effect switches the scope to `workspace:N`. The plan, RESEARCH, PATTERNS, and UI-SPEC all call for the gate; the implementation omitted it.

## Warnings

### WR-01: `useActivity` `enabled` gate (Pitfall 6 mitigation) is never wired by ActivityPage

**File:** `web/src/pages/ActivityPage.tsx:24` (cross-ref `web/src/api/queries.ts:95-107`)
**Issue:** The Phase 11 design deliberately added an `enabled` parameter to `useActivity` so the page can defer the fetch "until the persisted scope resolves (Phase 11 RESEARCH Pitfall 6 — don't fire with a null-derived scope)." This is documented in four planning artifacts:
- `11-RESEARCH.md:400` — "Gate the query `enabled` until scope is a valid non-null grammar string."
- `11-PATTERNS.md:648` — `enabled,  // defer until scope resolves (RESEARCH Pitfall 6)`
- `11-01-PLAN.md:93` — "Include the `enabled` parameter so the page can defer the fetch…"
- `11-UI-SPEC.md:165` — "The query is `enabled` only once scope resolves to a valid grammar string."

`ActivityPage.tsx:24` calls `useActivity(scope, window)` without a third argument, so `enabled` defaults to `true` (`queries.ts:98`) and the fetch is never gated. Concrete consequence for first-time users (no `kamacu.activity.scope` in localStorage): render 1 initializes scope to the `"global"` fallback (`useActivityView.ts:64-66`) and fires `GET /api/activity?scope=global…`; the promotion effect (`useActivityView.ts:72-79`) then runs, flips scope to `workspace:<id>`, and the query re-fires for `workspace:<id>`. The first response is discarded (queryKey changed). The documented warning sign ("first paint shows Global when the user's last scope was a workspace") is partially realized as a wasted fetch and a brief wrong-scope flash on first open.

**Fix:** Gate the fetch until either a persisted scope exists or the active-workspace default has been adopted. In `useActivityView`, expose whether the scope has settled:

```ts
// useActivityView.ts — expose a "resolved" flag
const [scopeResolved, setScopeResolved] = useState(
  () => localStorage.getItem(ACTIVITY_SCOPE_KEY) !== null,
);
useEffect(() => {
  if (activeWorkspaceId !== null) setScopeResolved(true);
}, [activeWorkspaceId]);
// … return { scope, setScope, window, setWindow, scopeResolved };

// ActivityPage.tsx
const { scope, setScope, window, setWindow, scopeResolved } = useActivityView();
const { data, isLoading, isError, refetch } = useActivity(scope, window, scopeResolved);
```

(If the wasted fetch is deemed acceptable for a localhost app, the alternative fix is to delete the `enabled` parameter and the misleading comment so the contract stops promising a mitigation the code does not provide.)

### WR-02: `ScopeSelector` shows "Global" label for a stale/deleted scope

**File:** `web/src/components/activity/ScopeSelector.tsx:25-35`
**Issue:** `useDisplayName` resolves the saved scope's display name and falls back to the literal string `"Global"` when the referenced workspace/project can't be found (`ScopeSelector.tsx:31, 34`). But a stale scope is reachable at runtime: `useActivityView.readScope` validates only the *grammar* (`/^workspace:\d+$/`, `useActivityView.ts:34-38`) — it does NOT validate that the referenced ID still exists. If a user saves `kamacu.activity.scope = "workspace:5"` and workspace 5 is later deleted, on next visit:
- The trigger label reads `"Global"` (workspace 5 not found → fallback).
- The underlying `scope` state is still `"workspace:5"`, so `useActivity` keeps fetching `?scope=workspace:5` (which returns empty data for the dead workspace).
- No dropdown item is checked (`scope === "global"` is false; `scope === "workspace:5"` matches nothing in the list).

The trigger label actively contradicts both the actual scope and the rendered (empty) data, and the stale scope never self-heals — the user must manually re-pick. The `useActiveWorkspace` analog this hook claims to mirror (`useActivityView.ts:16-17`) resolves stale IDs against live rows; the Activity scope does not.

**Fix:** Either resolve the stale id against live rows and fall back to `"global"` *as the scope value* (not just the label), or surface the staleness honestly. Minimal fix in `useActivityView`:

```ts
// On mount, if the saved scope references a missing workspace/project,
// demote to "global" so the label, the query, and the dropdown agree.
```

Alternatively, in `useDisplayName`, return a distinct label like `"Global"` only when the scope actually is `"global"`, and something like `"—"` / `"Deleted workspace"` for a missing ID so the mislabel is at least visible.

### WR-03: `ReviewsList` external-link `aria-label` drops the PR title for screen readers

**File:** `web/src/components/activity/ReviewsList.tsx:80-95`
**Issue:** Each review entry is an `<a>` with both visible text content (`#42 Title 3h ago`) and `aria-label="Open PR #42 on GitHub"` (`ReviewsList.tsx:85`). Per the WAI-ARIA accessible name computation, when an element has `aria-label`, that label *replaces* the visible text content as the accessible name — screen readers announce "Open PR #42 on GitHub" and NOT the title or the timestamp. A screen-reader user scanning the reviews list hears only PR numbers, never the PR titles that distinguish them. (This matches the UI-SPEC prescription at `11-UI-SPEC.md:174`, so the spec itself is the source — but the a11y loss is real and worth flagging.)

**Fix:** Include the title in the accessible name, or drop `aria-label` and let the visible text be the name (relying on `target="_blank"` + a visually-hidden "opens in new tab" hint for the external-link affordance):

```tsx
<a
  key={review.number}
  href={review.url}
  target="_blank"
  rel="noreferrer"
  aria-label={`Open PR #${review.number} ${review.title} on GitHub`}
  …
>
```

## Info

### IN-01: `useActivity`'s `enabled` parameter is dead in practice

**File:** `web/src/api/queries.ts:95-107`
**Issue:** `useActivity` declares `enabled: boolean = true` and documents it as the Pitfall 6 gate, but the sole caller (`ActivityPage.tsx:24`) never passes it. The default is the only value ever used. This is the same root cause as WR-01, called out separately because it is also a maintainability smell: a future reader of `queries.ts` will believe the gate is live. Fix by wiring it (WR-01) or removing the parameter and updating the comment.

### IN-02: `ReviewsList` `href={review.url}` has no scheme validation

**File:** `web/src/components/activity/ReviewsList.tsx:81`
**Issue:** `href={review.url}` renders the backend-provided URL verbatim. React does not sanitize `href` against `javascript:` / `data:` schemes. The URL is constructed server-side from gh API data (`internal/github/prlist.go`, the `https://github.com/…` form), and the app is local-only/single-user, so the practical risk is very low — but defense-in-depth would assert the scheme before rendering. If `url` were ever sourced from less-trusted input (or a compromised/tampered gh response), a `javascript:` URL would execute on click.

**Fix:**
```ts
const safeUrl = review.url.startsWith("https://") || review.url.startsWith("http://")
  ? review.url : undefined;
// render <a href={safeUrl}> and degrade gracefully when undefined
```

### IN-03: `ScopeSelector` calls `useWorkspaces`/`useProjects` twice per render

**File:** `web/src/components/activity/ScopeSelector.tsx:26-27, 44-45`
**Issue:** `useDisplayName` (lines 26-27) and the `ScopeSelector` body (lines 44-45) each call `useWorkspaces()` and `useProjects()`. TanStack Query dedupes the network requests via queryKey, so there are no extra fetches, but the four hook invocations and the derived `useDisplayName` re-resolution on every render are redundant. Fetch the data once in the component and pass the resolved name in, or fold `useDisplayName`'s logic inline.

---

_Reviewed: 2026-07-31T06:11:53Z_
_Reviewer: the agent (gsd-code-reviewer)_
_Depth: standard_
