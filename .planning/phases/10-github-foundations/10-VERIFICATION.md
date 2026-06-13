---
phase: 10-github-foundations
verified: 2026-06-13T00:00:00Z
status: gaps_found
score: 5/5 must-haves verified in code; human UAT found 2 behavioral gaps (gh-gated enablement + copy)
human_verification:
  - test: "OFF cascade click-through (GHSET-02)"
    expected: "Turn the GitHub integration Switch OFF on /settings; open a project's ⋯ → Project settings: the GitHub repository field is gone, Description remains; no Review column or other GitHub UI appears anywhere; board/tasks/sessions behave exactly as before v1.3."
    why_human: "The gate is verified in code (repo field is `{integrationOn && ...}`, settings page is the only other GitHub surface), but 'byte-for-byte unchanged across every existing flow' is a visual/behavioral assertion across the whole app that grep cannot prove."
  - test: "Live gh-removed degrade test (GHSET-03)"
    expected: "Rename/remove the `gh` binary (or de-authenticate it), then link a project to a syntactically valid repo (e.g. owner/name): the PATCH still 200s and stores the syntactic owner/name, the dialog closes, and nothing in core flows breaks."
    why_human: "`gh` is PRESENT (v2.82.0) on this host, so the no-gh / unauthenticated branch of ValidateRepo is not exercised live here. The code path is verified (Available()→false ⇒ soft-save) and the API tests use syntactic refs so they pass regardless of gh, but the actual binary-removed run is a host-state test."
  - test: "Origin prefill on dialog open (GHPRJ-03 / D-08)"
    expected: "Open Project settings for an unlinked project whose git origin is a GitHub remote: the repository field prefills with the canonicalized owner/name; for a non-GitHub or absent origin the field stays empty; the origin endpoint is hit only on open, never on the project list."
    why_human: "Endpoint + query wiring verified in code and by TestGithubOriginSuggestion; the live prefill-on-open UX (and that it never fires on the list) is an interactive behavior best confirmed in the browser."
---

# Phase 10: github-foundations Verification Report

**Phase Goal:** A user can turn GitHub integration on/off globally and link a project to a GitHub repo with an optional description — and when `gh` is missing or the toggle is off, the rest of Kangent is byte-for-byte unchanged.

**Verified:** 2026-06-13
**Status:** gaps_found (human UAT — see `## Gaps`)
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth (Success Criterion) | Status | Evidence |
| - | ------------------------- | ------ | -------- |
| 1 | Toggle GitHub integration on/off from /settings, ON by default (GHSET-01) | ✓ VERIFIED | `Defaults[github_integration]="on"` (settings.go:37, spot-check confirmed); `Validate` accepts only `on`/`off` (validate.go:57-62, spot-check confirmed); `GithubSection` in SettingsPage.tsx wires `useSaveSetting("github_integration")` to `onCheckedChange` mutating `"on"`/`"off"`, reads `settings.github_integration?.value === "on"`. Always-rendered last section. |
| 2 | OFF hides all GitHub UI; app unchanged before v1.3 (GHSET-02) | ✓ VERIFIED (code) / ? human (full cascade) | Repo field gated by `{integrationOn && (...)}` (ProjectSettingsDialog.tsx:203); origin query disabled when off (`open && integrationOn`, :90); submit preserves saved link when off (:129). Grep found NO GitHub UI surface outside the gated dialog + settings page. The whole-app "byte-for-byte unchanged" claim routed to human. |
| 3 | Set/edit/clear a project description (GHPRJ-01, GHPRJ-02) | ✓ VERIFIED | Partial PATCH in projects.go:171-178 (280-char cap, `""` clears); `TestUpdateProjectPartial` asserts set→"hello", clear→"", name-only-PATCH leaves description untouched. Dialog Textarea with `Description` label, 280 hard-stop, `{n}/280` counter, `Project settings` ⋯ menu item between Rename and Delete. |
| 4 | Link/change/clear repo, validated via gh, syntactic fallback (GHPRJ-03) | ✓ VERIFIED | `github.ParseRepoRef` canonicalizes all 5 ref forms to owner/name (spot-checked); `ValidateRepo` uses `gh repo view ... --json nameWithOwner` (github.go:88) with LookPath gate (:105); PATCH stores canonical, `""`→NULL unlink, invalid ref→400 (projects.go:179-195); `TestUpdateProjectPartial` covers URL→`cli/cli`, unlink→null, invalid→400 row-unchanged. |
| 5 | Degrade test: gh missing/unauth leaves app fully usable (GHSET-03) | ✓ VERIFIED (code) / ? human (live) | `ValidateRepo`: `!Available()`→`(parsed,false,nil)` soft-save; any gh nonzero/parse-fail→soft-save, no error (github.go:80-99). Only syntactic invalidity hard-blocks. API tests use syntactic refs so they pass with or without gh. Live binary-removed run routed to human (gh is present on this host). |

