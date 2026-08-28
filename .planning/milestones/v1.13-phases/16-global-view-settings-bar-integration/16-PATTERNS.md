# Phase 16: Global view, Settings & bar integration - Pattern Map

**Mapped:** 2026-08-27
**Files analyzed:** 10 (3 new, 7 modified)
**Analogs found:** 10 / 10 — this phase is pure composition over in-repo patterns; every file has a strong analog.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `web/src/api/global.ts` (NEW) | service (API client hooks) | request-response (5s poll) | `web/src/api/settings.ts` + `web/src/api/agents.ts` | exact |
| `web/src/pages/GlobalTaskPage.tsx` (NEW) | component (page shell) | request-response (poll-driven tab lifecycle) | `web/src/pages/TaskPage.tsx` | exact (copy-then-trim) |
| `web/src/components/settings/ScratchpadSection.tsx` (NEW) | component (settings section) | request-response (instant-save + gated dialog) | `AddProjectDialog.tsx` + `SettingsPage.tsx` GithubSection + `ProjectSettingsDialog.tsx` | exact |
| `web/src/api/agents.ts` (EDIT) | model (wire type) | — (type-only widening) | itself; backend truth `internal/api/agents.go:182-196` | exact |
| `web/src/api/sessions.ts` (EDIT) | service + model | request-response | its own task-scoped hooks (`useSessions`/`useSpawnAgent`/…) | exact |
| `web/src/api/client.ts` (EDIT) | utility (error type) | request-response | itself; wire truth `internal/api/global.go:341-344` | role-match (additive field) |
| `web/src/components/task/AgentTab.tsx` (EDIT) | component | request-response | itself (variant discriminator, research Pattern 4) | exact |
| `web/src/components/layout/ActiveSessionsBar.tsx` (EDIT) | component | event-driven (5s status-feed consumer) | itself (3 surgical edits) | exact |
| `web/src/pages/SettingsPage.tsx` (EDIT) | component (page) | request-response | its own section column (`:158-218`) | exact |
| `web/src/App.tsx` (EDIT) | route/config | — | `/activity` route precedent (`:136-142`) | exact |

**Data-flow note:** everything rides the two existing polling feeds — `useAgentStatuses()` (5s) and the session list (5s). No streaming, no new transport.

## Pattern Assignments

### `web/src/api/global.ts` (NEW — service, request-response)

**Analogs:** `web/src/api/settings.ts` (query+mutation shape) and `web/src/api/agents.ts` (5s refetch idiom). Wire type is frozen at `internal/api/global.go:48-56`.

**Imports pattern** — copy `settings.ts:1-3` verbatim:
```typescript
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, put } from "./client";
import type { ApiError } from "./client";
```

**Core query pattern** — `useAgentStatuses` idiom (`agents.ts:20-26`) + `useSettings` idiom (`settings.ts:14-19`); research Pattern 2 is the locked spelling:
```typescript
// agents.ts:20-26 — the 5s poll every feed consumer uses
export function useAgentStatuses() {
  return useQuery({
    queryKey: ["agent-statuses"],
    queryFn: () => get<AgentStatusEntry[]>("/api/agents/status"),
    refetchInterval: 5000, // UI-SPEC: waiting surfaces within 5s
  });
}
```
→ `useGlobal()`: `queryKey: ["global"]`, `queryFn: () => get<GlobalConfig>("/api/global")`, `refetchInterval: 5000`.

