---
phase: 20-kamacu-rebrand-brand
reviewed: 2026-07-01T12:35:29Z
depth: standard
files_reviewed: 11
files_reviewed_list:
  - cmd/kamacu/main.go
  - cmd/kamacu/main_test.go
  - web/index.html
  - web/public/favicon.svg
  - web/src/api/sessions.ts
  - web/src/api/types.ts
  - web/src/App.tsx
  - web/src/components/brand/KamacuMark.tsx
  - web/src/components/sidebar/ProjectMenu.tsx
  - web/src/components/sidebar/ProjectSidebar.tsx
  - web/src/pages/SettingsPage.tsx
findings:
  critical: 0
  warning: 1
  info: 3
  total: 4
status: issues_found
---

# Phase 20: Code Review Report

**Reviewed:** 2026-07-01T12:35:29Z
**Depth:** standard
**Files Reviewed:** 11
**Status:** issues_found

## Summary

Phase 20 is the Kamacu rebrand: a Go module/binary rename (`kangent` → `kamacu`,
`cmd/kangent` → `cmd/kamacu`, `go.mod` `module kamacu`) plus new frontend brand
assets (`KamacuMark` SVG, `favicon.svg`) and a sidebar brand lockup with
user-facing copy renamed.

Verification performed during review:
- `go build ./...`, `go vet ./cmd/kamacu`, and `go test ./cmd/kamacu` all pass.
- No stale `"kangent/..."` Go imports remain; every internal import resolves to
  `kamacu/internal/...`.
- `cmd/kamacu/main.go` and `cmd/kamacu/main_test.go` are **pure mechanical
  renames** of the old `cmd/kangent/*` files — after normalizing the brand token
  they are byte-identical except for `Kangent`→`Kamacu` comment text. No backend
  logic changed, so no new backend defects were introduced.
- The keep-out boundary is respected in `main.go`: `~/.kangent/kangent.db`,
  `~/.kangent/worktrees`, `kangent-tmux.conf`, and the `kangent-` tmux session
  prefix are all deliberately preserved for Phase 21. Not flagged.
- Frontend brand copy rename is complete: no user-facing `Kangent` strings remain
  in `web/src`. The residual `kangent:*` / `kangent.sidebar` strings are
  `localStorage` keys (persisted client state), correctly left alone as part of
  the same migration keep-out.
- `web/public/favicon.svg` is valid XML and carries `role="img"` + `aria-label`.

The rebrand is clean. The one substantive issue is a broken smoke-test script
that still invokes the pre-rename binary path; the remaining items are minor
frontend quality nits in the new brand components.

## Warnings

### WR-01: Smoke test invokes the pre-rename binary `./bin/kangent`, which no longer exists

**File:** `scripts/smoke.sh:27` (also the header comment at `scripts/smoke.sh:3`)
**Issue:** The directory/binary rename that is the core of this phase moved the
build output from `bin/kangent` to `bin/kamacu` (`Makefile:9`:
`go build -o bin/kamacu ./cmd/kamacu`). `scripts/smoke.sh` runs `make build`
(line 45) — which now produces `bin/kamacu` — but then launches the server via
`./bin/kangent --addr "$ADDR" --db "$WORK/k.db" &` (line 27). After this rename
`./bin/kangent` does not exist, so the smoke test's server never starts and every
subsequent HTTP assertion (the STOR-01 restart-persistence check the script
exists to prove) fails. This is a direct, provable consequence of the in-scope
`cmd/kamacu` rename; the file is outside the reviewed set but is broken by it.
**Fix:**
```sh
# scripts/smoke.sh:27
./bin/kamacu --addr "$ADDR" --db "$WORK/k.db" &
```
Also update the line 3 header comment (`...against bin/kangent.`) to `bin/kamacu`.
Grep the script for any other `bin/kangent` occurrences before closing.

## Info

### IN-01: Redundant `className` declaration in `KamacuMarkProps`

**File:** `web/src/components/brand/KamacuMark.tsx:37-39`
**Issue:** `interface KamacuMarkProps extends React.ComponentProps<"svg">` already
includes `className?: string`. The explicit `className?: string;` member on the
next line is redundant and can drift from the base type.
**Fix:** Drop the redundant member: `interface KamacuMarkProps extends React.ComponentProps<"svg"> {}` (or use the type directly).

### IN-02: Brand mark re-announces "Kamacu" to screen readers next to the visible wordmark

**File:** `web/src/components/brand/KamacuMark.tsx:52-53`, consumed at `web/src/components/sidebar/ProjectSidebar.tsx:54-57`
**Issue:** `KamacuMark` hardcodes `role="img"` + `aria-label="Kamacu"`. In the
sidebar lockup it renders immediately before a visible `<span>Kamacu</span>`, so
assistive tech announces "Kamacu" twice ("Kamacu Kamacu"). When an icon sits next
to redundant visible text it is conventionally decorative.
**Fix:** Either let callers mark the mark decorative in the lockup
(`<KamacuMark aria-hidden className="size-5 shrink-0" />`, which the `{...props}`
spread already allows), or gate the built-in `aria-label` so a caller-supplied
`aria-hidden`/`aria-label` isn't doubled up. The standalone favicon keeps its own
label, so this only affects the paired lockup.

### IN-03: Redundant `group-data-[collapsible=icon]:hidden` on the inner wordmark span

**File:** `web/src/components/sidebar/ProjectSidebar.tsx:53-58`
**Issue:** The outer lockup `<span>` already carries
`group-data-[collapsible=icon]:hidden`; the inner wordmark `<span>` repeats the
same class. Because the parent is already hidden in icon mode, the class on the
child never changes rendering — it is dead styling that can mislead future edits.
**Fix:** Remove `group-data-[collapsible=icon]:hidden` from the inner wordmark
`<span>` (line 55); the parent controls collapse visibility.

---

_Reviewed: 2026-07-01T12:35:29Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
