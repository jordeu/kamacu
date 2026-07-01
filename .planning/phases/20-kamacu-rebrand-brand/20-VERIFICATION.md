---
phase: 20-kamacu-rebrand-brand
verified: 2026-07-01T12:44:44Z
status: passed
score: 5/5 must-haves verified
overrides_applied: 1
overrides:
  - must_have: "D-08: The collapsed icon rail shows the KamacuMark at the top of the rail, above the project avatars"
    reason: "Revised at the 20-04 blocking human-verify checkpoint per user feedback (commit 7b5c170): the collapsed-rail mark cluttered the rail, so the mark is now expanded-sidebar-only and the collapsed rail shows only the toggle. Roadmap SC3 does not require a collapsed-rail mark; the expanded lockup (mark + Kamacu wordmark) satisfies BRAND-01 / REBRAND-01."
    accepted_by: "user (20-04 human-verify checkpoint)"
    accepted_at: "2026-07-01T00:00:00Z"
re_verification:
  previous_status: none
  note: "Initial verification. A prior 20-VERIFICATION.md existed but was deleted in the working tree (git status shows deletion); no gaps: section was available to load, so this is treated as initial-mode verification."
---

# Phase 20: Kamacu Rebrand & Brand Verification Report

**Phase Goal:** The product presents itself as Kamacu everywhere — name, binary, Go module, logo, favicon, and a README — with runtime data paths/socket deliberately left unchanged (that flip belongs to Phase 21's gated migration).
**Verified:** 2026-07-01T12:44:44Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (Roadmap Success Criteria)

| # | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1 | Tab title, sidebar brand, page/section headers, settings/about copy read "Kamacu" — no user-facing "kangent" remains (REBRAND-01, REBRAND-03) | ✓ VERIFIED | `web/index.html:6` `<title>Kamacu</title>`; `ProjectSidebar.tsx:54-57` mark + "Kamacu" wordmark; `App.tsx:27` "Point Kamacu at…", `ProjectMenu.tsx:95` "removed from Kamacu", `SettingsPage.tsx:43` "across Kamacu". `grep -rn 'Kangent' web/src` = 0. Only lowercase `kangent` in web/src are the 4 keep-out localStorage keys / path-comment tokens. |
| 2 | `make build` produces a `kamacu` binary; module renamed + all imports updated; `go build`/`vet`/`test ./...` green (REBRAND-02) | ✓ VERIFIED | `go.mod` line 1 = `module kamacu`; 0 stale `"kangent/` imports; `cmd/kamacu/` exists, `cmd/kangent/` gone; `make backend` → executable `bin/kamacu`; `go build ./...` exit 0; `go vet ./...` exit 0; fresh `go test ./...` (cache cleared) — 12 packages all `ok`, exit 0. |
| 3 | UI brand area renders the new Kamacu logo (SVG) + browser tab shows the Kamacu favicon (BRAND-01, BRAND-02) | ✓ VERIFIED | `KamacuMark.tsx` inline-SVG, named export, `role="img"` + `aria-label="Kamacu"`, radial+linear gradients (multi-tone, not a flat amber dot), no `animate-*`; imported and rendered in `ProjectSidebar.tsx`. `favicon.svg` valid SVG, linked via `rel="icon" href="/favicon.svg"`, copied to `dist/favicon.svg` by Vite build. Screenshot confirms the ember mark + "Kamacu" render in the sidebar header. |
| 4 | Repo root README.md describing what Kamacu is, build/run, project→task→agent→review workflow (BRAND-03) | ✓ VERIFIED | `README.md` (73 lines): title Kamacu + kamaq naming story; prerequisites (Claude Code CLI, git, Go 1.26, Node 20.19+/22.12+, tmux); `make build` → single `bin/kamacu`; project→task→agent→review walkthrough; screenshot embed `![…](docs/kamacu-board.png)`. No `~/.kamacu` path refs; not the Vite template. `docs/kamacu-board.png` is a real 2810×1530 PNG, git-tracked. |
| 5 | Rebrand touches only code identity — `~/.kangent` data dir and `-L kangent` tmux socket still used at runtime; existing install keeps working (REBRAND-03) | ✓ VERIFIED | Keep-out literals byte-for-byte intact (see Keep-Out table). Human confirmed at the 20-04 blocking checkpoint that an existing `~/.kangent` install still opens. Screenshot's agent-terminal text shows a live `.kangent/worktrees/…` path — a correct, unchanged runtime path. |

**Score:** 5/5 roadmap success criteria verified

### Keep-Out Boundary Verification (Success Criterion 5 detail)

| Runtime literal | Location | Expected (still kangent) | Status |
| --------------- | -------- | ------------------------ | ------ |
| `DefaultSocket = "kangent"` | `internal/tmux/tmux.go:22` | unchanged | ✓ intact |
| `reposBase = "~/.kangent/repos/"` | `internal/api/projects.go:195` | unchanged | ✓ intact |
| `KeyWorktreeBase: "~/.kangent/worktrees/"` | `internal/settings/settings.go:31` | unchanged | ✓ intact |
| `fmt.Sprintf("kangent-%d-%d", …)` | `internal/api/sessions.go:302` | unchanged | ✓ intact |
| `~/.kangent/kangent.db`, `kangent-tmux.conf`, `~/.kangent/worktrees`, `HasPrefix(name,"kangent-")` | `cmd/kamacu/main.go:35,96,124,270` | unchanged | ✓ intact |
| `X-Kangent-Token` (auth header) | 6 sites (agent.go×2, hooks.go, agent_test, hooks_test×2) | unchanged | ✓ intact (6/6) |
| `kangent.sidebar`, `kangent:sessions-bar-collapsed`, `kangent:review-collapsed` | AppLayout/ActiveSessionsBar/ReviewColumn | unchanged | ✓ intact |

### Plan-Frontmatter Must-Haves (additional detail)

| # | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| P-01 | D-13 cosmetic prose sweep complete — no cosmetic `kangent` prose remains where safe | ✓ VERIFIED | Completeness grep (`kangent` minus keep-out/sentinel exclusions) = 0; log lines say "kamacu listening", "kamacu is listening", "kamacu serves a shell". |
| P-02 | `kangentSessionID` identifier renamed to `kamacuSessionID` | ✓ VERIFIED | 0 `kangentSessionID` matches; `kamacuSessionID` present at `agent.go:59,64`. |
| P-03 | D-01/D-03/D-04 ember mark abstract, multi-tone, distinct from amber waiting dot | ✓ VERIFIED (auto) + human-confirmed | Gradients + halo + ember particles; no flat amber circle, no `animate-pulse`. Human confirmed distinctness at 20-04 checkpoint. |
| P-04 | D-02 kamaq/kamacu naming story documented in the component | ✓ VERIFIED | JSDoc in `KamacuMark.tsx:8-14`. |
| P-05 | D-08 collapsed rail shows the mark | PASSED (override) | Revised at 20-04 UAT (commit 7b5c170): mark is now expanded-only per user feedback. Human-approved deviation; roadmap SC3 unaffected. |

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `go.mod` | `module kamacu` | ✓ VERIFIED | line 1 = `module kamacu` |
| `Makefile` | builds `bin/kamacu` | ✓ VERIFIED | `-o bin/kamacu ./cmd/kamacu`; `dev-backend` runs `./cmd/kamacu` |
| `cmd/kamacu/main.go` | renamed entrypoint | ✓ VERIFIED | dir exists, `cmd/kangent/` removed, builds |
| `web/src/components/brand/KamacuMark.tsx` | ember-spark SVG component | ✓ VERIFIED (WIRED) | named export; imported+rendered in ProjectSidebar |
| `web/public/favicon.svg` | spark-derived favicon | ✓ VERIFIED (WIRED) | valid SVG; linked in index.html; copied to dist/ |
| `web/index.html` | Kamacu title + favicon link | ✓ VERIFIED | `<title>Kamacu</title>` + `rel="icon" href="/favicon.svg"`; `class="dark"` preserved |
| `web/src/components/sidebar/ProjectSidebar.tsx` | brand lockup | ✓ VERIFIED | mark + "Kamacu" wordmark (expanded-only per D-08 revision) |
| `README.md` | shareable Kamacu README | ✓ VERIFIED | 73 lines, all required sections |
| `docs/kamacu-board.png` | branded screenshot | ✓ VERIFIED | real 2810×1530 PNG, git-tracked, shows Kamacu branding |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `ProjectSidebar.tsx` | `brand/KamacuMark.tsx` | import + render in SidebarHeader | ✓ WIRED | `import { KamacuMark } from "@/components/brand/KamacuMark"` + `<KamacuMark className="size-5 shrink-0" />` |
| `web/index.html` | `web/public/favicon.svg` | `<link rel="icon">` | ✓ WIRED | `rel="icon" type="image/svg+xml" href="/favicon.svg"`; asset copied into `dist/` |
| `README.md` | `docs/kamacu-board.png` | markdown image embed | ✓ WIRED | `![Kamacu board…](docs/kamacu-board.png)`; target file committed |
| `cmd/kamacu/main.go` | `kamacu/internal/*` | import block | ✓ WIRED | all internal imports resolve under `kamacu/internal/`; 0 stale `"kangent/` |
| `internal/tmux/tmux.go` | runtime tmux socket | unchanged `DefaultSocket` | ✓ WIRED (keep-out) | `DefaultSocket = "kangent"` preserved |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Backend compiles | `go build ./...` | exit 0 | ✓ PASS |
| Static analysis clean | `go vet ./...` | exit 0 | ✓ PASS |
| Full backend suite | `go clean -testcache && go test ./...` | 12 pkgs `ok`, exit 0 | ✓ PASS |
| Binary target | `make backend` | produces executable `bin/kamacu` | ✓ PASS |
| Frontend build | `cd web && npm run build` (tsc -b + vite build) | exit 0; `dist/favicon.svg` + `<title>Kamacu</title>` in dist | ✓ PASS |
| Smoke script points at renamed binary (WR-01) | `grep bin/kamacu scripts/smoke.sh` | line 3 + line 27 use `bin/kamacu`; no `bin/kangent` | ✓ PASS |

### Probe Execution

No conventional `scripts/*/tests/probe-*.sh` probes and no probe declarations in the PLANs. Backend `go test ./...` (green) and the frontend `npm run build` (green) serve as the phase's runnable verification. `scripts/smoke.sh` (STOR-01 restart-persistence) was not executed here (it needs a live server + `~/.kangent` install); its WR-01 breakage was fixed and confirmed by static check (uses `./bin/kamacu`).

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| REBRAND-01 | 20-02, 20-03 | Name shown as "Kamacu" everywhere brand appears | ✓ SATISFIED | tab title, sidebar wordmark, all user-facing copy |
| REBRAND-02 | 20-01 | Builds as `kamacu` binary; module renamed; build/vet/test green | ✓ SATISFIED | module kamacu, bin/kamacu, all Go checks green |
| REBRAND-03 | 20-01, 20-03 | Code identifiers/logs/strings say kamacu where safe; runtime compat deferred | ✓ SATISFIED | prose sweep complete; keep-out runtime literals intact |
| BRAND-01 | 20-02, 20-03 | New Kamacu logo (SVG) rendered in UI brand area | ✓ SATISFIED | KamacuMark rendered in sidebar header |
| BRAND-02 | 20-02 | Browser tab shows Kamacu favicon | ✓ SATISFIED | favicon.svg linked + embedded |
| BRAND-03 | 20-04 | Repo README describing Kamacu, build/run, workflow | ✓ SATISFIED | README.md + screenshot |

All 6 phase requirement IDs (REBRAND-01/02/03, BRAND-01/02/03) are accounted for. REQUIREMENTS.md maps exactly these 6 to Phase 20 — matches PLAN frontmatter with no orphaned or unmapped IDs. The MIGRATE-* set is correctly mapped to Phase 21 (not this phase).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| — | — | No TBD/FIXME/XXX debt markers in phase files | — | None |
| `web/src/api/types.ts` | 1,3,6 | `"todo"` string matches | ℹ️ Info | False positive — kanban Status enum values, not debt markers |
| `web/src/components/brand/KamacuMark.tsx` | 38 | Redundant `className?` member (review IN-01) | ℹ️ Info | Cosmetic; dispositioned Info in 20-REVIEW.md; non-blocking |
| `web/src/components/brand/KamacuMark.tsx` | 52-53 | Hardcoded `aria-label` may double-announce (review IN-02) | ℹ️ Info | a11y nit; non-blocking |
| `web/src/components/sidebar/ProjectSidebar.tsx` | 55 | Redundant `group-data-[collapsible=icon]:hidden` on inner span (review IN-03) | ℹ️ Info | Dead styling; parent already hides; non-blocking |

No blocker or warning-level anti-patterns. Code-review WR-01 (the one warning) was fixed and verified (commit 958fa47).

### Human Verification Required

None outstanding. Phase 20 included a **blocking** human-verify checkpoint (Task 2 of 20-04) that the user executed and **APPROVED**: tab title + ember-spark favicon = Kamacu; expanded sidebar lockup = mark + "Kamacu"; collapsed rail refined (mark removed per feedback, commit 7b5c170); no user-facing "Kangent" copy; ember mark visually distinct from the amber waiting dot; existing `~/.kangent` install still opens. All visual / real-time / existing-install items were confirmed at that checkpoint, so no new human items remain.

### Deferred Items (not phase gaps)

- **Runtime path/socket/localStorage/auth-header flip** — deliberately left as `kangent` (`~/.kangent`, `-L kangent` socket, `kangent-<task>` prefix, `kangent-tmux.conf`, `X-Kangent-Token`, `kangent.*` localStorage keys). Addressed in **Phase 21** (MIGRATE-01…05). This is the roadmap-locked keep-out boundary — a remaining `kangent` in these locations is correct, not a defect.
- **8 pre-existing gofmt-dirty Go files** and **20 pre-existing frontend ESLint errors** — verified present at the phase base commit, not caused by the rename, and do not gate `go build`/`vet`/`test` or the `tsc`/`vite` build. Logged to `deferred-items.md` for a dedicated cleanup pass. Out of scope for Phase 20's success criteria.

### Gaps Summary

No gaps. All five roadmap success criteria are verified against the codebase: the Go module/binary/imports/cmd-dir/Makefile are `kamacu` with `build`/`vet`/`test ./...` green; the `KamacuMark` SVG and `favicon.svg` render in the UI and browser tab; the tab title, sidebar wordmark, and all user-facing copy read "Kamacu" with zero user-facing "Kangent"; a concise shareable `README.md` with a post-rebrand screenshot exists; and every runtime path/socket/session-prefix/config-filename/auth-header/localStorage-key literal is byte-for-byte unchanged for Phase 21's gated migration. The single plan-level deviation (D-08 collapsed-rail mark → expanded-only) was a human-approved UAT refinement recorded as an override; it does not reduce roadmap scope. The one code-review warning (WR-01, smoke.sh) was fixed and verified.

---

_Verified: 2026-07-01T12:44:44Z_
_Verifier: Claude (gsd-verifier)_
