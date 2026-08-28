# Phase 16: Global view, Settings & bar integration - Context

**Gathered:** 2026-08-27
**Status:** Ready for planning

<domain>
## Phase Boundary

The entire user-facing surface of the global scratchpad, consuming the Phase-14 config API and Phase-15 session/status backend: the `/global` route rendering the TaskPage-derived shell with ONLY the Agent tab + bash tabs (GVIEW-01), the honest unconfigured/vanished-root states (GVIEW-04), the Settings Global section — root config + default agent + "Open Scratchpad" affordance (GCONF-05) — and the live global row in the Active Sessions bar with click-through to `/global` and current-page highlight (GINT-01). Frontend-only plus the small TS wire-type widenings the widened feeds already emit (`source:"global"` status entries, `scope:"global"` session spawn). No backend behavior changes (Phases 14–15 shipped them), no hardening (Phase 17).

</domain>

<decisions>
## Implementation Decisions

### Carry-Forward Locked (research + Phases 13–15 — do not reopen)

- **Shell shape (research-locked):** `GlobalTaskPage` — a trimmed TaskPage (agent + bash tabs only), `/global` as a sibling of `/settings` OUTSIDE workspace sync; `web/src/api/global.ts` config client; `AgentTab` props loosened (not forked); bar row via a `source:"global"` branch. Zero new packages.
- **Bar contract (P6, research-locked):** bar rows key by `sessionId` (NOT `taskId` — a global entry's taskId is 0); global row click navigates to `/global`; current-page highlight gains the `/global` case (no taskId param exists there).
- **Naming (13-CTX D-09..D-11):** all user-facing copy says "Scratchpad"; bar row reads `Global · Scratchpad`; internal route/scope stays `global`.
- **Honesty posture (D-28..D-30, 14-CTX D-17):** never a silent `$HOME`/cwd fallback — the unconfigured and vanished-root states are distinct, both server-truth-driven off one GET /api/global round-trip.
- **Gates are backend-done:** unconfigured 409, vanished-root 409 naming the path (D-29), one-agent 409 (D-34) — the view only surfaces what the API already says.
- **Bar-row visual treatment (globe badge vs text-only) settles at UAT in Phase 17 (13-CTX D-12).** Sidebar entry stays parked (GT-FUT-05).
- **No new scope:** no description tab, no diff tab, no kanban presence, no delete-task analog, no root-path legibility line (GT-FUT-01) or git-status line (GT-FUT-02) beyond the safety banner.

### View chrome & ⋯ menu (new — D-35..D-38)

- **D-35: No back-arrow.** `/global` is a top-level destination like `/settings` — plain h1 title ("Scratchpad"), no back-arrow; the sidebar remains the navigation. NOT board-adjacent.
- **D-36: Agent-tab ⋯ menu = Stop only.** While running, the menu renders exactly one item: Stop. "Insert description" and "Insert review prompt" are task/PR concepts that degrade away — no Settings shortcut joins them.
- **D-37: QuotaIndicator under the same claude-engine rule as TaskPage.** Renders when the global default agent's engine is claude (hidden for custom/opencode). Both queries are already cached app-wide — no new fetches.
- **D-38: D-06 un-isolation banner = persistent one-line under the header, above the tab strip.** Visible whichever tab is active (bash tabs run un-isolated too), using the locked copy ("agent runs directly in \<root\> — no worktree isolation"). Hidden when the root is a managed clone.

### Honest degraded states (new — D-39..D-42)

- **D-39: Unconfigured state = copy + CTA button.** Full-page: short copy ("Scratchpad isn't configured yet"), one-line explanation (root + default agent live in Settings), and a primary button navigating to the Settings Global section. Driven by GET /api/global's unconfigured shape — D-17's one round-trip.
- **D-40: Vanished root = distinct state naming the path verbatim.** "the configured root no longer exists on disk: \<path\>" + CTA to Settings — mirrors D-29's deliberate misconfiguration-vs-disk-rot distinction. NEVER rendered as the generic unconfigured state.
- **D-41: Configured + idle = exact task-parity shell.** Agent tab front-and-center with "Start agent" (spawns the configured default agent), "+" for bash tabs. No new idle-hero concept.
- **D-42: Root vanishes while live = warn, don't disturb.** Attached terminals keep streaming (their processes own their cwd); a non-blocking warning line appears. Nothing is killed or detached on the user's behalf — stopping/reconfiguring stays their call.

### Settings Global section (new — D-43..D-46)

- **D-43: Root capture = segmented "GitHub repo | Local folder" toggle, AddProjectDialog pattern verbatim.** Repo default when GitHub integration is on, folder-only when off; blocking "Cloning…" spinner; inline clone failures with the dialog kept open and values preserved (no half-configured state).
- **D-44: Section layout = summary card + Change dialog.** The card shows the current root (folder path, or owner/name with a managed badge) and hosts the default-agent selector + the "Open Scratchpad" affordance (GCONF-05); "Change root…" opens the AddProjectDialog-derived dialog; Clear rides the same dialog behind the same 409 gate.
- **D-45: 409 gate surfacing = inline reasons list.** A blocked root change/clear surfaces the `{error, reasons[...]}` body verbatim as an inline error list in the open dialog ("agent session running", "2 bash tabs running") — degrade-don't-break posture, dialog stays open, values preserved.
- **D-46: Default-agent selector = inline dropdown on the summary card.** Instant save with an "applies at the next Start" hint (D-24: agent changes are never 409-gated). Deliberately OUTSIDE the root dialog so the ungated lightweight action isn't hidden behind the gated/clone-heavy flow.

### the agent's Discretion

- Shell derivation mechanics — extract a shared shell component from TaskPage vs. a sibling `GlobalTaskPage` reusing `AgentTab`/`TaskTabs`/`TerminalPane` directly (research prescribes the trimmed-page lean; the exact factoring is the planner's).
- `AgentTab`/`TaskTabs` prop-loosening shape (how much task-coupling — `useResumeAgent(task.id)`, `useSessions(taskId)` — moves to props or scope-aware variants).
- TS wire-type widening mechanics (`AgentStatusEntry.source` gaining `"global"`; `TermSession` scope fields; global session hook spellings).
- How `/global` consumes GET /api/global (TanStack query key, refetch cadence for `root_exists` honesty).
- The unconfigured/vanished state component shape and exact copy wording (D-39/D-40 lock content, not pixels).
- "Open Scratchpad" button styling and the Clear-root affordance's exact placement inside the dialog.
- Route registration mechanics in `App.tsx` (inside `AppLayout` — the bar must be present).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` — GCONF-05, GVIEW-01, GVIEW-04, GINT-01 (this phase); the Out-of-Scope table; GT-FUT-01..07 parked deferrals; the mapping note (GCONF-01..04's UX is delivered here as GCONF-05; GVIEW-02/03 tab UI rides GVIEW-01)
- `.planning/ROADMAP.md` §Phase 16 — goal, success criteria 1–5, dependency on Phases 14–15, UI hint: yes

### Milestone research (frontend architecture-locked)
- `.planning/research/SUMMARY.md` §"Phase 4: Frontend view + reachability" — GlobalTaskPage, AgentTab loosening, global session hooks, bar integration, Settings section + "Open Global Task" idle affordance; §P6 — key bar rows by sessionId, global-route highlight case; the reachability-asymmetry note (bar shows agent-live only)
- `.planning/research/ARCHITECTURE.md` component 7 — Frontend: `GlobalTaskPage`, `web/src/api/global.ts`, loosened `AgentTab`, bar + Settings sections
- `.planning/research/PITFALLS.md` — P3 (no worktree safety net — the D-38 banner is its UI half), P6 (bar contract break on a project-less entry)

### Prior-phase decisions this phase exercises
- `.planning/phases/13-global-data-foundation-safety-net/13-CONTEXT.md` — D-06 (un-isolation banner copy + this-phase delivery), D-09/D-10/D-11 (Scratchpad naming, `Global · Scratchpad` row, internal `global`), D-12 (bar visuals → Phase 17 UAT)
- `.planning/phases/14-global-config-api/14-CONTEXT.md` — D-17 (GET one round-trip serving both Settings and the view's honest states), D-20..D-23 (PUT grammar, clear semantics, returns GET shape), D-24 (agent change applies at next Start), D-27 (`~/.kamacu` block copy)
- `.planning/phases/15-global-sessions-backend/15-CONTEXT.md` — D-28..D-34 (the 409 family the view surfaces), the status-feed contract (source:"global", taskId 0, sessionId real, Resumable flag)

### Code-level precedents (read in-repo)
- `web/src/pages/TaskPage.tsx` — the shell being trimmed (header :488+, agent-tab ⋯ menu wiring, tab strip, Start/Stop parity)
- `web/src/components/task/AgentTab.tsx` :140–:200 — the ⋯ menu (Insert items + Stop) and Start/Resume/Reset states D-36/D-41 mirror
- `web/src/components/layout/ActiveSessionsBar.tsx` :129–:136 — the `key={entry.taskId}` + task-route navigate + openTaskId highlight that P6 re-keys
- `web/src/api/agents.ts` :4–:16 — `AgentStatusEntry` (the `source: "manual" | "github_pr"` type gaining `"global"`)
- `web/src/api/sessions.ts` — the taskId-coupled hooks (`useSessions`, `useSpawnSession`, `useResumeAgent`) needing scope-aware variants
- `web/src/components/sidebar/AddProjectDialog.tsx` — the segmented repo|folder toggle + clone UX D-43 mirrors
- `web/src/pages/SettingsPage.tsx` — the sections layout D-44 joins
- `internal/api/agents.go` :166–:196 — the shipped global status entries (labels, Resumable) the TS type must widen to match
- `internal/api/global.go` — the GET/PUT surface (config + root_exists + counts + agent summary) the section and view consume

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `TaskPage` shell + `AgentTab`/`TaskTabs`/`TerminalPane` — the derived-shell source; research prescribes loosening `AgentTab` props over forking
- `AddProjectDialog` — segmented "GitHub repo | Local folder" toggle (ui/tabs.tsx), blocking "Cloning…" spinner, inline failure with dialog-open — D-43's template
- `SettingsPage` section composition (GithubSection/AgentsSection/WorktreeCleanupSection) — D-44 slots a GlobalSection in beside them
- `ProjectSettingsDialog`'s per-project agent selector — D-46's inline dropdown pattern
- `useAgentStatuses` 5s poll — already carries global entries server-side; the bar consumes the same feed (no new hook)
- `QuotaIndicator` + the cached `useProjects`/`useAgents` engine lookup — D-37's rule reuses TaskPage's exact approach

### Established Patterns
- Degrade-don't-break error surfacing (inline error, dialog stays open, values preserved) — clone failures AND 409 reasons (D-43/D-45)
- One GET round-trip drives multiple views (D-17) — Settings section and /global share the config query
- Zero new dependencies (verified across the milestone); UI-SPEC-first for frontend phases (roadmap "UI hint: yes" → run `/gsd-ui-phase 16` before planning)
- Task-parity-first: when a global surface has a task analog, mirror it exactly (Start, ⋯ Stop, Resume/Reset, tab behavior) unless a D-decision diverges

### Integration Points
- `web/src/App.tsx` routes — `/global` registers inside `AppLayout` (bar present), sibling of `/settings`, outside workspace sync
- `ActiveSessionsBar` row render — sessionId keying, `/global` navigation branch, pathname-based current-page highlight for the global row
- `web/src/api/` — new `global.ts` (GET/PUT client); widened `agents.ts`/`sessions.ts` types + scope-aware spawn/list/resume hooks
- Settings → `/global` navigation from the "Open Scratchpad" affordance; `/global` unconfigured state → Settings Global section (D-39 CTA both directions)

</code_context>

<specifics>
## Specific Ideas

- The unconfigured CTA and the vanished-root notice both land on the Settings Global section — the two degraded states differ ONLY in copy and the path being named (D-39 vs D-40).
- The 409 reasons render verbatim from the wire — no client-side re-wording of "agent session running" / "2 bash tabs running".
- The bar's global row already sorts/places naturally via the feed's status field — no special sort branch beyond the navigation/highlight keying.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope. GT-FUT-01..07 remain parked in REQUIREMENTS.md; bar-row visual treatment lands at Phase 17 UAT (D-12).

</deferred>

---

*Phase: 16-Global view, Settings & bar integration*
*Context gathered: 2026-08-27*