**Score:** 5/5 truths verified in code; 3 carry a human confirmation item for inherently interactive/host-state aspects.

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/store/migrations/00007_github_foundations.sql` | 5 columns, one ALTER per stmt, reverse-order down | ✓ VERIFIED | All 5 ADD COLUMNs present with specified types/constraints; 5 reverse-order DROP COLUMNs. `go test ./internal/store/...` green (migration applies on fresh DB). |
| `internal/settings/settings.go` | KeyGithubIntegration const + Defaults "on" | ✓ VERIFIED | Const at :21, Defaults `"on"` at :37. Get/GetAll/Set unchanged (absent row → default for free). |
| `internal/settings/validate.go` | on/off validation case | ✓ VERIFIED | `case KeyGithubIntegration` at :57, exact error `Choose on or off.` |
| `internal/github/github.go` | ParseRepoRef, ValidateRepo, Available; stdlib-only | ✓ VERIFIED | All 3 funcs present, canonical error verbatim, arg-array exec, no third-party imports, no `sh -c`. |
| `internal/api/projects.go` | grown Project + partial PATCH + origin handler | ✓ VERIFIED | Project struct +Description/+GithubRepo(*string), projectColumns includes both, scanProject maps NullString→*string, partial PATCH, `githubOrigin` handler. |
| `internal/api/routes.go` | github-origin route | ✓ VERIFIED | `GET /api/projects/{id}/github-origin` registered at :24. |
| `web/src/components/ui/switch.tsx` | shadcn Switch primitive | ✓ VERIFIED | Exports `Switch` from `radix-ui` consolidated package (dep `radix-ui ^1.5.0` present, matches codebase pattern). |
| `web/src/pages/SettingsPage.tsx` | GitHub section + Switch | ✓ VERIFIED | `GithubSection` with verbatim copy, `useSaveSetting`, `Couldn't save. Try again.` revert. |
| `web/src/components/sidebar/ProjectSettingsDialog.tsx` | description + gated repo field, one PATCH | ✓ VERIFIED | Full dialog: counter, 280 cap, gated repo field, origin prefill, hard-error-stays-open, muted advisory render path wired for future verify-flag. |
| `web/src/components/sidebar/ProjectMenu.tsx` | ⋯ item + dialog mount | ✓ VERIFIED | `Project settings` item between Rename and Delete; `<ProjectSettingsDialog>` mounted. |
| `web/src/api/{types,mutations,queries}.ts` | grown Project type + mutation + origin query | ✓ VERIFIED | Project type +description/+github_repo; `useUpdateProjectSettings` PATCHes one body; `useProjectGithubOrigin` enabled-gated, staleTime Infinity. |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| validate.go | settings.go | `case KeyGithubIntegration` | ✓ WIRED | Switch references the const; default+validation agree. |
| 00007 migration | goose embed runner | ADD COLUMNs picked up by `//go:embed migrations/*.sql` | ✓ WIRED | store migrate test green on fresh DB. |
| projects.go | github.ParseRepoRef / ValidateRepo | PATCH canonicalizes + soft-validates | ✓ WIRED | ValidateRepo at :184; ParseRepoRef in origin handler at :239. |
| projects.go | `git remote get-url origin` | origin handler arg-array exec | ✓ WIRED | projects.go:234. |
| routes.go | githubOrigin handler | mux.HandleFunc | ✓ WIRED | routes.go:24. |
| SettingsPage.tsx | PUT /api/settings/github_integration | Switch onCheckedChange via useSaveSetting | ✓ WIRED | useSaveSetting("github_integration") at SettingsPage.tsx:54. |
| ProjectSettingsDialog.tsx | PATCH /api/projects/{id} | form submit { description, github_repo } | ✓ WIRED | useUpdateProjectSettings.mutateAsync at :131. |
| ProjectSettingsDialog.tsx | github_integration toggle | repo field renders only when `value === "on"` | ✓ WIRED | `integrationOn` gate at :203. |
| ProjectMenu.tsx | ProjectSettingsDialog | ⋯ item opens dialog | ✓ WIRED | item :66, dialog mount :84. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| SettingsPage GithubSection | `settings.github_integration.value` | `useSettings()` → GET /api/settings (server returns Defaults["on"] when no row) | Yes | ✓ FLOWING |
| ProjectSettingsDialog repo field | `repo` ← `project.github_repo` / origin suggestion | `useProjects` row + `useProjectGithubOrigin` → real `git remote get-url origin` | Yes | ✓ FLOWING |
| ProjectSettingsDialog description | `description` ← `project.description` | project row (real DB column) | Yes | ✓ FLOWING |
| PATCH github_repo store | canonical owner/name | `github.ValidateRepo` (gh-canonicalized or syntactic) | Yes | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| ParseRepoRef canonicalizes 5 forms + rejects 4 bad forms | in-package test driving ParseRepoRef | All 5 valid → `cli/cli`; all 4 invalid → canonical error | ✓ PASS |
| Available() does not panic | spot-check | returned `true` (gh present) | ✓ PASS |
| Settings default = "on" | spot-check Defaults | `"on"` | ✓ PASS |
| on/off validation | spot-check Validate | on/off→nil; yes/""/ON→`Choose on or off.` | ✓ PASS |
| Migration on fresh DB | `go test ./internal/store/...` | ok | ✓ PASS |
| Phase-10 Go suite | `go test ./internal/github ./internal/settings ./internal/api` | all ok | ✓ PASS |
| Go build | `go build ./...` | exit 0 | ✓ PASS |
| Live gh-removed degrade | (host has gh present) | not exercised live | ? SKIP → human |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| GHSET-01 | 10-01, 10-03 | Toggle on/off, ON by default | ✓ SATISFIED | Default "on" + Switch wiring (truth 1) |
| GHSET-02 | 10-03 | OFF hides all GitHub UI, app unchanged | ✓ SATISFIED (code); human-confirm full cascade | Repo field gated; no UI leak (truth 2) |
| GHSET-03 | 10-02 | Best-effort; gh missing never breaks | ✓ SATISFIED (code); human-confirm live | Soft-save degrade paths (truth 5) |
| GHPRJ-01 | 10-02, 10-03 | Per-project config section | ✓ SATISFIED | Project settings dialog + ⋯ item (truth 3) |
| GHPRJ-02 | 10-02, 10-03 | Set/edit/clear description | ✓ SATISFIED | PATCH + TestUpdateProjectPartial (truth 3) |
| GHPRJ-03 | 10-02, 10-03 | Link/change/clear repo, gh-or-syntactic validation | ✓ SATISFIED | ValidateRepo + canonicalization (truth 4) |

