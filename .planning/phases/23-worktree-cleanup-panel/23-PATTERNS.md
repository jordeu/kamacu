# Phase 23: Worktree Cleanup Panel - Pattern Map

**Mapped:** 2026-07-02
**Files analyzed:** 12 (5 backend, 7 frontend)
**Analogs found:** 11 / 12 (the porcelain `List` parser is genuinely new — see § No Analog Found)

> This is an **extension phase**. Every destructive mechanism already exists and
> flows through one audited path (`CleanupWorktreeGated`). The panel is its
> **third caller**. Only two backend pieces are new engineering: the
> `git worktree list --porcelain -z` parser and the D-01 permission-blocked
> outcome. Everything else copies an in-repo analog verbatim.

---

## File Classification

### Backend (Go)

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/worktree/worktree.go` (+`List`, maybe +`Prune`) | service (git shell-out) | transform (parse `--porcelain -z`) | `internal/worktree/worktree.go` `DirtyCount`/`StashCount` (same file, `gitRun` idiom) | role-match (no list parser exists yet) |
| `internal/api/cleanup.go` (extend `CleanupWorktreeGated`: orphan mode + `blocked` outcome) | service (shared removal core) | CRUD (gated delete) | *itself* — additive `taskID==0` + EACCES branches | exact (modify-in-place) |
| `internal/api/cleanuppanel.go` (NEW: `list`/`remove`/`cleanEligible`/`clearPointer` + `WorktreeCleanupRoutes`) | controller | request-response (GET annotate + POST mutate) | `internal/api/worktrees.go` `worktreeHandlers` + `internal/api/projects.go` `deleteManaged`/`removeManaged` | role-match (compose two analogs) |
| `internal/api/routes.go` OR `cmd/kamacu/main.go` (register the new routes) | route wiring | request-response | `main.go:191-192` (`ghSvc := github.New`; `api.PullRequestRoutes(...)`) | exact |
| `internal/api/cleanuppanel_test.go` (NEW) | test | — | existing `worktrees`/`projects` DELETE tests (D-04 regression guard) | role-match |

### Frontend (React / TS)

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `web/src/api/worktreeCleanup.ts` (NEW: `useWorktreeList`, `useRemoveWorktree`, `useCleanEligible`, `useClearPointer`) | api hooks (query + mutations) | request-response | `web/src/api/diffs.ts` (`useTaskDiff` query) + `web/src/api/worktrees.ts` (`useCleanupWorktree` mutation) | exact |
| `web/src/components/settings/WorktreeCleanupSection.tsx` (NEW) | component (Settings `<section>`) | request-response | `SettingsPage.tsx` `GithubSection` (section shell) + `DiffTab.tsx` (fetch-on-mount + spinning Refresh) | exact (compose two analogs) |
| `web/src/components/settings/WorktreeRow.tsx` (NEW) | component (row) | — | `DiffTab.tsx` totals-bar row + `CleanupWorktreeDialog.tsx` chip/warning idioms | role-match |
| `web/src/components/settings/ForceRemoveDialog.tsx` (NEW) | component (confirm dialog) | request-response | `web/src/components/task/CleanupWorktreeDialog.tsx` | exact |
| `web/src/components/settings/CleanEligibleDialog.tsx` (NEW: preview→confirm bulk) | component (confirm dialog) | request-response | `CleanupWorktreeDialog.tsx` (AlertDialog scaffold) | role-match |
| `web/src/pages/SettingsPage.tsx` (+`<WorktreeCleanupSection/>`) | page | — | `SettingsPage.tsx` itself (append one `<section>`) | exact (modify-in-place) |
| `web/src/components/ui/badge.tsx` (NEW shadcn primitive) | ui primitive | — | `web/src/components/ui/*` (copied-in shadcn) — `npx shadcn add badge` | tooling (no hand-roll) |

---

## Pattern Assignments

### `internal/worktree/worktree.go` — new `List(ctx, repo) ([]Entry, error)` (service, transform)

**Analog:** the `gitRun`-based helpers in the SAME file (`DirtyCount` @ 288, `StashCount` @ 326). Copy their shape: single `gitRun` call, parse stdout, return typed data. Use `-z` NUL-safety exactly like `DirtyCount` already does.

**`gitRun` shell-out idiom** (`internal/worktree/worktree.go:36-49`) — the invariant: arg-array `exec.CommandContext`, never a shell; error is trimmed git stderr:
```go
func gitRun(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		msg = strings.TrimPrefix(msg, "fatal: ")
		if msg == "" { msg = err.Error() }
		return "", fmt.Errorf("%s", msg)
	}
	return out.String(), nil
}
```

**NUL-split parse precedent** (`DirtyCount`, `internal/worktree/worktree.go:288-300`) — mirror this for the porcelain record/attribute split:
```go
out, err := gitRun(ctx, wt, "status", "--porcelain=v2", "--untracked-files=all", "-z")
if err != nil { return 0, err }
n := 0
for _, rec := range strings.Split(out, "\x00") {
	if strings.TrimSpace(rec) != "" { n++ }
}
```

**New parser body:** use RESEARCH.md §Pattern 1 verbatim (`Entry` struct + records `\x00\x00`-split, attributes `\x00`-split; `worktree `/`HEAD `/`branch `/`bare`/`detached`/`locked`/`prunable` cases). The FIRST entry per repo is the **main worktree** — exclude it by `filepath.Clean(Entry.Path) == filepath.Clean(project.repo_path)` (RESEARCH Pitfall 2).

**Reuse for flags (do NOT re-derive):** `DirtyCount(ctx, wt)` @288, `UnpushedCount(ctx, wt, base)` @307, `StashCount(ctx, wt)` @326, `ResolveBase(ctx, repo)` @125, `DefaultBranch(ctx, repo)` @163. For a STALE-POINTER "clear", `Remove` already runs `git worktree prune` at its tail (`worktree.go:378`) — a dedicated `Prune` is optional.

**Anti-pattern (verified, RESEARCH Pattern 4):** do NOT add a blanket `--force` retry to `Remove` for the permission case. `Remove` @356-380 only retries `--force` on `"submodules"` in the error — keep it that way; a permission failure must surface the PLAIN-remove error (the "Permission denied" / "Directory not empty" text), never the misleading `--force` "is not a working tree".

---

### `internal/api/cleanup.go` — extend `CleanupWorktreeGated` (service, CRUD)

**Analog:** the function itself (`internal/api/cleanup.go:36-91`). Both extensions are **additive** — the existing two callers pass `taskID>0` and never hit EACCES, so the D-04 regression tests stay green (RESEARCH Open Q2).

**Current signature + gate structure** (`internal/api/cleanup.go:36-91`) — the byte-equivalent shared core:
```go
func CleanupWorktreeGated(
	ctx context.Context, db *sql.DB, wt *worktree.Service,
	mgr *session.Manager, tmuxClient tmux.Client, liveTmux []string,
	taskID int64, repo, path string, sessionCount int,
	stopSessions, force bool,
) (removed bool, reason string, err error) {
	if sessionCount > 0 && !stopSessions { return false, "sessions running", nil }
	dirty := 0
	if _, statErr := os.Stat(path); statErr == nil {
		dirty, err = wt.DirtyCount(ctx, path)
		if err != nil { return false, "", err }
	}
	if dirty > 0 && !force { return false, "worktree has uncommitted changes", nil }
	// ...StopAllForTask, kill liveTmux, then:
	if rerr := wt.Remove(cctx, repo, path, dirty > 0); rerr != nil {
		return false, "", rerr          // ← Extension point A: classify EACCES here
	}
	if _, uerr := db.Exec(              // ← Extension point B: skip this when taskID == 0
		`UPDATE tasks SET branch = NULL, worktree_path = NULL, worktree_error = NULL WHERE id = ?`, taskID); uerr != nil {
		return false, "", uerr
	}
	return true, "", nil
}
```

**Extension A — D-01 blocked outcome** (RESEARCH Pattern 4). At the `wt.Remove` failure, before returning the raw error, detect a permission block and return a distinct signal. Prefer the typed error the research recommends (planner picks one shape):
```go
// var ErrRemoveBlocked = errors.New("blocked")
// type BlockedError struct{ Path string }  // .Error() carries the offending path
var pe *fs.PathError
if errors.As(rerr, &pe) && errors.Is(rerr, fs.ErrPermission) {
	return false, "blocked", &BlockedError{Path: pe.Path}   // handler → 200 {outcome:"blocked", path}
}
// stderr fallback: strings.Contains(rerr.Error(), "Permission denied") ...
```
The app NEVER runs `sudo`/`pkexec` (D-01, Security Domain V5) — it only carries `pe.Path` up for the copyable hint.

**Extension B — orphan mode:** when `taskID == 0`, skip the null-columns `UPDATE` (there is no task row). Additive branch:
```go
if taskID > 0 {
	if _, uerr := db.Exec(`UPDATE tasks SET branch = NULL, worktree_path = NULL, worktree_error = NULL WHERE id = ?`, taskID); uerr != nil {
		return false, "", uerr
	}
}
```

---

### `internal/api/cleanuppanel.go` — NEW handlers (controller, request-response)

**Analogs:** `internal/api/worktrees.go` (`worktreeHandlers`, the 2nd caller) for the handler struct + session-count helpers + single-item remove; `internal/api/projects.go` `deleteManaged`/`removeManaged` (@546/682) for the multi-worktree gated loop.

**Handler struct + session helpers** — copy from `worktreeHandlers` (`internal/api/worktrees.go:35-127`). The struct carries `db, wt, mgr, tmuxClient` and (new) `pr PRStateGetter`. The three session helpers are **deliberately replicated per handler** (~30 lines, see `projects.go:741` note) — copy `liveTmuxNames` (@84), `cleanupSessionCount` (@119), `runningSessions` (@63):
```go
func (h *worktreeHandlers) cleanupSessionCount(ctx context.Context, taskID int64) int {
	count := h.runningSessions(taskID)
	for _, name := range h.liveTmuxNames(ctx, taskID) {
		if !h.mgr.HasLiveTmux(name) { count++ }
	}
	return count
}
```

**Single force-remove — call the shared helper** (mirror `worktrees.go:250-266`, D-03 passes both flags true):
```go
count := h.cleanupSessionCount(ctx, taskID)
live := h.liveTmuxNames(ctx, taskID)
removed, reason, err := CleanupWorktreeGated(
	ctx, h.db, h.wt, h.mgr, h.tmuxClient, live,
	taskID /* 0 for orphan */, repo, path, count, true /*stopSessions*/, true /*force*/)
