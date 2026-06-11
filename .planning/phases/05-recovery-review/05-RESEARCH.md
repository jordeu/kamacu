# Phase 5: Recovery & Review - Research

**Researched:** 2026-06-11
**Domain:** `claude --resume` semantics (live binary), restart reconciliation with memory-only sessions, git diff plumbing for the read-only review tab
**Confidence:** HIGH — every load-bearing claim was verified empirically on this host: scripted PTY probes against the installed claude binary (zero prompts submitted, zero API usage) and scratch-repo git runs against git 2.43.0. All probe artifacts cleaned up.

⚠️ **Version note:** the installed claude is now **v2.1.173** — it auto-updated past the v2.1.170 that Phase 4 verified. All Phase-5-relevant flags (`--resume`, `--session-id`, `--settings`, `--fork-session`) were re-verified on 2.1.173, including a fresh `--session-id` + `--settings` SessionStart-hook run that confirmed the Phase 4 overlay mechanism still works unchanged.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Resume (RCVR-02 + the Phase 4 checkpoint commitment)
- **D-54:** Resume is available in BOTH places: (a) the exited agent banner within a server run — **Resume session** (primary) + **Reset session** (ghost/secondary); (b) after a server restart, the Agent tab's pre-start layout with headline `Agent session ended.`, body explaining a previous conversation can be resumed, and the same button pair. One consistent pair everywhere a resumable session exists.
- **D-55:** Resume always runs `claude --resume <uuid>` in the task's worktree using the persisted `tasks.claude_session_id`. The UUID workflow is locked end-to-end: Kangent mints the UUID at spawn (`claude --session-id <uuid>`), stores it in SQLite next to the task, never parses claude output. Each fresh spawn/Reset overwrites with a new UUID (newest conversation wins); Resume targets the stored one.
- **D-56:** Resume failure (transcript gone, claude errors) is honest: the terminal shows claude's error output; the banner returns with Reset session. No silent fallback to a fresh session.
- **D-57:** After a restart, resumable tasks show the normal muted-gray exited dot — no new dot states; the Resume affordance lives in the task view.
- **D-58:** Bash sessions after a restart vanish quietly — tabs mirror live sessions (D-28) and none are live. No exited stubs.

#### Diff Tab (REVW-01)
- **D-59:** Scope: everything the task changed vs the **merge-base** with the base branch (three-dot semantics) — commits on the task branch PLUS uncommitted changes (staged, unstaged) PLUS untracked files rendered as full additions. "What did this task change" in one view, even if the base branch moved on.
- **D-60:** Presentation: totals bar at top ("N files changed, +A −B vs <base>") with a refresh button; collapsible per-file sections with unified diffs, add/remove line coloring, per-file stats (+x −y) in headers.
- **D-61:** Refresh on tab open + manual refresh button. No auto-refresh polling.
- **D-62:** Large-diff handling: files over ~400 changed lines render collapsed (stats only, click to expand); binary files always stat-only ("Binary file changed").
- **D-63:** Empty state: "No changes yet. The worktree matches <base branch>." (quiet, consistent with existing empty states).
- **D-64:** Tab placement: **Agent, Description, Diff, Bash 1..N**. On tasks without a worktree the Diff tab is present but disabled with an explanation (same pattern as the bash `+`).

#### Post-Restart / Reconciliation (RCVR-01)
- **D-65:** Reconciliation is silent: on startup the server reconciles task agent-state recorded in the DB (no ghost "running"), no banners or notices. The UI simply reflects reality (gray dots, Resume offered).
- **D-66:** Persistence stays task-level: the task's last agent session id + whether the agent ended cleanly (whatever minimal columns recovery needs). NO session-history table; bash sessions remain fully ephemeral.
- **D-67:** Never resume one session into two PTYs (roadmap criterion): one agent per task is already enforced server-side; resume goes through the same one-per-task 409 gate.

### Claude's Discretion
- Mechanics of detecting "agent was running at shutdown" vs "ended cleanly" (e.g. a status column updated on exit vs reconciled at startup) — keep it minimal per D-66
- Diff implementation: git plumbing choice (`git diff <merge-base>` + status for untracked), parsing into structured JSON vs raw patch text rendering, syntax coloring depth (add/remove only is fine)
- Whether the diff renders client-side from a unified patch or the server pre-structures per-file hunks
- Exact body copy for the resumable pre-start state (follow UI-SPEC patterns)
- The `/terminal` dev route's fate at v1 close (keep — costs nothing — unless it interferes)
- Carried UAT item: verify whether plan-mode exit-plan approval triggers the amber waiting dot (research OQ1) during this phase's human verification

