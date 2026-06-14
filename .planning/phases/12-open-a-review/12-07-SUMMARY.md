---
phase: 12-open-a-review
plan: 07
subsystem: ui
tags: [react, github-pr, review-view, tanstack-query, settings, xterm-layout, gap-closure]

# Dependency graph
requires:
  - phase: 12-open-a-review
    provides: "12-05 frontend review-view wiring (source-branched TaskPage, useOpenReview cache seed, AgentTab seed prop) whose human-verify surfaced the 5 follow-ups"
  - phase: 12-open-a-review
    provides: "12-06 backend: named-branch CheckoutPR, prWire.headRefName+commits in the open response, the pr_review_seed settings key — the data this frontend consumes"
provides:
  - "Once-per-agent-session PR-review seed guard (module-level seededSessionIds Set keyed by agent session id) — reopening/remounting never re-injects; only a fresh session seeds again"
  - "Configurable PR-review seed sourced from the pr_review_seed setting with <n>/<title> interpolated on the frontend; a blank setting means no injection"
  - "Single-line GitHub-style PR header: #<num> (clickable GitHub link) @<author> wants to merge <N> commits into <base> from <head>, rendered from the 12-06 wire fields inside the shrink-0 header block"
  - "SettingsField control='textarea' variant (commit-on-blur, Enter=newline, Escape=revert) + a PR review prompt field in the GitHub Settings section bound to pr_review_seed"
  - "GET /api/projects/{id}/pull-requests/{n} — read-only live PR detail (a shared prWireFrom builder, same gating as list/review, no worktree, no DB write) + usePullRequestDetail re-hydrating the review header on a hard reload"