// map (removed, reason, err/BlockedError) → 204 / 409 reason / 200 {outcome:"blocked"} / 500
```

**Bulk clean-eligible — two-pass gate-then-remove, best-effort per item** (mirror `deleteManaged` gate phase @566-640 + `removeManaged` remove loop @682-704, but per-item skip instead of all-or-nothing; RESEARCH A6). Gate phase (network-free unpushed base, exactly like `deleteManaged`, `projects.go:549-604`):
```go
defBranch, dberr := h.wt.DefaultBranch(ctx, clone)
unpushedBase := "origin/" + defBranch
// per worktree: stat → DirtyCount → (dberr==nil) UnpushedCount(path, unpushedBase) → StashCount → cleanupSessionCount
```
Remove phase — loop `CleanupWorktreeGated(..., stopSessions=false, force=false)` (NEVER forces, D-04); a raced-tripped gate ⇒ skip that item and continue (unlike `removeManaged` which 409s the whole op).

**PR merged/closed eligibility** (`internal/reaper/reaper.go:262-269`) — the degrade-don't-break pattern; call `PRState` only for `source='github_pr'` rows, warn-and-skip on any gh error:
```go
state, err := r.pr.PRState(ctx, t.repo, int(t.n))
if err != nil { continue }                        // degrade-don't-break (D-03)
if state != "MERGED" && state != "CLOSED" { continue }
```
`PRStateGetter` interface (`reaper.go:65`): `PRState(ctx, repo string, n int) (string, error)` → `"OPEN"|"CLOSED"|"MERGED"`. `*github.Service` satisfies it.

**Response helpers** (`internal/api/respond.go:9-18`): `writeJSON(w, status, v)` / `writeError(w, status, msg)` → `{"error": msg}`. The structured multi-item response precedent is `deleteManaged`'s `{"error":..., "reasons":[...]}` (`projects.go:643-649`).

**DB query shape for the union** — the DB side of D-06/D-07. `taskColumns` (`internal/api/tasks.go:61`) has `id, project_id, ..., branch, worktree_path, ..., source, pr_number, pr_base_ref`. Projects: `projectColumns` (`projects.go:90`) = `id, name, repo_path, ..., managed, ...`. Enumerate repos via `SELECT id, name, repo_path, managed FROM projects` (mirror `projects.go:113`), and DB task rows via `SELECT id, worktree_path, source, pr_number, status, pr_base_ref, branch FROM tasks WHERE worktree_path IS NOT NULL` (mirror `projectWorktrees` @658-673).

---

### `internal/api/routes.go` / `cmd/kamacu/main.go` — register routes (route wiring)

**Analog:** `cmd/kamacu/main.go:191-192` — the `ghSvc` is already constructed and shared with the reaper + PR routes; wire the new routes there (they need `ghSvc` for `PRState`):
```go
ghSvc := github.New(github.Config{})
api.PullRequestRoutes(mux, db, ghSvc, wtSvc)
// + api.WorktreeCleanupRoutes(mux, db, wtSvc, mgr, tmuxClient, ghSvc)
```

**Route-registration func shape** — mirror `WorktreeRoutes` (`internal/api/worktrees.go:28-33`), method+path ServeMux idiom (Go 1.22+):
```go
func WorktreeCleanupRoutes(mux *http.ServeMux, db *sql.DB, wt *worktree.Service,
	mgr *session.Manager, tmuxClient tmux.Client, pr PRStateGetter) {
	h := &cleanupPanelHandlers{db: db, wt: wt, mgr: mgr, tmuxClient: tmuxClient, pr: pr}
	mux.HandleFunc("GET /api/worktrees", h.list)
	mux.HandleFunc("POST /api/worktrees/remove", h.remove)
	mux.HandleFunc("POST /api/worktrees/clean-eligible", h.cleanEligible)
	// + clear-pointer for D-07
}
```
Note: `api.SettingsRoutes(mux, db)` is called INSIDE `api.Routes` (`routes.go:33`); the worktree/diff/PR routes are called directly in `main.go`. Follow the `main.go`-direct convention (needs `ghSvc`).

---

### `web/src/api/worktreeCleanup.ts` — NEW hooks (api hooks, request-response)

**Analog (query):** `web/src/api/diffs.ts` `useTaskDiff` (@52-57) — fetch-on-mount, no `refetchInterval`, D-61. Copy verbatim:
```ts
export function useWorktreeList() {
  return useQuery<WorktreeListResponse, ApiError>({
    queryKey: ["worktrees"],
    queryFn: () => get<WorktreeListResponse>("/api/worktrees"),
    // staleTime 0 default = fetch on mount; refetch() = the manual Refresh button; NO refetchInterval (no poll)
  });
}
```

**Analog (mutation):** `web/src/api/worktrees.ts` `useCleanupWorktree` (@48-62) — invalidate on success:
```ts
export function useRemoveWorktree() {
  const queryClient = useQueryClient();
  return useMutation<RemoveResult, ApiError, RemoveArgs>({
    mutationFn: (body) => post<RemoveResult>("/api/worktrees/remove", body),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["worktrees"] }),
  });
}
```
`useCleanEligible` and `useClearPointer` follow the same POST + `invalidateQueries(["worktrees"])` shape.

**HTTP helpers** (`web/src/api/client.ts`): `get`/`post`/`put`/`del`; `ApiError` carries `.status` + `.message` (the server `{error}` string). A 204 returns `undefined`; a `{outcome:"blocked", path}` 200 is a normal JSON body — the mutation reads it (not an error).

---

### `web/src/components/settings/WorktreeCleanupSection.tsx` — NEW section (component)

**Analog (section shell):** `SettingsPage.tsx` `GithubSection` (@63-122) — the `<section className="flex flex-col gap-3">` + byte-identical heading class:
```tsx
<section className="flex flex-col gap-3">
  <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Worktree cleanup`}</h2>
  <p className="text-xs text-muted-foreground">{`Every git worktree across all projects. Removing a worktree never deletes its branch.`}</p>
  {/* header controls row, per-project groups, dialogs */}
</section>
```
UI-SPEC copy literals are single grep-able template strings (Phase 3 convention); reuse `withMono(text, tokens)` (`SettingsPage.tsx:16-36`) to render paths/branches in `font-mono` inside a contract literal.