### Deferred Ideas (OUT OF SCOPE)
- Sessions audit table (no v1 consumer)
- Restart notice banner (silent reconciliation chosen; revisit only if silence proves confusing)
- Diff comments / feed-line-to-agent interactions (v2 territory)
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| RCVR-01 | On server startup, sessions recorded as running but no longer alive are reconciled to an exited state (no ghost sessions) | §Reconciliation: nothing durable ever records "running" (verified across migrations + code) — ghosts are impossible by architecture; the visible reconciliation is the `/api/agents/status` DB-derived extension (Pattern 2). No migration needed (D-66 minimal reading). D-67 two-PTY safety rides the existing 409 gate + no auto-resume anywhere. |
| RCVR-02 | App persists each task's Claude session ID and, after a server restart, offers "Resume session" which relaunches via `claude --resume` in the worktree | `tasks.claude_session_id` already persisted (Phase 4). §Claude v2.1.173 Verified: `--resume <uuid>` reuses the SAME session ID (no fork), combines with the `--settings` hook overlay (SessionStart fires with `source:"resume"`), fails honestly (exit 1, visible error) when the transcript is missing. Resumable derivation via transcript glob (Pattern 1); resume spawn variant (Pattern 3). |
| REVW-01 | User can open a read-only diff tab in the task view showing the worktree's changes vs the base branch | §Diff Plumbing Verified: `git diff <merge-base>` covers committed+staged+unstaged (incl. staged new files), excludes untracked; `ls-files --others` + `diff --no-index /dev/null` (exit 1 = success) renders untracked as additions; `--numstat -z` rename/binary record formats captured. Diff service + endpoint (Patterns 4–5), hand-rolled renderer + shadcn `collapsible` (Pattern 6). |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Locked stack:** Go 1.26 + stdlib ServeMux, `creack/pty` v1, `coder/websocket`, `modernc.org/sqlite` + goose, `os/exec` + system git with `--porcelain`/machine output (go-git forbidden), React 19 + Vite + TanStack Query 5 + shadcn/Tailwind 4, binary WS frames. **No new Go modules or npm packages** beyond the shadcn `collapsible` block the approved 05-UI-SPEC already sanctions (it vendors `@radix-ui/react-collapsible` via the official registry).
- **Never `sh -c` with interpolation** — `exec.Command` with arg arrays everywhere (diff service follows the Phase 3 `gitRun` pattern).
- The app spawns the real `claude` CLI, never reimplements it; no SDK chat UI.
- REST under `/api/*` via route-registration functions; JSON respond helpers; table-driven httptest; fake-claude stub via `AgentConfig.ClaudeBin` — real claude never spawned in CI.
- Frontend: typed TanStack hooks; verbatim UI-SPEC copy in single template literals.
- GSD workflow enforcement: implementation happens through `/gsd:execute-phase`.
- User global rule: never mention/co-author Claude or happy-otter on commits/PRs.

## Summary

Phase 5 closes v1 with three workstreams, and research resolved every open unknown empirically:

1. **Resume is simpler than feared.** On the installed claude v2.1.173: `--resume <uuid>` **reuses the original session ID** (verified via SessionStart hook payload — same UUID, `source:"resume"`, no new transcript file), so `tasks.claude_session_id` never needs updating after a resume. The `--settings` hook overlay **works combined with `--resume`** — status detection survives resume unchanged. A missing transcript fails fast and honestly: `No conversation found with session ID: <uuid>`, **exit code 1**, no silent fallback, no interactive picker — D-56's honest failure is claude's native behavior. Bonus finding: resume lookup is **global across project dirs** (not cwd-scoped, correcting PITFALLS.md) — claude finds the transcript by ID from any cwd and runs with the launch cwd, so resuming in the task's worktree always works even if path-encoding assumptions drift.

2. **RCVR-01 needs no migration.** Verified across all three migrations and the whole API/session layer: nothing durable ever records "running". Sessions are memory-only; the only persisted agent state is `tasks.claude_session_id` (never a status). Ghost "running" sessions are impossible by architecture — a restart leaves an empty manager and an empty sessions list. The one real gap: `/api/agents/status` is purely manager-derived, so post-restart it returns `[]` and D-57's gray dot + the UI-SPEC's server-reported `resumable` flag have no carrier. The fix IS the reconciliation: extend that endpoint with DB-derived entries for resumable tasks. D-66's "whether the agent ended cleanly" turns out to need **zero columns** — D-57 mandates the same muted-gray dot for every post-restart resumable task regardless of how it ended, and resumability itself is best derived from **transcript existence** (a glob of `~/.claude/projects/*/<uuid>.jsonl`), which also makes D-56's "resumable: false after a failed resume" fall out automatically: the resume failed precisely because the transcript is gone.

3. **Diff plumbing is two git calls plus one per untracked file.** Verified in a linked-worktree scratch repo: `git -C <wt> diff <merge-base>` covers committed + staged + unstaged (including staged **new** files) and correctly excludes the base branch's later movement; untracked files come from `ls-files --others --exclude-standard -z` and render via `git diff --no-index /dev/null <file>` (which produces a proper `new file` patch and **exits 1 on success** — a must-know). Rename detection is on by default; `--numstat -z` has a documented-here rename record quirk. Recommend server-side parsing into structured JSON (the approved UI-SPEC already says "hand-written from the structured API response").

**Primary recommendation:** No schema change. Add `resume` to the existing spawn endpoint (same one-per-task 409 gate, `--resume <id>` instead of `--session-id <id>`), extend `/api/agents/status` with DB-derived resumable entries (+ `resumable` field), build `internal/diff` (or extend `internal/worktree`) on the Phase 3 `gitRun` pattern with a Go unified-diff parser returning structured JSON, and render with a hand-rolled `DiffTab` + the official shadcn `collapsible` block.

## Claude v2.1.173 — Verified Behaviors (empirical, 2026-06-11)

