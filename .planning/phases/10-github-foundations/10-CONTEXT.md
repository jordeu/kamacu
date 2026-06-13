# Phase 10: GitHub Foundations - Context

**Gathered:** 2026-06-13
**Status:** Ready for planning

<domain>
## Phase Boundary

Deliver the config substrate for the v1.3 GitHub PR Review milestone:

1. A **global GitHub integration on/off toggle** (default ON) in the Settings page. When OFF, all GitHub UI disappears app-wide and Kangent behaves byte-for-byte as it did pre-v1.3.
2. A **per-project config section** for an optional short project description and a linked GitHub repository (`owner/name`), validated.
3. The **schema foundation** (migration 00007) that later phases consume.

Out of scope for this phase (later phases): listing PRs / the Review column (Phase 11), opening a PR as a review workspace / worktree checkout (Phase 12), auto-cleanup (Phase 13). This phase adds NO `gh` PR calls — only the toggle, the project link/description, repo-link validation, and the columns.

</domain>

<decisions>
## Implementation Decisions

### Global toggle (GHSET-01/02/03)
- **D-01:** A `github_integration` key in the existing settings KV store, code default `"on"` (absent row = on), following the `settings.Defaults` + `Validate` + `Set` pattern. Stored raw; `Validate` accepts only `"on"`/`"off"`.
- **D-02:** Rendered as a real on/off **Switch** (add the shadcn `switch` component) in a new **"GitHub"** section on `/settings`. Use a Switch UI, not a dropdown. (`SettingsField` is input/select only today — extend it with a `"switch"` control or add a small dedicated toggle field; planner's discretion, but the value still flows through the existing settings KV REST surface.)
- **D-03:** When OFF, ALL GitHub UI is hidden app-wide: the per-project GitHub config fields AND (from Phase 11 on) the PR Review column. Read via the shared `useSettings()` query on the frontend; also gated server-side. OFF = the pre-v1.3 experience exactly.
- **D-04 (degrade):** GitHub features are best-effort — a missing/unauthenticated `gh`, or any GitHub failure, never breaks the board or any existing feature (GHSET-03). Verified by the "rename/remove `gh` → app stays fully usable" success criterion.

### Per-project config surface (GHPRJ-01/02/03)
- **D-05:** A **"Project settings" dialog** opened from the existing project `⋯` dropdown (alongside Rename / Delete), mirroring `RenameProjectDialog`. Holds the description field + the GitHub repo-link field. NOT a new full-page route, NOT an inline board panel.
- **D-06:** The dialog's GitHub-specific fields (repo link) appear only when the global toggle is ON (D-03). The description field is integration-agnostic and may show regardless (planner's discretion).
- **D-07:** Description is **plain text, optional, short**; editable and clearable. Shown **only in the config dialog for this phase** — no board-header or sidebar surfacing yet (deferred). Apply a reasonable soft length cap (planner's discretion).

### Repo linking (GHPRJ-03)
- **D-08:** The repo-link field **auto-detects from the project's local git `origin` remote** (prefill) — projects already point at a checkout, so read `git -C <repo_path> remote get-url origin`. The user can edit, override, or clear the prefill. Auto-detect must NOT shell out to git on every project-list response — fetch the suggestion when the dialog opens (a dedicated suggestion endpoint or an on-open derived read; planner's discretion).
- **D-09:** Accept both `owner/name` shorthand and full GitHub URLs (https and `git@github.com:owner/name(.git)`); **canonicalize to `owner/name`** for storage. When `gh` is present, canonicalize/verify via `gh repo view <ref> --json nameWithOwner` (see STACK.md); otherwise parse syntactically.
- **D-10:** `projects.github_repo` is nullable; NULL/empty = not linked.

### Link validation behavior (GHPRJ-03 / GHSET-03)
- **D-11:** **Soft-save with a warning.** Accept any syntactically valid `owner/name`. If `gh` is present but can't verify the repo (no access / not found / error), save it anyway and surface a non-blocking "couldn't verify" note in the dialog. If `gh` is absent/unauthenticated, save syntactically. Never hard-block linking on `gh` — `gh auth status` exit codes are unreliable (cli/cli#8845), and the Review column later shows its own empty/degraded state.

### Schema foundation
- **D-12:** Migration **00007** adds `projects.description TEXT` (nullable) and `projects.github_repo TEXT` (nullable). It ALSO lands the Phase-12 task columns now so no later migration is needed: `tasks.source TEXT NOT NULL DEFAULT 'manual'` (CHECK in `('manual','github_pr')`), `tasks.pr_number INTEGER`, `tasks.pr_base_ref TEXT`. One `ALTER TABLE ... ADD COLUMN` per statement (modernc/SQLite, as migration 00006 did); down-migration drops in reverse order.
- **D-13:** Extend `PATCH /api/projects/{id}` from rename-only to a partial update also accepting `description` and `github_repo` (server-side canonicalization + soft validation per D-09/D-11). Grow the `Project` JSON shape + `projectColumns` + `scanProject` to include the two new fields. The `github_integration` toggle reuses the existing settings KV REST surface (`PUT /api/settings/{key}`) — no new settings endpoint.

