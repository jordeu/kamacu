---
phase: 10-github-foundations
verified: 2026-06-13T17:30:00Z
status: passed
score: 5/5 must-haves verified in code; both UAT gaps closed and re-verified in code
re_verification:
  previous_status: gaps_found
  previous_score: "5/5 in code; 2 behavioral gaps from human UAT"
  gaps_closed:
    - "Gap 1: gh-gated default + enable guard (GHSET-01/GHSET-03) — backend GET /api/github/status (10-04) + frontend gh-aware toggle (10-05)"
    - "Gap 2: over-claiming toggle copy (GHSET-02) — trailing clause removed"
  gaps_remaining: []
  regressions: []
human_verification:
  - test: "Live gh-removed toggle behavior (GHSET-01/GHSET-03) — interactive confirmation only"
    expected: "With `gh` ABSENT (current host state): /settings GitHub integration Switch shows OFF (never on, even if a stored row says \"on\"); the help line reads `Install the GitHub CLI (gh) before enabling GitHub integration.`; clicking the Switch to enable does NOT turn it on and surfaces that same install message. Then install/restore `gh` on PATH and reload: the toggle behaves normally (reflects the stored on/off state, enable commits, help line reverts to `Show GitHub features across Kangent. Turn off to hide all GitHub UI.`). No server restart needed (Available() is call-time)."
    why_human: "The gate, guard, effective-state, copy, and endpoint contract are all verified in code, and the gh-absent degrade path is exercised live (gh is absent on this host, TestGithubStatus confirms the endpoint reports gh_available:false). The remaining item is the inherently-interactive browser click-through (Switch visual state + click-to-enable surfacing the message, and the restore-gh round trip)."
  - test: "OFF cascade click-through (GHSET-02) — interactive confirmation only"
    expected: "Turn the GitHub integration Switch OFF on /settings; open a project's ⋯ → Project settings: the GitHub repository field is gone, Description remains; no Review column or other GitHub UI appears anywhere; board/tasks/sessions behave as before v1.3."
    why_human: "The gate is verified in code (repo field is `{integrationOn && ...}`, settings page is the only other GitHub surface), but the whole-app visual/behavioral assertion across every existing flow cannot be proven by grep."
  - test: "Origin prefill on dialog open (GHPRJ-03 / D-08) — interactive confirmation only"
    expected: "Open Project settings for an unlinked project whose git origin is a GitHub remote: the repository field prefills with the canonicalized owner/name; for a non-GitHub or absent origin the field stays empty; the origin endpoint is hit only on open, never on the project list."
    why_human: "Endpoint + query wiring verified in code and by TestGithubOriginSuggestion; the live prefill-on-open UX (and that it never fires on the list) is an interactive behavior best confirmed in the browser."
---

# Phase 10: github-foundations Verification Report

**Phase Goal:** A user can turn GitHub integration on/off globally and link a project to a GitHub repo with an optional description — and when `gh` is missing or the toggle is off, the rest of Kangent is byte-for-byte unchanged.

**Verified:** 2026-06-13 (re-verified after gap closure 10-04 + 10-05)
**Status:** passed — all 5 success criteria verified in code; both UAT-discovered gaps now closed and re-verified in code; remaining items are inherently-interactive browser confirmations.
**Re-verification:** Yes — after Gap 1 (gh-gated default/enable guard) and Gap 2 (toggle copy) closure.

## Re-Verification Summary (Gap Closure)

`gh` is **currently ABSENT** on this host (`command -v gh` → exit 1; not on PATH), which lets the degrade path be exercised live rather than reasoned about.

### Gap 1 — `gh`-gated default + enable guard (GHSET-01 / GHSET-03): CLOSED

**Backend (Plan 10-04):**
- `internal/api/github.go` — `githubStatus` handler returns `writeJSON(w, http.StatusOK, map[string]bool{"gh_available": github.Available()})`. Always-200, no DB, degrade-safe (github.go:14-16). ✓
- `internal/api/routes.go:25` — `mux.HandleFunc("GET /api/github/status", githubStatus)` registered. ✓
- `internal/github/github.go:104-107` — `Available()` does `exec.LookPath("gh")` at call time → returns `false` here (gh absent). ✓
- `internal/api/github_test.go` — `TestGithubStatus` asserts status 200, boolean field named exactly `gh_available`, value == `github.Available()` in-process. **PASS.** Because `gh` is absent in this process, the test confirms the endpoint reports `gh_available:false` on this host (the live degrade contract). ✓

