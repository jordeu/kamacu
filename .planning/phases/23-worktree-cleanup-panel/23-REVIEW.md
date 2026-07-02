---
phase: 23-worktree-cleanup-panel
reviewed: 2026-07-02T00:00:00Z
depth: standard
files_reviewed: 14
files_reviewed_list:
  - cmd/kamacu/main.go
  - internal/api/cleanup.go
  - internal/api/cleanup_test.go
  - internal/api/cleanuppanel.go
  - internal/api/cleanuppanel_test.go
  - internal/worktree/worktree.go
  - internal/worktree/worktree_test.go
  - web/src/api/worktreeCleanup.ts
  - web/src/components/settings/CleanEligibleDialog.tsx
  - web/src/components/settings/ForceRemoveDialog.tsx
  - web/src/components/settings/WorktreeCleanupSection.tsx
  - web/src/components/settings/WorktreeRow.tsx
  - web/src/components/ui/badge.tsx
  - web/src/pages/SettingsPage.tsx
findings:
  critical: 1
  warning: 6
  info: 5
  total: 12
status: partially_resolved
fixed: [CR-01, WR-01, WR-02]
open_deferred: [WR-03, WR-04, WR-05, WR-06, IN-01, IN-02, IN-03, IN-04, IN-05]
---

# Phase 23: Code Review Report

**Reviewed:** 2026-07-02T00:00:00Z
**Depth:** standard
**Files Reviewed:** 14
**Status:** issues_found

## Summary

Reviewed the worktree-cleanup panel: a localhost force-remove endpoint for real git worktrees, plus the bulk clean-eligible / clear-pointer flows and the React UI. The core removal path (`CleanupWorktreeGated`) is a clean, well-tested extraction, the D-01 blocked-outcome path is correctly implemented and covered by tests (200 blocked banner, no `--force` retry, DB row intact), the never-force invariant of the bulk path holds, and the `sudo rm -rf` hint is copy-only. Test coverage is genuinely strong.

The primary concern is that the panel's `POST /api/worktrees/remove` handler takes `repo` and `path` **verbatim from the request body** and hands them straight to `git worktree remove` with no verification that the pair is a registered, enumerated worktree — a deliberate departure from every other removal caller in the codebase, which derive the path from the DB. On this no-auth localhost app that is not a privilege escalation, but it removes the last correctness guardrail and invites deregistering/removing the wrong tree from a stale client snapshot. A secondary cluster of robustness issues: a transient `git worktree list` failure misclassifies live worktrees as "stale" (offering a metadata-destroying Clear-pointer), and the `github_pr` eligibility path calls `gh pr view` twice per row, contradicting an explicit in-code invariant and risking inconsistent eligible/displayed state.

## Resolution (2026-07-02)

The three correctness findings on the destructive remove path were fixed TDD-first (6 commits, 6 new tests, all green):

- **CR-01 (fixed)** — `POST /api/worktrees/remove` now validates `(repo, path)` against `enumerate()` via a new `findEnumerated` helper (404 on no match) and derives the null-columns `task_id` from the **matched** worktree's task, never `req.TaskID`. Commits `9134db5` (RED) / `1729cbe` (fix). Tests: `TestWorktreeCleanupRemoveRejectsUnenumeratedPath`, `TestWorktreeCleanupRemoveNullsMatchedTaskNotBodyTaskID`, `TestWorktreeCleanupRemoveOrphanTaskIDZero`.
- **WR-01 (fixed)** — `enumerate` tracks `listOK` and no longer fabricates "stale" rows when `git worktree list` fails. Commits `f8c0a83` / `ecb3d73`. Test: `TestWorktreeCleanupListNoStaleOnFailedList`.
- **WR-02 (fixed, closes IN-02)** — PR state is resolved once per row (`enumWorktree.prState`/`prStateOK` via `resolvePRState`); `eligibilityReason`/`buildRow`/`computeEligible` read the stored value → exactly one `gh` call per PR row, no eligible-vs-displayed disagreement. Commits `a6de21d` / `63ec46c`. Tests: `TestWorktreeCleanupListSinglePRStateCall`, `TestWorktreeCleanupCleanEligibleSinglePRStateCall`.

**Deferred (open):** WR-03 (unpushed-base under-count for non-PR referenced tasks), WR-04 (client-honored force override), WR-05 (dead-end confirm state in CleanEligibleDialog), WR-06 (`sudo` hint truncation on quote-containing paths), and IN-01/03/04/05 — tracked for a future `/gsd:code-review 23 --fix` pass; none block phase completion.