**Analog (spinning Refresh, D-10):** `DiffTab.tsx:181-200` — the EXACT pattern to copy:
```tsx
<Button variant="ghost" size="icon-sm" aria-label="Refresh worktrees"
  disabled={isFetching} onClick={() => refetch()}>
  <RefreshCw className={isFetching ? "size-4 animate-spin motion-reduce:animate-none" : "size-4"} />
</Button>
```

**Analog (load-error + retry):** `SettingsPage.tsx:127-137` (`Couldn't load settings.` + `Retry loading` outline button) and `DiffTab.tsx:90-100` — scope the error to the section, never blank the page. **Loading skeletons:** `SettingsPage.tsx:147-155` (3-4 `Skeleton` rows).

**Analog (per-project group avatar):** `ProjectAvatar` (`web/src/components/ui/ProjectAvatar.tsx`) — `size="inline"` monogram for a quiet group header (needs `letters`, `color` from the project row).

---

### `web/src/components/settings/WorktreeRow.tsx` — NEW row (component)

**Analog:** `DiffTab.tsx` totals-bar flex row (@148-201) for the `[flex-1] [chips] [actions]` layout; `CleanupWorktreeDialog.tsx:126-131` for the `TriangleAlert` inline-warning idiom (blocked banner). Classification/flag chips use the new `badge` primitive (variants per UI-SPEC § Color table: `Blocked` = `destructive`, everything else muted `outline`/`secondary`). Copy the sudo-hint clipboard call from RESEARCH:
```ts
await navigator.clipboard.writeText(`sudo rm -rf ${blockedPath}`);
```