**Frontend (Plan 10-05):**
- `web/src/api/queries.ts:50-55` — `useGithubStatus()` `useQuery(["github-status"])` reading `GET /api/github/status` typed `{ gh_available: boolean }`. ✓
- `web/src/pages/SettingsPage.tsx:65-67` — `ghAvailable = ghStatus?.gh_available === true` (loading/error/missing all fail safe to not-available); `effectiveEnabled = enabled && ghAvailable`. ✓
- `SettingsPage.tsx:76` — `<Switch checked={effectiveEnabled}>` (the stale `checked={enabled}` is **absent** — grep confirmed). A stored `"on"` row cannot show on when gh is missing. ✓
- `SettingsPage.tsx:77-86` — `onCheckedChange` enable-guard: `if (checked && !ghAvailable) { setError(GITHUB_GH_MISSING_HELP); return; }` — blocks enable, does not commit; turn-off always commits. ✓
- `SettingsPage.tsx:44, 96` — help line shows `Install the GitHub CLI (gh) before enabling GitHub integration.` when `!ghAvailable`, even before any click. ✓

### Gap 2 — over-claiming toggle copy (GHSET-02): CLOSED

- `SettingsPage.tsx:43` — `GITHUB_INTEGRATION_HELP = \`Show GitHub features across Kangent. Turn off to hide all GitHub UI.\`` — **exact** required string. ✓
- `behaves exactly as it did before` is **ABSENT** from the file (grep confirmed). ✓

### Regression Checks (no regressions)

| Check | Command | Result |
| ----- | ------- | ------ |
| Targeted endpoint test | `go test ./internal/api/ -run TestGithubStatus -v` | PASS (gh_available matched github.Available()=false) |
| Full Go build | `go build ./...` | exit 0 |
| Full Go test suite | `go test ./...` | all packages ok (api, github, settings, store, ... all green) |
| Web build + typecheck | `cd web && npm run build` (`tsc -b && vite build`) | exit 0 (only pre-existing chunk-size advisory) |
| Anti-pattern scan (gap files) | grep TODO/FIXME/stub in github.go, github_test.go, queries.ts, SettingsPage.tsx | none found (clean) |

The pre-existing lint debt (errors outside phase-10 files, logged in `deferred-items.md`) remains out of scope and unchanged.

## Goal Achievement

### Observable Truths

| # | Truth (Success Criterion) | Status | Evidence |
| - | ------------------------- | ------ | -------- |
| 1 | Toggle GitHub integration on/off from /settings, ON by default — now gh-gated (GHSET-01) | ✓ VERIFIED | `Defaults["github_integration"]="on"` (settings.go); `Validate` accepts only `on`/`off`. Toggle now reads `useGithubStatus()`; `effectiveEnabled = enabled && ghAvailable` so it shows OFF and is un-enableable when `gh` is missing (Gap 1 closed). |
| 2 | OFF hides all GitHub UI; app unchanged before v1.3; copy not over-claiming (GHSET-02) | ✓ VERIFIED (code) / ? human (full cascade) | Repo field gated by `{integrationOn && (...)}`; origin query disabled when off; no GitHub UI surface outside the gated dialog + settings page. Copy corrected (Gap 2 closed). Whole-app "byte-for-byte unchanged" routed to human. |
| 3 | Set/edit/clear a project description (GHPRJ-01, GHPRJ-02) | ✓ VERIFIED | Partial PATCH in projects.go (280-char cap, `""` clears); `TestUpdateProjectPartial` green. Dialog Textarea, counter, ⋯ menu item. |
| 4 | Link/change/clear repo, validated via gh, syntactic fallback (GHPRJ-03) | ✓ VERIFIED | `github.ParseRepoRef` canonicalizes; `ValidateRepo` uses `gh repo view` with LookPath gate; PATCH stores canonical, `""`→NULL, invalid→400; `TestUpdateProjectPartial` covers all. |
| 5 | Degrade test: gh missing/unauth leaves app fully usable (GHSET-03) | ✓ VERIFIED (code + live endpoint) / ? human (full UI round trip) | `ValidateRepo`: `!Available()`→soft-save. With gh now ABSENT on this host, `TestGithubStatus` confirms the endpoint reports `gh_available:false` and the toggle gates OFF/un-enableable (Gap 1). Full live UI round trip (enable-blocked + restore-gh) routed to human. |