affects: [12-verification, 13-cleanup-on-merge-close]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Durable once-per-session guard via a MODULE-LEVEL Set keyed by the live agent session id (not a per-mount ref): a fresh session id seeds once on first connect; remount onto the same id is already in the set → never re-injects; reconnects (same id) also never re-inject (12-07 fix #4)"
    - "Frontend placeholder interpolation of a stored settings template: pr_review_seed holds literal <n>/<title>, TaskPage replaceAll-substitutes the live PR number + title; empty/whitespace template → seed undefined → plain Start with no injection"
    - "Re-hydrate cache-only live detail with a same-key query: usePullRequestDetail re-fetches under the SAME prDetailKey useOpenReview seeds (staleTime 30s) so a normal open uses the open-time seed with no flicker and a hard reload (cache wiped) re-fetches live — header degrades to task-field fallbacks while in flight / on error"
    - "Shared wire builder (prWireFrom) so the POST /review envelope and the GET .../{n} detail stay byte-identical — re-hydration renders exactly what the open seeded"

key-files:
  created: []
  modified:
    - "web/src/components/task/AgentTab.tsx - module-level seededSessionIds Set replaces the per-mount oneShotSeededRef; seed paste guarded by agentSession.id"
    - "web/src/pages/TaskPage.tsx - seed from the pr_review_seed setting (<n>/<title> interpolated); single-line clickable merge-line header; prDetail via usePullRequestDetail (hook up top, before early returns) replacing the getQueryData cache read; dropped the now-unused ExternalLink import + prDetailKey/PRDetailWire imports"
    - "web/src/api/pullRequests.ts - PRDetailWire extended with headRefName+commits; new usePullRequestDetail(projectId, prNumber, taskId, enabled)"
    - "web/src/components/settings/SettingsField.tsx - control='textarea' variant (Textarea, commit-on-blur, Enter inserts newline, Escape reverts, min-h-[80px])"
    - "web/src/pages/SettingsPage.tsx - PR review prompt textarea in GithubSection bound to pr_review_seed (settings threaded into the section); PR_REVIEW_SEED_HELP with <n>/<title> via withMono"
    - "internal/api/pullrequests.go - GET /api/projects/{id}/pull-requests/{n} read-only detail route; extracted prWireFrom shared by POST /review and GET"
    - "internal/api/pullrequests_test.go - TestPRDetail{OkWhenLinked,ToggleOff,UnlinkedProject,BadPRNumber}"

key-decisions:
  - "Seed guard keyed by the AGENT SESSION ID at module scope (not a per-mount ref): the durability the once-per-session contract needs comes from the guard surviving unmount/remount; a fresh session (new id) is the only thing that re-seeds. (12-05 follow-up #3 closed.)"
  - "pr_review_seed is interpolated on the FRONTEND: the stored template carries literal <n>/<title>; TaskPage replaceAll-substitutes the live PR number + title. A blank/whitespace template yields seed=undefined → a plain Start with no injection (the acceptance: empty = no injection)."
  - "PR header collapsed to ONE line with the PR number as the clickable GitHub link (user refinement at the checkpoint): #<num> @<author> wants to merge <N> commits into <base> from <head>; the standalone ↗ icon row removed. Kept inside the w-full shrink-0 header block so the fix-#6 terminal-height chain is preserved."
  - "Hard-reload re-hydration via a read-only GET .../pull-requests/{n} (user-reported F5 bug): the live PR detail was cache-only (useOpenReview.onSuccess); a reload wiped it and the header degraded. usePullRequestDetail re-fetches under the same prDetailKey (staleTime 30s) so a normal open has no flicker and F5 re-hydrates. No schema change, no migration; a shared prWireFrom keeps POST/GET identical."
  - "12-05 follow-up #6 (PR-review terminal height) needed no body-layout fork: a line-by-line diff (git show 27f7491) confirmed 12-05 never branched the body layout below the header on isPR — the bash/agent tab wrappers and shared single <TaskTabs> are byte-identical to the manual view. The action was to keep the NEW merge line inside the shrink-0 header block so it can never steal flex height from the tabs."

patterns-established:
  - "Once-per-session side effect: guard a one-shot action with a module-level Set keyed by the stable session id, not a component ref, when the action must fire once across mounts/remounts but again for a new session."
  - "Settings template interpolation: store literal placeholders in the KV value, substitute live values at the use site with replaceAll; an empty value is a meaningful 'disabled' state, not a missing one."
  - "Re-hydratable cache: pair an open-time setQueryData seed with a same-key useQuery (staleTime) so the value survives both an instant route (seed) and a cold reload (re-fetch), degrading to fallbacks rather than blanking."

requirements-completed: [GHREV-01, GHREV-05]

# Metrics
duration: ~3h (incl. 3 human-verify checkpoint rounds)
completed: 2026-06-14
---

# Phase 12 Plan 07: Open-a-Review Gap Closure (Frontend) Summary

**Closed the four 12-05 frontend follow-ups — once-per-session settings-driven seed, a single-line clickable GitHub-style merge-line header, the PR-review terminal-height chain, and a PR review prompt field in Settings — plus two checkpoint-driven fixes (merge the two header rows into one clickable line; re-hydrate the PR detail on a hard reload via a new read-only GET .../pull-requests/{n}); the blocking human-verify of all five end-to-end fixes was APPROVED on 2026-06-14.**

## Status: all 4 tasks done, human-verify APPROVED

The three autonomous frontend tasks (1-3) and the two checkpoint-driven follow-ups are implemented, individually committed, and pass `cd web && npm run build` (tsc + vite) plus `go build ./...` and `go test ./internal/api/...`. The blocking human-verify checkpoint (Task 4) was run live by the user across three rounds and **approved** — all five 12-05/12-06 fixes plus the single-line clickable header and the F5 re-hydration verified working in the running app. GHREV-01/GHREV-05 are now marked Complete; the orchestrator re-runs phase verification next.

## Performance

- **Duration:** ~3h wall (autonomous tasks ~30 min; the rest is three human-verify checkpoint rounds + two refinement fixes)
- **Completed:** 2026-06-14
- **Tasks:** 4 of 4 (Task 4 = the human-verify gate, APPROVED)
- **Files modified:** 7 source (+ tracked web/dist/index.html)

## Accomplishments

- **Seed once per session, from settings (fix #4):** replaced AgentTab's per-mount `oneShotSeededRef` (which re-injected on every reopen) with a module-level `seededSessionIds` Set keyed by `agentSession.id`. TaskPage now computes the seed from the `pr_review_seed` setting (12-06) with `<n>`/`<title>` interpolated via `replaceAll`; a blank setting → `seed` undefined → a plain Start with no injection.
- **GitHub-style merge line (fix #5) → single clickable line:** `PRDetailWire` extended with `headRefName`+`commits` (the 12-06 wire); the review header renders `#<num> @<author> wants to merge <N> commits into <base> from <head>` where `#<num>` is the clickable GitHub link (the standalone ↗ row was removed at the checkpoint). The head branch is now visible (the #1 UX ask). Singular/plural "commit"/"commits" handled.
- **PR-review terminal height (fix #6):** confirmed via a line-by-line diff that 12-05 never forked the body layout below the header — the bash/agent tab wrappers (`flex h-full min-h-[320px] w-full flex-col`) and the single shared `<TaskTabs>` (`min-h-0 flex-1`) are identical to the manual view. The fix was to keep the new merge line inside the `w-full shrink-0 space-y-2` header block so it never steals flex height from the tabs; no changes to `TaskTabs.tsx`/`TerminalPane.tsx`.
- **PR review prompt Settings field (fix #7):** new `SettingsField` `control="textarea"` variant (commit-on-blur, Enter inserts a newline, Escape reverts) and a "PR review prompt" textarea in the GitHub Settings section bound to `pr_review_seed`, default-prefilled, persisting via the existing settings PUT.
- **F5 re-hydration (checkpoint bug):** added a read-only `GET /api/projects/{id}/pull-requests/{n}` (a fresh `gh pr view` → the shared `prWireFrom`, same toggle+link gating as list/review, no worktree, no DB write) and `usePullRequestDetail`, which re-fetches under the SAME `prDetailKey` the open seeds (`staleTime 30s`). A normal open uses the open-time seed (no flicker); a hard reload re-fetches so the header link/author/from-branch + the seed's live title re-hydrate instead of going blank.

## Task Commits

1. **Task 1: once-per-session settings-driven seed + GitHub merge line** — `28e6511` (feat)
2. **Task 3: PR review prompt field in GitHub Settings** — `5f693dd` (feat)
3. **Refinement (checkpoint): merge PR header into one clickable line** — `14c7f99` (fix)
4. **Bug fix (checkpoint): hydrate PR detail on reload via GET pull-requests/{n}** — `6c34886` (fix)

_Task 2 (terminal-height) produced no separate diff: the height chain was already structurally identical and correct; the only change it required — keeping the merge line inside `shrink-0` — landed in `28e6511`._

**Plan metadata:** this SUMMARY + STATE.md + ROADMAP.md + REQUIREMENTS.md + the Task-4 approval mark in 12-07-PLAN.md (docs commit).

## Files Created/Modified

- `web/src/components/task/AgentTab.tsx` — module-level `seededSessionIds` Set; seed paste guarded by `agentSession.id` (per-mount `oneShotSeededRef` removed).
- `web/src/pages/TaskPage.tsx` — seed from `pr_review_seed` with `<n>`/`<title>` interpolation; single-line clickable merge-line header inside the `shrink-0` block; `prDetail` via `usePullRequestDetail` (hook called up top, before the early returns) replacing the `getQueryData` cache read; dropped now-unused `ExternalLink` / `prDetailKey` / `PRDetailWire` imports.
- `web/src/api/pullRequests.ts` — `PRDetailWire` + `headRefName`/`commits`; new `usePullRequestDetail`.
- `web/src/components/settings/SettingsField.tsx` — `control="textarea"` variant.
- `web/src/pages/SettingsPage.tsx` — PR review prompt textarea in `GithubSection` bound to `pr_review_seed` (settings threaded in); `PR_REVIEW_SEED_HELP`.
- `internal/api/pullrequests.go` — `GET .../pull-requests/{n}` read-only detail route; extracted `prWireFrom` shared by POST `/review` and GET.
- `internal/api/pullrequests_test.go` — `TestPRDetail{OkWhenLinked,ToggleOff,UnlinkedProject,BadPRNumber}`.

## Decisions Made

- **Seed guard keyed by agent session id at module scope** (not a per-mount ref) — durability across remounts is exactly what the once-per-session contract needs; only a fresh session re-seeds.
- **pr_review_seed interpolated on the frontend** — stored literal `<n>`/`<title>`, substituted at the use site; blank = no injection.
- **One-line clickable header** (user refinement) — `#<num>` is the GitHub link; the ↗ icon row removed; kept inside `shrink-0` to preserve fix-#6.
- **Read-only GET for F5 re-hydration** (user-reported bug) — re-fetch under the open-time key, no schema change/migration; shared `prWireFrom` keeps POST/GET identical.
- **Fix #6 needed no body-layout fork** — verified by `git show 27f7491`; the action was confining the new meta to `shrink-0`.

## Deviations from Plan

### Checkpoint-driven follow-ups (user-requested at the human-verify gate)

**1. [Rule 1 - Bug] Merge the two PR-header rows into one clickable line**
- **Found during:** Task 4 (human-verify, round 2)
- **Issue:** The user wanted the merge line and the `#num · @author · base · ↗` row collapsed into a single line with the PR number as the clickable GitHub link.
- **Fix:** Removed the second meta row; prepended an `#{pr_number}` anchor (`href=prDetail.url`) to the merge line; dropped the now-unused `ExternalLink` import. Kept the container inside the `shrink-0` header block (no fix-#6 regression).
- **Files modified:** `web/src/pages/TaskPage.tsx`
- **Verification:** `npm run build` (tsc + vite) green; grep-confirmed single merge line, no middot row, `<TaskTabs>` count 1, header wrapper intact.
- **Committed in:** `14c7f99`

**2. [Rule 1 - Bug] PR header blank on F5 (cache-only live detail)**
- **Found during:** Task 4 (human-verify, round 3)
- **Issue:** On a hard reload of a PR review the header link was dead, the author missing, and the "from" branch empty — the live PR detail was only in the TanStack cache (seeded by `useOpenReview.onSuccess`), which a reload wipes; the task row has only `pr_number`+`pr_base_ref`.
- **Fix:** Added a read-only `GET /api/projects/{id}/pull-requests/{n}` (live `gh pr view` → shared `prWireFrom`, same gating as list/review, no worktree/DB write) + `usePullRequestDetail` re-fetching under the same `prDetailKey` (staleTime 30s). TaskPage reads `prDetail` from the query (hook moved up top for rules-of-hooks).
- **Files modified:** `internal/api/pullrequests.go`, `internal/api/pullrequests_test.go`, `web/src/api/pullRequests.ts`, `web/src/pages/TaskPage.tsx`
- **Verification:** `go build ./...` + `go test ./internal/api/...` green (4 new tests, forced uncached); `npm run build` green; live route smoke test resolves (502 on a non-PR id, never 404).
- **Committed in:** `6c34886`

---

**Total deviations:** 2 checkpoint-driven fixes (both Rule 1 bugs surfaced by the human-verify gate — exactly what the gate is for).
**Impact on plan:** Both essential for the user-facing correctness the phase requires (a usable single-line header; a header that survives reload). No scope creep — both stayed within the review view + the existing PR/settings surfaces; no new deps, no schema change, no migration.

## Issues Encountered

- The pre-existing chunk-size warning from `vite build` (the single >500 kB bundle) is unchanged and out of scope (deferred — code-splitting is a future cleanup).

## User Setup Required

None - no external service configuration required (the GET detail endpoint reuses the host's already-authenticated `gh`).

## Next Phase Readiness

- **All five 12-05 follow-ups are closed** across 12-06 (backend) + 12-07 (frontend): named-branch checkout + GHREV-05 safety, the once-per-session settings-driven seed, the single-line clickable merge-line header, the PR-review terminal height, the Settings prompt field — plus F5 re-hydration. The human-verify gate is APPROVED.
- **GHREV-01 / GHREV-05 are marked Complete** (the gap fixes landed and were verified by the human gate). The orchestrator runs the gsd-verifier next to re-verify the whole phase; do NOT treat this SUMMARY as the phase verification.
- **No new Go dependency, no new npm dependency, no new DB migration.**

## Known Stubs

None — no stub patterns introduced. (The single "not available" string in `SettingsPage.tsx` is a pre-existing gh-availability doc comment, not a UI stub.)

## Self-Check: PASSED

- All 7 modified source files exist on disk + the new `12-07-SUMMARY.md`.
- Commits `28e6511` (Task 1), `5f693dd` (Task 3), `14c7f99` (header merge), `6c34886` (F5 re-hydration) present in git history.
- `cd web && npm run build` (tsc + vite) green; `go build ./...` green; `go test ./internal/api/...` green (incl. the 4 new `TestPRDetail*`, forced uncached).
- Human-verify checkpoint (Task 4): APPROVED 2026-06-14 — all 5 fixes + single-line clickable header + F5 re-hydration verified live. No stubs introduced.

---
*Phase: 12-open-a-review*
*Completed (frontend gap closure): 2026-06-14 — human-verify APPROVED; phase re-verification by the orchestrator pending*