All from scripted PTY runs on this host. No prompts submitted; zero API usage. Probes ran in a throwaway detached worktree of the (trusted) kangent repo — no trust dialog (re-confirms Phase 4 finding 12 on 2.1.173).

| # | Question | Verified Answer | Evidence |
|---|----------|-----------------|----------|
| 1 | Installed version | **2.1.173** (auto-updated from Phase 4's 2.1.170). `--resume [value]`, `--fork-session`, `--session-id <uuid>`, `--settings <file-or-json>` all present and unchanged; new flags (`--no-session-persistence`, `--from-pr`) irrelevant here | `claude --version`, `--help` |
| 2 | `--resume <bogus-uuid>` behavior (D-56) | Prints `No conversation found with session ID: <uuid>` (red), **exits code 1, fast**. No fallback to a fresh session, no picker. Run in a trusted dir — in an UNtrusted dir the trust dialog renders BEFORE resume resolution (claude sits waiting; see Pitfall 5) | bogus-resume probe |
| 3 | Does `--resume` reuse or fork the session ID? | **Reuses.** SessionStart hook payload on the resumed session: `session_id` == the original UUID, `transcript_path` == the original file; **no new .jsonl created**; the original transcript grew slightly (resume appends bookkeeping lines). `--fork-session` exists for opt-in forking — do NOT pass it. `tasks.claude_session_id` stays valid across resumes forever | resume probe payload |
| 4 | `--resume` + `--settings` overlay compatibility | **Works.** The inline-JSON overlay's SessionStart hook fired on the resumed session with `source: "resume"` (vs `"startup"` on fresh spawns). Hook injection / status detection needs zero changes for resume | resume probe payload |
| 5 | Is resume cwd-scoped? | **No — global lookup by session ID across all of `~/.claude/projects/*`** (corrects PITFALLS.md's "directory-scoped" note). Resuming the probe session from a different cwd loaded it (appended to the transcript in its ORIGINAL project dir) and ran with the new cwd. Kangent still always resumes in the task's worktree (D-55) — the cwd determines where the resumed agent operates | cross-cwd probe |
| 6 | Does a never-prompted session have a transcript? | **No.** After spawn + SIGTERM with no prompt ever sent, the project dir was never created and no `.jsonl` exists — even though the SessionStart payload reported a `transcript_path`. `--resume` of such an ID fails per #2. **Transcript existence == resumability** | startup-only probe |
| 7 | Transcript path encoding | `<cwd with every `/` and `.` → `-`>`: `/tmp/claude5-probe/wt` → `-tmp-claude5-probe-wt`; `~/.kangent/worktrees/...` → `-home-jordi--kangent-worktrees-...` (the `.` makes a double dash). Because of #5, **don't compute the encoding — glob `~/.claude/projects/*/<uuid>.jsonl` instead**, which matches claude's own lookup | `~/.claude/projects` inspection |
| 8 | `--session-id` + inline `--settings` on 2.1.173 | Still work exactly as Phase 4 verified on 2.1.170: payload echoes the supplied UUID, hook fires, fields `{session_id, transcript_path, cwd, hook_event_name, source, model}` | fresh-spawn probe |

## Reconciliation Design (RCVR-01) — recommendation: no migration

**Verified current state** (read 2026-06-11): migrations 00001–00003 contain no session table and no status column; `internal/session` is memory-only; `internal/api/sessions.go` persists only `tasks.claude_session_id` at agent spawn; `/api/agents/status` derives entirely from `mgr.List()`. **Nothing in the DB can claim "running".**

Therefore:

- **Ghost sessions cannot exist.** After a restart the manager is empty: `GET /api/sessions?task_id=N` → `[]` (bash tabs vanish — D-58 already true by architecture), `GET /api/agents/status` → `[]`. The roadmap's "reconciled to exited" is satisfied by construction; the planner should document this argument in the plan and lock it with an integration test (fresh Manager + existing DB ⇒ no running sessions, no auto-spawn).
- **No startup mutation pass is needed.** There is no row to flip to "exited". `cmd/kangent/main.go` needs no reconciliation hook.
- **D-66's "whether the agent ended cleanly" needs zero columns.** Its only conceivable consumer is the post-restart dot color — and D-57 fixes that to the normal muted gray for every resumable task. Within a run, exit codes live in the in-memory session as today.
- **The real work is the status/resumable carrier:** post-restart, D-57 wants a gray exited dot and D-54b wants the resumable pre-start — both need data the manager no longer has. Extend `/api/agents/status` (Pattern 2).

**Resumability derivation (single server-side rule, used everywhere):**

```
resumable(task) =
    task.claude_session_id != NULL
 && task.worktree_path != NULL              // resume spawns in the worktree (D-55)
 && glob(~/.claude/projects/*/<id>.jsonl) hits   // transcript exists (verified: existence == resumability)
 && no RUNNING agent session for the task in the manager
```

Why the transcript glob (instead of `claude_session_id != NULL` alone):
- A never-prompted session (started, never typed, exited or died with the server) has **no transcript** (verified #6) — offering Resume would guarantee an error. The glob suppresses it; the banner correctly shows Reset-only.
- D-56 + UI-SPEC ("server reports `resumable: false` after a failed resume") falls out with **zero state mutation**: a transcript-gone resume failure means the glob misses on the next poll — no "clear the column on non-zero exit" heuristics, no false negatives when a successfully resumed session later crashes (transcript still there ⇒ still resumable).
- It matches claude's own global lookup (verified #5), so it can't disagree with what `--resume` will do.
- Cost: one `filepath.Glob` per task with a session ID per status poll (~80 dirs on this host, single-user) — negligible; trivially cacheable later if ever needed.

`tasks.claude_session_id` is never cleared; fresh spawns/Reset overwrite it (existing behavior, D-55 newest-wins).

## Architecture Patterns

### Pattern 1: Resumable computation (shared helper)

```go
// internal/api (or a small internal/resume helper): one function, used by
// the agents/status handler and the resume-spawn validation.
func transcriptExists(claudeSessionID string) bool {
    home, err := os.UserHomeDir()
    if err != nil || claudeSessionID == "" {
        return false
    }
    // Guard: the ID is a kangent-minted UUID, but never glob raw input.
    if _, err := uuid.Parse(claudeSessionID); err != nil {
        return false
    }
    matches, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", claudeSessionID+".jsonl"))
    return len(matches) > 0
}
```

### Pattern 2: `/api/agents/status` extension (D-57, D-54b carrier)

Two changes to `internal/api/agents.go`:

1. **`resumable` field** on every entry, computed per the derivation above (manager-derived entries too — the within-run exited banner needs it for the D-54a button pair).
2. **DB-derived entries** appended for tasks that have NO session in the manager (post-restart) but are resumable:

```
SELECT id, project_id, claude_session_id, worktree_path FROM tasks
WHERE claude_session_id IS NOT NULL AND worktree_path IS NOT NULL
```

For each id not already covered by a manager-derived entry and whose transcript globs: emit `{taskId, projectId, sessionId: "", status: "exited", exitCode: null, stopRequested: false, resumable: true}`.

- Emit DB-derived entries ONLY when resumable — a non-resumable past session post-restart gets no dot and the plain "No agent session" pre-start, which keeps dot-vs-tab consistency (D-57 only constrains resumable tasks).
- **Frontend contract change:** `dotMeta` in `web/src/components/StatusDot.tsx` currently renders exited with `exitCode === null && !stopRequested` as RED and tooltips `Exited (code null)`. Add a null-exit-code branch: gray (`bg-zinc-600`), tooltip `Exited` (no code). This is the D-57 muted-gray dot.
- `AgentStatusEntry` (web/src/api/agents.ts) gains `resumable: boolean`; TaskPage/AgentTab read it from the already-polled `["agent-statuses"]` query — the UI never infers resumability (UI-SPEC rule).

### Pattern 3: Resume spawn (RCVR-02, D-55/D-56/D-67)

REST: extend the existing `POST /api/sessions` body — `{task_id, kind: "agent", resume: true}`. A dedicated endpoint buys nothing; the variant reuses every existing gate in `internal/api/sessions.go`:

- Same task lookup / worktree 409 (`task has no worktree`).
- Same one-agent-per-task 409 (`agent session already running`) — this IS D-67's never-two-PTYs guarantee; concurrent Resume clicks race the gate and the loser gets the standard inline error (UI-SPEC).
- New pre-spawn validation: read `claude_session_id`; if NULL or transcript missing → 409 (e.g. `no session to resume`) — the client should rarely see this (resumable gating), but the server never trusts the client.
- Never auto-resume anywhere (startup, reconnect) — resume happens only on this explicit request.

`internal/session`: `SpawnOpts` gains `ResumeSessionID string`. In `Manager.Spawn`, the agent branch becomes:

```go
claudeSessionID = opts.ResumeSessionID         // resume: reuse (verified — claude keeps this ID)
if claudeSessionID == "" {
    claudeSessionID = uuid.NewString()         // fresh spawn: mint (existing behavior)
}
idFlag := "--session-id"
if opts.ResumeSessionID != "" {
    idFlag = "--resume"
}
cmd = exec.Command(bin, idFlag, claudeSessionID,
    "--settings", buildOverlayJSON(cfg.BaseURL, cfg.Token, id))   // overlay verified to apply on resume
```

- Do NOT pass `--fork-session`. Do NOT combine `--resume` with `--session-id`.
- The existing post-spawn `UPDATE tasks SET claude_session_id = ...` line stays as-is: on resume it rewrites the same value (harmless, keeps one code path).
- Status machine: unchanged. A resumed spawn starts `working` (startup output flows); a failed resume prints claude's error and exits code 1 → the standard exited flow (red dot per D-43 — an exit Kangent didn't request; banner returns Reset-only because the transcript glob now misses).
- Frontend: `useResumeAgent(taskId)` posts `{task_id: taskId, kind: "agent", resume: true}`; AgentTab renders the button pair when `resumable` (banner via TerminalPane's new second-action slot per UI-SPEC; pre-start via the new resumable variant). Mutual exclusion while either mutation is in flight (UI-SPEC).

### Pattern 4: Diff service (`internal/diff` or extending `internal/worktree`)

One `gitRun`-style exec per operation (Phase 3 pattern: arg arrays, ctx timeout, exit-code success, trimmed stderr for UI). Recommended flags on every diff invocation: `--no-color --no-ext-diff` (belt-and-braces against user git config; non-tty already disables color by default) and `-c core.quotePath=false` so non-ASCII paths arrive verbatim.

Full pipeline for `GET /api/tasks/{id}/diff` (D-59):

```
1. task row → worktree_path (NULL → 409 "task has no worktree"; dir missing → error state)
2. repo := projects.repo_path; base := worktree.ResolveBase(ctx, repo)   // existing, reused
3. mb   := git -C <wt> merge-base <base> HEAD                            // exit 128 → error state w/ stderr
4. stats: git -C <wt> diff --no-color --no-ext-diff --numstat -z <mb>
5. patch: git -C <wt> diff --no-color --no-ext-diff <mb>                 // ONE call, split on "^diff --git"
6. untracked: git -C <wt> ls-files --others --exclude-standard -z
7. per untracked file: git -C <wt> diff --no-index --no-color /dev/null <relpath>
   → EXIT CODE 1 MEANS SUCCESS-WITH-DIFF (0 = identical, >1 = real error)  ← must special-case
8. correlate numstat ↔ patch sections by path; parse into the JSON shape below
```

Response shape (server pre-structures — resolves the discretion area; UI-SPEC already says the renderer is "hand-written from the structured API response"):

```jsonc
{
  "base": "main",                       // ResolveBase result, for the totals bar + empty state
  "totals": { "files": 3, "additions": 120, "deletions": 8 },
  "files": [
    {
      "path": "web/src/App.tsx",
      "oldPath": null,                  // set for renames
      "status": "modified",             // modified | new | deleted | renamed (untracked → "new")
      "binary": false,
      "additions": 12, "deletions": 3,  // null/omitted for binary
      "hunks": [
        { "header": "@@ -1,4 +1,5 @@",
          "lines": [ { "kind": "context|add|del", "text": "..." } ] }
      ]
    }
  ]
}
```

Parsing notes (all verified):
- **`--numstat -z` rename records** look like `added<TAB>deleted<TAB><NUL><old><NUL><new><NUL>` — an EMPTY path field then two NUL-separated paths. Non-renames are `added<TAB>deleted<TAB><path><NUL>`. The parser must handle both.
- **Binary** files: numstat emits `-<TAB>-<TAB><path>`; the patch section says `Binary files ... differ` and has no hunks → `binary: true`, stat-only section (D-62).
- **Rename detection is ON by default** (git ≥2.9 `diff.renames=true`; verified: `similarity index`, `rename from/to` headers appear with no `-M` flag). Keep defaults; renames render as a `renamed` header (100% similarity → no hunks).
- Unified-diff parser must tolerate: `\ No newline at end of file` lines (attach to the preceding line or skip), mode-change-only sections (no hunks), `deleted file mode` / `new file mode` headers.
- Per-file "changed lines" for the D-62 >400 collapse = `additions + deletions` — the frontend computes the default open/closed state from stats; no server flag needed.
- Empty diff: steps 4–6 all empty → `files: []` → D-63 empty state with `base`.
- File ordering: tracked files in git's output order, untracked appended sorted — or sort everything by path for stability (planner's pick; suggest path-sorted).

### Pattern 5: Diff REST + frontend data flow (D-61)

- `GET /api/tasks/{id}/diff` registered alongside the worktree routes. Computation is on-demand only; no caching, no polling.
- `useTaskDiff(taskId)` TanStack query, fetched when the Diff tab activates and on the manual refresh button (`refetch()`). While refetching with data present, TanStack keeps the previous data rendered (UI-SPEC: no blanking). Errors render the UI-SPEC error state with the server-relayed git stderr.
- Disabled tab (no worktree): the existing `TabDef[]` seam in `TaskTabs.tsx` needs a small extension (`disabled?: boolean` + tooltip on a span wrapper — mirror the bash `+` pattern). Diff inserted at index 2: Agent, Description, Diff, Bash 1..N (D-64). Worktree removed while open → tab disables, Agent-tab fallback (existing Pitfall-7 effect).

### Pattern 6: Diff renderer (frontend, no new deps beyond the sanctioned block)

- `npx shadcn@latest add collapsible` — official registry block (vendors `@radix-ui/react-collapsible`), sanctioned by the approved 05-UI-SPEC. Radix Collapsible unmounts closed content by default — exactly what D-62 needs: collapsed >400-line files cost zero DOM.
- `DiffTab` (totals bar + file list + empty/loading/error states) and `DiffFileSection` (Collapsible: header row trigger with chevron/path/status-suffix/stats; body maps hunks → rows). All copy, colors, spacing per 05-UI-SPEC verbatim (diff content palette reuses ANSI green-400/red-400 ramp values; true minus U+2212 in stats).
- Rendering is a dumb map over the structured JSON — no diff parsing client-side, no syntax highlighting (CONTEXT discretion resolved by UI-SPEC: add/remove coloring only), `overflow-x: auto` per file body, selectable text.

### Anti-Patterns to Avoid

- **Auto-resuming at startup or on reattach** — explicit-click only; auto-resume is the two-PTY/ghost-run bug class (PITFALLS, D-67).
- **Passing `--fork-session`** — forks a NEW session ID, orphaning `tasks.claude_session_id`.
- **Clearing `claude_session_id` on resume failure** — unnecessary state mutation; the transcript glob already makes `resumable` false, and clearing breaks the "crashed but still resumable" case.
- **Computing the transcript path from the cwd encoding** — undocumented scheme (`/`→`-`, `.`→`-`); the glob matches claude's actual global lookup and survives encoding drift.
- **Treating `git diff --no-index` exit 1 as failure** — 1 means "differences found" (success here); only >1 is an error.
- **`git add -N` to surface untracked files in the diff** — mutates the user's index in a read-only feature. Use `--no-index` per file.
- **Parsing human git output / switching on exit codes** for diff metadata — `-z` machine formats + exit-0/1 contract only (Phase 3 lesson holds).
- **A sessions table or status column "for completeness"** — explicitly deferred (no v1 consumer); RCVR-01 needs neither.
- **Hiding resume errors / silent fallback to fresh spawn** — D-56: claude's own error text in the dimmed terminal IS the contract.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Resume identity | Transcript scanning/mtime heuristics | `claude --resume <uuid>` with the stored ID | Verified deterministic; same-ID reuse confirmed on 2.1.173 |
| Resumability truth | "Ended cleanly" columns + reconcile passes | Transcript-existence glob | Matches claude's own lookup; zero schema; self-heals D-56 |
| Untracked-as-additions | Manual hunk synthesis from file bytes | `git diff --no-index /dev/null <f>` | Git produces exact `new file` patches incl. binary detection (verified) |
| Binary detection | NUL-byte sniffing | `--numstat` `-` markers / `Binary files` patch lines | Git's own heuristic, already in the outputs we parse |
| Diff base | Stored creation-time base | `worktree.ResolveBase` re-resolution + `git merge-base` | Existing tested chain; D-59 is defined against the current base |
| Per-file stats / >400 gate | Counting parsed hunk lines | `--numstat` additions+deletions | One source of truth for totals bar, file headers, and collapse gate |
| Collapsible sections | Custom disclosure component | shadcn `collapsible` (Radix) | Official block, UI-SPEC-sanctioned, unmounts closed content for free |

**Key insight:** claude v2.1.173 and git both already implement the hard parts (global resume lookup, ID reuse, rename/binary classification, no-index addition patches). Phase 5's custom code is a ~150-line unified-diff parser and a ~30-line resumable derivation — everything else is wiring through existing seams.

## Common Pitfalls

### Pitfall 1: `git diff --no-index` exit code 1
**What goes wrong:** The shared `gitRun` helper treats any nonzero exit as failure; every untracked file "errors".
**How to avoid:** A dedicated runner (or flag) for `--no-index`: 0 = identical (skip), 1 = diff produced (success), ≥2 = real error.
**Warning signs:** Untracked files missing from the diff; error state on worktrees with new files only.

### Pitfall 2: numstat `-z` rename records desync the parser
**What goes wrong:** Naive `fields[2] = path` parsing breaks on renames (`added\tdeleted\t\0old\0new\0` — empty path, then TWO NUL-terminated paths), shifting every subsequent record.
**How to avoid:** Tokenize on NUL; when the path field after the second TAB is empty, consume the next two tokens as old/new. Table-test with a rename fixture.
**Warning signs:** Wrong paths/stats after the first renamed file in a diff.

### Pitfall 3: Post-restart dot renders RED
**What goes wrong:** DB-derived entries have `exitCode: null, stopRequested: false`; current `dotMeta` maps that to red + tooltip `Exited (code null)`.
**How to avoid:** Add the null branch to `dotMeta` (gray, tooltip `Exited`) in the same plan that ships the endpoint extension — they're one contract.
**Warning signs:** Every task with a past agent shows an alarming red dot after restart.

### Pitfall 4: Offering Resume for never-prompted sessions
**What goes wrong:** Deriving `resumable` from `claude_session_id != NULL` alone offers Resume for sessions that never wrote a transcript (verified: startup-only sessions create NO .jsonl) — guaranteed `exit 1` on click.
**How to avoid:** The transcript glob in the derivation (Pattern 1).
**Warning signs:** "Resume session" reliably erroring on tasks where the agent was started but never used.

### Pitfall 5: Trust dialog masquerading as a hung resume
**What goes wrong:** In a repo never opened in claude, the trust dialog renders BEFORE resume resolution (verified) — the session sits "working/idle", no error, no conversation.
**How to avoid:** Nothing to build (D-51 posture: user answers in the terminal) — but don't let anyone "fix" it with timeouts. Known Phase 4 caveat (Pitfall 7 there), now also applies to Resume; keep it in the verification checklist.
**Warning signs:** Resume "stuck" reports on brand-new projects.

### Pitfall 6: Resume validation raced by Reset
**What goes wrong:** Client shows Resume (poll-stale `resumable`), user clicks after a Reset already minted a new ID — server resumes the NEW id (fine) or, if cleanup raced, errors confusingly.
**How to avoid:** Server reads `claude_session_id` fresh inside the handler (single source of truth at spawn time) + the existing one-per-task gate; client disables both buttons while either mutation is in flight (UI-SPEC mutual exclusion).
**Warning signs:** Flaky resume-vs-reset integration tests.

### Pitfall 7: Diff endpoint on a vanished/dirty-edge worktree
**What goes wrong:** Worktree dir manually deleted, or base branch deleted since creation — `merge-base`/`diff` exit 128 and a raw 500 leaks.
**How to avoid:** Stat the worktree dir first; relay trimmed git stderr in the UI-SPEC error state (`Couldn't load the diff.` + muted git error + `Reload diff`). ResolveBase's 4-step chain already covers most base weirdness; its failure also routes to the error state.
**Warning signs:** Blank diff tab instead of the error card.

### Pitfall 8: `CLAUDE_CONFIG_DIR` / non-default claude home
**What goes wrong:** If the user relocates claude's config dir, the transcript glob misses → `resumable` false even though `--resume` would work.
**How to avoid:** Accept for v1 (user runs defaults — verified `~/.claude` populated); optionally honor `$CLAUDE_CONFIG_DIR` in the glob root. Honest failure path still works either way.
**Warning signs:** Resume never offered on a machine where claude works fine.

## Code Examples

### Verified resume probe payloads (what the hooks deliver)

```jsonc
// Fresh spawn (claude --session-id aaaa... --settings <overlay>):
{"session_id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeee0001",
 "transcript_path":"/home/jordi/.claude/projects/-tmp-claude5-probe-wt/aaaaaaaa-….jsonl",
 "cwd":"/tmp/claude5-probe/wt","hook_event_name":"SessionStart","source":"startup","model":"…"}

// Resume (claude --resume aaaa... --settings <overlay>) — SAME session_id, source "resume":
{"session_id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeee0001",
 "transcript_path":"/home/jordi/.claude/projects/-tmp-claude5-probe-wt/aaaaaaaa-….jsonl",
 "cwd":"/tmp/claude5-probe/wt","hook_event_name":"SessionStart","source":"resume"}
```

### Verified failure output (D-56 contract)

```
$ claude --resume 11111111-2222-3333-4444-555555555555     # transcript doesn't exist
No conversation found with session ID: 11111111-2222-3333-4444-555555555555
$ echo $?
1
```

### Verified diff command set (D-59, scratch worktree, git 2.43.0)

```bash
MB=$(git -C "$WT" merge-base "$BASE" HEAD)                 # exit 128 + stderr on failure
git -C "$WT" diff --no-color --no-ext-diff --numstat -z "$MB"
#  → "1\t0\tkeep.txt\0"  "-\t-\ttracked.bin\0"  "0\t0\t\0oldname.txt\0newname.txt\0"
git -C "$WT" diff --no-color --no-ext-diff "$MB"           # committed+staged+unstaged, incl. staged NEW files
git -C "$WT" ls-files --others --exclude-standard -z       # untracked (staged-new correctly absent)
git -C "$WT" diff --no-index --no-color /dev/null untracked.txt   # exit 1 = success:
# diff --git a/untracked.txt b/untracked.txt
# new file mode 100644
# --- /dev/null
# +++ b/untracked.txt
# @@ -0,0 +1 @@
# +brand new
```

Three-dot semantics verified: after `main` advanced post-branch, `merge-base` ≠ `main` tip and `git diff $MB` contained zero of main's new lines.

### fake-claude stub extension (CI, no real claude)

```bash
#!/usr/bin/env bash
# testdata/fake-claude: record argv so tests assert --resume vs --session-id.
printf '%s\n' "$@" > "${FAKE_CLAUDE_ARGS_FILE:-/dev/null}"
printf '\x1b[?2004h'
if [[ "$1" == "--resume" && -n "$FAKE_CLAUDE_RESUME_FAIL" ]]; then
  echo "No conversation found with session ID: $2"; exit 1   # mirrors verified v2.1.173 behavior
fi
echo "fake claude ready"
trap 'exit 143' TERM
sleep 300 & wait
```

Resumable-derivation tests don't need claude at all: create/remove a `<tmpHome>/.claude/projects/x/<uuid>.jsonl` fixture and point the glob root at it (make the glob root injectable for tests).

## State of the Art

| Old Assumption (Phase 4 / PITFALLS) | Verified Reality (v2.1.173, 2026-06-11) | Impact |
|---|---|---|
| `--resume` is directory-scoped (PITFALLS.md: "resume must run with cwd = the same worktree") | **Global lookup by session ID** across all project dirs; cwd only determines where the resumed agent operates | Resume in the worktree per D-55 still right, but path-encoding fragility concerns dissolve; transcript check = glob, not path computation |
| "`--resume` cwd-keyed semantics MEDIUM confidence — verify during Phase 5 planning" (STATE.md blocker) | Verified end-to-end; blocker resolved | Plans can rely on it |
| Resume might fork a new session ID (would force re-persisting) | **Same ID reused**; `--fork-session` is the opt-in fork | `tasks.claude_session_id` is stable forever; persist line unchanged |
| Phase 4 suggested "existence-check before offering Resume" via `<encoded-cwd>` path | Right idea, better mechanism: glob by UUID filename (encoding undocumented, lookup global anyway) | Pattern 1 |
| Installed claude = 2.1.170 | **Auto-updated to 2.1.173**; all load-bearing flags re-verified | Binary drifts under us between phases — see Open Questions 2 |
| D-66 anticipates "whether the agent ended cleanly" columns | Zero columns needed: D-57 fixes post-restart color, transcript glob fixes resumability | Migration 00004 not needed; phase ships schema-free |

## Open Questions

1. **Plan-mode exit-plan approval → amber dot (carried from Phase 4 OQ1 / 04-05 UAT).**
   - What we know: permission-prompt amber confirmed live; plan-mode approval goes through the same permission system; the matcher `permission_prompt|elicitation_dialog` should cover it.
   - What's unclear: the user never explicitly confirmed it during the 04-05 checkpoint.
   - Recommendation: include as an explicit item in THIS phase's human-verify checklist (it pairs naturally with live resume verification). If it misses, widen the matcher to `"*"` and filter server-side (payload carries `notification_type`).
2. **Claude binary auto-update drift.**
   - What we know: the binary moved 2.1.170 → 2.1.173 between phases without action; all Phase 5 behaviors re-verified on 2.1.173; CI never runs real claude (stub-injected).
   - What's unclear: future updates could change resume failure text/exit codes (Kangent only relies on "non-zero exit + visible error", which is robust).
   - Recommendation: keep reliance minimal (no error-text parsing anywhere); note the live-verify-at-UAT posture in the plan.
3. **Resume of a transcript claude considers corrupt/partial.**
   - What we know: missing transcript verified (exit 1); a present-but-corrupt one is untested (probe used a transplanted real transcript — resumed fine).
   - Recommendation: nothing to build — any failure mode lands in the D-56 honest-failure flow (error visible, exit non-zero, Reset offered). Not a blocker.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| claude CLI | resume spawn | ✓ | **2.1.173** (`~/.local/bin/claude`, auto-updated since Phase 4) | — (spawn/resume errors surface in UI per D-56) |
| git | diff service, merge-base, no-index | ✓ | 2.43.0 (all diff behaviors verified on this binary) | — |
| Go | backend | ✓ | 1.26.0 | — |
| Node | frontend build + shadcn CLI | ✓ | 24.4.1 | — |
| `~/.claude/projects` | transcript glob | ✓ | populated (~80 project dirs); worktree-cwd encodings observed | glob simply misses → resumable false (honest) |
| shadcn registry (collapsible) | DiffFileSection | ✓ | official registry, `web/components.json` present | hand-rolled disclosure (avoid — block is sanctioned) |
| curl | hook transport (unchanged) | ✓ | 8.5.0 | — |

**Missing dependencies with no fallback:** none.

## Sources

### Primary (HIGH confidence)
- **Empirical claude probes, this host, 2026-06-11, v2.1.173** (zero prompts, zero API usage; artifacts cleaned): `--help` flag inventory; bogus-UUID resume in trusted dir (error text + exit 1); fresh spawn with `--session-id` + inline `--settings` SessionStart hook (payload captured; no transcript created without a prompt); fabricated-transcript resume with the same overlay (same-ID reuse, `source:"resume"`, transcript appended in place); cross-cwd resume (global lookup confirmed); `~/.claude/projects` dir-name encoding inspection.
- **Empirical git probes, git 2.43.0, 2026-06-11** (linked-worktree scratch repos): merge-base vs moved base tip; `git diff <mb>` content coverage (committed/staged/unstaged/staged-new in; untracked + base-movement out); `--numstat -z` plain/binary/rename record formats; default rename detection headers; `ls-files --others --exclude-standard -z`; `diff --no-index /dev/null` text + binary outputs and exit-code-1 contract; `merge-base` failure mode (exit 128).
- **Repo inspection (2026-06-11):** `internal/api/{sessions,agents,routes,tasks}.go`, `internal/session/{manager,agent,session}.go`, `internal/store/migrations/00001–00003`, `internal/worktree/worktree.go` (ResolveBase reuse), `web/src/{api/{agents,sessions}.ts, components/{StatusDot.tsx, task/{AgentTab,TaskTabs}.tsx}, components/ui/}` (collapsible not yet installed).
- `.planning/phases/05-recovery-review/05-UI-SPEC.md` (approved contract), `05-CONTEXT.md` (D-54..D-67), `04-RESEARCH.md` + `04-05-SUMMARY.md` (carried blockers), `03-RESEARCH.md` (gitRun/ResolveBase patterns).

### Secondary (MEDIUM confidence)
- `--fork-session` help text as confirmation of default-reuse semantics (corroborated by the empirical payload — effectively HIGH in combination).
- git `diff.renames` default-on since 2.9 (corroborated empirically on 2.43.0).

### Tertiary (LOW confidence)
- `$CLAUDE_CONFIG_DIR` relocation behavior (noted as Pitfall 8; not relied upon).

## Metadata

**Confidence breakdown:**
- Resume semantics (ID reuse, overlay compat, failure mode, global lookup): HIGH — verified end-to-end on the exact installed binary.
- Reconciliation/no-migration argument: HIGH — grounded in exhaustive read of migrations + session/API code; locked by planned integration test.
- Diff plumbing: HIGH — every command/exit-code/format claim executed on the installed git, in a linked worktree.
- Unified-diff parser edge coverage (`\ No newline`, mode-only sections): MEDIUM — formats are stable and documented, but the parser needs table-test fixtures for each case.
- Frontend wiring (dotMeta null branch, TabDef disabled extension, Collapsible unmount-when-closed): MEDIUM-HIGH — read from code + Radix defaults; verify Collapsible mounting behavior at implementation time.

**Research date:** 2026-06-11
**Valid until:** ~2026-07-11 for claude-dependent claims (binary auto-updates — re-verify resume behaviors if it crosses 2.2.x before execution); git claims stable until a host git major upgrade.