**Score:** 5/5 truths verified in code; both UAT gaps closed; remaining human items are inherently-interactive browser confirmations.

### Required Artifacts (gap-closure additions)

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/api/github.go` | always-200 `gh_available` handler | ✓ VERIFIED | `githubStatus` returns `{"gh_available": github.Available()}`, status 200, no DB. |
| `internal/api/github_test.go` | TestGithubStatus contract | ✓ VERIFIED | Asserts 200 + boolean `gh_available` == `github.Available()`. PASS on gh-absent host. |
| `internal/api/routes.go` | `GET /api/github/status` route | ✓ VERIFIED | Registered at routes.go:25. |
| `web/src/api/queries.ts` | `useGithubStatus()` hook | ✓ VERIFIED | `useQuery(["github-status"])` → `GET /api/github/status` typed `{ gh_available }`. |
| `web/src/pages/SettingsPage.tsx` | gh-gated toggle + corrected copy | ✓ VERIFIED | `effectiveEnabled`, enable guard, install-gh help line; exact corrected copy; over-claim clause absent. |
| `web/dist` | rebuilt embedded SPA | ✓ VERIFIED | `npm run build` exit 0; `go build ./...` exit 0 (binary ships the change). |

(All originally-verified artifacts from the initial pass remain verified — no regressions.)

### Key Link Verification (gap-closure additions)

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| routes.go | githubStatus handler | `mux.HandleFunc("GET /api/github/status", ...)` | ✓ WIRED | routes.go:25. |
| github.go (api) | github.Available() | call-time LookPath | ✓ WIRED | Returns false here (gh absent). |
| SettingsPage.tsx | GET /api/github/status | `useGithubStatus()` | ✓ WIRED | queries.ts:50; consumed at SettingsPage.tsx:65. |
| SettingsPage.tsx Switch | effectiveEnabled gate | `checked={effectiveEnabled}`, `effectiveEnabled = enabled && ghAvailable` | ✓ WIRED | Stale `checked={enabled}` removed. |
| SettingsPage.tsx onCheckedChange | enable guard | `if (checked && !ghAvailable) { setError(...); return; }` | ✓ WIRED | Blocks enable, surfaces install message. |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| GHSET-01 | 10-01, 10-03, 10-04, 10-05 | Toggle on/off, ON by default, gh-gated | ✓ SATISFIED | Default "on" + Switch wiring + gh-gate (truths 1, 5; Gap 1 closed) |
| GHSET-02 | 10-03, 10-05 | OFF hides all GitHub UI, accurate copy | ✓ SATISFIED (code); human-confirm full cascade | Repo field gated; no UI leak; corrected copy (truth 2; Gap 2 closed) |
| GHSET-03 | 10-02, 10-04, 10-05 | Best-effort; gh missing never breaks; enable guarded | ✓ SATISFIED (code + live endpoint); human-confirm UI round trip | Soft-save degrade + gh-gated toggle; endpoint reports gh_available:false live (truth 5; Gap 1 closed) |
| GHPRJ-01 | 10-02, 10-03 | Per-project config section | ✓ SATISFIED | Project settings dialog + ⋯ item (truth 3) |
| GHPRJ-02 | 10-02, 10-03 | Set/edit/clear description | ✓ SATISFIED | PATCH + TestUpdateProjectPartial (truth 3) |
| GHPRJ-03 | 10-02, 10-03 | Link/change/clear repo, gh-or-syntactic validation | ✓ SATISFIED | ValidateRepo + canonicalization (truth 4) |

No orphaned requirements: REQUIREMENTS.md maps exactly GHSET-01/02/03 + GHPRJ-01/02/03 to Phase 10, all claimed by plans and all verified.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | No TODO/FIXME/PLACEHOLDER/stub in any phase-10 file (incl. gap-closure files) | — | Clean |

Pre-existing lint (errors outside phase-10 files) logged to `deferred-items.md` — confirmed out of scope, unchanged.

### Human Verification Required

See `human_verification` frontmatter. Three items, all with code paths verified — these are the inherently-interactive browser confirmations: (1) the live gh-removed toggle behavior (Switch shows OFF + click-to-enable surfaces the install message; gh-restore round trip), (2) the OFF-cascade click-through across the whole app, (3) the origin prefill-on-open UX. Note: `gh` is absent on this host, so item (1)'s degrade direction is the live default; the gh-restored direction needs the binary back on PATH.

### Gaps Summary

Both gaps from the initial pass are now CLOSED and re-verified in code:

- **Gap 1 (gh-gated default + enable guard, GHSET-01/GHSET-03):** Backend `GET /api/github/status` (10-04, always-200, reports `gh_available` from call-time `github.Available()`, `TestGithubStatus` green) + frontend gh-aware toggle (10-05: `useGithubStatus()`, `effectiveEnabled = enabled && ghAvailable`, enable guard with install-gh message, fail-safe-to-OFF on loading/error/missing). Verified live: `gh` is absent on this host and the endpoint reports `gh_available:false`.
- **Gap 2 (over-claiming copy, GHSET-02):** `GITHUB_INTEGRATION_HELP` is now exactly `Show GitHub features across Kangent. Turn off to hide all GitHub UI.`; the `behaves exactly as it did before` clause is absent.

No regressions: `go build ./...`, `go test ./...`, and `cd web && npm run build` all green.

## Gaps

### Gap 1: `gh`-gated default + enable guard (GHSET-01 / GHSET-03) — RESOLVED (closed by 10-04 + 10-05)
- **Source:** Human UAT (user renamed `gh` → unavailable on host).
- **Requirements:** GHSET-01 (default state), GHSET-03 (degrade behavior).
- **Resolution (verified in code):**
  - Backend `GET /api/github/status` (internal/api/github.go) returns `{"gh_available": github.Available()}`, always 200, registered in routes.go:25; `TestGithubStatus` passes (and confirms `gh_available:false` on this gh-absent host).
  - Frontend `useGithubStatus()` (queries.ts) feeds `GithubSection`: `ghAvailable = ghStatus?.gh_available === true` (loading/error/missing → not-available), `effectiveEnabled = enabled && ghAvailable` drives `checked` (so a stale stored "on" never shows on), and `onCheckedChange` blocks an enable attempt when `gh` is missing, surfacing `Install the GitHub CLI (gh) before enabling GitHub integration.` instead of saving "on".
- **Status:** CLOSED. Remaining: the inherently-interactive browser confirmation (Switch visual OFF state + click-to-enable surfacing the message; gh-restore round trip) — see human_verification.

### Gap 2: Remove over-claiming toggle copy (GHSET-02) — RESOLVED (closed by 10-05)
- **Source:** Human UAT.
- **Requirement:** GHSET-02 (copy).
- **Resolution (verified in code):** `GITHUB_INTEGRATION_HELP` (SettingsPage.tsx:43) is now exactly `Show GitHub features across Kangent. Turn off to hide all GitHub UI.`; the trailing `— the app behaves exactly as it did before.` clause is absent from the file (grep confirmed).
- **Status:** CLOSED.

### Deferred (still inherently human-testable after gaps close)
- Origin prefill-on-open UX (GHPRJ-03 / D-08) — verified in code; confirm interactively. Tracked in `10-HUMAN-UAT.md`.

---

_Initial verification: 2026-06-13 (gaps_found)_
_Re-verified after gap closure: 2026-06-13 (passed)_
_Verifier: Claude (gsd-verifier)_
