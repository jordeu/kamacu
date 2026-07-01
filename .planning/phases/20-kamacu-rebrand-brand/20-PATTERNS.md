# Phase 20: Kamacu Rebrand & Brand - Pattern Map

**Mapped:** 2026-07-01
**Files analyzed:** 5 buckets (Go identity rename, net-new brand assets, sidebar lockup, copy renames, README)
**Analogs found:** in-repo analogs for all frontend components; README + favicon are net-new (no analog)

> This is a rebrand phase, not a feature build. "Patterns to copy" here means:
> (a) the existing code idioms new/edited files must match, and
> (b) — most importantly — the **keep-out boundary** (D-14/D-15): the runtime
> literals and test assertions that MUST NOT be edited or `go test ./...` breaks.
> The planner should treat the "Keep-Out Boundary" section as a hard constraint,
> not advisory.

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `go.mod` (module line) | config | identity | self | n/a |
| 18 non-test `*.go` importing `kangent/…` | mixed (api/session/tmux/…) | identity rename | any file's import block | exact (mechanical) |
| 23 `*_test.go` importing `kangent/…` | test | identity rename | same | exact (mechanical) |
| `cmd/kangent/` → `cmd/kamacu/` (dir move: `main.go`, `main_test.go`) | entrypoint | identity rename | self | exact |
| `Makefile` build target/output | config | build | self | exact |
| Go comment/log **prose** "Kangent"→"Kamacu" | prose | n/a | see D-13 list below | exact |
| **NEW** logo mark component `web/src/components/brand/…tsx` | component | transform/render (static SVG) | `web/src/components/ui/ProjectAvatar.tsx` | role-match |
| **NEW** favicon `web/public/favicon.svg` | asset/config | static | *(no analog — net-new)* | none |
| `web/index.html` (`<title>` + `<link rel="icon">`) | config | n/a | self | exact |
| `web/src/components/sidebar/ProjectSidebar.tsx` `SidebarHeader` | component | render | self (lines 46–57) + `ProjectAvatar` rail/expanded idiom | exact |
| `web/src/App.tsx:27` copy | component | n/a | self | exact |
| `web/src/components/sidebar/ProjectMenu.tsx:95` copy | component | n/a | self | exact |
| `web/src/pages/SettingsPage.tsx:43` copy | component | n/a | self | exact |
| `web/src/api/types.ts:18`, `web/src/api/sessions.ts:14,16` comment copy | prose | n/a | self | exact |
| **NEW** `README.md` (repo root) | doc | n/a | *(no analog — see note)* | none |

**Verified counts (live codebase):**
- `53` `.go` files contain a `kangent` token (matches CONTEXT.md "~53").
- Of those, `41` import `"kangent/…"` and MUST have their import paths rewritten: **18 non-test + 23 test**.
- The remaining ~12 contain only prose/keep-out literals (handled per D-13/D-14 below).

---

## Pattern Assignments

### Go module + import rename (`go.mod`, 41 `.go` files, `cmd/` dir, `Makefile`)

**Analog:** self — this is a mechanical, project-wide `kangent/ → kamacu/` seam. The module name is a **bare name, not a URL** (`module kangent`), so the rename is a pure token swap.

**`go.mod` (line 1):**
```
module kangent   →   module kamacu
```

**Import block idiom (every importing file, e.g. `cmd/kangent/main.go:19`):**
```go
import (
	...
	"kangent/internal/api"      →  "kamacu/internal/api"
	"kangent/internal/session"  →  "kamacu/internal/session"
	"kangent/internal/tmux"     →  "kamacu/internal/tmux"
	// …
)
```
Rename target set (distinct import prefixes seen): `kangent/internal/{api,session,tmux,settings,worktree,github,diff,store,quota,ws,reaper}`. A single `sed`/gofmt-safe replace of `"kangent/` → `"kamacu/` across `**/*.go` covers all 41 files; `go build ./...` + `go vet ./...` is the check.

**`cmd/` package directory (D-13):** move `cmd/kangent/{main.go,main_test.go}` → `cmd/kamacu/`. Both files stay `package main` (Go package name is `main`, unaffected by dir name). After the move, prose that references the old path must follow (see "cosmetic path-doc updates" below).

**`Makefile` (verified current contents):**
```make
backend:
	go build -o bin/kangent ./cmd/kangent      # → -o bin/kamacu ./cmd/kamacu

dev-backend:
	go run ./cmd/kangent                        # → ./cmd/kamacu
```
`make build` must produce `bin/kamacu` (REBRAND-02).