## Critical Issues

### CR-01: `remove` handler force-removes an arbitrary, unvalidated `repo` + `path` from the request body

**File:** `internal/api/cleanuppanel.go:518-564` (with `internal/api/cleanup.go:147` → `internal/worktree/worktree.go:457`)
**Issue:**
The panel's per-item remove reads `repo` and `path` directly from the JSON body and passes them, unvalidated, into `CleanupWorktreeGated`, which runs `git -C <repo> worktree remove <path>` and then (for `task_id>0`) nulls that task's DB columns:

```go
if strings.TrimSpace(req.Repo) == "" || strings.TrimSpace(req.Path) == "" {
    writeError(w, http.StatusBadRequest, "repo and path are required")
    return
}
// ... no check that (repo, path) is an enumerated/registered worktree ...
removed, reason, err := CleanupWorktreeGated(
    ctx, h.db, h.wt, h.mgr, h.tmuxClient, live,
    req.TaskID, req.Repo, req.Path, count, req.StopSessions, req.Force)
```

Every other caller of this path is scoped: `worktreeHandlers.remove` (`internal/api/worktrees.go:230-243`) looks up the task and uses `*t.WorktreePath` from the DB; the reaper enumerates before removing. This handler alone trusts the client for both the repo and the target path. Because `force=true` is the documented default from the UI, the dirty/session gates are bypassed as well, so nothing server-side constrains *which* directory is destroyed. Additional consequences beyond the intended target:
- `req.TaskID` is also client-supplied and independent of `req.Path`. A body with a valid live `task_id` but a different `path` removes one worktree and then nulls a *different, unrelated* task's `branch`/`worktree_path`/`worktree_error` columns (`cleanup.go:162-166`) — silent data-model corruption of a task whose worktree still exists on disk.
- On this app's own threat model (localhost, no auth, "serves a shell") this is not privilege escalation, but it is the removal of the last correctness guardrail on a destructive filesystem operation and diverges from the audited pattern the phase claims to reuse.

**Fix:** Re-derive (or validate) the target against server truth instead of trusting the body. Either resolve the path from the DB via `task_id` (as `worktreeHandlers.remove` does), or, for orphan (`task_id==0`) rows, require that `(repo, path)` appears in `enumerate()`'s git-listed set for that project before removing, and require the `task_id`↔`path` pairing to match a real row:

```go
// After decoding req, confirm the pair is a real, enumerated worktree.
groups, err := h.enumerate(ctx)
if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
ew, ok := findEnumerated(groups, req.Repo, req.Path) // clean-path match within the project
if !ok {
    writeError(w, http.StatusNotFound, "no such worktree")
    return
}
// Derive task_id from ew.task (ignore the client's task_id for the null-columns UPDATE).
```

## Warnings

### WR-01: transient `git worktree list` failure misclassifies live worktrees as "stale", enabling a metadata-destroying Clear-pointer

**File:** `internal/api/cleanuppanel.go:182-228`
**Issue:** In `enumerate`, when `h.wt.List` fails for a project the code logs and continues with `gitByPath` empty:

```go
entries, lerr := h.wt.List(ctx, p.repoPath)
if lerr != nil {
    slog.Warn("worktree list failed for project", ...)
}
// gitByPath stays empty ...
for i := range dbTasks {
    if _, inGit := gitByPath[t.cleanPath]; !inGit {
        grp.worktrees = append(grp.worktrees, enumWorktree{classification: "stale", ...})
    }
}
```

A transient failure (a momentary git error, a lock contention, a permissions blip, a `ctx` deadline mid-scan) therefore marks **every** DB task in that project as a "stale pointer", even ones whose worktree directory is present and healthy. The UI then offers "Clear pointer" (`WorktreeRow.tsx:126-133`), which nulls `branch` + `worktree_path` and prunes — severing a live task's worktree link based on a false signal. Files aren't deleted and the branch survives in git, so it is recoverable, but it is silent metadata corruption driven by a degraded read.
**Fix:** Distinguish "git list succeeded and the path is absent" (true stale) from "git list failed" (unknown). On `lerr != nil`, either skip emitting stale rows for that project, or tag them "unknown" and suppress the Clear-pointer action:

```go
listOK := lerr == nil
// ...
if _, inGit := gitByPath[t.cleanPath]; !inGit {
    if !listOK { continue } // don't fabricate stale rows from a failed list
    grp.worktrees = append(grp.worktrees, enumWorktree{classification: "stale", ...})
}
```

### WR-02: `github_pr` rows call `gh pr view` twice, contradicting the stated "never double-call" invariant and risking inconsistent state

**File:** `internal/api/cleanuppanel.go:346-353, 445-452, 676-697`
**Issue:** The list loop comment asserts hidden rows "never double-call gh's PRState", but a *shown* `github_pr` referenced row calls `PRState` twice: once in `isCleanupCandidate` → `eligibilityReason` (line 685) and again in `buildRow` (line 446). Each call is a separate `gh pr view` subprocess/network round-trip. Beyond the wasted call, the two lookups are not atomic: if the second call fails or returns a different value (rate-limit, transient error, a merge landing between calls), the row can be shown as an eligibility candidate while `pr_state` renders null, or vice-versa — an inconsistent snapshot the user acts on.
**Fix:** Resolve PR state once per row and thread it through. Have `enumerate`/`buildRow` compute `state` a single time and pass it to both the eligibility decision and the display field, e.g. add a resolved `prState string` to `enumWorktree` populated once, and make `eligibilityReason`/`buildRow` read it instead of each calling `h.pr.PRState`.

### WR-03: `unpushedBase` returns a wrong base for a **non-github_pr referenced** task, silently under-counting unpushed commits

**File:** `internal/api/cleanuppanel.go:480-499`
**Issue:** For a referenced *manual* (non-`github_pr`) task, `unpushedBase` falls straight through to `h.wt.ResolveBase(ctx, repo)`, which returns the repo's **default branch** (`main`/`master`/origin/HEAD). But a manual task's worktree was branched from whatever `ResolveBase` returned *at creation time*, which for a task created off a non-default branch (or after the default advanced) is not today's default tip. `git rev-list --count <default>..HEAD` then measures commits relative to the wrong base, so the "unpushed" count — used both for the display chip and, critically, for the bulk `passesAllGates` unpushed==0 gate (`cleanuppanel.go:729-732`) — can under-report. An under-report means `passesAllGates` can pass a worktree that actually carries commits not present on its real upstream, and bulk clean removes it (never forced, but the commits are only on that branch, which *is* kept — so recoverable via the branch). Direction of the error (under-count on a non-default-branched task) is the unsafe one for the eligibility gate.
**Fix:** For referenced rows prefer the task's tracked upstream / recorded base when available (e.g. `@{upstream}` of the branch, or a stored base ref) before falling back to `ResolveBase`. At minimum verify the resolved base is an ancestor relationship that makes the count meaningful, or treat an ambiguous base as unresolvable (`ok=false`) so the row degrades to `unpushed:null` and is conservatively ineligible rather than silently under-counted.

### WR-04: `remove` accepts and honors client-supplied `force`/`stop_sessions` with no server-side authorization of the override

**File:** `internal/api/cleanuppanel.go:518-545`
**Issue:** The handler reads `Force` and `StopSessions` from the body and passes them through. The doc comment says "the client always sends force=true", and the gates are re-checked, but there is no server constraint that force is only used where appropriate — combined with CR-01's unvalidated path, a single request can force-destroy a dirty worktree with running sessions at an arbitrary path. Even absent CR-01, honoring an unconditional client `force=true` means the D-33 dirty gate is purely advisory for this endpoint. The reference reaper deliberately passes `force=false` (`cleanup.go` doc, `main.go:242`); the panel's per-item override is the one place a caller can demand force, so it deserves an explicit server-side justification (e.g. requiring the row to have been surfaced as a candidate).
**Fix:** Gate the force override behind server-verified preconditions: only allow `force=true` for a `(repo, path)` that `enumerate` currently classifies as removable, and reject a force request for a path not in the enumerated set (folds into the CR-01 fix).

### WR-05: `CleanEligibleDialog` confirm run has no terminal/auto-close state; success leaves a permanently-disabled dialog

