# Phase 14: Managed Checkout Foundations - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-14
**Phase:** 14-managed-checkout-foundations
**Areas discussed:** Provisioning model, Fetch-fail behavior, Gated delete scope, Reattach rules

---

## Gray Area Selection

| Option | Description | Selected |
|--------|-------------|----------|
| Provisioning model | Sync clone-then-create vs async background clone | ✓ |
| Fetch-fail behavior | On a failed pre-task fetch: proceed from local vs block | ✓ |
| Gated delete scope | Block all-or-nothing vs partial remove | ✓ |
| Reattach rules | Reuse-on-match/error-on-mismatch vs always re-clone | ✓ |

**User's choice:** All four areas.

---

## Provisioning model

| Option | Description | Selected |
|--------|-------------|----------|
| Sync clone-then-create | Clone first; create the project row only after clone succeeds. Atomic — failed clone leaves no project + no dir (satisfies CKOUT-04). Add call blocks until ready. | ✓ |
| Async background clone | Create the project immediately in a 'provisioning' state, clone in background, surface progress/failure after. More moving parts; live progress deferred (CKUX-01). | |

**User's choice:** Sync clone-then-create.
**Notes:** Atomicity is the deciding factor — directly satisfies CKOUT-04 "no half-created project" with no extra state machine. → D-01.

---

## Fetch-fail behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Proceed from local base | Best-effort freshness: ignore fetch error, branch from local default tip; task creation works offline. Degrade-don't-break. | ✓ |
| Block task creation | Treat fetch as required; fail task creation when origin is unreachable. | |

**User's choice:** Proceed from local base.
**Notes:** Consistent with the app-wide degrade-don't-break philosophy; the fetch is freshness, not a gate. Managed-checkout-only relaxation of worktree.ResolveBase's D-24 local-only rule (folder projects unchanged). → D-04/D-05.

---

## Gated delete scope

| Option | Description | Selected |
|--------|-------------|----------|
| Block all-or-nothing | Refuse delete with a clear reason listing blockers; remove nothing until everything clean + idle. Protects unpushed work (removing the clone discards local branches). Matches conservative worktree-cleanup gates. | ✓ |
| Remove clean, keep the rest | Remove clean/idle worktrees, keep dirty/busy ones and the clone, leaving a partial project. More state. | |

**User's choice:** Block all-or-nothing.
**Notes:** The unpushed gate is the safety net against losing local-only commits when the clone dir is removed. Reuses CleanupWorktreeGated's gate set. → D-07/D-08/D-09.

---

## Reattach rules

| Option | Description | Selected |
|--------|-------------|----------|
| Reuse if origin matches, else error | Existing dir whose origin canonicalizes to the same owner/name → reattach/reuse (no re-clone); origin mismatch → error, don't clobber. | ✓ |
| Always re-clone fresh | Remove/replace the existing dir and clone anew — riskier, could discard local work. | |

**User's choice:** Reuse if origin matches, else error.
**Notes:** Origin match = existing dir's origin remote canonicalized via github.ParseRepoRef equals requested owner/name. → D-10/D-11.

---

## Claude's Discretion

- Repos base location — hardcoded `~/.kangent/repos/` for v1.4 (no new setting); planner may mirror `worktree_base` with a `repos_base` setting only if trivial.
- Schema-marker column shape (managed vs folder) — planner's call (migration 00008).
- Package split for the clone verb + provisioning/reattach/gated-delete orchestration — planner's call.
- Whether Phase 14's primitive or Phase 15's form sets the managed project's `github_repo` link.

## Deferred Ideas

- CKMNT-01 on-demand sync, CKMNT-02 non-default base branch, CKMNT-03 shallow clone, CKUX-01 live clone progress — all REQUIREMENTS.md Future.
- Repo-first Add-project UI / auto-fill / inline clone-failure surfacing — Phase 15 (next phase, not deferred).
