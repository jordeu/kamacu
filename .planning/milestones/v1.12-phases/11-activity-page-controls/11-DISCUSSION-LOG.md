# Phase 11: Activity Page & Controls - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-31
**Phase:** 11-activity-page-controls
**Areas discussed:** Sidebar entry & collapsed rail, Scope selector control, Stats presentation & layout, Reviews section & click-through

---

## Sidebar entry & collapsed rail

### Where should the Activity entry live in the sidebar?

| Option | Description | Selected |
|--------|-------------|----------|
| Footer next to Settings | Both are app-level (not workspace-scoped), so they pair naturally. Today the footer holds 'Add project' + the Settings gear. Activity joins them there. | ✓ |
| Top nav row above projects | A dedicated primary-nav slot at the top of the content area, separate from projects — more prominent, like a top-level destination. | |

**User's choice:** Footer next to Settings
**Notes:** Activity is app-level like Settings; pairing them in the footer keeps the project list the primary nav surface.

### How should Activity behave in the collapsed icon rail?

| Option | Description | Selected |
|--------|-------------|----------|
| Hide when collapsed | Match the Settings gear exactly — the footer vanishes in icon mode. Activity reached only from the expanded sidebar. | |
| Show as an icon when collapsed | Keep Activity reachable from the collapsed rail (an icon button like the project avatars). Settings stays hidden; Activity gets a rail icon. | ✓ |

**User's choice:** Show as an icon when collapsed
**Notes:** Activity should always be one click away even with the sidebar collapsed — unlike Settings.

### Which icon for Activity?

| Option | Description | Selected |
|--------|-------------|----------|
| Activity (pulse line) | lucide's 'Activity' icon — a heartbeat/pulse line. The natural semantic match for an 'activity feed'. | ✓ |
| BarChart3 | A bar chart icon — leans toward the statistics half of the page. | |
| You decide | Agent picks a fitting lucide icon (Activity is the leading candidate). | |

**User's choice:** Activity (pulse line)

---

## Scope selector control

### What control shape for the Global / Workspace / Project scope?

| Option | Description | Selected |
|--------|-------------|----------|
| Single dropdown | One dropdown menu (like the workspace switcher) listing 'Global', each workspace, and projects under each workspace. Fewer controls, mirrors the existing switcher pattern. | ✓ |
| Mode segmented + target | A segmented control (Global \| Workspace \| Project) to pick the MODE, then a second picker for which workspace/project. Explicit but two controls. | |
| Two dropdowns | A 'Workspace' dropdown + a 'Project' dropdown (Global = all unset). Mirrors the two dimensions literally. | |

**User's choice:** Single dropdown

### What scope should the Activity page open with by default?

| Option | Description | Selected |
|--------|-------------|----------|
| Active workspace | Open scoped to the currently-active workspace. Most relevant to what the user is working on — Activity follows the workspace context. | ✓ |
| Global | Always open at Global (everything across all workspaces). A consistent 'big picture' default. | |
| Current project | Open scoped to the project you were viewing (if any) when you clicked Activity. | |

**User's choice:** Active workspace

### Should the chosen scope + window survive a page reload?

| Option | Description | Selected |
|--------|-------------|----------|
| localStorage (no URL) | Persist scope + window in localStorage (kamacu.* prefix). Reload returns to your last view; Activity is never deep-linkable. Consistent with how workspaces work. | ✓ |
| URL query params | Put scope + window in the URL so the view is shareable/deep-linkable and reload-safe. Adds a new URL convention. | |
| Don't persist | Every open resets to the default. Simplest, but loses the user's last choice on reload. | |

**User's choice:** localStorage (no URL)

### How should the Week / Month toggle be presented?

| Option | Description | Selected |
|--------|-------------|----------|
| Segmented control | A small two-option segmented toggle (Week \| Month) next to the scope dropdown. A binary switch reads naturally as a segmented control. | ✓ |
| Dropdown | A second dropdown for Week/Month, matching the scope dropdown's control type for visual consistency. | |
| You decide | Agent picks the control (segmented is the leading candidate). | |

**User's choice:** Segmented control

---

## Stats presentation & layout

### How should the page lay out stats vs the two lists?

| Option | Description | Selected |
|--------|-------------|----------|
| Stats strip on top, lists below | A compact stats strip across the top (counts + cycle/dwell readouts), then tasks-done and reviews-done as stacked sections below. Scannable summary first, then the detail. Matches SettingsPage's single-column scroll. | ✓ |
| Two columns (lists side-by-side) | Tasks-done and reviews-done in side-by-side columns; stats as a strip above both. Uses horizontal space but lists get narrower. | |
| You decide | Agent lays it out; strip-on-top is the leading candidate. | |

**User's choice:** Stats strip on top, lists below