**Mutation pattern** — `useSaveSetting` setQueryData idiom (`settings.ts:27-37`), plus the prefix-invalidation from `sessions.ts:53-54`:
```typescript
// settings.ts:27-37 — onSuccess writes the response into the cache (no refetch)
export function useSaveSetting(key: string) {
  const qc = useQueryClient();
  return useMutation<SettingEntry, ApiError, string>({
    mutationFn: (value) => put<SettingEntry>(`/api/settings/${key}`, { value }),
    onSuccess: (entry) =>
      qc.setQueryData<Settings>(["settings"], (old) => old && { ...old, [key]: entry }),
  });
}
```
→ `useSaveGlobal()`: `mutationFn: (body) => put<GlobalConfig>("/api/global", body)`, `onSuccess: (g) => qc.setQueryData(["global"], g)` — the PUT response IS the GET shape (`global.go:448-460`, D-23). Type `agent.engine` as plain `string` (mirror the Go wire; do NOT reuse `types.ts:62`'s stale union — research Pitfall 2).

**Wire contract (verified):** `globalConfig` at `global.go:48-56`:
```go
type globalConfig struct {
    RootPath   string      `json:"root_path"`
    GithubRepo *string     `json:"github_repo"`   // nil ⇒ folder root / unconfigured
    AgentID    int64       `json:"agent_id"`
    UpdatedAt  string      `json:"updated_at"`
    RootExists bool        `json:"root_exists"`
    Live       globalLive  `json:"live"`           // {agent, bash, tmux} counts
    Agent      globalAgent `json:"agent"`          // {id, name, engine}
}
```
PUT body grammar (`global.go:292-311`): pointer semantics — `{root_path?: string; repo?: string; agent_id?: number}`; omit untouched keys; `root_path: ""` is THE clear.

---

### `web/src/pages/GlobalTaskPage.tsx` (NEW — component/page, request-response)

**Analog:** `web/src/pages/TaskPage.tsx` — copy-then-trim (research Anti-Patterns: do NOT extract a shared shell from TaskPage; it's the hottest page in the app).

**Imports pattern** (`TaskPage.tsx:1-44`) — the trimmed subset: react, react-query, react-router (NO `useParams` — `/global` has no params), lucide `Ellipsis`/`Plus` (NO `ArrowLeft` — D-35), the scope-aware session hooks from `sessions.ts`, `Button`/`Skeleton`/`DropdownMenu`/`Tooltip`, `StatusDot`, `QuotaIndicator`, `AgentTab`, `TaskTabs`+`TabDef`, `TerminalPane`. Drop: `useTask`, `useUpdateTask`, `useCreateWorktree`, `usePullRequestDetail`, `useSettings`, `DescriptionTab`, `DiffTab`, `WorktreeMetaLine`, `DeleteTaskDialog`, `CleanupWorktreeDialog`, `Input`.

**State 1/2 — skeleton + error** (`TaskPage.tsx:246-270` for skeleton; error/retry from `SettingsPage.tsx:128-138`):
```typescript
// TaskPage.tsx:246-256 — pending skeleton (reuse shape)
if (isPending) {
  return (
    <div className="mx-auto w-full max-w-[860px] space-y-4 p-4">
      <Skeleton className="h-7 w-2/3" />
      <Skeleton className="h-8 w-48" />
      ...
    </div>
  );
}
```

**States 3/4 — degraded heroes (D-39/D-40)** — the centered-hero idiom from `App.tsx:41-54` (NOT TaskPage's "Task not found" — that one links back to a board that doesn't exist here):
```tsx
// App.tsx:41-54 — the hero shape both states share
<div className="flex h-full items-center justify-center">
  <div className="flex flex-col items-center gap-4 py-8 text-center">
    <div className="flex flex-col items-center gap-1">
      <h1 className="text-xl font-medium">…</h1>
      <p className="text-muted-foreground">…</p>   // max-w-[480px] when two-line
    </div>
    <Button onClick={…}>Open Settings</Button>      // navigate("/settings")
  </div>
</div>
```
The two states differ ONLY in copy + the mono-span path (D-40 names `root_path` verbatim).

**Core pattern — the bash-tab lifecycle block to copy wholesale** (`TaskPage.tsx:113-221`), then trim:
- `activeTab`/`closingIds`/`keepExitedIds` state (`:114-120`) — always lands on Agent tab
- keepExited poll effect (`:134-146`)
- **tmux reattach effect with the `reattachedRef` one-shot guard** (`:148-162`) — keep exactly; swap hook for the global variant
- `visibleSessions` filter+sort memo (`:168-188`) — `kind !== "agent"`, `!orphaned`, running-or-kept-exited
- `tabIds` memo (`:194-202`) — minus the `"description"`/`"diff"` entries: `["agent", ...visibleSessions.map(s => s.id)]`
- **Pitfall-7 neighbor-reactivation `useLayoutEffect`** (`:208-221`) — keep verbatim; `"agent"` is the universal fallback
- `removeTab`/`handleCloseTab`/`handleSpawn` (`:307-346`) — spawning ONLY from click handlers (StrictMode rule); invalidate key becomes `["sessions","global"]`
- Drop entirely: title editing (`:122-125`, `:294-303`), Esc handler (`:228-244` — research Anti-Pattern: no parent board), PR branch (`:278-292`), worktree meta line, cleanup/delete dialogs

**Tab strip wiring** (`TaskPage.tsx:348-475`):
```typescript
// :350-367 — the Agent TabDef: leading StatusDot, keepMounted, NO onClose
{
  id: "agent",
  label: "Agent",
  leading: agentEntry ? <StatusDot entry={agentEntry} /> : undefined,
  keepMounted: true,
  content: (<AgentTab scope={…} agentSession={agentSession} … />),
}
// :397-436 — bash TabDefs: muted/closing, onClose, onRename (running-only), keepMounted,
//            TerminalPane with onSessionExit → invalidate ["sessions","global"]
```
`agentEntry` lookup (`TaskPage.tsx:87-89`): swap `.find(e => e.taskId === taskId)` for the global entry — `.find(e => e.source === "global")` (taskId 0 never matches a real task; grep-verified no other consumer collides).

**Trailing `+` + spawn error** (`TaskPage.tsx:439-475`) — `canSpawn` is always true in the shell (states 3/4 are full-page replacements; research Pitfall 9); keep the 409-verbatim error span (`:465-473`):
```typescript
// :465-473 — 409 copy renders verbatim, 500s stay generic
{spawn.isError && !spawn.isPending && (
  <span className="text-xs whitespace-nowrap text-red-500">
    {spawn.error instanceof ApiError && spawn.error.status === 409
      ? spawn.error.message
      : "Couldn't start a session. Try again."}
  </span>
)}
```

**Header (D-35/D-37/D-38)** — plain h1 "Scratchpad", no back-arrow, no title button; QuotaIndicator gate keys on `engine === "claude"` from the `["global"]` query (research Pitfall 2 — do NOT copy TaskPage's `!== "custom"` predicate at `TaskPage.tsx:64`); the D-38/D-42 banner sits under the header above the tab strip using the amber idiom (see Shared Patterns).

---

### `web/src/components/settings/ScratchpadSection.tsx` (NEW — component, request-response)

**Analogs:** three-way composite, all locked by research:
1. **Summary card + section wrapper** — `SettingsPage.tsx:158-218` section composition
2. **Change-root dialog** — `AddProjectDialog.tsx` pattern verbatim (D-43)
3. **Agent selector** — `ProjectSettingsDialog.tsx:232-250` Select idiom (D-46)

**Section wrapper pattern** (`SettingsPage.tsx:160-169`):
```tsx
// Every settings section: this exact header + gap composition
<section className="flex flex-col gap-3">
  <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Scratchpad`}</h2>
  …card body…
</section>
```
The summary "card" is a composed `rounded-lg border bg-card` div (no `card.tsx` exists — research Standard Stack). Mount point: the 640px column beside `AgentsSection` (`SettingsPage.tsx:158-159`).

**Dialog core pattern** — copy `AddProjectDialog.tsx` structure with research-Pattern-5 deltas (no Name field, `PUT` submit, Clear-root destructive):

*Segmented toggle with integration-off degradation* (`AddProjectDialog.tsx:49,67,166-179`):
```tsx
const activeMode: Mode = integrationOn ? mode : "folder";   // :67
{integrationOn && (                                          // :166 — toggle renders only when on
  <Tabs value={mode} onValueChange={handleModeChange}>
    <TabsList aria-label="…" className="w-full">
      <TabsTrigger value="repo" disabled={createProject.isPending}>GitHub repo</TabsTrigger>
      <TabsTrigger value="folder" disabled={createProject.isPending}>Local folder</TabsTrigger>
    </TabsList>
  </Tabs>
)}
```

*Reset-on-open WITHOUT setState-in-effect* — the `prevOpen` adjust-during-render tracker (`AddProjectDialog.tsx:73-95`):
```typescript
const [prevOpen, setPrevOpen] = useState(open);
if (open !== prevOpen) {
  setPrevOpen(open);
  if (open) {
    setMode("repo"); setOwnerName(""); setRepoPath(""); setError(null);  // reset each open
  }
}
```

*Mono inputs + exactly-one-of {error, help}* (`AddProjectDialog.tsx:181-206`):
```tsx
<Input id="…" className="font-mono" placeholder="owner/name" value={ownerName}
       onChange={…} disabled={isPending} autoFocus />
{error !== null ? (
  <p className="rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-xs text-destructive">
    {error}
  </p>
) : (
  <p className="text-xs text-muted-foreground">…help…</p>
)}
```

*Blocking clone spinner* (`AddProjectDialog.tsx:250-259`):
```tsx
{cloning && (
  <p className="flex items-center gap-2 text-sm text-muted-foreground">
    <Loader2 className="size-4 animate-spin motion-reduce:animate-none" />
    <span>{`Cloning `}<span className="font-mono">{ownerName.trim()}</span>{`…`}</span>
  </p>
)}
```

**Error handling pattern** — degrade-don't-break catch (`AddProjectDialog.tsx:131-147`), the D-43/D-45 contract:
```typescript
} catch (err) {
  if (err instanceof ApiError) {
    let message = sentenceCase(err.message);          // :27-29 helper — copy it
    if (err.status === 409 && !message.endsWith(".")) message += ".";
    setError(message);
  } else {
    setError("Couldn't add the project. Try again.");
  }
  // Field values + active mode are intentionally kept so the input can be
  // corrected and resubmitted.   ← dialog stays OPEN; only a 2xx closes
}
```
For D-45's reasons list, extend with `err.reasons` (see `client.ts` assignment below) rendered as a `<ul>` of `reasons[].target` mono spans after the message paragraph.

**Agent selector pattern (D-46)** — `ProjectSettingsDialog.tsx:232-250` Select + the GithubSection controlled-fallback posture (`SettingsPage.tsx:85-99`):
```tsx
// ProjectSettingsDialog.tsx:234-246 — the Select itself
<Select value={agentId} onValueChange={setAgentId}>
  <SelectTrigger className="w-full"><SelectValue /></SelectTrigger>
  <SelectContent>
    {(agents ?? []).map((a) => (
      <SelectItem key={a.id} value={String(a.id)}>
        {a.name}{a.is_default ? " (default)" : ""}
      </SelectItem>
    ))}
  </SelectContent>
</Select>
```
Instant-save variant (research Code Examples): `value={String(g?.agent_id ?? "")}` controlled by the CACHE (not local state) + `save.mutate({agent_id: Number(v)}, { onError: () => setErr("Couldn't save. Try again.") })` — on failure the untouched cache snaps the control back automatically (research Pitfall 10). Help line: `Applies at the next Start.` (D-24).

**"Open Scratchpad" CTA (GCONF-05):** a primary `Button` + `navigate("/global")` on the summary card — plain `useNavigate` usage, no analog needed beyond `AddProjectDialog.tsx:130`'s post-create navigate.

---

### `web/src/api/agents.ts` (EDIT — model, type-only)

**Analog:** itself. One-line widening at `:13`:
```typescript
// agents.ts:13 — current
source: "manual" | "github_pr";
// → widen to:
source: "manual" | "github_pr" | "global";
```
**Verified safe:** grep-complete branch audit — the only runtime `source ===` site on the feed is the PR badge (`ActiveSessionsBar.tsx:268`, `=== "github_pr"` — global flows through harmlessly). `TaskPage.tsx:105,278` branch on `task.source` (a different field). No other edits to this file.

---

### `web/src/api/sessions.ts` (EDIT — service + model)

**Analog:** its own task-scoped hooks — the global variants mirror each one's onSuccess contract exactly (research Pattern 3).

**Model widening** — add to `TermSession` (`sessions.ts:5-22`), alongside `orphaned?`/`tmuxName?`:
```typescript
global?: boolean; // session.Info.Global (session.go:73) — orphaned global tmux ghosts carry it
```

**Hook spellings — the three idioms to copy:**

*Query* (`sessions.ts:24-36`) → `useGlobalSessions()`: `queryKey: ["sessions", "global"]`, `queryFn: () => get<TermSession[]>("/api/sessions?scope=global")` (backend branch `sessions.go:70-75`), `refetchInterval: 5000`.

*Mutation with spawn-select race fix* (`sessions.ts:38-57`):
```typescript
onSuccess: (session) => {
  // write the fresh session into the scoped cache BEFORE invalidating
  queryClient.setQueryData<TermSession[]>(["sessions", taskId], (old) =>
    old ? [session, ...old] : [session]);
  // Prefix-matches the scoped keys too.
  queryClient.invalidateQueries({ queryKey: ["sessions"] });
}
```
→ `useSpawnGlobalSession` posts `{ scope: "global" }`; `useSpawnGlobalAgent` posts `{ scope: "global", kind: "agent" }` and ALSO invalidates `["agent-statuses"]` (`sessions.ts:99` precedent); `useResumeGlobalAgent` posts `{ scope: "global", kind: "agent", resume: true }`; `useReattachGlobalTmux(name)` posts `{ scope: "global", reattach_tmux_name: name }` (field spellings verified at `sessions.go:337-349`; scope branch `:438-470`; global resume `:502-524`). All write `setQueryData(["sessions","global"], …)`.

**Unchanged shared hooks:** `useStopSession`, `useRenameSession` (needs a cache-key variant or param for the global scope's setQueryData), `useDeleteSession` — scope-blind via the `["sessions"]` prefix.

**Consumers verified:** `TaskPage.tsx:75-84`, `TerminalPage.tsx:21-22` (dev scope, untouched), `AgentTab.tsx:48-49`.

---

### `web/src/api/client.ts` (EDIT — utility, additive)

**Analog:** itself. Extend `ApiError` (`client.ts:1-9`) and the error extraction (`client.ts:19-28`) — research Pitfall 1's minimal mechanics:
```typescript
// current client.ts:19-27 — extracts only body.error, DISCARDS reasons
if (!res.ok) {
  let message = res.statusText;
  try {
    const body = (await res.json()) as { error?: string };
    if (body.error) message = body.error;
  } catch { /* non-JSON error body */ }
  throw new ApiError(message, res.status);
}
```
→ additively type the body as `{ error?: string; reasons?: { kind: string; target: string }[] }` and populate a new optional `ApiError.reasons` field when present. ~5 lines, no behavior change for existing callers.

**Wire truth** (`global.go:341-344` — the 409 the dialog surfaces):
```go
writeJSON(w, http.StatusConflict, map[string]any{
    "error":   "the Scratchpad root can't be changed while sessions are running",
    "reasons": blockers,   // []deleteBlocker — {Kind: "sessions", Target: "<tmux-name-or-label>"}
})
```
`deleteBlocker` grammar at `projects.go:681-689` (`kind: "sessions"`, `target` string). Render `reasons[].target` verbatim in mono spans — NO client re-wording.

---

### `web/src/components/task/AgentTab.tsx` (EDIT — component, variant loosening)

**Analog:** itself. Research Pattern 4's recommended shape: a `variant`/scope discriminator, single file, thin call sites.

**What the task variant consumes today (the loosening surface):**
- Hooks at `:48-50`: `useSpawnAgent(task.id)` / `useResumeAgent(task.id)` / `useStopSession()` → resolve off `scope.kind`
- Resumable lookup at `:57-59`: `.find(e => e.taskId === task.id)?.resumable` → global: `.find(e => e.source === "global")?.resumable`
- Pre-start CTA gating at `:95-100`: `hasWorktree` gates Start → global: CTA enabled unconditionally (D-41); body copy names the root in a mono span instead of `task.branch` (`:104-109`)
- ⋯ menu at `:147-184`: `showInsertDescription`/`showInsertSeed` both false for global → menu renders Stop only, NO separator before a lone item (D-36)
- Cache writes at `:191-199` (`handleConnect` optimistic waiting→idle) and `:242-244` (`onSessionExit` invalidate) → keys become `["agent-statuses"]` find-by-source and `["sessions","global"]`

**Menu pattern to trim** (`AgentTab.tsx:153-184`):
```tsx
const agentMenu = running ? (
  <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button variant="ghost" size="icon-sm" aria-label="Agent actions">
        <Ellipsis className="size-4" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end">
      {showInsertDescription && (…)}   // global: false
      {showInsertSeed && (…)}          // global: false
      {(showInsertDescription || showInsertSeed) && <DropdownMenuSeparator />}  // global: never
      <DropdownMenuItem variant="destructive"
        onSelect={() => stopSession.mutate(agentSession.id)}>
        {`Stop`}
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
) : undefined;
```

**States preserved 1:1 for global** (D-41 task-parity): pre-start (`:65-133`), running TerminalPane (`:229-251` with `headerMenu`, `dimWhenExited`, `showExitedClose={false}`, Reset/Resume pair `:213-227`). The task call site passes `{kind:"task", taskId}` + description + seed — byte-for-byte behavior preserved. The dead `projectId` prop (`:43`, declared never used) is droppable.

---

### `web/src/components/layout/ActiveSessionsBar.tsx` (EDIT — component, event-driven)

**Analog:** itself — exactly 3 surgical changes (research Code Examples, verified against the read):

**Current code** (`ActiveSessionsBar.tsx:74,127-139`):
```tsx
const { taskId: openTaskId } = useParams(); // current-task highlight (D-07)
…
{sorted.map((entry) => (
  <SessionRow
    key={entry.taskId}                                    // ← CHANGE 1
    entry={entry}
    isCurrent={String(entry.taskId) === openTaskId}       // ← CHANGE 3
    onOpen={() => {
      navigate(`/projects/${entry.projectId}/tasks/${entry.taskId}`);  // ← CHANGE 2
      collapse();
    }}
  />
))}
```

**Target** (research Code Examples, LOCKED by P6):
```tsx
import { useLocation, useNavigate, useParams } from "react-router";
const location = useLocation();                       // NEW
…
<SessionRow
  key={entry.sessionId}                               // CHANGE 1 — safe: "" only on exited rows,
                                                      //   removed by the LIVE filter (:79-85)
  isCurrent={
    entry.source === "global"                         // CHANGE 3 — pathname highlight
      ? location.pathname === "/global"
      : String(entry.taskId) === openTaskId
  }
  onOpen={() => {
    if (entry.source === "global") navigate("/global");   // CHANGE 2 — nav branch
    else navigate(`/projects/${entry.projectId}/tasks/${entry.taskId}`);
    collapse();
  }}
/>
```
`SessionRow` itself is UNTOUCHED — `Global · Scratchpad` flows from the wire labels (`agents.go:193-194` `TaskTitle:"Scratchpad"`, `ProjectName:"Global"`) through the standard slots; the aria-label derives to "Open Scratchpad in Global" via the existing template (`:247`). No new `source ===` sites beyond these two. No badge/tint (D-12 → Phase 17).

---

### `web/src/pages/SettingsPage.tsx` (EDIT — component, one-line mount)

**Analog:** its own column composition (`:158-218`). Insert `<ScratchpadSection />` as a sibling in the `flex flex-col gap-6` column (recommended after `AgentsSection` at `:159`; placement is planner-adjustable — research A1). Follow the existing import style (`:8-10`).

---

### `web/src/App.tsx` (EDIT — route/config)

**Analog:** the `/activity` precedent (`App.tsx:136-142`) — plain top-level sibling inside `<Route element={<AppLayout />}>`, NOT wrapped in `BoardWorkspaceSync`:
```tsx
<Route path="/settings" element={<SettingsPage />} />
{/* Phase 11 — bare top-level Activity route (sibling of /settings).
    … NO :projectId and is NOT wrapped in BoardWorkspaceSync. */}
<Route path="/activity" element={<ActivityPage />} />
```
→ add `<Route path="/global" element={<GlobalTaskPage />} />` + a plain default-import (`App.tsx:8-12` style — no lazy loading anywhere in this file).

## Shared Patterns

### TanStack Query feed idiom (all new hooks)
**Source:** `web/src/api/agents.ts:20-26`, `web/src/api/sessions.ts:24-36,45-56`
**Apply to:** `global.ts`, all global session hooks
- Queries: stable string/tuple `queryKey`, `refetchInterval: 5000`, `get<T>()` helper
- Mutations: `setQueryData` the response into the scoped cache FIRST (spawn-select race fix), then `invalidateQueries({queryKey: ["sessions"]})` (prefix-matches scoped keys); agent variants also invalidate `["agent-statuses"]`

### Degrade-don't-break dialog errors (D-43/D-45)
**Source:** `web/src/components/sidebar/AddProjectDialog.tsx:131-147` + `sentenceCase` helper `:27-29`
**Apply to:** ScratchpadSection's Change-root dialog
- `catch` → `instanceof ApiError` → `sentenceCase(err.message)` verbatim; generic fallback otherwise; dialog stays OPEN, values + active mode preserved; only a 2xx closes
- Reasons extension: `<ul>` of `err.reasons[].target` mono spans below the message (wire: `{kind:"sessions", target}` objects)

### Amber warning banner (D-38/D-42)
**Source:** `TaskPage.tsx:640-659` (merged/closed PR banner)
**Apply to:** GlobalTaskPage banner slot (under h1, above tab strip, every tab)
```tsx
<div role="status"
     className="mt-1 flex flex-wrap items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-2 py-1 text-xs text-amber-700 dark:text-amber-400">
  …text + <span className="font-mono">{path}</span>…
</div>
```

### Degraded-state hero (D-39/D-40)
**Source:** `App.tsx:41-54` (empty-workspace state)
**Apply to:** GlobalTaskPage states 3/4; `text-xl font-medium` h1 + muted body (`max-w-[480px]` when two-line) + primary Button → `/settings`. Retry state (`SettingsPage.tsx:128-138`) for state 2.

### Reset-on-open without setState-in-effect
**Source:** `AddProjectDialog.tsx:73-95` (`prevOpen` adjust-during-render tracker)
**Apply to:** the Change-root dialog's reset; instant-save select uses controlled-by-cache instead (research Pitfall 8 — the repo's known lint-debt class; new files must lint clean in isolation)

### Exactly-one-of {error, help} + mono wire strings
**Source:** `AddProjectDialog.tsx:195-206`, `ProjectSettingsDialog.tsx:301-308`
**Apply to:** all dialog inputs rendering server-supplied paths/names — always `<span className="font-mono">`, never `dangerouslySetInnerHTML` (React auto-escaping is the XSS mitigation)

### Route registration
**Source:** `App.tsx:136-142`
**Apply to:** `/global` — plain sibling Route inside `AppLayout`, outside `BoardWorkspaceSync`, no URL params

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| (none with zero analog) | — | — | — |

Two partial-gap notes for the planner:
- **409 reasons `<ul>` rendering** — no existing UI renders a structured `reasons[]` list client-side (`ApiError` drops them today; `ForceRemoveDialog.tsx:186-190` renders only `error.message` inline). The mechanics are research-prescribed (additive `ApiError.reasons` + `<ul>` of mono targets); treat `ForceRemoveDialog`'s inline-error-keeps-dialog-open posture as the closest behavioral analog.
- **QuotaIndicator engine predicate** — TaskPage's `!== "custom"` (`TaskPage.tsx:64`) is the WRONG predicate to copy for `/global` (it shows for the runtime `"opencode"` engine; research Pitfall 2). Implement the D-37 outcome: `engine === "claude"` off the `["global"]` query. Do NOT touch TaskPage or `types.ts:62` this phase.

## Metadata

**Analog search scope:** `web/src/**` (api, pages, components/{task,layout,settings,sidebar,quota,ui}, App.tsx), `internal/api/{global,agents,sessions,projects}.go`, `internal/session/session.go` — read at file:line
**Files scanned:** 16 analog files read in full or targeted; 2 grep audits (`source ===` branch sites; session-hook consumers)
**Pattern extraction date:** 2026-08-27
**Cross-references:** 16-CONTEXT.md (D-35..D-46 + carry-forward), 16-RESEARCH.md (Patterns 1-6, Pitfalls 1-10), 16-UI-SPEC.md (view-state matrix — planner should read alongside)
