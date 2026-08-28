# Phase 16: Global view, Settings & bar integration - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-27
**Phase:** 16-Global view, Settings & bar integration
**Areas discussed:** View chrome & ⋯ menu, Honest degraded states, Settings Global section

---

## View chrome & ⋯ menu

### Q1: TaskPage has a back-arrow to its project board. /global has no owning board — what replaces it?

| Option | Description | Selected |
|--------|-------------|----------|
| No back-arrow (Recommended) | Mirrors Settings/Activity (plain h1 title, no back arrow — sidebar always available); /global is a top-level destination like /settings | ✓ |
| Back-arrow to board | Task-style back-arrow pointing at the active workspace's board (deep-links flip workspace) | |
| You decide | Planner picks based on route hierarchy | |

**User's choice:** No back-arrow (Recommended)
**Notes:** D-35.

### Q2: What does the global agent tab's ⋯ menu hold?

| Option | Description | Selected |
|--------|-------------|----------|
| Stop only (Recommended) | Natural degradation — Insert description / Insert review prompt are task/PR concepts; exact task parity | ✓ |
| Stop + Settings shortcut | Adds a "Configure in Settings…" item as a fast path to reconfigure | |

**User's choice:** Stop only (Recommended)
**Notes:** D-36.

### Q3: Should /global show the Claude quota indicator for the configured default agent?

| Option | Description | Selected |
|--------|-------------|----------|
| Show, same rule (Recommended) | Same rule as TaskPage: renders when the global default agent's engine is claude; cached queries, no new fetches | ✓ |
| Don't show | Keep the /global header minimal | |

**User's choice:** Show, same rule (Recommended)
**Notes:** D-37.

### Q4: Where does the D-06 folder-root un-isolation banner render in the /global view?

| Option | Description | Selected |
|--------|-------------|----------|
| Under header, always (Recommended) | Persistent one-line notice under the header, above the tab strip — visible on every tab (bash tabs run un-isolated too); hidden for managed clones | ✓ |
| Agent tab only | Notice above the Agent tab's terminal content only | |

**User's choice:** Under header, always (Recommended)
**Notes:** D-38 — the D-06 copy is locked from Phase 13.

---

## Honest degraded states

### Q1: With no root configured, what shape does the unconfigured state (GVIEW-04) take?

| Option | Description | Selected |
|--------|-------------|----------|
| Copy + CTA button (Recommended) | Full-page: short copy, one-line explanation, primary button to the Settings Global section; driven by GET /api/global (D-17) | ✓ |
| Copy only, no button | Copy pointing at Settings; user navigates via the sidebar | |

**User's choice:** Copy + CTA button (Recommended)
**Notes:** D-39.

### Q2: GET /api/global reports root_exists=false — what does /global render?

| Option | Description | Selected |
|--------|-------------|----------|
| Distinct state + path (Recommended) | Names the path verbatim + CTA to Settings; mirrors D-29's misconfiguration-vs-disk-rot distinction | ✓ |
| Same as unconfigured | Render the unconfigured empty state — simpler but blurs the two cases D-29 kept apart | |

**User's choice:** Distinct state + path (Recommended)
**Notes:** D-40.

### Q3: Root configured, nothing live — what does /global show on open?

| Option | Description | Selected |
|--------|-------------|----------|
| Task parity (Recommended) | Agent tab with "Start agent" button, "+" for bash tabs — GVIEW-01's full-shell read, zero new landing concepts | ✓ |
| Idle hero with details | Friendlier hero naming the configured agent + root above the Start button | |

**User's choice:** Task parity (Recommended)
**Notes:** D-41.

### Q4: If the configured root disappears on disk while global sessions are running, what does /global do?

| Option | Description | Selected |
|--------|-------------|----------|
| Warn, don't disturb (Recommended) | Terminals keep streaming; non-blocking warning line appears; user decides whether to stop/reconfigure | ✓ |
| You decide | Planner picks least-surprising behavior consistent with backend gates | |

**User's choice:** Warn, don't disturb (Recommended)
**Notes:** D-42.

---

## Settings Global section

### Q1: How does the Settings Global section capture the root (folder path or GitHub repo)?

| Option | Description | Selected |
|--------|-------------|----------|
| Segmented toggle (Recommended) | v1.4 AddProjectDialog pattern verbatim: "GitHub repo | Local folder" toggle, repo default when GH on, blocking "Cloning…" spinner, inline failures, dialog stays open | ✓ |
| Single auto-detect input | One smart input dispatching on the non-empty field (D-20) — lighter but a second mental model next to Add Project | |

**User's choice:** Segmented toggle (Recommended)
**Notes:** D-43.

### Q2: How is the Global section laid out in Settings?

| Option | Description | Selected |
|--------|-------------|----------|
| Summary card + dialog (Recommended) | Card shows current root (managed badge for clones) + hosts the agent selector + "Open Scratchpad"; "Change root…" opens the AddProjectDialog-derived dialog; Clear behind the same 409 gate | ✓ |
| Inline per-field rows | SettingsField-style rows with per-field commit; inline blocking-clone UX is a new shape | |

**User's choice:** Summary card + dialog (Recommended)
**Notes:** D-44.

### Q3: When a root change/clear is blocked by live global sessions (GCONF-04), how does the Settings UI surface the 409?

| Option | Description | Selected |
|--------|-------------|----------|
| Inline reasons list (Recommended) | {error, reasons[...]} rendered verbatim as an inline error list in the open dialog; dialog stays open, values preserved | ✓ |
| You decide | Planner picks surfacing consistent with existing error idioms | |

**User's choice:** Inline reasons list (Recommended)
**Notes:** D-45.

### Q4: Where does the default-agent selector live? (D-24: agent changes are never 409-gated)

| Option | Description | Selected |
|--------|-------------|----------|
| Inline on card (Recommended) | Inline agent dropdown on the summary card (ProjectSettingsDialog pattern), instant save, "applies at the next Start" hint | ✓ |
| Inside the dialog | Agent picker in the Change-root dialog — one dialog owns all config but hides a no-409 action behind a heavyweight dialog | |

**User's choice:** Inline on card (Recommended)
**Notes:** D-46.

---

## the agent's Discretion

- Shell derivation mechanics (shared shell extraction vs. sibling GlobalTaskPage)
- AgentTab/TaskTabs prop-loosening shape; scope-aware hook variants
- TS wire-type widening mechanics (AgentStatusEntry.source "global"; TermSession scope fields)
- GET /api/global consumption (query key, refetch cadence)
- Unconfigured/vanished state component shape and exact copy wording
- "Open Scratchpad" button styling; Clear-root placement inside the dialog
- Route registration mechanics in App.tsx

## Deferred Ideas

None — discussion stayed within phase scope. GT-FUT-01..07 remain parked; bar-row visual treatment lands at Phase 17 UAT (D-12).
