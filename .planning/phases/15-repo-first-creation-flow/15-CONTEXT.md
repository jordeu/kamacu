# Phase 15: Repo-First Creation Flow - Context

**Gathered:** 2026-06-15
**Status:** Ready for planning

<domain>
## Phase Boundary

The **Add-project UI** (and a thin backend touch) that puts the repo-first creation flow on top of Phase 14's managed-checkout primitive. When GitHub integration is on, the "Add project" dialog defaults to entering a GitHub repo (`owner/name`); the project name prefills from the repo (editable), the GitHub link + description are captured automatically, and a folder path is the optional alternative. When integration is off, the dialog is folder-only exactly as before v1.4. A failed clone surfaces inline in the dialog leaving no half-created project.

Covers **RPROJ-01, RPROJ-02, RPROJ-03, RPROJ-04, CKOUT-04**.

**Builds on Phase 14 (already shipped):** `POST /api/projects` already accepts `{repo: "owner/name", name?}` (repo-first → `createByRepo`: gh-validate → `gh repo clone` → atomic INSERT with `managed=1` + `github_repo`) OR `{repo_path, name?}` (folder, unchanged). The atomicity / no-orphan / reattach guarantees are backend-proven — Phase 15 only drives them from the UI.

**NOT in this phase:** the managed-checkout backend mechanics (Phase 14 — clone, marker, fetch, gated delete). Phase 15 adds only the creation UI + the small description-capture backend touch below.
</domain>

<decisions>
## Implementation Decisions

### Mode selection (RPROJ-01, RPROJ-03, RPROJ-04)
- **D-01:** When integration is on, the Add-project dialog shows a **segmented two-option toggle at the top — "GitHub repo" | "Local folder" — defaulting to "GitHub repo"**. Switching swaps the input below (repo `owner/name` input ↔ the existing folder-path input). Repo-first is the default selection.
- **D-02:** When integration is **off**, the dialog renders **folder-only** with **no toggle and no repo field** — byte-for-byte the current `AddProjectDialog` (repo_path + name). The repo-first UI must not appear at all. Gate on `useSettings().github_integration?.value === "on"` (which already implies `gh` is present per Phase 10's gh-gate; no separate `gh_available` check needed, but it's fine to also read `useGithubStatus()` if cleaner).
- **D-03:** Folder mode preserves today's exact behavior and request shape (`{ repo_path, name? }` → unchanged `validateRepoPath` path). Existing folder-based projects and the folder-create flow are untouched (RPROJ-04 backward-compat).

### Name + description auto-fill (RPROJ-02)
- **D-04:** In repo mode, the **project name field prefills from the `name` segment of the typed `owner/name`** (e.g. `seqeralabs/nf-aggregate` → `nf-aggregate`) and stays **editable before create**. If the user clears it, the backend already defaults the name to the repo name (`createByRepo` does `filepath.Base(dest)` when `name` is empty), so a blank name is safe.
- **D-05:** **Description is captured server-side at create time — NOT shown/prefilled in the Add dialog.** The repo-first create path persists the GitHub repo's description (from `gh repo view --json description`) into `projects.description`, so it appears (editable) in **Project settings** afterward. This is the chosen, simpler reading of RPROJ-02's "auto-fills … description": persisted automatically, editable later, with no description field or preview in the Add dialog. **This requires a thin Phase-15 backend touch** to `createByRepo` (capture + persist the description); no new lookup endpoint.
  - Implementation latitude (planner): `github.ValidateRepo` already shells `gh repo view`; the description can ride that same call (extend it to also return description) OR a small dedicated read. Keep it best-effort/degrade-don't-break — a missing/empty description just persists `""` (the existing default), never blocks create.