---

### `web/src/components/settings/ForceRemoveDialog.tsx` — NEW confirm (component)

**Analog:** `web/src/components/task/CleanupWorktreeDialog.tsx` (whole file) — copy nearly verbatim. The load-bearing bits:

**Fresh-state-at-open + reset on flip** (`CleanupWorktreeDialog.tsx:44-56`): fetch on `open` (Pitfall 8, never list cache), clear typed confirm + reset mutation on open/close.

**Type-to-confirm dirty gate** (`CleanupWorktreeDialog.tsx:70-75`) — copy EXACTLY:
```ts
const target = state.data?.path.split("/").pop() ?? "";  // worktree dir basename
const gateOpen = dirty === 0 || confirmText === target;
const confirmDisabled = loading || cleanup.isPending || !gateOpen;
```

**Body variants + branch-kept line** (`CleanupWorktreeDialog.tsx:110-140`): compose sessions / dirty (`TriangleAlert`) / branch-kept lines; the branch-kept reassurance is the last line of EVERY variant (D-02/D-34). **CTA** (@96-100, 178-189): `AlertDialogAction variant="destructive"`, pending `Removing…`; keep the dialog open until the mutation lands (`e.preventDefault()` in `onClick`). New for D-01: a `blocked` outcome closes the dialog and the row shows the banner (not a red error); a real error renders inline (`@168-172`).