---

### Go cosmetic prose rename (D-13) — product-name word only

**Analog:** self. Rename the **word** "Kangent"/"kangent" where it is the *product name* in a comment or log/error string, and NEVER where it is (or builds) a runtime literal (see Keep-Out Boundary).

**Safe prose sites (rename the word):**
- Logs/errors in `cmd/kangent/main.go`: `:57` `"SECURITY: kangent is listening …"`, `:208` `slog.Info("kangent listening", …)`, `:298` error `"…kangent serves a shell and must stay local"`.
- Comments referencing the product: `internal/quota/quota.go:2,5,47`; `internal/session/session.go:60`; `internal/api/hooks.go:30`; `internal/settings/tokenize.go:18`; `internal/api/sessions.go:68,132`; `internal/tmux/tmux.go:2,4,20,24,51`; `internal/session/manager.go:65,134`; `internal/api/tasks.go:135`; `internal/api/projects.go:32,137,138,142,512`.

**Safe identifier rename (D-13, "non-runtime code identifiers"):**
- `internal/session/agent.go:59,64` — param/var `kangentSessionID` → `kamacuSessionID` (holds a UUID, not the literal word; rename all references together).
- `internal/api/icons.go:66` — comment example `("kangent" -> "KA")`; if renamed, keep it consistent with the test fixture at `internal/api/icons_test.go:15` (both `kangent` and `kamacu` monogram to `"KA"`, so the test stays green either way — low value, optional).

**Cosmetic path-doc updates (do these AFTER the `cmd/` dir move, for accuracy):** comments that spell the old path `cmd/kangent/main.go` should become `cmd/kamacu/main.go`:
- `internal/ws/integration_test.go:2,27,215`; `internal/api/agent_integration_test.go:51`; `internal/api/integration_test.go:24`; `cmd/kamacu/main.go:116` (`go run ./cmd/kangent …` → `./cmd/kamacu …`).

---

### NEW logo mark component — `web/src/components/brand/KamacuMark.tsx` (component, static-render)

