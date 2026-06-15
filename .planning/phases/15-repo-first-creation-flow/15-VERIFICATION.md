---
phase: 15-repo-first-creation-flow
verified: 2026-06-15T05:40:00Z
status: passed
score: 17/17 must-haves verified
gaps: []
---

# Phase 15: Repo-First Creation Flow Verification Report

**Phase Goal:** When GitHub integration is on, a user adds a project by naming a GitHub repo — name/link/description auto-fill from the repo and a folder path is the optional alternative — and a failed clone surfaces inline leaving no half-created project; when integration is off the Add-project flow is folder-only, exactly as before v1.4.
**Verified:** 2026-06-15T05:40:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #   | Truth (from 15-01/02/03 must_haves)                                                                                              | Status     | Evidence |
| --- | ------------------------------------------------------------------------------------------------------------------------------- | ---------- | -------- |
| 1   | Repo-first create persists the GitHub description into projects.description (best-effort)                                        | ✓ VERIFIED | projects.go:264-267 captures `github.RepoDescription(r.Context(), canonical)` and inserts it; github.go:139-144 RepoDescription |
| 2   | A missing/empty/failed description read persists "" and never blocks/fails create                                               | ✓ VERIFIED | github.go:139-163 maps gh-absent/empty/nonzero/parse-fail → ""; error-free `string` signature; `TestRepoDescription` + `TestCreateRepoEmptyDescriptionStill201` PASS |
| 3   | useCreateProject accepts a body with optional repo field alongside repo_path                                                     | ✓ VERIFIED | mutations.ts:10-14 `{ name?; repo_path?; repo? }` |
| 4   | Project wire type carries the managed marker                                                                                     | ✓ VERIFIED | types.ts:21 `managed: boolean` |
| 5   | Integration ON → segmented "GitHub repo" \| "Local folder" toggle, defaulting to GitHub repo                                     | ✓ VERIFIED | AddProjectDialog.tsx:48 `useState<Mode>("repo")`, :153-166 gated Tabs, repo trigger leftmost |
| 6   | Integration OFF → byte-for-byte folder-only form (no toggle, no repo field, repo branch never mounts)                            | ✓ VERIFIED | AddProjectDialog.tsx:62 `activeMode = integrationOn ? mode : "folder"`, :153 toggle gated on `integrationOn`, :168 branch keyed on `activeMode` |
| 7   | Repo mode prefills Name from the name segment of owner/name, editable, never clobbering a user edit                              | ✓ VERIFIED | AddProjectDialog.tsx:35-40 `deriveName` (pure local parse), :83-90 adjust-state-on-change prefill guarded by `nameEdited` |
| 8   | Repo-mode submit POSTs { repo } and shows a blocking "Cloning <owner/name>…" spinner, inputs disabled, dialog open               | ✓ VERIFIED | AddProjectDialog.tsx:108 `{ repo: ownerName.trim(), name? }`, :144 `cloning`, :237-246 Loader2 + animate-spin + U+2026 |
| 9   | Clone/validate failure surfaces inline in the destructive-alert box; dialog open, values + mode preserved; no half-created copy  | ✓ VERIFIED | AddProjectDialog.tsx:118-133 catch keeps fields, :184/:210 boxed alert (ProjectSettingsDialog classes); Phase-14 atomicity (projects.go:243-251 RemoveAll + no row) |
| 10  | Folder mode behaviorally unchanged: POSTs { repo_path }, same handling                                                           | ✓ VERIFIED | AddProjectDialog.tsx:109 `{ repo_path: repoPath.trim(), name? }`, :125-126 same sentenceCase/409 handling; backend folder INSERT projects.go:174 byte-for-byte unchanged |
| 11  | On success the dialog closes and navigates to /projects/{id}                                                                     | ✓ VERIFIED | AddProjectDialog.tsx:111-117 `onOpenChange(false)` + `navigate(\`/projects/${project.id}\`)` |
| 12  | web/dist rebuilt so the Go binary embeds the new dialog                                                                          | ✓ VERIFIED | dist/index.html references `assets/index-C43eIU7Y.js`; that bundle (on disk) contains "Cloning"; `go build ./...` OK |
| 13  | Human confirms the flow end-to-end in the running app (9 CONFIRM steps)                                                          | ✓ VERIFIED | 15-03-SUMMARY APPROVED 2026-06-15; committed 858d661 |

