# Phase 17: Global Active Sessions Bar - Context

**Gathered:** 2026-06-18
**Status:** Ready for planning

<domain>
## Phase Boundary

A persistent **bottom status bar** in the app shell that surfaces every **live** Claude agent session across **all projects** at a glance. Collapsed by default it shows global stats; expanded it lists every active session (clickable to jump straight into that task's full-page agent view, cross-project). Covers SBAR-01..SBAR-10.

Discussion clarified HOW to present and interact with the bar. Adding new capabilities (notifications, per-session actions, terminal sessions, duration) is out of scope — those are deferred (see below).
</domain>

<decisions>
## Implementation Decisions

### Collapsed summary
- **D-01:** Collapsed bar shows **per-state colored counts** (working / waiting / idle) using the existing `dotMeta()` color language (green / amber / zinc) plus a **total** active count. The waiting count keeps the **amber pulse** so it draws the eye without expanding (SBAR-02/03).
- **D-02:** With zero active sessions the bar **stays present** (never hides) and shows muted **"No active sessions"** text — it recedes, never grabs attention (SBAR-09).

### Expand behavior
- **D-03:** Expanding **floats a panel UPWARD over the content** (popover/overlay anchored to the bottom bar). Main content and the height-sensitive **xterm terminals never reflow or resize** — this is the explicit reason overlay was chosen over a docked "push content up" layout.
- **D-04:** The **whole collapsed bar is the click target** to toggle expand/collapse (mirrors `ReviewColumn`'s collapsed rail); a chevron indicates direction. State persists in `localStorage` under a **global** key (e.g. `kangent:sessions-bar-collapsed`), **default collapsed** — absent key ⇒ collapsed, matching the ReviewColumn `!== "0"` idiom (SBAR-07).

### List shape
- **D-05:** Expanded list is a single **FLAT list across all projects**, sorted **attention-first: waiting → working → idle**. No grouping by project. Project name is shown on each row.
- **D-06:** Each row shows: a **state indicator** (reuse `dotMeta()` dot/colors), the **project name**, and the **task or PR title**. PR-review sessions (`source='github_pr'`) get a small **`#<n>` PR badge**.
- **D-07:** The row matching the **currently-open task** is subtly **highlighted** ("you are here").
- **D-08:** Clicking a row **navigates to that task's full-page agent view** (`/projects/:projectId/tasks/:taskId`) via react-router, including when the task is in a different project than the one currently open (SBAR-06). Both `taskId` and `projectId` are already on each feed entry.

### Scope edges
- **D-09:** The bar lists **ONLY live sessions** (status `working`/`waiting`/`idle`). Exited/resumable sessions — including post-restart DB-derived entries — are **EXCLUDED**; they drop off the bar the moment the agent exits and stay discoverable on the card / via Resume. ("Show exited briefly" was considered and rejected — keeps "active sessions" honest.)
- **D-10:** Many-sessions overflow: the expanded panel is **height-capped and scrolls** (reuse the `overflow-y-auto` idiom from `ReviewColumn`). No count cap / truncation.

### Data / backend (SBAR-10)
- **D-11:** The ONLY backend change is widening the `GET /api/agents/status` query to **JOIN `tasks.title` and the project `name`** onto each entry — in **both** passes in `internal/api/agents.go` (the manager-derived pass and the DB-derived/post-restart pass). **No new endpoint, no new DB migration** (`tasks.title` and `projects.name` already exist; same mechanical shape as v1.5's `prNumber`/`source` SELECT widening). The wire type `agentStatusEntry` (Go) + `AgentStatusEntry` (TS, `web/src/api/agents.ts`) gain `taskTitle` + `projectName`.
- **D-12:** The bar reuses the existing **`useAgentStatuses()` 5s poll** as its single data source — becoming a **fourth consumer** alongside card dots, the Agent-tab dot, and the sidebar chips. No new polling hook (SBAR-08). NOTE: the DB-derived pass currently emits only `status:"exited"` resumable entries — given D-09 the bar simply filters those out client-side; no need to change that pass's behavior beyond the title/name JOIN.

### Placement / integration
- **D-13:** The bar mounts in the app shell (`web/src/components/layout/AppLayout.tsx`), pinned to the **bottom, outside the `<Outlet/>`**, so it is present on every route (board, task, settings) (SBAR-01). The per-project sidebar "N waiting" chips (`ProjectSidebar.tsx`) are **KEPT unchanged** — the bar is additive.

### Alerting (carried from milestone scope)
- **D-14:** **In-bar alerting ONLY** — pulsing-amber emphasis + waiting count. **No** browser/OS desktop notifications, **no** tab-title/favicon changes this phase (deferred SBAR-FUT-03/04).

### Claude's Discretion
- Exact bar height, expanded-panel max-height, spacing, and typography.
- The overlay panel's open/close animation/transition.
- Loading-vs-empty distinction on first unresolved poll (likely mirror `ReviewColumn`: quiet, no spinner).
- Exact visual treatment of the "current task" row highlight and the `#<n>` PR badge.
- Whether the collapsed bar carries any tiny extra hint beyond the counts.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

No external specs/ADRs exist for this project — requirements and decisions are captured in the planning docs and the existing code. Read:

### Phase scope & requirements
- `.planning/REQUIREMENTS.md` — SBAR-01..SBAR-10 (v1.6 requirements) + the Out-of-Scope / Future tables (what is explicitly excluded).
- `.planning/ROADMAP.md` → "### Phase 17: Global Active Sessions Bar" — goal, dependency note, and the 5 success criteria.
- `.planning/PROJECT.md` → "## Current Milestone: v1.6" + Key Decisions — locked milestone-level scope (bottom bar, agent-only, in-bar alerting only, additive, default collapsed).

### Existing code the phase extends (full paths in Code Context below)
- `internal/api/agents.go`, `web/src/api/agents.ts`, `web/src/components/StatusDot.tsx`, `web/src/components/board/ReviewColumn.tsx`, `web/src/components/layout/AppLayout.tsx`, `web/src/components/sidebar/ProjectSidebar.tsx`, `web/src/App.tsx`.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`web/src/api/agents.ts`** — `useAgentStatuses()` (TanStack Query, `refetchInterval: 5000`) + `AgentStatusEntry` type. THE data source; extend the type with `taskTitle` + `projectName` (D-11/D-12).
- **`internal/api/agents.go`** — `GET /api/agents/status` handler. Two SELECTs (manager-derived pass ~L89; DB-derived/post-restart pass ~L140) both build `agentStatusEntry`. Widen both with a JOIN to `tasks.title` + the project `name` (SBAR-10). Mirrors how v1.5 added `pr_number`/`source` (no migration).
- **`web/src/components/StatusDot.tsx`** — `dotMeta()` is the canonical pure status→color/tooltip map (green working / amber-pulse waiting / zinc idle / gray exited). Reuse for collapsed per-state counts AND per-row state indicators — never hardcode status colors.
- **`web/src/components/board/ReviewColumn.tsx`** — the collapse/expand idiom to mirror: `localStorage` default-collapsed (`!== "0"`), `overflow-y-auto` scroll panel, dark chrome `bg-[#101013]`, inline loading/empty states (quiet skeletons, no spinner).
- **`web/src/components/layout/AppLayout.tsx`** — app shell; mount the bar here (bottom, outside `<Outlet/>`). Also demonstrates the localStorage-controlled UI-state pattern (`kangent.sidebar`).
- **`web/src/components/sidebar/ProjectSidebar.tsx`** — existing `waitingByProject` derivation from the same feed (kept, unchanged) — a model for deriving counts client-side.
- **`web/src/App.tsx`** — react-router routes; row click-through targets `/projects/:projectId/tasks/:taskId` (use `Link`/`useNavigate`).

### Established Patterns
- **Single status feed → many consumers** (04-RESEARCH Pattern 4): the bar is the 4th consumer of `/api/agents/status`; do NOT add a parallel poll.
- **localStorage-backed collapse, default-collapsed**, dark panel chrome.
- **`dotMeta()` is the single source of status color/tooltip** — the bar must reuse it.

### Integration Points
- App shell mount: `AppLayout.tsx` (bottom, every route).
- Backend: `/api/agents/status` query (JOIN title + project name) and the `agentStatusEntry` Go struct.
- Frontend wire type: `AgentStatusEntry` in `web/src/api/agents.ts`.
</code_context>

<specifics>
## Specific Ideas

- Visual language should match the existing app: reuse `dotMeta()` status colors and the dark `bg-[#101013]` panel chrome used by the Review column, and the chevron + click-the-whole-thing collapse interaction from `ReviewColumn`'s collapsed rail.
- Overlay-up-over-content was chosen specifically so the xterm terminals in the task view never resize when the bar expands.
</specifics>

<deferred>
## Deferred Ideas

Captured so they're not lost — explicitly out of scope for Phase 17.

- **SBAR-FUT-01** — Per-session quick actions (stop / resume / start) from the bar without navigating.
- **SBAR-FUT-02** — Time-in-state / duration per row (needs a state-transition timestamp the feed doesn't expose today).
- **SBAR-FUT-03** — Browser/OS desktop notification when a session starts waiting (NOTF-01).
- **SBAR-FUT-04** — Tab title / favicon waiting badge.
- **SBAR-FUT-05** — Include bash/tmux terminal sessions in the bar.
- **SBAR-FUT-06** — Group/filter the list by project or by state. ("Grouped by project" was considered for the expanded list and deferred — flat attention-first chosen, D-05.)
- **"Show exited briefly" grace period** — considered for the bar, rejected (live-only chosen, D-09).
- **"Push content up" docked layout** — considered, rejected (overlay-up chosen, D-03).

No pending todos matched this phase (none folded, none deferred).
</deferred>

---

*Phase: 17-global-active-sessions-bar*
*Context gathered: 2026-06-18*