**Analog:** `web/src/components/ui/ProjectAvatar.tsx` — the closest existing "compact identity mark that must render legibly at tiny AND normal sizes" (exactly D-05/D-08's requirement). It is the ONLY in-repo primitive whose job is a small brand-ish glyph at multiple sizes.

**No SVG-asset-import convention exists.** Verified: `web/src` has **zero** `.svg` imports, no `?url`/`?raw` imports, no `web/src/assets/` dir. All icons come from `lucide-react` as inline-SVG React components. Therefore the idiomatic choice is an **inline-SVG React component** (like a lucide icon), not an imported `.svg` file. The favicon is the exception (must be a real file — see next).

**Component idiom to copy (ProjectAvatar.tsx lines 1–3, 25–46, 51–63, 75):**
```tsx
import * as React from "react";
import { cn } from "@/lib/utils";

/** JSDoc block explaining the mark, its sizes, and the D-04 constraint. */
interface KamacuMarkProps extends React.ComponentProps<"svg"> {
  className?: string;
}

function KamacuMark({ className, ...props }: KamacuMarkProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      role="img"
      aria-label="Kamacu"
      className={cn("<size + ember classes>", className)}
      {...props}
    >
      {/* ember spark geometry */}
    </svg>
  );
}

export { KamacuMark };
```
Conventions this pins: named export (not default), `cn()` for class merge, `...props` spread, sizing via Tailwind `size-*` classes on the element (ProjectAvatar uses `size-6`/`size-5`), `role="img"` + `aria-label` for a11y (matches StatusDot.tsx:59–61 and ProjectAvatar.tsx:54). Import via the `@/` alias (`@/*` → `./src/*`, verified in `tsconfig.app.json` + `vite.config.ts`).

**D-04 ember-vs-amber differentiation reference (MUST honor).** The "agent waiting" visuals the mark must NOT be confusable with — do a side-by-side against these exact classes:
- `web/src/components/StatusDot.tsx:20–21` — waiting dot: `bg-amber-400 animate-pulse motion-reduce:animate-none` (a plain 8px amber dot).
- `web/src/components/ui/ProjectAvatar.tsx:68` — rail waiting overlay: `size-2 rounded-full bg-amber-400`.
- `ProjectSidebar.tsx:126` — waiting count chip: `bg-amber-400/10 … text-amber-400`.

Implication: a flat `bg-amber-400` circle is exactly the waiting dot. The mark must be a **multi-tone glow/shape with motion of form** (e.g. gradient orange-500→amber-300, radial glow, spark/wisp silhouette), never a single-hue dot. There is **no** `--ember`/custom accent token in `index.css` (verified: theme is a muted oklch zinc palette; amber is only ever Tailwind's `amber-*` utilities), so the ember hue lives inside the SVG (gradient stops / fills) or as `text-orange-*`/`text-amber-*` utility classes — implementer's call within D-03.

---

### NEW favicon — `web/public/favicon.svg` (asset, static)

**No analog — net-new.** Verified: `web/public/` exists but is **empty**; `web/index.html` has **no** favicon link. This is a fresh Vite `public/` asset.

**Vite `public/` convention (this is the load-bearing fact for the planner):** files in `web/public/` are served at the site root, so `web/public/favicon.svg` is referenced as `/favicon.svg` (leading slash, NOT `@/` or a relative path — the `@/` alias only covers `./src/*`). It is copied verbatim into the build output (`web/dist/`) which the Go binary embeds via `web/embed.go` (`//go:embed all:dist` — the `all:` prefix already includes such files). D-06: derive from the spark mark, simplified for 16–32px, legible on light AND dark tab chrome. D-Discretion: one shared SVG source or two optimized variants is the implementer's call.

---

### `web/index.html` (config) — title + favicon link

**Analog:** self (verified full contents, 12 lines). Two edits in `<head>`:
```html
<title>Kangent</title>                    →  <title>Kamacu</title>
<!-- add, after <title>: -->
<link rel="icon" type="image/svg+xml" href="/favicon.svg" />
```
Keep the existing `<html lang="en" class="dark">` (dark theme is load-bearing for how the ember reads).

---

### `web/src/components/sidebar/ProjectSidebar.tsx` `SidebarHeader` (component, render)

**Analog:** self — the current `SidebarHeader` (lines 46–57) is the exact site being reworked (D-07/D-08), and the expanded/collapsed dual-render idiom to copy already lives lower in this same file.

**Current header (lines 47–50) — the `kangent` `<span>` to replace:**
```tsx
<SidebarHeader className="flex-row items-center justify-between group-data-[collapsible=icon]:justify-center">
  <span className="px-1 text-sm font-medium group-data-[collapsible=icon]:hidden">
    kangent
  </span>
```

**Collapsible dual-render idiom to copy (from this file's project rows, lines 95 & 116):**
- Expanded-only element: append `group-data-[collapsible=icon]:hidden` (line 116, and the current wordmark span at line 48).
- Collapsed-rail-only element: `className="hidden group-data-[collapsible=icon]:flex"` (line 95).

**Target composition (D-07 expanded = mark + "Kamacu" wordmark; D-08 collapsed = mark at top of rail):**
```tsx
<SidebarHeader className="…">
  {/* mark: shown in BOTH states (fits the ~3rem rail — the reason it must be compact) */}
  <KamacuMark className="size-5 …" />
  {/* wordmark: expanded only */}
  <span className="px-1 text-sm font-medium group-data-[collapsible=icon]:hidden">
    Kamacu
  </span>
  {/* keep the existing SidebarTrigger + Tooltip (lines 51–56) */}
</SidebarHeader>
```
Note casing D-09: **"Kamacu"** (title case) replaces today's lowercase `kangent`. The rail is ~3rem; the mark's compactness (D-08) is why the SVG must read at ~20px — mirror `ProjectAvatar` size `rail` = `size-6` / `inline` = `size-5` for sizing precedent.

---

### User-facing copy renames (trivial, self-analog)

Rename the visible product name **"Kangent" → "Kamacu"** at these exact verified sites:
- `web/src/App.tsx:27` — `"Point Kangent at a local git repository to get a board."`
- `web/src/components/sidebar/ProjectMenu.tsx:95` — `…will be removed from Kangent. The repository on disk is untouched.`
- `web/src/pages/SettingsPage.tsx:43` — `GITHUB_INTEGRATION_HELP = \`Show GitHub features across Kangent. …\``
- `web/src/api/types.ts:18` — comment `…true when Kangent cloned and owns the checkout under` (rename the word **Kangent**; the `~/.kangent/repos/` path token on line 19 is KEEP-OUT — see boundary).
- `web/src/api/sessions.ts:14` — comment `Kangent-initiated stop` → `Kamacu-initiated stop`; `:16` — `outlived a Kangent restart` → `Kamacu restart`.

---

### NEW `README.md` at repo root (doc)

**No usable analog in-repo.** `web/README.md` exists but is the **stale default Vite/React template** ("React + TypeScript + Vite") — do NOT copy it; it is unrelated boilerplate. The content source is **`./CLAUDE.md`** (project intro + the full recommended stack table) and the ROADMAP/REQUIREMENTS refs in CONTEXT.md.

Per D-10/D-11, the README must include (shareable-OSS framing):
- What it is / why (the *kamaq* animating-life-force story, D-02) — 1–2 lines.
- Prerequisites: **Claude Code CLI, git, Go 1.26, Node 20.19+/22.12+, tmux**.
- Build/run: `make build` → single `bin/kamacu` binary (from the reworked Makefile).
- One screenshot/GIF of the board + a task's agent terminal (**captured AFTER the rebrand lands** — D-12; likely a human step at phase end).
- A short project → task → agent → review workflow walkthrough.

---

## Shared Patterns

### Icon / mark rendering
**Source:** `lucide-react` usage across `web/src` (e.g. `ProjectSidebar.tsx:3` `import { Plus, Settings } from "lucide-react"`) + `ProjectAvatar.tsx`.
**Apply to:** the new `KamacuMark` component — inline SVG React component, named export, `cn()` + `...props`, `role="img"` + `aria-label`, sized by Tailwind `size-*`. No `.svg` file import (none exist in the codebase).

### Path alias & asset location
**Source:** `web/tsconfig.app.json:9–11` + `web/vite.config.ts` (`@` → `./src`).
**Apply to:** component imports use `@/components/brand/…`; the **favicon is the exception** — it lives in `web/public/` and is referenced as `/favicon.svg` (root-served, embedded via `web/embed.go` `//go:embed all:dist`).

### a11y label idiom
**Source:** `StatusDot.tsx:59–61`, `ProjectAvatar.tsx:54` (`role="img"` + `aria-label`).
**Apply to:** `KamacuMark` and any decorative-vs-labelled brand element.

### Go import block form
**Source:** `cmd/kangent/main.go:3–19` (stdlib group, blank line, then `kangent/internal/*` group).
**Apply to:** all 41 import-rewrite files — preserve grouping/ordering; `gofmt` after.

---

## Keep-Out Boundary (D-14 / D-15) — DO NOT EDIT — breaks `go test ./...`

> **This is the most important section for the planner.** These literals are (or
> construct) real runtime paths / socket / session-name prefix / data-dir filename,
> or are test assertions on them. Renaming any of these fails `go build`/`go test`
> or silently breaks runtime path resolution. They are **Phase 21** territory.
> Plans MUST scope edits to avoid these lines. When editing a file that also
> contains a keep-out literal, rename only the surrounding product-name *prose*.

### Runtime literals in source — KEEP verbatim
| File:Line | Literal | What it is |
|-----------|---------|------------|
| `internal/tmux/tmux.go:22` | `const DefaultSocket = "kangent"` | tmux `-L kangent` socket name |
| `internal/tmux/tmux.go:29` | `# kangent-managed tmux config …` (Config string body) | content written to `kangent-tmux.conf`; leave to be safe |
| `internal/settings/settings.go:31` | `KeyWorktreeBase: "~/.kangent/worktrees/"` | worktree base path default |
| `internal/api/projects.go:195` | `const reposBase = "~/.kangent/repos/"` | managed-repo clone base |
| `internal/api/sessions.go:302` | `fmt.Sprintf("kangent-%d-%d", …)` | tmux session-name prefix |
| `cmd/kangent/main.go:35` | `"~/.kangent/kangent.db"` | default SQLite path |
| `cmd/kangent/main.go:96` | `"kangent-tmux.conf"` | generated config filename (in data dir) |
| `cmd/kangent/main.go:124` | `"~/.kangent/worktrees"` | worktree root |
| `cmd/kangent/main.go:270` | `strings.HasPrefix(name, "kangent-")` | orphan-sweep session-prefix guard |
| `internal/worktree/worktree.go:84` | comment `~/.kangent/worktrees` | describes the real path — leave the path token |
| `internal/session/session.go:84` | comment `kangent-<task>-<n>` | documents the live session prefix — leave |

### Frontend runtime keys — KEEP verbatim (localStorage keys, D-14)
| File:Line | Literal |
|-----------|---------|
| `web/src/components/layout/AppLayout.tsx:8` | `SIDEBAR_STORAGE_KEY = "kangent.sidebar"` |
| `web/src/components/layout/ActiveSessionsBar.tsx:33` | `"kangent:sessions-bar-collapsed"` |
| `web/src/components/board/ReviewColumn.tsx:56` | `` `kangent:review-collapsed:${projectId}` `` |
| `web/src/api/types.ts:19` | path token `~/.kangent/repos/` inside the comment (rename only "Kangent" on line 18, keep the path) |

### Test assertions on real literals — KEEP verbatim (would turn `go test ./...` red)
| File | What it asserts |
|------|-----------------|
| `internal/api/projects_test.go:27,28,35` | `filepath.Join(home, ".kangent", "repos")` — **the D-15 canonical example** |
| `internal/settings/settings_test.go:29` | `"~/.kangent/worktrees/"` KeyWorktreeBase default |
| `internal/worktree/worktree_test.go:142` | `/home/u/.kangent/worktrees` path derivation |
| `internal/api/agents_test.go:36` | comment guards against hitting the real `~/.kangent` default |
| `internal/tmux/tmux_test.go:41` | `"kangent-tmux.conf"` filename; also `kangent-1-*` session names throughout |
| `internal/session/tmux_lifecycle_test.go:117,135,150,163,179,182,193` | `kangent-1-*` session names |
| `internal/api/sessions_test.go:697,698,727,767,807,839,874` | `kangent-<T>-n` session names |
| `internal/api/worktrees_test.go:409,443` | `kangent-*` / `kangent-%d-1` session names |

### AMBIGUOUS / RISKY — recommend KEEP unless renamed atomically both-ends
- **`X-Kangent-Token` HTTP header** — a live auth header on the agent→server hook wire, NOT in D-14's enumerated list but key-like in spirit (D-15). Sender: `internal/session/agent.go:23,63`; receiver: `internal/api/hooks.go:39`. Tests: `internal/session/agent_test.go:94`, `internal/api/hooks_test.go:87,103`. **Recommendation: KEEP** (treat as a runtime key). A `claude` session spawned by the old binary would 401 against a renamed header after restart — same cross-restart hazard D-14 avoids. If the planner insists on renaming, it MUST change all 5 sites (2 source + 3 test) in one atomic plan and accept the in-flight-session risk. Safer default: defer to Phase 21.

### Test-only sentinels — cosmetically safe but LOW-VALUE; default to leaving them
Renaming these keeps tests green (they are self-consistent, not product paths), but they carry no user-facing brand value, so leaving them minimizes churn/regression surface:
- `internal/session/session_test.go:144,147,148,156,159,184,194` — `echo kangent-marker` shell sentinel.
- `internal/api/icons_test.go:15` — fixture `{"single", "kangent", "KA"}` (`kamacu` also → `"KA"`).
- `internal/api/projects_test.go:765` — bogus repo slug `octocat/this-repo-does-not-exist-kangent-test`.

---

## No Analog Found

| File | Role | Reason | Planner guidance |
|------|------|--------|------------------|
| `web/public/favicon.svg` | asset | No favicon/`public/` asset exists yet | Use Vite `public/` convention (`/favicon.svg`, embedded via `web/embed.go`); derive from the spark mark (D-06) |
| `README.md` (repo root) | doc | No real README exists (`web/README.md` is a stale Vite template — do NOT copy) | Source content from `./CLAUDE.md` (stack table) + CONTEXT.md refs; structure per D-10/D-11 |
| logo mark SVG geometry | design | No brand mark exists; ember/spark is net-new visual design | Component *shape* follows `ProjectAvatar.tsx`; the geometry is open (D-01..D-05, discretion) |

---

## Metadata

**Analog search scope:** `web/src` (components, ui, api, layout, sidebar, pages), `web/{index.html,public,vite.config.ts,embed.go,tsconfig*}`, `go.mod`, `Makefile`, `cmd/`, `internal/{tmux,settings,api,session,worktree,quota,ws,reaper}` (source + `_test.go`).
**Files scanned:** ProjectSidebar.tsx, ProjectAvatar.tsx, StatusDot.tsx, App.tsx, ProjectMenu.tsx, SettingsPage.tsx, index.html, package.json, vite.config.ts, embed.go, tsconfig.app.json, web/README.md, go.mod, Makefile, cmd/kangent/main.go(+test); plus repo-wide grep of `kangent` across all `*.go`, `*.tsx`, `*.ts`.
**Pattern extraction date:** 2026-07-01