**Score:** 13/13 truths verified (17/17 counting the artifact + key-link sub-checks below)

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/github/github.go` | RepoDescription best-effort read + descriptionRunner seam | ✓ VERIFIED | :125 `var descriptionRunner = ghDescription`, :139 `func RepoDescription(...) string`, :151 `ghDescription` (arg array `gh repo view … --json description`); `ValidateRepo` signature unchanged (:91) |
| `internal/github/testhooks.go` | SetDescriptionRunnerForTest mirroring SetValidateRunnerForTest | ✓ VERIFIED | :38-42 returns restore func |
| `internal/api/projects.go` | createByRepo captures + persists description at INSERT | ✓ VERIFIED | :264 capture, :265-267 INSERT (name, repo_path, github_repo, managed, description); folder INSERT :174 untouched |
| `web/src/api/mutations.ts` | useCreateProject body { name?; repo_path?; repo? } | ✓ VERIFIED | :10-14 |
| `web/src/api/types.ts` | Project.managed: boolean | ✓ VERIFIED | :21 |
| `web/src/components/sidebar/AddProjectDialog.tsx` | Integration-gated toggle, repo/folder modes, prefill, spinner, inline alert | ✓ VERIFIED | 266 lines; all 14 acceptance greps match; 0 useEffect |
| `web/dist` (embedded SPA) | Rebuilt bundle contains "Cloning" | ✓ VERIFIED | index-C43eIU7Y.js contains "Cloning", referenced by index.html |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| createByRepo | github.RepoDescription | best-effort read before INSERT | ✓ WIRED | projects.go:264 → github.go:139 |
| useCreateProject | POST /api/projects {repo} | widened mutationFn body | ✓ WIRED | mutations.ts:10-14 → AddProjectDialog.tsx:108 |
| AddProjectDialog | useSettings().github_integration | integration-on render gate (=== "on") | ✓ WIRED | AddProjectDialog.tsx:44 |
| AddProjectDialog | useCreateProject {repo} | repo-mode submit posts widened body | ✓ WIRED | AddProjectDialog.tsx:106-110 |
| AddProjectDialog | ui/tabs.tsx | segmented toggle import (existing primitive) | ✓ WIRED | AddProjectDialog.tsx:16; tabs.tsx pre-existing (added in 01-03 scaffold) — no new ui/ file |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| AddProjectDialog (mode/inputs) | `mode`/`ownerName`/`repoPath`/`name` | local useState driven by user input | Yes (user-driven) | ✓ FLOWING |
| AddProjectDialog (integration gate) | `integrationOn` | `useSettings().github_integration?.value` | Yes (real settings query) | ✓ FLOWING |
| AddProjectDialog (Name prefill) | `name` | `deriveName(ownerName)` pure local parse | Yes (derived from typed input, no network) | ✓ FLOWING |
| createByRepo (persisted description) | `desc` | `github.RepoDescription` → `gh repo view --json description` | Yes (real gh read, best-effort "" fallback) | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| RepoDescription degrade cases | `go test ./internal/github -run RepoDescription -count=1` | ok | ✓ PASS |
| Repo-first create persists description | `go test ./internal/api -run TestCreateRepoCapturesDescription` | PASS | ✓ PASS |
| Empty description still 201 | `go test ./internal/api -run TestCreateRepoEmptyDescriptionStill201` | PASS | ✓ PASS |
| Clone-failure atomicity (no orphan row/dir) | `go test ./internal/api -run TestCreateRepoCloneAtomicity` | PASS | ✓ PASS |
| Full backend suite | `go test ./...` | all packages ok | ✓ PASS |
| Frontend typecheck | `cd web && npx tsc --noEmit` | exit 0 | ✓ PASS |
| Dialog lint | `npx eslint AddProjectDialog.tsx` | exit 0 | ✓ PASS |
| Binary embeds dialog | `go build ./...` + bundle "Cloning" grep | OK + match | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
| ----------- | -------------- | ----------- | ------ | -------- |
| RPROJ-01 | 15-02 | Integration on → Add flow defaults to repo-first | ✓ SATISFIED | Toggle defaults to "repo" (AddProjectDialog.tsx:48); REQUIREMENTS.md:12 `[x]`, table Complete |
| RPROJ-02 | 15-01, 15-02 | Valid repo auto-derives name + auto-fills link + description | ✓ SATISFIED | Name prefill :35-40/:83-90; github_repo set backend (projects.go:266); description captured (projects.go:264); REQUIREMENTS.md:13 `[x]` Complete |
| RPROJ-03 | 15-02 | Folder path is the optional alternative when GitHub is on | ✓ SATISFIED | "Local folder" trigger + folder branch (AddProjectDialog.tsx:160-163,:195-215); REQUIREMENTS.md:14 `[x]` Complete |
| RPROJ-04 | 15-02 | Integration off → folder-only, no repo-first UI | ✓ SATISFIED | `integrationOn` gate (:44,:62,:153); REQUIREMENTS.md:15 `[x]` Complete |
| CKOUT-04 | 15-02 | Clone failure surfaces inline, no half-created project | ✓ SATISFIED | Inline boxed alert (:184/:210), values preserved (:118-133); Phase-14 atomicity (projects.go:243-251); REQUIREMENTS.md:23 `[x]` Complete |

All 5 declared requirement IDs are checked `[x]` in REQUIREMENTS.md and mapped to Phase 15 with status **Complete** (lines 58-66). No orphaned requirements — REQUIREMENTS.md:71 maps exactly RPROJ-01..04 + CKOUT-04 to Phase 15, all claimed by the plans.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| — | — | None | — | No TODO/FIXME/stub/placeholder in phase-15 source; no `sh -c`/`exec.Command("sh"` in production; 0 `useEffect` in the dialog (no new setState-in-effect); the only `"todo"`/`placeholder=` hits are the Status enum literal and HTML input placeholders (not anti-patterns) |

### Human Verification Required

None outstanding. The 15-03 blocking human-verify gate was exercised in the running binary and **APPROVED by the user on 2026-06-15** (15-03-SUMMARY, committed 858d661) — all nine CONFIRM steps passed (repo-default toggle, name prefill + don't-clobber, Cloning spinner + navigate, auto-captured description in settings, folder fallback, inline clone-failure with no orphan project/dir, 409 already-added, integration-off folder-only).

### Gaps Summary

None. Every must-have across the three plans is substantiated by the actual codebase, not just the SUMMARYs:

- **Backend (15-01):** `github.RepoDescription` is a real, error-free best-effort leaf (separate from the unchanged `ValidateRepo`); `createByRepo` captures and persists the description at INSERT while leaving the folder-create INSERT byte-for-byte unchanged. Verified by reading github.go/projects.go and running the targeted tests (description capture, empty-description-still-201, atomicity all PASS).
- **Frontend (15-02):** The dialog is the genuine repo-first surface — integration-gated controlled Tabs (repo default), pure-local name prefill with a `nameEdited` don't-clobber guard via the adjust-state-on-change idiom (0 `useEffect`), blocking Loader2 "Cloning …" spinner, boxed destructive-alert failure with values/mode preserved and no half-created wording, and an unchanged folder path. All 14 acceptance greps match; tsc + eslint clean. The embedded `web/dist` bundle referenced by index.html contains the "Cloning" string, so `go build` ships it.
- **Human gate (15-03):** Recorded APPROVED on 2026-06-15.
- **Gates:** `go build ./...`, `go vet ./...`, full `go test ./...`, and `tsc --noEmit` all green.

---

_Verified: 2026-06-15T05:40:00Z_
_Verifier: Claude (gsd-verifier)_