**File:** `web/src/components/settings/CleanEligibleDialog.tsx:141-162`
**Issue:** After a successful confirm run, `summary` is set and the action becomes `disabled={... || summary !== null}` forever, with no auto-close and no path back to a fresh preview. The only exit is the Cancel button, whose label still reads "Cancel" after a completed destructive action. If the user reopens, a fresh mount re-previews (fine), but the post-run dialog state is a dead end that reads as if the action is still pending/blocked. Additionally, `loadingPreview = clean.isPending && summary === null && items.length === 0` re-enters the skeleton state during the confirm run whenever the eligible set was small enough to have already been consumed — a confusing flicker.
**Fix:** After the confirm settles, either auto-close (`onOpenChange(false)`) or replace the footer with a single "Done"/"Close" affordance and stop showing the preview skeleton once a run has started (`const showedPreview = items.length > 0 || previewSettled`).

### WR-06: `parseCouldNotOpenDir` can capture a truncated path when git's message quotes a path containing a single quote

**File:** `internal/api/cleanup.go:66-79`
**Issue:** `parseCouldNotOpenDir` extracts the directory between `could not open directory '` and the next `'`. Git single-quotes the path in that warning; a directory name legitimately containing a `'` character would be truncated at the embedded quote, producing a `BlockedError.Path` that points at a shorter/incorrect path. That path is then rendered into the copyable `sudo rm -rf <path>` hint. A user copy-pasting a truncated `sudo rm -rf` target is a real footgun (removes the wrong, shorter path). The fallback to the worktree path exists, but only when the marker is entirely absent, not when it's present-but-truncated.
**Fix:** Prefer the passed worktree `fallbackPath` (already trusted, server-derived) over the parsed value for the destructive hint, or only trust the parsed dir when it is a descendant of the worktree path:

```go
if dir := parseCouldNotOpenDir(msg); dir != "" && strings.HasPrefix(filepath.Clean(dir), filepath.Clean(fallbackPath)) {
    path = dir
}
```

## Info

### IN-01: `prSuffix` is guarded twice — redundant null check

**File:** `web/src/components/settings/WorktreeRow.tsx:119`
**Issue:** `{row.pr_state !== null && prSuffix(row.pr_state)}` — `prSuffix` already returns `""` for a falsy state (`return state ? ...`), so the outer `!== null` guard is dead. Harmless but signals the two were written to different assumptions.
**Fix:** Drop the guard: `{prSuffix(row.pr_state)}`.

### IN-02: Misleading in-code invariant comment ("never double-call gh's PRState")

**File:** `internal/api/cleanuppanel.go:349`
**Issue:** The comment claims shown rows do no needless gh work and never double-call `PRState`, which is false for `github_pr` rows (see WR-02). Comments asserting invariants the code violates are worse than no comment — they mislead the next reader.
**Fix:** Correct or remove the claim once WR-02 is resolved.

### IN-03: `badge.tsx` defines `default`, `ghost`, and `link` variants unused by this phase

**File:** `web/src/components/ui/badge.tsx:11-22`
**Issue:** The cleanup panel uses only `destructive`, `secondary`, and `outline`. `default`, `ghost`, and `link` are defined but unreferenced here. If this badge component was added/modified for this phase, the extra variants are dead surface; if it is shared shadcn scaffolding, this is expected. Low signal — noted for completeness.
**Fix:** If badge is phase-local, trim unused variants; if shared, ignore.

### IN-04: `remove`/`clearPointer` decode ignore a non-EOF trailing error only, but silently accept extra JSON

**File:** `internal/api/cleanuppanel.go:526-529, 746-749`
**Issue:** `json.NewDecoder(r.Body).Decode(&req)` accepts a body with unknown/extra fields silently (no `DisallowUnknownFields`). Not a security issue given the parameterized queries, but a typo'd field name in a client request is swallowed rather than surfaced, which can mask contract drift with the frontend.
**Fix:** Optional: use a decoder with `DisallowUnknownFields()` for these small, fixed request shapes to catch client/server contract drift early.

### IN-05: `CleanEligibleBody` uses `${item.repo}:${item.path}` as React key; not guaranteed unique across projects sharing a repo

**File:** `web/src/components/settings/CleanEligibleDialog.tsx:129` (and `WorktreeCleanupSection.tsx:106`, `WorktreeRow.tsx` list)
**Issue:** Rows are keyed `${repo}:${path}`. Within one repo the path is unique, so collisions are unlikely, but if two projects point `repo_path` at the same checkout (the app allows multiple projects), the same `(repo, path)` could appear under two project groups and collide as a React key. Minor rendering-stability risk, not a correctness bug for the data itself.
**Fix:** Include `project_id`/`task_id` in the key where available: `${project_id}:${row.task_id}:${row.path}`.

---

_Reviewed: 2026-07-02T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
