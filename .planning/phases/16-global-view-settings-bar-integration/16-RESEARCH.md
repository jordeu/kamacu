# Phase 16: Global view, Settings & bar integration - Research

**Researched:** 2026-08-27
**Domain:** Frontend composition over shipped Phase 14–15 APIs (React 19 + TanStack Query 5 + react-router 7, zero new packages)
**Confidence:** HIGH

## Summary

Phase 16 is pure frontend composition: every backend surface it drives is shipped and contract-frozen. `GET/PUT /api/global` (Phase 14, `internal/api/global.go`), the `scope:"global"` spawn/list/reattach branches (Phase 15, `internal/api/sessions.go:68-75,438-493`), and the `/api/agents/status` global entry (`internal/api/agents.go:166-196,263-284`) all emit their final wire shapes today — verified file-by-file in this worktree. The work is: one new page (`GlobalTaskPage` at `/global`), one new API client (`web/src/api/global.ts`), three surgical edits to `ActiveSessionsBar.tsx`, a Settings Scratchpad section (summary card + Change-root dialog), small TS widenings (`AgentStatusEntry.source`, `TermSession.global`, scope-aware session hooks), and `AgentTab` prop loosening. The approved 16-UI-SPEC.md prescribes pixels, copy, and a 7-state view matrix — this research supplies the wire mechanics and in-repo precedents underneath it.

Three discrepancies between planning prose and shipped code need planner attention. (1) The 409 `reasons` on `PUT /api/global` are structured `{kind:"sessions", target:"<tmux-name-or-label>"}` objects (`deleteBlocker`, `global.go:123-144`) — NOT the prose strings "agent session running" sketched in 13-CONTEXT D-15 (14-CONTEXT explicitly resolved that sketch as illustrative) — and `client.ts`'s `ApiError` discards everything but `body.error` (`client.ts:22`), so D-45's "render reasons verbatim" needs a small additive mechanics decision. (2) TaskPage's QuotaIndicator predicate (`!== "custom"`, `TaskPage.tsx:64`) actually SHOWS for the runtime `"opencode"` engine (the TS `Agent.engine` union in `types.ts:62` is stale — the OpenCode system seed emits `engine:"opencode"`, `agents_backfill.go:102`), while D-37/UI-SPEC say "hidden for custom/opencode"; the global view should key on `agent.engine === "claude"` from the `["global"]` query. (3) `TermSession` (TS) lacks the `global?: boolean` flag the orphaned-tmux ghosts already carry (`session.go:73`).

**Primary recommendation:** Build a sibling `GlobalTaskPage` (copy-then-trim from TaskPage — never refactor TaskPage itself), loosen `AgentTab` behind a task/global variant prop, key every new hook off the frozen wire contracts below, and treat the 16-UI-SPEC view-state matrix as the load-bearing interaction contract.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Carry-Forward Locked (research + Phases 13–15 — do not reopen)**