No orphaned requirements: REQUIREMENTS.md maps exactly GHSET-01/02/03 + GHPRJ-01/02/03 to Phase 10, all claimed by plans and all verified.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | No TODO/FIXME/PLACEHOLDER/stub in any phase-10 file | — | Clean |

The `verify_state` heuristic in ProjectSettingsDialog (:142) reads a field the current server never sends — this is documented dead-but-intentional wiring for a future server verify-flag (the muted advisory render path), not a stub: the default branch closes the dialog, which is the specified current behavior. Not flagged as a blocker.

Pre-existing lint (19 errors, none in phase-10 files) logged to deferred-items.md — confirmed out of scope.

### Human Verification Required

See `human_verification` frontmatter. Three items: (1) the OFF-cascade click-through across the whole app, (2) the live gh-removed degrade run (gh is installed on this host), (3) the origin prefill-on-open UX. All three have their code paths verified; the human items confirm the inherently interactive / host-state aspects the success criteria flagged as human-testable.

### Gaps Summary

Automated verification found no missing or stubbed artifacts (all 5 truths verified in code). However, **human UAT discovered 2 behavioral gaps** that change `gh`-availability handling and copy. These are refinements to GHSET-01/02/03 surfaced by running the live `gh`-removed scenario — not implementation defects in what was specified, but the specified behavior is now wrong. Recorded below for `/gsd:plan-phase 10 --gaps`.