### Clone-in-progress + failure UX (CKOUT-04)
- **D-06:** On submit in repo mode: **disable the inputs, show a blocking spinner with "Cloning <owner/name>…", keep the dialog open** until the synchronous create returns. (The clone can take a while for large repos; sync is the Phase-14 contract.)
- **D-07:** On failure, show the **server message inline as a destructive alert** (reusing `ProjectSettingsDialog`'s destructive-alert-under-the-field pattern), **keep the dialog open**, and **preserve the field values** so the user can correct and retry. Because Phase 14 guarantees atomicity, a failure means **no orphan project row and no partial directory** — the UI states this truthfully (no "half-created" project to clean up). No cancel button (live cancel is deferred — CKUX-01).
- **D-08:** On success, close the dialog and navigate to the new project (current `AddProjectDialog` behavior: `navigate(/projects/{id})`).

### Validation timing + error display
- **D-09:** **Validate on submit**, reusing the existing backend gh-check — **no as-you-type / on-blur gh calls**. Syntactic-invalid (400), gh-unverifiable (400), clone-failure, and "already added" (409) all render in the **same destructive-alert style** the repo field already uses, dialog stays open, values preserved. Mirror `AddProjectDialog`/`ProjectSettingsDialog`'s `ApiError` → `sentenceCase` handling.

### Claude's Discretion
- The **segmented-toggle component** — reuse an existing shadcn primitive (Tabs / a two-Button group / a simple toggle); planner picks what's consistent with the current `ui/` set. No new dependency.
- Exact **name-prefill trigger** (debounced as-you-type vs derive on a valid-looking `owner/name`) — keep it simple; never clobber a name the user has edited (mirror `ProjectSettingsDialog`'s `repoEdited` guard).
- Whether the description capture extends `ValidateRepo`'s existing `gh repo view` call or adds a small read — planner decides; keep it best-effort.
- Empty/placeholder copy for the repo input (`owner/name`) and the toggle labels — match existing UI-SPEC tone.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Milestone scope & locked decisions
- `.planning/REQUIREMENTS.md` — RPROJ-01/02/03/04 + CKOUT-04 (this phase's requirements); CKMNT/CKUX Future; Out of Scope rows
- `.planning/ROADMAP.md` §"Phase 15: Repo-First Creation Flow" — goal, success criteria, depends-on Phase 14
- `.planning/phases/14-managed-checkout-foundations/14-CONTEXT.md` — Phase 14 locked decisions D-01..D-11 (the primitive this UI drives; esp. D-01 atomicity, D-10 reattach)
- `.planning/phases/14-managed-checkout-foundations/14-VERIFICATION.md` — the proven Phase-14 contract (what the UI can rely on)
- `.planning/PROJECT.md` — v1.4 milestone section + Key Decisions

### Existing code to reuse / extend (verified during scout)
- `web/src/components/sidebar/AddProjectDialog.tsx` — THE dialog to evolve: current folder-only form (`repoPath`+`name`), `useCreateProject`, `ApiError`→`sentenceCase` error handling, `navigate` on success, disable-while-pending. The folder path here must stay behaviorally unchanged.
- `web/src/components/sidebar/ProjectSettingsDialog.tsx` — the gh-validated repo-field UX to mirror: integration-on gate (`settings.github_integration.value === "on"`), submit-validate, destructive-alert-under-the-field error, dialog-stays-open, `repoEdited` "don't clobber a user edit" guard, "adjust state on prop change" (no setState-in-effect) reset pattern.
- `web/src/api/mutations.ts` — `useCreateProject` (`POST /api/projects` with `{ name?, repo_path }`); extend its body type to also allow `{ repo }` for the repo-first path.
- `web/src/api/queries.ts` — `useGithubStatus()` (`{ gh_available }`); `useProjectGithubOrigin` (origin prefill precedent, dialog-open-gated).
- `web/src/api/settings.ts` — `useSettings()` → `github_integration` gate.
- `web/src/api/types.ts` — `Project` wire shape (now carries `managed`, `github_repo`, `description`).
- `internal/api/projects.go` — `create` (dispatches `{repo}`→`createByRepo`, else folder), `createByRepo` (name derivation `filepath.Base(dest)`, INSERT — **add description capture here, D-05**), `github.ValidateRepo` (the existing `gh repo view` call to possibly extend).
- `internal/github/github.go` — `ValidateRepo`/`ParseRepoRef` (canonicalization; the `gh repo view` shell-out the description capture rides on).
- `web/src/components/ui/` — shadcn primitives (Dialog, Input, Button, Label; check for Tabs/toggle for the segmented control).

No external specs/ADRs — requirements + decisions are fully captured here and in REQUIREMENTS/ROADMAP/PROJECT and the Phase-14 artifacts.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`AddProjectDialog.tsx`** is the single surface — add the toggle + repo branch; keep the folder branch as-is.
- **`ProjectSettingsDialog.tsx`** is a near-complete template for the repo-field: gh-validate-on-submit, destructive alert, `repoEdited` guard, integration-on gate, render-time state-reset pattern — copy these, don't reinvent.
- **`useCreateProject`** already posts to `/api/projects`; only its body type needs widening to `{ name?; repo_path? ; repo? }`.
- **Backend `createByRepo`** already does the whole atomic clone+insert+reattach; Phase 15 adds only the description capture to it.

### Established Patterns
- **`ApiError` → `sentenceCase`, 409 gets a trailing period, dialog stays open, values preserved** (AddProjectDialog) — the canonical create-error UX.
- **Integration gate = `useSettings().github_integration?.value === "on"`** (ProjectSettingsDialog) — already implies gh present.
- **"Adjust state on prop/open change," no setState-in-effect** (ProjectSettingsDialog `prevOpen`/`prevSuggestion`) — the project's lint-clean reset idiom.
- **Backend: arg-array `gh`/git shell-out, never `sh -c`; degrade-don't-break** — the description capture must follow this and never block create.

### Integration Points
- `web/src/components/sidebar/AddProjectDialog.tsx` (the form: toggle + repo/folder modes + clone spinner + inline error).
- `web/src/api/mutations.ts` (`useCreateProject` body type).
- `internal/api/projects.go` `createByRepo` (+ possibly `internal/github/github.go` `ValidateRepo`) for D-05 description capture.
- No new route is required (description capture rides the existing create); a lookup endpoint was explicitly NOT chosen.
</code_context>

<specifics>
## Specific Ideas

- Segmented toggle labels: "GitHub repo" (default) | "Local folder".
- Repo input placeholder `owner/name`; spinner copy "Cloning <owner/name>…".
- Name prefills from the repo's `name` segment, editable; never clobber a user edit (mirror `repoEdited`).
- Description is invisible in the Add dialog — it shows up afterward, editable, in Project settings (captured from GitHub at create).
- Failure copy must reflect Phase-14 atomicity: nothing half-created, safe to retry.
</specifics>

<deferred>
## Deferred Ideas

- **Live clone-progress streaming + cancel button** — CKUX-01 (REQUIREMENTS.md Future). D-06's blocking spinner is the v1.4 stand-in.
- **A dedicated repo-info lookup endpoint** (prefilled, editable description field before create) — considered and **not** chosen for v1.4 (D-05 captures server-side instead). Revisit if a richer create preview is wanted.
- **Non-default base branch / shallow clone at create** — CKMNT-02/03.
- The managed-checkout backend mechanics — Phase 14 (shipped), not deferred.

No scope creep surfaced; discussion stayed within the creation-flow UI boundary.
</deferred>

---

*Phase: 15-repo-first-creation-flow*
*Context gathered: 2026-06-15*