- **Shell shape (research-locked):** `GlobalTaskPage` — a trimmed TaskPage (agent + bash tabs only), `/global` as a sibling of `/settings` OUTSIDE workspace sync; `web/src/api/global.ts` config client; `AgentTab` props loosened (not forked); bar row via a `source:"global"` branch. Zero new packages.
- **Bar contract (P6, research-locked):** bar rows key by `sessionId` (NOT `taskId` — a global entry's taskId is 0); global row click navigates to `/global`; current-page highlight gains the `/global` case (no taskId param exists there).
- **Naming (13-CTX D-09..D-11):** all user-facing copy says "Scratchpad"; bar row reads `Global · Scratchpad`; internal route/scope stays `global`.
- **Honesty posture (D-28..D-30, 14-CTX D-17):** never a silent `$HOME`/cwd fallback — the unconfigured and vanished-root states are distinct, both server-truth-driven off one GET /api/global round-trip.
- **Gates are backend-done:** unconfigured 409, vanished-root 409 naming the path (D-29), one-agent 409 (D-34) — the view only surfaces what the API already says.
- **Bar-row visual treatment (globe badge vs text-only) settles at UAT in Phase 17 (13-CTX D-12).** Sidebar entry stays parked (GT-FUT-05).
- **No new scope:** no description tab, no diff tab, no kanban presence, no delete-task analog, no root-path legibility line (GT-FUT-01) or git-status line (GT-FUT-02) beyond the safety banner.

**View chrome & ⋯ menu (D-35..D-38)**

- **D-35: No back-arrow.** `/global` is a top-level destination like `/settings` — plain h1 title ("Scratchpad"), no back-arrow; the sidebar remains the navigation. NOT board-adjacent.
- **D-36: Agent-tab ⋯ menu = Stop only.** While running, the menu renders exactly one item: Stop. "Insert description" and "Insert review prompt" are task/PR concepts that degrade away — no Settings shortcut joins them.
- **D-37: QuotaIndicator under the same claude-engine rule as TaskPage.** Renders when the global default agent's engine is claude (hidden for custom/opencode). Both queries are already cached app-wide — no new fetches.
- **D-38: D-06 un-isolation banner = persistent one-line under the header, above the tab strip.** Visible whichever tab is active (bash tabs run un-isolated too), using the locked copy ("agent runs directly in \<root\> — no worktree isolation"). Hidden when the root is a managed clone.

**Honest degraded states (D-39..D-42)**

- **D-39: Unconfigured state = copy + CTA button.** Full-page: short copy ("Scratchpad isn't configured yet"), one-line explanation (root + default agent live in Settings), and a primary button navigating to the Settings Global section. Driven by GET /api/global's unconfigured shape — D-17's one round-trip.
- **D-40: Vanished root = distinct state naming the path verbatim.** "the configured root no longer exists on disk: \<path\>" + CTA to Settings — mirrors D-29's deliberate misconfiguration-vs-disk-rot distinction. NEVER rendered as the generic unconfigured state.
- **D-41: Configured + idle = exact task-parity shell.** Agent tab front-and-center with "Start agent" (spawns the configured default agent), "+" for bash tabs. No new idle-hero concept.
- **D-42: Root vanishes while live = warn, don't disturb.** Attached terminals keep streaming (their processes own their cwd); a non-blocking warning line appears. Nothing is killed or detached on the user's behalf — stopping/reconfiguring stays their call.

**Settings Global section (D-43..D-46)**

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

### Deferred Ideas (OUT OF SCOPE)

None — discussion stayed within phase scope. GT-FUT-01..07 remain parked in REQUIREMENTS.md; bar-row visual treatment lands at Phase 17 UAT (D-12).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| GCONF-05 | Settings provides an "Open Scratchpad" affordance so the global view is reachable when idle (the Active Sessions bar covers the live case) | Settings Scratchpad section pattern (D-43..D-46); summary-card row 3 primary CTA navigating to `/global` (UI-SPEC Surface 3); SettingsPage section composition precedent at `SettingsPage.tsx:158-218` |
| GVIEW-01 | A `/global` route renders a task-like view with ONLY the Agent tab + bash tabs (no description tab, no diff tab, never on a kanban board) — the full TaskPage-derived shell with explicit Start and ⋯ Stop parity | GlobalTaskPage trimmed-shell pattern (this research §Architecture Patterns); TaskPage tab-strip machinery at `TaskPage.tsx:348-475`; route registration precedent `App.tsx:136-142`; `TaskTabs`/`TabDef`/`TerminalPane` reuse verified |
| GVIEW-04 | With no root configured, the global view shows an honest unconfigured state pointing to Settings — never a silent `$HOME` or cwd fallback | View-state matrix driven by `GET /api/global` (`root_path`/`root_exists`/`live` fields verified at `global.go:48-56,238-263`); degraded-state hero idiom (`App.tsx:41-54`); D-39/D-40 distinct states |
| GINT-01 | A live global agent session appears as a row in the global Active Sessions bar (server-synthesized label, non-nullable wire contract preserved) with click-through to `/global` and current-page highlight | Global status entry verified at `agents.go:182-196` (taskId 0, source "global", "Global"/"Scratchpad" labels); bar's three surgical changes mapped to `ActiveSessionsBar.tsx:129-137,244-274`; `source` union branch-site audit complete (only `:268` branches) |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Global config read/write (root, agent) | API / Backend (shipped) | Browser (render only) | `GET/PUT /api/global` owns validation, 409 gating, clone lifecycle — the view never derives truth client-side (D-17) |
| View-state derivation (7 states) | Browser | API (supplies `root_exists`/`live`) | Pure precedence evaluation over the one GET response — client-side render concern |
| Session spawn/stop/resume | API / Backend (shipped) | Browser (click handlers) | `POST /api/sessions {scope:"global"}` owns gates; UI only fires from click handlers (StrictMode rule) |
| Tab lifecycle (visible/closing/keepExited, Pitfall-7 reactivation) | Browser | — | Client-only interaction state; server is tab truth via 5s poll (`["sessions","global"]`) |
| Bar row render/navigation/highlight | Browser | API (status feed) | Feed already emits global entries; bar adds key/nav/highlight branches only |
| QuotaIndicator visibility rule | Browser | API (`agent.engine` on the global wire) | Engine resolved from the `["global"]` query — no `useAgents` fan-out needed |
| Liveness/root existence honesty | API (derived at GET) | Browser (5s refetch) | `root_exists` + `live` computed server-side per request (`global.go:238-263`) |

## Standard Stack

Zero new packages (carry-forward locked, verified against `web/package.json`). Every building block exists in-repo:

### Core (all [VERIFIED: in-repo at file:line])

| Building block | Location | Role in this phase |
|----------------|----------|---------------------|
| `TaskTabs` + `TabDef` | `web/src/components/task/TaskTabs.tsx:21-44` | Tab strip reused verbatim — `leading` StatusDot, `onClose`/`onRename`, `keepMounted`, trailing slot |
| `AgentTab` (loosened) | `web/src/components/task/AgentTab.tsx:36-47` | Agent tab content — 4 states map 1:1 onto global scope; loosening surface enumerated in Pitfall 4 |
| `TerminalPane` | `web/src/components/terminal/TerminalPane.tsx` | WS attach/replay — untouched; `headerMenu`, `exitedActions`, `onSessionExit` props are the ⋯/Stop/Reset hooks |
| `StatusDot` / `dotMeta` | `web/src/components/StatusDot.tsx` | Agent-tab dot + bar row dot from the status feed entry |
| `QuotaIndicator` | `web/src/components/quota/QuotaIndicator.tsx:155` | Header quota — self-gates on `["usage"]` data states; visibility keyed on engine |
| `Dialog`, `Tabs`, `Select`, `Badge`, `Skeleton`, `Tooltip`, `DropdownMenu` | `web/src/components/ui/*` | Dialog chrome, segmented toggle, agent dropdown, managed badge — NO shadcn adds (no `card.tsx` exists; the summary card is a composed `rounded-lg border bg-card` div) |
| TanStack Query 5 | `web/src/api/*.ts` patterns | `["global"]` shared query (5s), setQueryData-then-invalidate mutations, `["sessions"]` prefix invalidation |
| react-router 7 | `web/src/App.tsx:118-143` | `/global` as a plain sibling Route inside `<Route element={<AppLayout/>}>` — the `/activity` precedent (:137-140) |

**Installation:** none. `web/node_modules` present; Node v24.4.0+ / npm 11.6.0 verified on host.

## Package Legitimacy Audit

No packages installed this phase — the phase is contractually zero-add ("Zero new packages", CONTEXT carry-forward; UI-SPEC Registry Safety: zero shadcn adds, zero third-party registries, `components.json:24 "registries": {}`).

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| (none) | — | — | — | — | — | N/A — no installs |

**Packages removed due to [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
                    ┌──────────────────────────────────────────────┐
                    │                Browser (SPA)                 │
                    │                                              │
  /global route ───▶│ GlobalTaskPage                               │
  (App.tsx, inside  │   │ uses ["global"] query (5s, shared)      │
  AppLayout, NOT    │   ├─ state 1/2: skeleton / retry            │
  BoardWorkspaceSync)│   ├─ state 3: D-39 unconfigured ──┐        │
                    │   ├─ state 4: D-40 vanished ──────┤ CTA    │
  /settings ───────▶│ ScratchpadSection (Settings col)  │ nav    │
                    │   ├─ summary card (root+agent+CTA)│        │
                    │   └─ Change-root dialog ◀─────────┘        │
                    │        (AddProjectDialog pattern)           │
                    │                                              │
                    │ ActiveSessionsBar (AppLayout, all routes)   │
                    │   key={sessionId} ─ source:"global" branch  │
                    │   navigate("/global") · pathname highlight  │
                    └───────┬──────────────────┬──────────────────┘
                            │ GET/PUT          │ GET ?scope=global
                            │ /api/global      │ POST /api/sessions {scope:"global"}
                            ▼                  ▼ + /api/agents/status (5s poll, global entry since P15)
                    ┌──────────────────────────────────────────────┐
                    │ Go backend (SHIPPED — Phases 14–15, no edits)│
                    │  global.go: config+derived state, 409 gate   │
                    │  sessions.go: spawn/list/reattach global     │
                    │  agents.go: synthesized status passes 1b/2b  │
                    └──────────────────────────────────────────────┘
```

Primary use case trace: user clicks "Open Scratchpad" in Settings (or a bar row) → `/global` → `["global"]` query resolves → state matrix picks shell (6/7) → Agent tab "Start agent" → `POST {scope:"global", kind:"agent"}` → session appears in `["sessions","global"]` and `/api/agents/status` → bar renders `Global · Scratchpad` row.

### Recommended Project Structure

```
web/src/
├── api/
│   ├── global.ts          # NEW — GlobalConfig type + useGlobal + useSaveGlobal (D-17 one round-trip)
│   ├── agents.ts          # EDIT — AgentStatusEntry.source += "global" (line 13)
│   └── sessions.ts        # EDIT — TermSession.global?: boolean + scope-aware hooks (["sessions","global"])
├── pages/
│   ├── GlobalTaskPage.tsx # NEW — trimmed TaskPage shell + view-state matrix + degraded states
│   └── SettingsPage.tsx   # EDIT — mount <ScratchpadSection/> in the 640px column
├── components/
│   ├── settings/
│   │   └── ScratchpadSection.tsx  # NEW — summary card + Change-root dialog + agent selector + Open CTA
│   ├── task/
│   │   ├── AgentTab.tsx   # EDIT — variant-based loosening (task | global)
│   │   └── TaskTabs.tsx   # UNTOUCHED — TabDef already covers the global shape
│   └── layout/
│       └── ActiveSessionsBar.tsx  # EDIT — exactly 3 changes (key, nav branch, highlight)
└── App.tsx                # EDIT — one Route line inside AppLayout
```

### Pattern 1: The View-State Matrix (LOCKED — the core interaction contract)

**What:** One `GET /api/global` response drives the entire `/global` render, evaluated top-down (16-UI-SPEC, LOCKED):

| # | Condition (from GET) | Renders |
|---|----------------------|---------|
| 1 | query pending | Page skeleton (TaskPage pending idiom) |
| 2 | query error | Centered `Couldn't load the scratchpad.` + outline `Retry loading` |
| 3 | `root_path === ""` | D-39 unconfigured full-page state → Settings |
| 4 | `root_path !== "" && !root_exists && live.{agent,bash,tmux} all 0` | D-40 vanished-root full-page state → Settings |
| 5 | `root_path !== "" && !root_exists && any live > 0` | Full shell + D-42 warning banner; terminals keep streaming |
| 6 | folder root (`github_repo === null`) && `root_exists` | Full shell + D-38 isolation banner (persistent) |
| 7 | managed clone (`github_repo !== null`) && `root_exists` | Full shell, NO banner |

**When to use:** the single `useGlobal()` consumer inside GlobalTaskPage. The `live` counts are the wire's real Phase-15 values — the client never guesses liveness. The banner slot renders exactly one of {D-38 isolation, D-42 warning} in states 5/6, never both, never neither.

**Example:**
```typescript
// Source: 16-UI-SPEC.md "The View State Matrix" + global.go:48-56 (wire verified)
const { data: g, isPending, isError, refetch } = useGlobal();
// ... states 1-2 early-returns ...
const liveAnywhere = g.live.agent + g.live.bash + g.live.tmux > 0;
if (g.root_path === "") return <ScratchpadUnconfigured />;          // state 3 (D-39)
if (!g.root_exists && !liveAnywhere) return <ScratchpadVanished path={g.root_path} />; // state 4 (D-40)
// states 5/6/7 → shell; banner = !g.root_exists ? vanishedWarning : (g.github_repo === null ? isolationBanner : null)
```

### Pattern 2: The shared `["global"]` query (D-17 one round-trip)

**What:** ONE TanStack query, key `["global"]`, `refetchInterval: 5000`, consumed by BOTH the Settings section and `/global`. Mutations write the PUT response into the cache (the PUT returns the GET shape — `global.go:448-460`), then invalidate.

**When to use:** `web/src/api/global.ts`.

**Example:**
```typescript
// Source: useAgentStatuses idiom (agents.ts:20-26) + useSpawnSession setQueryData idiom (sessions.ts:45-56)
// [VERIFIED: in-repo pattern; cross-checked TanStack v5 docs]
export interface GlobalConfig {
  root_path: string; github_repo: string | null; agent_id: number; updated_at: string;
  root_exists: boolean;
  live: { agent: number; bash: number; tmux: number };
  agent: { id: number; name: string; engine: string }; // engine: plain string — Go emits "claude"|"custom"|"opencode"
}

export function useGlobal() {
  return useQuery({
    queryKey: ["global"],
    queryFn: () => get<GlobalConfig>("/api/global"),
    refetchInterval: 5000,
  });
}

export function useSaveGlobal() {
  const qc = useQueryClient();
  return useMutation({
    // Partial-PATCH pointer semantics (global.go:292-311): omit untouched keys
    mutationFn: (body: { root_path?: string; repo?: string; agent_id?: number }) =>
      put<GlobalConfig>("/api/global", body),
    onSuccess: (g) => qc.setQueryData(["global"], g), // response IS the GET shape (D-23)
  });
}
```

### Pattern 3: Scope-aware session hooks

**What:** Global variants of the taskId-coupled hooks, keyed `["sessions","global"]` — the `["sessions"]` prefix invalidation keeps working unchanged (TanStack prefix-matching, `sessions.ts:53-54` precedent). The contract (keys, bodies, 5s parity) is UI-SPEC-locked; whether these are new hooks or loosened params is planner discretion.

**Example:**
```typescript
// Source: UI-SPEC "Wire-Type Widenings" + sessions.ts hook idioms; backend branches verified
// sessions.go:70 (GET ?scope=global), :438-470 (spawn branch + gates), :502-524 (global resume)
export function useGlobalSessions() {
  return useQuery({
    queryKey: ["sessions", "global"],
    queryFn: () => get<TermSession[]>("/api/sessions?scope=global"),
    refetchInterval: 5000,
  });
}
// useSpawnGlobalSession:        post("/api/sessions", { scope: "global" })
// useSpawnGlobalAgent:          post("/api/sessions", { scope: "global", kind: "agent" })
// useResumeGlobalAgent:         post("/api/sessions", { scope: "global", kind: "agent", resume: true })
// useReattachGlobalTmux(name):  post("/api/sessions", { scope: "global", reattach_tmux_name: name })
```
Each mutation's `onSuccess` mirrors the task hooks: `setQueryData(["sessions","global"], …)` then `invalidateQueries({queryKey: ["sessions"]})`; agent variants also invalidate `["agent-statuses"]` (`sessions.ts:99` precedent).

### Pattern 4: AgentTab variant loosening (not a fork)

**What:** `AgentTab` currently consumes exactly five things from its `task: Task` prop: `task.id` (hooks + status lookup + cache invalidation key), `task.description` (Insert menu), `task.worktree_path` (CTA gating + copy), `task.branch` (pre-start copy), plus the `seed` prop (Insert review prompt). The global variant needs: scope-aware hooks, NO Insert items (D-36), NO worktree gating (CTA enabled whenever the shell renders, D-41), and root-naming copy. The `projectId` prop is declared but never used — dead, droppable.

**Recommended shape:** a `variant` discriminator with internal branches (thin call sites, single file):
```typescript
// Source: AgentTab.tsx consumption audit (lines 48-59, 95-133, 147-184)
type AgentTabScope =
  | { kind: "task"; taskId: number }
  | { kind: "global" };  // hooks resolve to the ["sessions","global"]/scope:"global" spellings

export function AgentTab({ scope, agentSession, description, seed }: { ... }) {
  // spawn/resume/stop + resumable lookup resolve off scope.kind
  // D-36: showInsertDescription/showInsertSeed both false for global — menu renders Stop only,
  //        no separator before a lone item
  // D-41: pre-start CTA enabled unconditionally for global; body copy names the root (mono span)
}
```
The task call site passes `{kind:"task", taskId}` + description + seed — byte-for-byte behavior preserved. Alternative (full prop injection of spawn/resume/resumable callbacks) is heavier at the call site for the same contract; either satisfies "loosened, not forked" — planner's call.

### Pattern 5: The Change-root dialog (AddProjectDialog pattern verbatim, with deltas)

**What:** Copy `AddProjectDialog.tsx` structure — segmented repo|folder `Tabs` (rendered only when integration on, repo default), mono inputs, exactly-one-of {help, error}, blocking `Cloning…` spinner, reset-on-open via the adjust-state-during-render `prevOpen` tracker (:73-95). Deltas: NO Name field/prefill machinery; submit `PUT {repo}` / `PUT {root_path}`; a `Clear root` destructive button (only when a root is configured) sending `PUT {root_path: ""}` behind the same gate; every failure keeps the dialog OPEN with values + active mode preserved (only a 2xx closes).

### Pattern 6: Degraded-state hero (D-39/D-40)

**What:** Full-page replacements sharing one component shape — the centered-hero idiom (`App.tsx:41-54`): `flex h-full items-center justify-center` → `flex flex-col items-center gap-4 py-8 text-center` → h1 `text-xl font-medium` + muted body (`max-w-[480px]` when two-line) + primary `Open Settings` Button navigating to `/settings`. The two states differ ONLY in copy and the path being named (mono span). No hash/anchor routing to the section — the app has none; do not invent one.

### Anti-Patterns to Avoid

- **Extracting a shared shell from TaskPage.** TaskPage is the hottest page in the app (PR branches, title editing, worktree meta, cleanup/delete dialogs, Esc handler). Refactoring it for reuse risks task-path regressions for zero global benefit. Copy-then-trim into GlobalTaskPage instead (SUMMARY's "trimmed TaskPage" is exactly this).
- **Guessing liveness/resumability client-side.** The server's `live` counts and `resumable` flag are the only drivers (AgentTab's single-driver rule, `AgentTab.tsx:54-59`); the client never infers either from cache state.
- **Wrapping `/global` in `BoardWorkspaceSync` or adding `:projectId`-style params.** No workspace linkage exists (sentinel-leak posture; Phase 17 sweeps enumeration surfaces).
- **An Esc handler on `/global`.** TaskPage's Esc-to-board (`TaskPage.tsx:228-244`) has no analog — there is no parent board. Sidebar and bar are the navigation.
- **Spawning from effects.** StrictMode double-mounts; every spawn/reset/resume/reattach fires from a click handler (standing TaskPage rule, `TaskPage.tsx:336-346`). The tmux reattach effect fires `reattach.mutate` guarded by a `reattachedRef` Set — mirror that exactly (`TaskPage.tsx:148-162`).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Config truth + liveness | Client-side rootExists probes / cached mirrors | `GET /api/global` via the shared `["global"]` query | Server derives `root_exists` + `live` per request (`global.go:238-263`); a client mirror drifts within one poll |
| Terminal attach/replay | New WS glue | `TerminalPane` untouched | Scope is transparent to the terminal engine — sessions are cwd-driven |
| Tab strip | New tab component | `TaskTabs`/`TabDef` verbatim | `leading`/`onClose`/`onRename`/`keepMounted`/trailing already cover the global shape |
| Bar row rendering | Custom global row | Existing `SessionRow` slots | Wire-synthesized `"Global"`/`"Scratchpad"` labels render through the standard template; aria-label derives automatically |
| Root validation / clone / gate logic | Any client-side pre-validation beyond empty-field disable | The shipped backend | `validateRepoPath`, D-07 footguns, D-27 `~/.kamacu` block, 409 gate — all server-owned; the dialog surfaces messages verbatim |
| Segmented repo|folder UX | New toggle component | `AddProjectDialog`'s `Tabs` pattern | Includes the integration-off folder-only degradation and pending-disable semantics |

**Key insight:** Phase 16 adds ZERO business logic. Every gate, validation, label, and truth is server-shipped; the phase's entire risk surface is TS wiring and render precedence.

## Common Pitfalls

### Pitfall 1: The 409 `reasons` wire is `{kind, target}` objects — and `ApiError` discards them
**What goes wrong:** D-45/UI-SPEC row 22 prescribe rendering the `{error, reasons[...]}` body "verbatim" and cite prose strings ("agent session running", "2 bash tabs running") from 13-CONTEXT D-15. The SHIPPED wire emits `deleteBlocker` objects: `reasons: [{kind: "sessions", target: "kamacu-global-3"}, {kind:"sessions", target: "Agent"}]` (`global.go:123-144`, grammar at `projects.go:681-689`) — 14-CONTEXT explicitly resolved the 13-CONTEXT strings as illustrative. Meanwhile `client.ts:22` extracts only `body.error` into `ApiError.message`; the reasons never reach the catch block.
**Why it happens:** The gated-delete grammar predates the prose sketch; the ApiError was built for single-message errors.
**How to avoid:** `err.message` already carries the human lead sentence ("the Scratchpad root can't be changed while sessions are running") — render `sentenceCase(err.message)` verbatim. For the reasons `<ul>`, the minimal clean mechanics: additively extend `ApiError` with `reasons?: {kind: string; target: string}[]` populated in `client.ts` when present, then list `reasons[].target` (mono spans — tmux names and labels are literals). NO client re-wording either way.
**Warning signs:** a dialog that renders `reasons` as `[object Object]`, or that silently drops the list.

### Pitfall 2: QuotaIndicator engine rule — TaskPage's predicate and D-37's stated outcome disagree for opencode
**What goes wrong:** D-37 says "same claude-engine rule as TaskPage" AND "hidden for custom/opencode". TaskPage's actual predicate is `projectEngine !== "custom"` (`TaskPage.tsx:64`) — which SHOWS the indicator for the runtime `"opencode"` engine, because `types.ts:62`'s `Agent.engine: "claude" | "custom"` union is stale: the OpenCode system seed emits `engine: "opencode"` (`agents_backfill.go:102`).
**How to avoid:** For the global view, key visibility off the `["global"]` query's `agent.engine` (type it as plain `string` in `global.ts`, mirroring the Go wire) and render iff `engine === "claude"` — this satisfies D-37's locked outcome ("hidden for custom/opencode") without touching the stale `types.ts` union. Do NOT "fix" TaskPage's predicate in this phase (out of scope; note it for Phase 17/deferred).
**Warning signs:** the quota chip appearing on `/global` when the default agent is OpenCode.

### Pitfall 3: `TermSession` is missing the `global` flag the wire already emits
**What goes wrong:** `session.Info` carries `Global bool json:"global,omitempty"` (`session.go:73`); orphaned global tmux ghosts are synthesized with `Global: true` specifically so "the /global view keys off it" (`sessions.go:159-166`). The TS `TermSession` interface doesn't declare it.
**How to avoid:** Add `global?: boolean` to `TermSession` alongside `orphaned?`/`tmuxName?`. Not strictly load-bearing for rendering (the `?scope=global` list is already all-global), but it keeps the type honest for the reattach-effect guard parity.

### Pitfall 4: Under-scoping the AgentTab/TaskPage task-coupling audit
**What goes wrong:** The bash-tab lifecycle in TaskPage is ~80 lines of intertwined state (`closingIds`/`keepExitedIds`/`visibleSessions` filter+sort `:168-188`, Pitfall-7 neighbor-reactivation `useLayoutEffect` `:209-221`, `removeTab`/`handleCloseTab`/`handleSpawn` `:307-346`, tmux reattach effect `:148-162`). A naive trim that drops the reattach effect or the keepExited mechanics breaks GSESS-03 (invisible tmux survival) or exited-tab retention.
**How to avoid:** Copy the whole block into GlobalTaskPage, then delete description/diff/worktree/PR branches; swap the hooks for scope-aware variants; keep the `reattachedRef` one-shot guard. The invalidate keys become `["sessions","global"]`.

### Pitfall 5: Bar re-key edge — `sessionId: ""` rows
**What goes wrong:** Keying rows by `entry.sessionId` looks unsafe because DB-derived post-restart entries carry `sessionId: ""` (`agents.go:274`).
**How to avoid:** Safe by construction: the `""` only occurs on `exited` entries, and the LIVE filter (`ActiveSessionsBar.tsx:79-85`) removes `exited` before render. Post-restart the global row disappears until resumed — the accepted 15-UI-SPEC behavior. Task rows' sessionIds are unique per live row. No guard needed; add a comment citing this.

### Pitfall 6: Highlight needs `useLocation`, not `useParams`
**What goes wrong:** The current-page highlight reads `useParams().taskId` (`:74,131`) — undefined on `/global`, so a global row would never highlight.
**How to avoid:** Add `useLocation()` to the bar; a global entry's `isCurrent` is `location.pathname === "/global"`. Task rows keep `String(entry.taskId) === openTaskId` unchanged.

### Pitfall 7: `source` union widening — verify every branch site
**What goes wrong:** Adding `"global"` to `AgentStatusEntry.source` could silently change `source ===` sites. Audit result: exactly ONE runtime branch exists on this field — the PR badge (`ActiveSessionsBar.tsx:268`, `=== "github_pr"` — a global value flows through harmlessly). All other status-feed consumers find by `taskId` (`TaskPage.tsx:87-89`, `AgentTab.tsx:57-59`, PRCard) and real task ids are ≥ 1, never matching the global `0`. The widening is type-only plus the deliberate bar branches. [VERIFIED: grep-complete]
**How to avoid:** Keep it that way — no new `source ===` sites beyond the bar's navigation branch (`source === "global"`).

### Pitfall 8: New `setState-in-effect` lint debt
**What goes wrong:** `react-hooks/set-state-in-effect` is the repo's known lint-debt class (29 pre-existing errors, mostly TaskPage — STATE.md Phase 12 note). The Change-root dialog's reset-on-open and the agent-select fallback are the two spots at risk.
**How to avoid:** Use the adjust-state-during-render `prevOpen` tracker pattern (`AddProjectDialog.tsx:73-95`) for dialog resets; the instant-save select falls back to the saved value via controlled `value={saved}` + mutate-on-change (GithubSection posture, `SettingsPage.tsx:57-63`) — never a state mirror + effect. New files must lint clean in isolation.

### Pitfall 9: State-5 spawn expectations
**What goes wrong:** In state 5 (vanished root, sessions live) the `+`/Start stay ENABLED (warn-don't-block, UI-SPEC) — a click will 409 "global root no longer exists on disk: \<path\>" from the server.
**How to avoid:** That's the locked behavior: surface the 409 message verbatim inline next to the control (row 17 idiom, `TaskPage.tsx:465-473`); never pre-block in the client. Also: within the shell, `canSpawn` is effectively always true (states 3/4 are full-page replacements) — the tooltip always reads `New bash session`; no worktree-gating variant needed.

### Pitfall 10: PUT response handling and the agent-select race
**What goes wrong:** (a) Treating the PUT's 200 as a generic ack and re-fetching (wasteful — the response IS the GET shape); (b) the instant-save select sticking on the failed value.
**How to avoid:** `setQueryData(["global"], response)` on success (D-23). On failure the select is controlled by the cached `agent_id` — the cache is untouched on error, so the control snaps back automatically; only the inline `Couldn't save. Try again.` shows.

## Code Examples

### The bar's three surgical changes (GINT-01)
```tsx
// Source: ActiveSessionsBar.tsx:74,127-139 + UI-SPEC Surface 4 [VERIFIED: in-repo]
import { useLocation, useNavigate, useParams } from "react-router";
// ...
const location = useLocation();                       // NEW — global-route highlight
const { taskId: openTaskId } = useParams();

{sorted.map((entry) => (
  <SessionRow
    key={entry.sessionId}                             // CHANGE 1 — was key={entry.taskId} (:129)
    entry={entry}
    isCurrent={
      entry.source === "global"                       // CHANGE 3 — pathname highlight
        ? location.pathname === "/global"
        : String(entry.taskId) === openTaskId
    }
    onOpen={() => {
      if (entry.source === "global") {                // CHANGE 2 — navigation branch
        navigate("/global");
      } else {
        navigate(`/projects/${entry.projectId}/tasks/${entry.taskId}`);
      }
      collapse();
    }}
  />
))}
```
Row rendering is otherwise UNTOUCHED — `Global · Scratchpad` flows from the wire labels through the standard slots; the aria-label derives to `Open Scratchpad in Global` via the existing template; no badge/tint (D-12 → Phase 17).

### Route registration
```tsx
// Source: App.tsx:136-142 (the /activity precedent) [VERIFIED: in-repo]
import GlobalTaskPage from "@/pages/GlobalTaskPage";
// inside <Route element={<AppLayout />}>:
<Route path="/global" element={<GlobalTaskPage />} />
// Plain import (App.tsx does no lazy loading); NOT wrapped in BoardWorkspaceSync.
```

### The D-38/D-42 banner slot
```tsx
// Source: TaskPage.tsx:642 amber-warning idiom + 16-UI-SPEC banner contract
<div role="status"
     className="flex flex-wrap items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-2 py-1 text-xs text-amber-400">
  {/* D-38 (state 6): `Agent runs directly in ` + <span className="font-mono">{root}</span> + ` — no worktree isolation.`
      D-42 (state 5): `The configured root no longer exists on disk: ` + mono path + `. Running sessions are unaffected.` */}
</div>
```
Persistent (D-38) / advisory non-blocking (D-42); rendered under the h1 header, above the tab strip, visible on every tab.

### Instant-save agent selector (D-46)
```tsx
// Source: ProjectSettingsDialog.tsx:234-246 Select idiom + SettingsPage.tsx:57-63 fallback posture
const { data: g } = useGlobal();
const save = useSaveGlobal();
// controlled by server truth — snaps back on failure:
<Select value={String(g?.agent_id ?? "")}
        onValueChange={(v) => save.mutate({ agent_id: Number(v) },
          { onError: () => setErr(`Couldn't save. Try again.`) })}>
  <SelectTrigger className="w-full"><SelectValue /></SelectTrigger>
  <SelectContent>
    {(agents ?? []).map((a) => (
      <SelectItem key={a.id} value={String(a.id)}>
        {a.name}{a.is_default ? " (default)" : ""}
      </SelectItem>
    ))}
  </SelectContent>
</Select>
// help: `Applies at the next Start.` (D-24 hint)
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Bar keyed by `taskId`, task-route-only navigation | `sessionId` keying + `source:"global"` branch | Phase 15 wire / this phase | The P6 contract break — shipped in this phase's 3 edits |
| `AgentStatusEntry.source: "manual" \| "github_pr"` | Union gains `"global"` (server emits since Phase 15) | This phase | Type catches up to runtime; zero new branch sites beyond the bar |
| `TermSession` without scope fields | `global?: boolean` added | This phase | Honest typing for orphaned tmux ghosts |

**Deprecated/outdated in-repo (do NOT propagate):** `types.ts:62`'s `Agent.engine: "claude" | "custom"` union is stale w.r.t. the `"opencode"` system seed — type new code off the Go wire (plain `string`) rather than widening `types.ts` in this phase.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Settings Scratchpad section placement "directly after AgentsSection" (UI-SPEC recommendation, explicitly planner-adjustable) | Architecture Patterns | None — cosmetic reorder within the column |
| A2 | 409 reasons render as a `<ul>` of `reasons[].target` mono spans (mechanics are planner discretion; the wire shape is verified) | Pitfall 1 | Low — alternative renderings still satisfy D-45's verbatim rule |
| A3 | Two-plan phase split (STATE.md records "Total Plans in Phase: 2"); suggested seam: P1 = wire widenings + global.ts + hooks + bar + route, P2 = GlobalTaskPage + Settings section | Summary | None — planner re-splits freely |
| A4 | `npm run build` + `npm run lint` (new files isolated) + manual browser UAT are the phase's verification surface (no web test framework exists; `nyquist_validation: false`) | Environment | Low — if a test harness is desired it must be scoped explicitly |

**All other claims in this research were verified in-repo at file:line level** (the milestone research's own PRIMARY confidence standard) or cited from the approved 16-UI-SPEC.

## Open Questions

1. **D-37 engine-rule conflict resolution**
   - What we know: TaskPage's predicate (`!== "custom"`) shows the quota for opencode; D-37/UI-SPEC's stated outcome hides it; `types.ts`'s union is stale.
   - What's unclear: whether "same rule as TaskPage" means the predicate or the outcome.
   - Recommendation: implement the OUTCOME (`engine === "claude"`) on `/global`; do not touch TaskPage (out of scope). Flag the TaskPage predicate for the Phase 17 UAT/deferred list.
2. **409 reasons surfacing mechanics**
   - What we know: `ApiError` drops `reasons`; the wire emits `{kind, target}` objects.
   - What's unclear: additive `ApiError.reasons` extension vs message-only rendering.
   - Recommendation: additive extension (one optional field, populated in `client.ts` when the body carries reasons) + `<ul>` of targets — satisfies UI-SPEC row 22 with ~5 lines.
3. **Shell factoring depth**
   - What we know: CONTEXT discretion; research prescribes the trimmed-page lean.
   - What's unclear: extract a shared bash-tab-lifecycle hook vs duplicate-then-trim inside GlobalTaskPage.
   - Recommendation: duplicate-then-trim (zero TaskPage churn); extract only if the duplicated block exceeds ~80 lines unchanged.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Node.js | web build (Vite 8 needs ≥20.19/22.12) | ✓ | v24.4.0 | — |
| npm | dependency install/scripts | ✓ | 11.6.0 | — |
| web/node_modules | build/lint | ✓ | present | `npm install` if stale |
| Go toolchain | NOT needed (frontend-only phase; backend untouched) | ✓ | 1.26.0 | — |
| git / gh | NOT needed at runtime by this phase's code (clone flows hit the shipped API) | ✓ | — | — |

**Missing dependencies with no fallback:** none
**Missing dependencies with fallback:** none

## Security Domain

Local-only single-user app; this phase adds no auth, session, or crypto surface. Applicable ASVS categories:

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | local-only by design (PROJECT.md) |
| V3 Session Management | no | no auth sessions |
| V4 Access Control | no | localhost-bound API, unchanged |
| V5 Input Validation | yes (render-side) | Server validates all inputs (`validateRepoPath`, `ParseRepoRef`, D-07/D-27 blocks); the dialog disables submit on empty fields and renders server messages verbatim — React JSX escaping covers XSS for wire-supplied strings (paths, tmux names, reasons) |
| V6 Cryptography | no | nothing new |

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Wire-string injection (paths, labels, reasons → DOM) | Tampering | React auto-escaping; mono `<span>` rendering — never `dangerouslySetInnerHTML` |
| Un-isolated agent in user's checkout (P3) | Elevation | D-38 persistent banner + managed-clone default + server-side D-07/D-27 footgun blocks (all shipped; this phase only renders) |
| Clone-failure half-configured state | Tampering | Server SC2 atomicity (shipped); dialog failure posture preserves values, closes only on 2xx |

## Sources

### Primary (HIGH confidence)
- Live codebase at v1.13 Phase-15-complete (this worktree) — read directly at file:line: `internal/api/global.go`, `internal/api/sessions.go` (:68-226, :324-563, :1178-1183), `internal/api/agents.go` (:120-330), `internal/session/session.go` (:46-73), `internal/api/agents_backfill.go` (:82-102), `web/src/App.tsx`, `web/src/AppLayout` (`AppLayout.tsx`), `web/src/pages/{TaskPage,SettingsPage}.tsx`, `web/src/components/task/{AgentTab,TaskTabs,CleanupWorktreeDialog}.tsx`, `web/src/components/sidebar/{AddProjectDialog,ProjectSettingsDialog}.tsx`, `web/src/components/layout/ActiveSessionsBar.tsx`, `web/src/components/quota/QuotaIndicator.tsx`, `web/src/api/{agents,sessions,client,mutations,types}.ts`, `web/package.json`
- `.planning/phases/16-global-view-settings-bar-integration/16-UI-SPEC.md` — approved UI contract (view-state matrix, surfaces, copywriting rows, wire-widening table)
- `.planning/phases/{13,14,15}-*-CONTEXT.md` — locked D-decisions cited throughout
- `.planning/research/{SUMMARY,PITFALLS}.md` — §Phase 4, P3/P6 (architecture-locked)

### Secondary (MEDIUM confidence)
- TanStack Query v5 docs (tanstack.com — QueryClient.setRequestData/setQueryData, query-key dedup, refetchInterval) — cross-check only; the pinned-version behavior is verified in-repo

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — zero-add verified; every building block read at file:line
- Architecture: HIGH — wire contracts verified against shipped Go handlers; view matrix locked by approved UI-SPEC
- Pitfalls: HIGH — all pitfalls verified in-repo (grep-complete source-union audit; ApiError reason-dropping read at client.ts:22); the two prose-vs-code discrepancies (409 reasons shape, QuotaIndicator engine rule) are documented with resolutions

**Research date:** 2026-08-27
**Valid until:** 2026-09-27 (stable — composition phase over frozen in-repo contracts; no external dependencies)
