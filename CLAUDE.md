<!-- GSD:project-start source:PROJECT.md -->
## Project

**Kangent**

A local-only web app for organizing Claude Code agent sessions around projects and tasks. A left sidebar lists projects (each pointing at a local git repo checkout); the main area is a kanban board of tasks. Clicking a task expands a view with the main agent session (Claude Code CLI running in a PTY, rendered in a browser terminal) plus optional tabs with bash sessions. Single user, runs locally, accessed from a browser at localhost.

**Core Value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.

### Constraints

- **Tech stack**: Go backend, React frontend — user's choice
- **Deployment**: Single binary/process serving API + static frontend at localhost — local-only by design
- **Storage**: Local database (e.g., SQLite), no external services
- **Agent**: Claude Code CLI must be installed on the host; the app spawns it, never reimplements it
<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->
## Technology Stack

## Recommended Stack
### Core Technologies — Backend (Go)
| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go | 1.26.x (1.26.4 current) | Backend language | Current stable line (1.26.0 released 2026-02). Use 1.26 in go.mod; nothing in this project needs bleeding-edge features beyond 1.22's ServeMux patterns. |
| `net/http` stdlib ServeMux | stdlib | HTTP routing | Since Go 1.22, ServeMux supports method matching and path wildcards (`GET /api/projects/{id}`). For ~15 REST endpoints + 1 WS endpoint on localhost, a third-party router adds nothing. Zero dependencies, no framework lock-in. |
| `github.com/coder/websocket` | v1.8.14 | WebSocket (terminal attach) | The 2025/2026 default for new Go projects: concurrent-write safe (gorilla panics on concurrent `WriteMessage` — the classic production bug), `context.Context` throughout, zero deps, actively maintained by Coder (who run terminals-over-WS in production at scale). Formerly nhooyr/websocket. |
| `github.com/creack/pty` | v1.1.24 | PTY allocation | The de-facto standard Go PTY library (1,263 importers, used by gotty and most Go terminal tools). `pty.Start(cmd)` handles setsid + controlling TTY; `pty.Setsize` handles resize + SIGWINCH. **Import the v1 path** — see "What NOT to Use" re: the orphaned v2 tags. |
| `modernc.org/sqlite` | v1.52.0 | SQLite driver (pure Go) | CGO-free (C-to-Go transpiled), implements `database/sql`, tracks SQLite 3.53.2. Pure Go keeps the single-binary build trivial (`CGO_ENABLED=0`, easy cross-compile, no gcc in CI). Performance gap vs mattn is irrelevant at single-user localhost load. |
| `embed` + `http.FileServerFS` | stdlib | Ship React build in the binary | `//go:embed dist` of the Vite output, served via `http.FileServerFS` with an SPA fallback handler (unknown paths → `index.html`). This is the standard single-binary Go+SPA pattern; no library needed. |
| `os/exec` + git CLI | system git | Worktree management | Shell out to `git worktree add/remove/list --porcelain`. See dedicated section below — go-git is not viable for linked worktrees today. |
### Core Technologies — Frontend (React)
| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| React | 19.2.7 | UI framework | User-decided. React 19 is the current stable line; shadcn/ui and TanStack Query fully support it. |
| Vite | 8.0.16 | Build tool / dev server | The unambiguous standard for React SPAs in 2026. Dev-server proxy (`/api`, `/ws` → Go backend) makes local dev clean. Requires Node 20.19+ or 22.12+. |
| `@vitejs/plugin-react` | 6.0.2 | React fast-refresh | Standard companion plugin. |
| TypeScript | 6.0.3 | Type safety | Default for any new React codebase. |
| `@xterm/xterm` | 6.0.0 | Browser terminal | The only serious choice (VS Code's terminal). 6.0 (Dec 2024) removed the canvas renderer — use WebGL with DOM fallback. Note the scoped `@xterm/*` packages; the old unscoped `xterm` package is dead. |
| `@xterm/addon-fit` | 0.11.0 | Resize terminal to container | Required; on fit, send cols/rows over WS so the server calls `pty.Setsize`. |
| `@xterm/addon-webgl` | 0.19.0 | GPU rendering | Default renderer since canvas was removed in xterm 6.0. Listen for `onContextLoss` and fall back to the DOM renderer. |
| `@tanstack/react-query` | 5.101.0 | Server state (projects/tasks CRUD) | Standard data-fetching layer: cache, invalidation after mutations (move card → PATCH → invalidate board query), optimistic updates for drag-and-drop. |
| `@dnd-kit/core` + `@dnd-kit/sortable` | 6.3.1 / 10.0.0 | Kanban drag-and-drop | The 2026 default for React DnD: actively maintained, accessible, the documented multi-container/sortable pattern is exactly a kanban board. react-beautiful-dnd is dead. |
| Tailwind CSS | 4.3.0 | Styling | v4 (CSS-first config, `@theme`) is current; pairs with the Vite plugin (`@tailwindcss/vite`). |
| shadcn/ui | CLI latest | Component primitives | Fully supports Tailwind v4 + React 19 (components are copied in, not a dependency — no version pin to track). Gives dialog/dropdown/card/tabs for the task view without building primitives. |
### Supporting Libraries
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/pressly/goose/v3` | v3.27.1 | SQL migrations | Embed migration files via `embed.FS`, run `goose.Up` at startup. Schema is small but will evolve; migrations from day one are cheap. |
| `sqlc` | v1.31.1 | Type-safe query codegen | Optional. With ~3 tables (projects, tasks, sessions), hand-written `database/sql` is fine; adopt sqlc if the query surface grows. Works with the modernc driver. |
| `zustand` | 5.0.14 | Ephemeral client state | Only if needed — e.g., which task panel/tab is open, terminal attach status. Do NOT put server data here; that's TanStack Query's job. |
| `@xterm/addon-web-links` | 0.12.0 | Clickable URLs in terminal | Nice-to-have; Claude Code prints URLs (auth, docs). |
| `@xterm/addon-search` | 0.16.0 | Scrollback search | Nice-to-have, defer. |
| `log/slog` | stdlib | Structured logging | No logging library needed. |
### Development Tools
| Tool | Purpose | Notes |
|------|---------|-------|
| Makefile / Taskfile | Build orchestration | `vite build` → `web/dist` → `go build` with embed. One `make build` produces the single binary. |
| Vite dev proxy | Local dev | Proxy `/api` and `/ws` to the Go server (`ws: true` for the WebSocket route) so dev runs frontend HMR + real backend. |
| `golangci-lint` | Go linting | Standard. |
| `air` or `wgo` | Go hot reload in dev | Optional convenience. |
## Key Design Patterns the Stack Must Support
- `SessionManager` holding `map[sessionID]*Session` behind a mutex.
- Each `Session` owns the `*exec.Cmd`, the PTY `*os.File`, a fixed-size **ring buffer of raw output bytes** (256KB–1MB) for scrollback replay, and a set of attached WebSocket clients.
- One goroutine per session reads the PTY and fans out to attached clients + ring buffer. Sessions live independently of WS connections — that *is* the detach/reattach feature.
- On reattach: send ring buffer contents, then live stream. Claude Code is a full-screen TUI that redraws on resize, so a resize nudge after replay cleans up any artifacts.
- On exit: `cmd.Wait()` in a goroutine to reap; mark session dead, notify clients; recovery is `claude --resume` (per PROJECT.md).
## Git Worktrees: Shell Out, Don't Use go-git
- git is guaranteed present — the app's whole premise is local repo checkouts the user already works in.
- The needed surface is tiny: `git worktree add -b <branch> <path> <base>`, `git worktree remove <path>`, `git worktree list --porcelain` (stable machine-readable output), `git worktree prune`, `git rev-parse --git-common-dir`.
- This is what comparable tools do (vibe-kanban drives real git for worktrees; every CLI wrapper in this space shells out for worktree ops).
- Set `cmd.Dir` to the repo root; parse `--porcelain` output; never parse human-readable git output.
## Installation
# Backend (inside Go module)
# Frontend
## Alternatives Considered
| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| stdlib ServeMux | `go-chi/chi` v5.3.0 | If middleware stacks grow (request logging, panic recovery, compression composed per-route). chi is 100% net/http-compatible, zero-dep — the safe upgrade path if stdlib routing feels cramped. Echo/Gin/Fiber are overkill and pull you off net/http idioms. |
| coder/websocket | `gorilla/websocket` v1.5.x | If you want the largest body of examples/tutorials. It was un-archived in 2023 and is maintained again, but its API predates context and you must serialize writes yourself — exactly the bug class a multi-client fan-out invites. |
| modernc.org/sqlite | `mattn/go-sqlite3` (CGO) | If profiling ever shows the driver as a bottleneck (it won't here). Costs CGO: gcc required, slower builds, harder cross-compile. |
| modernc.org/sqlite | `ncruces/go-sqlite3` (WASM-based) | Also pure-Go-ish and well-benchmarked; legitimate alternative if you hit a modernc bug. modernc has broader adoption and the simpler mental model. |
| dnd-kit | `@atlaskit/pragmatic-drag-and-drop` 1.8.1 | If you need Jira/Trello-scale board performance (1000+ cards) or non-React surfaces. Lower-level: you implement collision/indicators yourself. Atlassian-backed and excellent, but more work for a fixed-4-column board. |
| TanStack Query | SWR | Smaller, fine for read-heavy apps; TanStack's mutation + optimistic-update story is stronger, which kanban dragging needs. |
| goose | `golang-migrate` | Equivalent capability; goose's library-mode + embed.FS integration is simpler for a run-at-startup design. |
## What NOT to Use
| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `github.com/creack/pty/v2` | Orphaned major: v2.0.1 tagged Oct 2023, then development continued on v1 (v1.1.24 shipped a year *later*, Oct 2024); the repo's main-branch go.mod is still module `github.com/creack/pty`, and the README documents v1. The entire ecosystem imports v1. | `github.com/creack/pty` v1.1.24 |
| go-git for worktrees | Stable v5 cannot create linked worktrees at all; v6 support is alpha-only, in an `x/` experimental package, with known performance and commondir bugs. | `os/exec` + system git, `--porcelain` output |
| `xterm` (unscoped npm package) | Abandoned name; stuck at 5.3.0. All current releases are under the `@xterm/` scope. | `@xterm/xterm` 6.0.0 |
| `@xterm/addon-canvas` | Removed in xterm.js 6.0. | `@xterm/addon-webgl` with DOM-renderer fallback |
| `@xterm/addon-attach` | Assumes a raw socket; no room for resize/control messages or replay-on-attach logic. | Hand-rolled WS glue (binary data frames + control messages) |
| `react-beautiful-dnd` | Archived by Atlassian (superseded by pragmatic-drag-and-drop); incompatible with React 19-era rendering. | dnd-kit |
| gorilla/websocket *for this app* | Concurrent-write panic foot-gun in fan-out scenarios (multiple browser tabs attached to one session). | coder/websocket |
| WebSocket **text** frames for PTY output | PTY output is arbitrary bytes; UTF-8 validation on text frames corrupts/rejects split multi-byte sequences. | Binary frames |
| SDK-driven chat UI (Agent SDK) | Already ruled out in PROJECT.md — loses plan mode, slash commands, permission prompts. | Real `claude` CLI in a PTY |
| Electron/Tauri packaging | Out of scope per PROJECT.md. | Single Go binary + browser |
## Stack Patterns by Variant
- Run Vite dev server with proxy → Go backend on another port; only embed `dist/` for release builds. Guard the embed with a build tag or always-build `dist` in CI so `go build` never fails on a missing directory (commit a placeholder or use `make build`).
- Persist session metadata (task ID, worktree path, claude session ID if parseable) in SQLite; on restart mark sessions dead and surface a "Resume" affordance that starts `claude --resume` in the worktree. Don't attempt PTY process adoption — not feasible cleanly.
- xterm 6.0's DOM renderer is the built-in fallback; wire `webglAddon.onContextLoss(() => webglAddon.dispose())`.
## Version Compatibility
| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| `@xterm/xterm` 6.0.0 | `@xterm/addon-fit` 0.11.0, `@xterm/addon-webgl` 0.19.0, `@xterm/addon-web-links` 0.12.0, `@xterm/addon-search` 0.16.0 | Current addon releases target 6.x; pin the set together. Canvas addon gone in 6.0. |
| Vite 8.0.16 | Node 20.19+ / 22.12+ | Hard requirement; CI must use a matching Node. |
| Tailwind 4.3.0 | shadcn/ui (current CLI), `@tailwindcss/vite` | shadcn fully migrated to Tailwind v4 + React 19; use the Vite plugin, not PostCSS config. |
| `modernc.org/sqlite` v1.52.0 | `modernc.org/libc` (exact version from its go.mod) | Do not independently bump `modernc.org/libc`; mismatches break the driver (upstream issue #177). Let `go mod tidy` resolve it. |
| `@dnd-kit/core` 6.3.1 | `@dnd-kit/sortable` 10.0.0, React 19 | Current pairing; the experimental `@dnd-kit/react` rewrite is not yet the stable recommendation. |
| Go 1.26 | `creack/pty` v1.1.24, `coder/websocket` v1.8.14, `goose` v3.27.1 | All actively released within the past ~18 months; no known conflicts. |
## Reference Implementations
| Project | Stack | What to Learn From It | Status (verified 2026-06) |
|---------|-------|----------------------|---------------------------|
| [vibe-kanban](https://github.com/BloopAI/vibe-kanban) | Rust (axum, sqlx+**SQLite**) + React/TS | Closest architectural twin: worktree-per-task lifecycle (incl. orphan/expired worktree cleanup), task→executor→session data model, local-only single binary. **Note: project is sunsetting** — read it, don't depend on it. 26.9k stars. | Sunsetting |
| [SlayZone](https://github.com/debuglebowski/SlayZone) | Electron + React + SQLite + node-pty + xterm.js | The UI/UX target per PROJECT.md: card → embedded terminal → agent model, worktree per card. Its node-pty/xterm wiring maps 1:1 to creack/pty + @xterm/xterm. | Active (v0.34.0, June 2026) |
| [gotty (sorenisanerd fork)](https://github.com/sorenisanerd/gotty) | Go + creack/pty + xterm.js | The canonical Go terminal-over-WebSocket implementation: PTY read loop, WS relay, resize handling. The original yudai/gotty is dead — read the fork. | Active (v1.8.0, May 2026) |
| [ttyd](https://github.com/tsl0922/ttyd) | C + libwebsockets + xterm.js | Best-in-class wire protocol design: one-byte command prefix on frames (INPUT, OUTPUT, RESIZE, PAUSE/RESUME), flow control. Copy the protocol shape, not the code. | Active |
## Sources
- [pkg.go.dev/github.com/coder/websocket](https://pkg.go.dev/github.com/coder/websocket) — v1.8.14, Sep 2025; feature set (HIGH)
- [websocket.org Go guide](https://websocket.org/guides/languages/go/) + [Go Forum: WebSocket in 2025](https://forum.golangbridge.org/t/websocket-in-2025/38671) — coder vs gorilla consensus (MEDIUM)
- [github.com/creack/pty](https://github.com/creack/pty) + [pkg.go.dev/github.com/creack/pty/v2](https://pkg.go.dev/github.com/creack/pty/v2) + raw go.mod — v1.1.24 current, v2 orphaned (HIGH)
- [pkg.go.dev/modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — v1.52.0 (Jun 2026), SQLite 3.53.2, libc pinning caveat (HIGH)
- [go-sqlite-bench](https://github.com/cvilsmeier/go-sqlite-bench) — driver benchmark landscape (MEDIUM)
- [go-git releases](https://github.com/go-git/go-git/releases) + [x/plumbing/worktree docs](https://pkg.go.dev/github.com/go-git/go-git/v6/x/plumbing/worktree) + [go-git#1956](https://github.com/go-git/go-git/issues/1956) — v5.19.1 stable, v6.0.0-alpha.4 worktree support experimental (HIGH)
- [xterm.js releases](https://github.com/xtermjs/xterm.js/releases) — 6.0.0 breaking changes, canvas removal, addon matrix (HIGH)
- npm registry (direct API queries, 2026-06-10) — all frontend versions (HIGH)
- GitHub releases API (2026-06-10) — goose v3.27.1, sqlc v1.31.1, chi v5.3.0 (HIGH)
- [go.dev/doc/devel/release](https://go.dev/doc/devel/release) — Go 1.26.4 current (HIGH)
- [ui.shadcn.com/docs/tailwind-v4](https://ui.shadcn.com/docs/tailwind-v4) — Tailwind v4 + React 19 support (HIGH)
- [pkgpulse dnd comparison](https://www.pkgpulse.com/guides/dnd-kit-vs-react-beautiful-dnd-vs-pragmatic-drag-drop-2026) + [HN discussion](https://news.ycombinator.com/item?id=40149120) — dnd-kit as 2026 default (MEDIUM)
- [github.com/BloopAI/vibe-kanban](https://github.com/BloopAI/vibe-kanban) + crates/db/Cargo.toml — Rust+SQLite confirmed; sunsetting notice (HIGH)
- [github.com/debuglebowski/SlayZone](https://github.com/debuglebowski/SlayZone) — Electron/node-pty/xterm stack, active (HIGH)
- [github.com/sorenisanerd/gotty](https://github.com/sorenisanerd/gotty) — maintained Go reference, v1.8.0 May 2026 (HIGH)
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd:quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd:debug` for investigation and bug fixing
- `/gsd:execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd:profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