## Gaps

### Gap 1: `gh`-gated default + enable guard (GHSET-01 / GHSET-03)
- **Source:** Human UAT (user renamed `gh` → now unavailable on host).
- **Requirements:** GHSET-01 (default state), GHSET-03 (degrade behavior).
- **Current (wrong) behavior:** GitHub integration is `ON` by default unconditionally (`Defaults["github_integration"]="on"`), and the `/settings` Switch toggles `on`/`off` with no awareness of whether `gh` is installed. `github.Available()` exists but is internal-only — nothing surfaces `gh` availability to the web UI (only the per-project `GET /api/projects/{id}/github-origin` route exists).
- **Required behavior:**
  1. When `gh` is **not available** on the host, GitHub integration is **OFF by default** (the effective/displayed enabled state must be gated on `gh` availability; do not show it as on when `gh` is missing).
  2. When the user attempts to **enable** the integration, the app checks `gh` availability. If `gh` is unavailable, the toggle is **not** enabled; instead the user is shown a message telling them to **install the GitHub CLI (`gh`) before enabling GitHub integration**.
- **Implementation hint:** Surface `gh` availability to the frontend — e.g. add a `gh_available` field to the settings GET response, or a small always-200 `GET /api/github/status` endpoint reusing `github.Available()` (checked at call time, modeled on the `internal/quota` degrade pattern). Frontend `GithubSection` reads it, gates the Switch (default off + block/guard enable when unavailable), and renders the install-`gh` guidance in place of the default help line.
- **Where:** backend `internal/github` + `internal/api` (new status surface); frontend `web/src/pages/SettingsPage.tsx` (`GithubSection`), and the shared `useSettings()`/queries layer.

### Gap 2: Remove over-claiming toggle copy (GHSET-02)
- **Source:** Human UAT.
- **Requirement:** GHSET-02 (copy).
- **Current:** `GITHUB_INTEGRATION_HELP` (SettingsPage.tsx:42) = `Show GitHub features across Kangent. Turn off to hide all GitHub UI — the app behaves exactly as it did before.`
- **Required:** Drop the trailing clause. New copy: `Show GitHub features across Kangent. Turn off to hide all GitHub UI.`
- **Where:** `web/src/pages/SettingsPage.tsx:42`.

### Deferred (still inherently human-testable after gaps close)
- Origin prefill-on-open UX (GHPRJ-03 / D-08) — verified in code; confirm interactively. Tracked in `10-HUMAN-UAT.md`.

---

_Verified: 2026-06-13_
_Verifier: Claude (gsd-verifier)_