### How should the cycle/dwell numbers be presented (stats are seconds: min/max/median)?

| Option | Description | Selected |
|--------|-------------|----------|
| Compact rows: min · median · max | Each metric is one line showing 'min · median · max' with the count beside it ('over 12 tasks'). Dense, scannable, no charts. | ✓ |
| Stat cards grid | A grid of small cards — one card each for counts, and one card per min/median/max for cycle + each dwell. More space, more visual emphasis per number. | |
| You decide | Agent picks the readout layout; compact-rows is the leading candidate. | |

**User's choice:** Compact rows: min · median · max

### How should durations (seconds) be formatted for display?

| Option | Description | Selected |
|--------|-------------|----------|
| Adaptive d/h/m (largest 1-2 units) | Pick the two largest non-zero units, e.g. '1d 3h', '4h 20m', '45m', '30s'. Readable across the range from minutes to days. | ✓ |
| Hours-only | Always hours (e.g. '27.3h', '4.5h', '0.5h'). One unit, easy to compare, but days feel large. | |
| You decide | Agent picks the formatter; adaptive d/h/m is the leading candidate. | |

**User's choice:** Adaptive d/h/m (largest 1-2 units)

### What should the stats show when there are no tasks done in the window (cycle/dwell n=0)?

| Option | Description | Selected |
|--------|-------------|----------|
| Em-dash / 'no data' | Counts show 0; cycle/dwell rows show an em-dash or '—' so the layout stays stable without fake zeros. | ✓ |
| Show zeros | Render min/max/median as '0m' / '0h' when n=0. Uniform but implies a measured zero. | |
| You decide | Agent picks the zero-state treatment; em-dash is the leading candidate. | |

**User's choice:** Em-dash / 'no data'

---

## Reviews section & click-through

### What should the reviews section show when GitHub integration is off / gh absent (ACT-04)?

| Option | Description | Selected |
|--------|-------------|----------|
| Quietly empty (no hint) | Reviews section renders empty (no entries, no error) while tasks + stats display normally. The most literal reading of ACT-04's 'quietly degrades to empty'. | ✓ |
| Empty + muted hint | Reviews section empty, with a small muted line like 'GitHub integration is off'. Explains the absence without blocking. | |
| Hide the section entirely | Don't render the reviews section at all when GH is off — tasks + stats take the whole page. | |

**User's choice:** Quietly empty (no hint)

### REVIEWS-03: clicking a reviews-done entry should open its review workspace if one exists, else the PR. How should 'exists' be resolved?

| Option | Description | Selected |
|--------|-------------|----------|
| Open the PR URL always | Every review entry is an external link to the PR on GitHub. Simplest; no cross-project task lookups. Satisfies the 'else the PR' fallback literally. | ✓ |
| Navigate to review workspace if loaded | Resolve against already-loaded data: if a github_pr task with a matching pr_number is in the client's task cache, navigate to its task view; otherwise open the PR URL. No extra fetches. | |
| Always try to find + fetch | Actively query tasks for in-scope projects to find a matching review workspace, then navigate or fall back. Most accurate but adds fetches on click. | |

**User's choice:** Open the PR URL always
**Notes:** Deliberate simplification (captured as D-14 in CONTEXT.md) — the success criterion's "else the PR" fallback is applied universally; the workspace-preferring refinement is dropped to avoid cross-project task lookups on click. Flagged for UAT.

### How should the tasks-done and reviews-done lists be grouped and rendered (TASKS-02 / REVIEWS-02)?

| Option | Description | Selected |
|--------|-------------|----------|
| Grouped by project subheadings | Entries grouped under a project-name subheading, each entry one row. Matches how the activity endpoint already returns project provenance per entry. | ✓ |
| Flat list, project inline | A flat list sorted by date (most recent first), with the project name shown inline on each entry. | |
| You decide | Agent decides the list rendering; project-grouped is the leading candidate. | |

**User's choice:** Grouped by project subheadings

---

## Claude's Discretion

- Route registration (`/activity` bare under `<AppLayout />`), new `ActivityPage.tsx` shell, the `useActivity(scope, window)` query + wire types, the scope/window state hook shape, the collapsed-rail icon DOM structure (D-02 behavior locked, structure flexible), exact copy / column width / sticky control bar, duration unit labels + rounding, and within-list project ordering.

## Deferred Ideas

- Preferring the in-app review workspace over the PR URL on click (D-14 simplification) — future enhancement.
- Custom date ranges / "All time" preset (ACTFUT-01), charts/graphs (ACTFUT-02), CSV/JSON export (ACTFUT-03), per-agent/workspace breakdown (ACTFUT-04), events beyond done/merged (ACTFUT-05).
- Deep-linkable scope/window via URL (D-06 chose localStorage-only for v1.12).
- Time stats for reviews — permanently out of scope (STATS-04).