### Claude's Discretion
- Switch integration approach (extend `SettingsField` with a `"switch"` control vs a dedicated toggle field) — Switch UI either way.
- Description input affordance (single-line input vs small textarea) and exact soft length cap.
- Delivery of the origin auto-detect (derived field vs dedicated endpoint), as long as it never shells git on every project list.
- Exact "couldn't verify" warning copy and its placement in the dialog.
- URL-parsing/canonicalization details for `owner/name`.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Milestone research (v1.3)
- `.planning/research/SUMMARY.md` — overall v1.3 plan, settled decisions, A→B→C→D build order; Phase A = this phase.
- `.planning/research/ARCHITECTURE.md` — migration 00007 column set, `internal/github` shape (foundation only this phase), `PATCH /api/projects` extension, integration map against real v1.2 files.
- `.planning/research/STACK.md` — exact `gh` commands incl. `gh repo view --json nameWithOwner` for link canonicalization/validation, `gh auth status` / `LookPath` degradation.
- `.planning/research/PITFALLS.md` — degrade-don't-break, unreliable `gh auth status` exit codes (cli/cli#8845); the board-leak `source` filter (mostly Phase 12, but `tasks.source` lands here).

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` — GHSET-01/02/03, GHPRJ-01/02/03 (this phase's requirements) + Out of Scope.
- `.planning/ROADMAP.md` § "Phase 10: GitHub Foundations" — goal + 5 success criteria (incl. the `gh`-removed degrade test and toggle-off test).

No external (non-`.planning`) specs — requirements fully captured above and in the research files.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/settings/settings.go` — `Defaults` map + `Get`/`GetAll`/`Set`; add `KeyGithubIntegration = "github_integration"` with default `"on"`.
- `internal/settings/validate.go` — `Validate` switch; add an on/off case (mirrors the `KeyShell` membership check).
- `internal/api/settings.go` — `entryFor`/`getAll`/`put`; the toggle flows through unchanged (value is just `"on"`/`"off"`); a control hint can be added like `Options` is for shell if a switch needs one.
- `web/src/components/settings/SettingsField.tsx` — per-field commit state machine; extend with a `"switch"` control (or model the new GitHub section after it).
- `web/src/api/settings.ts` (`useSettings`, `useSaveSetting`) — the toggle reads/writes through these as-is.
- `internal/api/projects.go` — `projectHandlers.update` (extend PATCH), `Project` struct + `projectColumns` + `scanProject` (add 2 fields), `validateRepoPath` (shell-out style to copy for `gh repo view` + `git remote get-url`).
- `web/src/components/sidebar/RenameProjectDialog.tsx` — template for the new `ProjectSettingsDialog`.
- `web/src/components/sidebar/ProjectMenu.tsx` — add a "Project settings" `DropdownMenuItem`.
- `web/src/api/types.ts` / `web/src/api/mutations.ts` — `Project` type + project mutations to extend for description/github_repo.

### Established Patterns
- Settings: absent row = code default, read-at-use, validate-before-write; canonical UI-SPEC error copy returned verbatim and mirrored inline.
- Migrations: one `ALTER TABLE ... ADD COLUMN` per statement; nullable columns, targeted backfill only when load-bearing (see 00006).
- Shell-out: arg-array `exec.Command`, never `sh -c`; `git -C <dir> ...` and `gh ... --json` parsed as machine output.
- PATCH returns the full updated row via `RETURNING projectColumns`.

### Integration Points
- `/settings` page: new "GitHub" section with the toggle Switch.
- Project `⋯` menu → new "Project settings" dialog (description + repo link).
- `internal/store/migrations/00007_*.sql`: the new columns.
- `PATCH /api/projects/{id}`: extended for description + github_repo with server-side canonicalize/validate.
- Origin auto-detect: a server-side read of the project's `origin` remote, surfaced when the dialog opens.

</code_context>

<specifics>
## Specific Ideas

- The toggle should read like an on/off **switch**, not a dropdown.
- Linking should feel effortless: prefill from the repo's `origin` remote so the common case is "confirm, not type."
- Validation should never get in the user's way — accept and warn rather than block (degrade-don't-break, consistent with how the quota indicator degrades).

</specifics>

<deferred>
## Deferred Ideas

- Surfacing the project description outside the config dialog (board header / sidebar) — chose dialog-only for Phase 10; a display surface can be added in a later phase or polish pass.

None of the discussion was scope creep — all four areas clarified HOW to implement the fixed Phase 10 scope.

</deferred>

---

*Phase: 10-github-foundations*
*Context gathered: 2026-06-13*