---

### `web/src/components/settings/CleanEligibleDialog.tsx` — NEW bulk preview (component)

**Analog:** `CleanupWorktreeDialog.tsx` AlertDialog scaffold (@102-193). Differences per UI-SPEC § Clean-eligible preview: previews the exact server-computed set (never forces, D-05); `AlertDialogAction` is the DEFAULT variant (NOT destructive — nothing destructive is touched by construction); pending `Cleaning up…`; on settle, `invalidateQueries(["worktrees"])`. The clear-stale-pointer confirm is a lighter variant of the same scaffold (non-destructive CTA `Clear pointer`).

---

### `web/src/pages/SettingsPage.tsx` — append section (page)

**Analog:** the file itself. Append `<WorktreeCleanupSection/>` to the section stack (`SettingsPage.tsx:157-224`), inside the `max-w-[640px]` `flex flex-col gap-6` column — one more `<section>`, no full-width breakout (UI-SPEC § Panel width).

---

### `web/src/components/ui/badge.tsx` — NEW shadcn primitive (ui primitive)

**Analog:** the copied-in shadcn primitives in `web/src/components/ui/` (`button.tsx`, `alert-dialog.tsx`, etc.). Do NOT hand-roll a `<span>` chip. Run `npx shadcn add badge` (official registry, `components.json` `registries: {}` — no vetting gate). It lands as `web/src/components/ui/badge.tsx` matching the existing `radix-nova`/`neutral`/`cssVariables` config.

---

## Shared Patterns

