# Phase 15: Repo-First Creation Flow - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-15
**Phase:** 15-repo-first-creation-flow
**Areas discussed:** Mode UI, Description auto-fill, Clone UX, Validation timing, Description fetch mechanism

---

## Mode selection UI (repo vs folder when integration on)

| Option | Description | Selected |
|--------|-------------|----------|
| Segmented toggle, repo default | Two-option switch ("GitHub repo" \| "Local folder") at the top, repo default; swaps the input below. | ✓ |
| Repo field + 'use a local folder' link | Repo field by default with a small link revealing the folder input; folder visually secondary. | |

**User's choice:** Segmented toggle, repo default. → D-01/D-02/D-03.

---

## Description auto-fill (RPROJ-02 — backend currently captures no description)

| Option | Description | Selected |
|--------|-------------|----------|
| Fetch GitHub description, prefill, persist | Backend captures the GitHub description at create; dialog shows it prefilled + editable. | |
| Name-only auto-fill, description blank | Prefill only the name; leave description empty for later. | |

**User's choice (first pass):** Fetch GitHub description, prefill, persist — THEN refined by the follow-up below.

### Follow-up: how does the frontend get the description before create?

| Option | Description | Selected |
|--------|-------------|----------|
| Backend captures it at create (no preview) | No prefilled description field in the dialog; repo-first create persists the GitHub description server-side so it appears editable in Project settings after. | ✓ |
| New lookup endpoint, prefill in form | A GET endpoint the dialog calls to prefill an editable description field before create. | |

**User's choice:** Backend captures it at create (no preview). → D-05. The Add dialog has no description field; description is persisted from `gh repo view --json description` at create, editable later in Project settings. Requires a thin `createByRepo` backend touch, no new endpoint.

---

## Clone-in-progress + failure UX (CKOUT-04)

| Option | Description | Selected |
|--------|-------------|----------|
| Blocking spinner + inline error | Disable inputs, spinner "Cloning <owner/name>…", dialog open; on failure inline destructive alert, no half-created project, values preserved. No cancel (CKUX-01 deferred). | ✓ |
| Close immediately, toast on result | Close on submit, surface success/failure as a toast. (No toast system exists today.) | |

**User's choice:** Blocking spinner + inline error. → D-06/D-07/D-08.

---

## Validation timing + error display

| Option | Description | Selected |
|--------|-------------|----------|
| On submit (reuse settings pattern) | Backend gh-check on submit; errors in the existing destructive-alert-under-the-field style; dialog stays open, values preserved. | ✓ |
| As-you-type / on-blur prevalidation | Debounced gh validation while typing/on blur; more immediate but adds gh calls + new state, inconsistent with existing UX. | |

**User's choice:** On submit, reuse the ProjectSettingsDialog pattern. → D-09.

---

## Claude's Discretion
- Segmented-toggle component choice (shadcn Tabs / Button group / toggle) — planner picks, no new dep.
- Name-prefill trigger (debounce vs derive-on-valid-ref); never clobber a user edit (mirror `repoEdited`).
- Whether description capture extends `ValidateRepo`'s `gh repo view` or adds a small read — best-effort either way.
- Repo input + toggle label copy.

## Deferred Ideas
- Live clone progress + cancel (CKUX-01); dedicated repo-info lookup endpoint (rejected for v1.4 in favor of D-05); non-default base branch / shallow clone (CKMNT-02/03).