### One shared gated-removal path — call it, never re-implement
**Source:** `internal/api/cleanup.go:36` `CleanupWorktreeGated`
**Apply to:** `cleanuppanel.go` single-remove AND bulk-clean loop (the panel is the **3rd caller** after `worktrees.go:252` and `projects.go:689` / `reaper.go:319`).
The load-bearing ordering (gate sessions → gate dirty → `StopAllForTask` → kill live tmux → `wt.Remove` → null-columns) is already correct; a second copy re-opens the orphan-shell hole (RESEARCH § Don't Hand-Roll).

### Session counting incl. detached tmux survivors
**Source:** `internal/api/worktrees.go:63-127` (`runningSessions` / `liveTmuxNames` / `cleanupSessionCount`) — replicated on `projectHandlers` (`projects.go:744-792`) and the reaper.
**Apply to:** the new `cleanupPanelHandlers` — copy the three methods onto the struct (deliberate ~30-line replication per `projects.go:741`).

### git shells out with `--porcelain`, arg-array exec only
**Source:** `internal/worktree/worktree.go:36` `gitRun` (CLAUDE.md invariant)
**Apply to:** the new `List` parser (`--porcelain -z`), every git call in the panel. Never `sh -c`; never parse human git output.

### Network-free unpushed base (read-heavy annotate GET)
**Source:** `internal/api/projects.go:549-604` (`deleteManaged`: `DefaultBranch` → `origin/<default>` → `UnpushedCount`, no fetch). NOT the reaper's per-worktree `FETCH_HEAD` path.
**Apply to:** the panel's per-row unpushed flag + D-04 eligibility (RESEARCH Pattern 3 / A2 / Pitfall 3). A base-resolution failure ⇒ show "unpushed: unknown" and treat as ineligible (conservative), never removable.

### Fetch-on-mount + manual Refresh, no polling (D-10 / D-61)
**Source:** `web/src/api/diffs.ts:52` (`useTaskDiff` — no `refetchInterval`) + `web/src/components/task/DiffTab.tsx:181-200` (spinning `RefreshCw`).
**Apply to:** `useWorktreeList` + the section's Refresh button. Every mutation invalidates `["worktrees"]` on settle (Pitfall 4).

### Fresh server state at confirm time, never the list cache (Pitfall 8)
**Source:** `web/src/components/task/CleanupWorktreeDialog.tsx:44-64` (fetch on open, reset on flip) + backend re-check inside `CleanupWorktreeGated`.
**Apply to:** `ForceRemoveDialog` (compose from a fresh fetch) and the server, which re-checks gates at remove time regardless of the client snapshot.

### Response shape (JSON error / structured reasons)
**Source:** `internal/api/respond.go:9-18` (`writeJSON` / `writeError` → `{"error": msg}`); `internal/api/projects.go:643-649` (structured `{"error":..., "reasons":[...]}` for multi-item).
**Apply to:** all panel handlers; the D-01 blocked outcome is a **200** with `{outcome:"blocked", path}` (RESEARCH A4), distinct from a 409 gate reason or a 500.

### degrade-don't-break on gh (PR state)
**Source:** `internal/reaper/reaper.go:262-269`; `internal/github/state.go:23` (`PRState` returns `("", err)` when gh absent).
**Apply to:** panel PR-merged eligibility — call `PRState` only for `source='github_pr'` rows, warn-and-skip on any error; when GitHub integration is off, PR eligibility is simply unavailable (Done-status + orphan rows still work).

---

## No Analog Found

| File / Piece | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `internal/worktree/worktree.go` `List(ctx, repo)` porcelain parser | service | transform | No `git worktree list` parser exists anywhere in the repo. Use RESEARCH §Pattern 1 verbatim (verified against git 2.43.0). Follows the `gitRun` + NUL-split idioms of `DirtyCount`/`StashCount` in the same file, but the parse logic itself is new. |
| D-01 `BlockedError` / `fs.ErrPermission` detection in `CleanupWorktreeGated` | service | — | No existing code inspects `fs.ErrPermission` / `*fs.PathError` on a remove. New logic (RESEARCH §Pattern 4, empirically verified). Additive to the existing function; the existing callers never hit this branch, so the D-04 regression tests stay green. |

Both new pieces are backend-only and were verified empirically in RESEARCH.md — the planner should use RESEARCH.md §Pattern 1 and §Pattern 4 as the source of truth for them.

---

## Metadata

**Analog search scope:** `internal/worktree/`, `internal/api/`, `internal/reaper/`, `internal/github/`, `cmd/kamacu/`, `web/src/api/`, `web/src/components/`, `web/src/pages/`
**Files scanned:** ~20 source files (all read this session; no re-reads)
**Pattern extraction date:** 2026-07-02
