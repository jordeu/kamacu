# Milestones

## v1.13 Global Task (Shipped: 2026-08-28)

**Phases completed:** 5 phases (13–17), 13 plans, 24 tasks

**Delivered:** A single global scratchpad — a `/global` view with agent + bash tabs running directly in a configured repo/folder, fully decoupled from projects, workspaces, and boards. 115 commits over 4 days (2026-08-25 → 2026-08-28), 220 files, +21k/−19k LOC. Milestone audit: 19/19 requirements, integration 25/26, 6/6 E2E flows (see milestones/v1.13-MILESTONE-AUDIT.md).

Known verification overrides: 6 (all prior-milestone carried-over quick tasks, none v1.13 — see STATE.md Deferred Items)

**Key accomplishments:**

- Two goose migrations — the `global_task` singleton (CHECK id=1, agent FK RESTRICT, seeded from the movable is_default flag) and the `tmux_sessions` scope rebuild (nullable task_id + XOR CHECK via CREATE-copy-drop-rename under NO TRANSACTION) — proven byte-for-byte-safe by staged-upgrade tests on the real goose runner
- BackfillGlobalTask (idempotent boot re-arm of the singleton), the scope-aware one-query orphan-sweep fix (RED-proven to kill a live global tab before the fix, GREEN after), the D-09 Scratchpad delete-guard 409, and a byte-for-byte real-install rehearsal of the real binary against a .backup copy of the live v1.12 database
- GET/PUT /api/global over the Phase-13 singleton: validated folder root, gh-validated managed clone into ~/.kamacu/repos/global/&lt;owner&gt;/&lt;name&gt; with reattach and atomic failure, settable default agent, and the forward-wired live-session 409 gate with resume-id clearing — zero new dependencies
- Live-tmux 409 gate + config-history invariants proven in-package, and the real `kamacu serve` binary driven through the complete /api/global curl contract on a fresh isolated install
- Global agent spawns behind the D-33/D-34/D-31 gates with singleton csid persist, a closure-parametrized opencode capture re-target, and /api/agents/status widened to ONE source:"global" entry in both passes — closing the SC3 co-phasing mandate with the task paths byte-for-byte clean.
- TS wire widenings (source union, TermSession.global, ApiError.reasons), the new global config client (["global"] query + partial PUT), six scope-aware session hooks keyed ["sessions","global"], and the Active Sessions bar's three GINT-01 edits (sessionId re-key, /global navigation branch, pathname highlight)
- The /global Scratchpad view: a copy-then-trim GlobalTaskPage rendering the 7-state matrix off the shared ["global"] query (D-39/D-40 heroes, D-38/D-42 banner slot), AgentTab loosened behind an AgentTabScope discriminator (task path byte-for-byte unchanged), and the /global route registered as the bar's click-through destination
- The Settings Scratchpad section: a server-truth summary card (root row with managed badge, instant-save default-agent Select, Open Scratchpad CTA to /global) plus the AddProjectDialog-derived Change-root dialog with segmented capture, blocking clone spinner, verbatim 409-reasons surfacing, and the gated no-confirm Clear root action
- The repo's first real-binary lifecycle E2E — builds cmd/kamacu in-test, spawns it in a fully isolated sandbox, drives the SC1 reconfigure-gate cycle and the SC2 SIGTERM→restart→resume narrative over real HTTP, and proves tmux tab survival with byte-identical labels.
- Permanent regression proofs that the global entity cannot leak onto any enumeration surface (live-agent + live-tmux harness, DB structural counts), that the managed-root ↔ project-delete interlock holds in both directions (same-ref clone into both namespaces + os.Stat), and that the MCP bridge is leak-free and honestly labeled — plus the 10-row D-55 audit table for 17-VERIFICATION.md.
- Full-suite green restored in both environment postures by stripping inherited `KAMACU_*` (incl. the hook-token secret) from the custom-agent spawn env, absorbing the documented PTY marker flake via 10s deadlines, and making the frontend honest about the opencode engine (union widening + claude-only quota chip).
- GSESS-02's opencode half proven through a real serve restart with the real binary — a command-PATCHed wrapper agent records the argv while capture, the resumable flip and the `-s ses_<id>` resume append run verbatim — plus the authored-pending 17-UAT.md walkthrough (six locked flows, honest safety posture, D-12 settlement procedure).

---

## v1.12 Activity & Statistics (Shipped: 2026-08-25)

**Phases completed:** 4 phases (10, 11, 12, 12.1-inserted), 9 plans, 20 tasks

Known verification overrides: 6 (all prior-milestone carried-over quick tasks, none v1.12 — see STATE.md Deferred Items)

**Key accomplishments:**

- `GET /api/activity` combined endpoint — scoped/windowed tasks-done, reviews-done (merged/closed via gh with bounded concurrency, 5min-TTL cache), in-Go cycle/dwell/median stats, and the always-200 degrade contract when GitHub integration is off
- Pure-helper + wire-type contract layer (mismatched-precision ISO parser, scope/window parsing, duration slices, review-state rollup; TanStack query, adaptive duration formatter, localStorage scope/window hook) — fully unit-tested
- Dedicated `/activity` page: route, ScopeSelector, Week/Month toggle, StatsStrip, grouped tasks/reviews lists, sidebar entry — plus the nil-slice "black page" fix (`tasks:[]` never `null` on every path)
- Daily stacked-bar ActivityChart (recharts ^3.8.0 via shadcn ChartContainer; sole new dep) with zero-day baseline stubs, sparse Month x-axis, per-bar hover tooltips, explanatory stat tooltips, and absolute formatDateTime timestamps
- Tech-debt closure (Phase 12.1, UAT 3/3): wired the dead `useActivity` `enabled` gate (WR-01), stale-scope useMemo demotion to global that never persists (WR-02), and restored PR titles to ReviewsList accessible names (WR-03)

---

## v1.11 Kamacu MCP Server (Shipped: 2026-07-29)

**Phases completed:** 4 phases, 11 plans, 22 tasks

**Key accomplishments:**

- `cmd/kamacu/main.go` slimmed to a 14-line `google/subcommands` dispatcher; today's entire server body moved verbatim into `cmd/kamacu/serve.go`'s `serveCmd`; bare `kamacu` now prints help and exits non-zero (D-04 break-clean).
- `internal/mcp/` package built on `github.com/modelcontextprotocol/go-sdk` v1.6.1 — `kamacu mcp serve` speaks JSON-RPC over stdio, registers exactly one production tool (`list_projects`), and is wired into the dispatcher via Pattern 2 nested Commander. The SC2 malformed-tool regression test (D-11) proves error handlers emit clean JSON-RPC error responses on stdout — never crashes, never stream desync.
- Single coordinated sweep renaming `X-Kangent-Token` → `X-Kamacu-Token` across 10 source occurrences in 7 files (3 production + 4 test), preserving byte-for-byte security posture (32-byte token, constant-time compare, fresh-per-start re-injection) with zero fallback.
- Three Kamacu endpoint additions (D-01 GET /api/tasks, D-02 ?workspace_id= on GET /api/projects, Gap 1 GET /api/projects/{id}) plus the D-07 per-resource split (tasks.go/projects.go/workspaces.go) and bridge.call shared helper — laying the foundation Plans 02/03/04 each fill one file.
- Six task MCP tools (list_tasks / get_task / create_task / update_task / move_task / delete_task) bridging to Kamacu's task endpoints — each a thin build-path-and-delegate to bridge.call, with 13 httptest cases and a Rule 1 fix widening bridge.call from HTTP 200-only to full 2xx.
- Four project MCP tools (get_project / create_project / update_project / delete_project) bridging to Kamacu's project endpoints — each a thin build-path-and-delegate to bridge.call, with 12 httptest cases covering the D-04 two-arg fork, D-03 partial-PATCH with excluded fields, and the Gap 2 managed-delete 409 structured body.
- Five workspace MCP tools (list_workspaces / create_workspace / update_workspace / delete_workspace / move_project_to_workspace) bridging to Kamacu's workspace endpoints plus the v1.9 PATCH /api/projects/{id} transfer surface — each a thin build-path-and-delegate to bridge.call, with 10 httptest cases covering both v1.9 guarded-delete 409 messages and the cross-route transfer's {workspace_id}-only body.
- Read-only HTTP endpoints (get_session / get_session_output / subscribe / list project_id filter) backing the four MCP session tools, with type-level read-only enforcement and no new long-lived goroutines
- Three non-streaming MCP session tools (list_sessions / get_session / get_session_output) bridged to the Plan 01 Kamacu endpoints, with a panic-recovery wrapper closing Phase 06 Open Question 1 ahead of Plan 03's streaming
- subscribe_session_output (MCPSESS-04) — the streaming panic surface — delivered with the SC3 cancellation/leak regression gate proven via the SDK in-memory transport + a fake streaming server; ctx cancel returns a partial D-08 envelope and detaches within 2s, goroutine count is stable across N cycles
- 3 MCP tools (list_pending_reviews, list_recently_reviewed, open_review) bridging Kamacu's existing v1.3/v1.5 GitHub-integration HTTP surface — the milestone's mechanical coda: pure translation, zero new endpoints/gh-calls/gates

**Known deferred items at close:** 6 (the pre-close artifact audit flagged 6 quick tasks as open; all 6 are carried over from prior milestones v1.3–v1.7 — none are v1.11 work. See STATE.md → Deferred Items.)

**Requirements:** 23/23 v1.11 requirements complete (MCPPROC·MCPTASK·MCPSESS·MCPREV·MCPPROJ). No formal milestone audit was run (override closeout — the 6 flagged items are all prior-milestone stale quick tasks).

---

## v1.10 Configurable Agents (Shipped: 2026-07-11)

**Phases completed:** 5 phases, 20 plans, 18 tasks

**Key accomplishments:**

- `agents` table (migration 00013, mirroring 00012 exactly) + `projects.agent_id` NOT NULL REFERENCES … ON DELETE RESTRICT FK backfilling every existing project to the Claude Code seed (is_system=1, is_default=1, engine='claude') in-SQL, plus an idempotent `BackfillAgents` startup hook — the whole data layer reuses the v1.9 workspaces pattern end-to-end.
- `/api/agents` CRUD + set-default (create/edit/delete custom agents, block-until-unassigned + is_system non-deletable, exactly-one-default transactional invariant) reusing the workspaces.go handler shape; the claude seed's engine locked but name+command editable.
- Spawn engine forked on `SpawnOpts.AgentEngine` — the claude path is byte-for-byte unchanged (`TestAgentLifecycle` fake-claude regression passes unchanged, retiring the key risk), while the custom path renders the command template (tokenize-first-then-substitute, `{{worktree}}`/`{{session_id}}` placeholders) in the worktree PTY with running/exited status only (D-M001-2).
- Agent management UI — a Settings Agents section (ordered list + engine badges + add/edit/delete/set-default reusing v1.9 dialog shapes), a per-project agent selector, and the Active Sessions bar LIVE filter widened to include `running` so custom-agent sessions appear.
- Extra Claude params (`--dangerously-skip-permissions`) consolidated from an orphaned global setting onto the Claude agent's own row (migration 00014 + `BackfillAgentExtraParams` copying the effective setting — no one loses their config), edited in the Claude agent's edit dialog; the spawn path reads `extra_params` from the joined agent row.
- opencode seeded as a second non-deletable built-in engine (migration 00015 + `BackfillOpenCodeAgent`, engine='opencode') with its own spawn branch (exempt from the custom-render tokenize path, `KAMACU_*` hook env injected for activity-based status).
- The opencode status plugin's `notify()` rewritten from a curl shell-out (a silent no-op when curl was absent, so waiting/idle never unlocked) to `fetch()`+`AbortController(3s)` — a load-bearing defect fix proven by a build-tagged real-opencode e2e harness (plugin loads in opencode 1.17.15, POSTs reach a receiver, env-gate no-ops).
- opencode restart-resume: `tasks.opencode_session_id` (nullable, migration 00016) persists the opaque `ses_…` id; `captureOpencodeSessionAsync` discovers it via an async bounded poll (best-effort, warn-only, Done-channel-gated); `--resume` argv closes the restart-resume loop.

**Known deferred items at close:** 6 (the pre-close artifact audit flagged 6 quick tasks as open; all 6 are from prior milestones v1.3–v1.7 — none are v1.10 work. See STATE.md → Deferred Items.)

**Requirements:** 18/18 v1.10 requirements complete (AGDATA·AGMGMT·AGSPAWN·AGUI·OCENG·OCRESUME). No formal milestone audit was run (override closeout — the 6 flagged items are all prior-milestone stale quick tasks).

---

## v1.9 Workspaces (Shipped: 2026-07-06)

**Phases completed:** 2 phases, 9 plans, 22 tasks

**Key accomplishments:**

- Migration 00012 adds a `workspaces` table + a DB-enforced `projects.workspace_id NOT NULL … REFERENCES … ON DELETE RESTRICT` FK, creating the protected default Personal (id=1) and assigning every existing project to it in-SQL — with a real staged-upgrade test proving no project/task data is lost.
- `workspace_id` now ships on the projects read wire (GET/POST/PATCH) and both create paths resolve and set the default Personal workspace, so every new project satisfies the NOT NULL FK.
- Every boot now guarantees a default Personal workspace exists via the idempotent `BackfillWorkspaces` startup hook, wired right after `BackfillProjectIcons` — the thin invariant-guard backstop for WSDATA-02.
- The `/api/workspaces` CRUD surface — list (name-sorted), create-by-name (case-insensitive dup → 409), rename (Personal included), and a guarded delete that refuses the default and non-empty workspaces server-side — all proven green under `go test`.
- Projects transfer between workspaces via an optional validated `workspace_id` on the existing PATCH, and new projects land in the requested (active) workspace with a safe fall-back to Personal — no new route (D-17).
- The interface-first workspace contract layer: `Workspace` type + `workspace_id` on `Project`, `useWorkspaces()` query, workspace CRUD + project-move mutations, and a single shared `ActiveWorkspaceProvider` context (localStorage-persisted, is_default-fallback) mounted in AppLayout.
- The full user-facing workspace-management surface — an expanded-only switcher dropdown (✓ active marker, switch-navigation), a mode-switched create/rename dialog, and a Manage-workspaces hub with client-side-guarded delete — all consuming the plan-03 data layer, awaiting mount in plan 06.
- Routing and project creation now honor the active workspace: the index redirect lands on the active workspace's first project (or a workspace-scoped empty state), a deep-linked project flips the active workspace to its own (URL wins), and new projects are created into the active workspace.
- The workspace layer is wired into the sidebar end-to-end: the switcher sits atop the expanded sidebar, both project surfaces filter to the active workspace, projects transfer (and the open one follows) from the ⋯ menu, and the cross-workspace Active Sessions bar is confirmed untouched — human-verified across all 10 checks after two post-gate fixes.

**Known deferred items at close:** 8 (the pre-close artifact audit flagged 8 quick tasks as open; all 8 are completed + committed — a false positive on the status-marker check, matching the v1.7 close. See STATE.md → Deferred Items.)

**Requirements:** 12/12 v1.9 requirements complete (WSDATA·WSMGMT·WSNAV·WSPROJ·WSBAR). No formal milestone audit was run; the final phase passed a live human-verify gate (23/23 must-haves).

---

## v1.8 Kamacu Rebrand & UX Polish (Shipped: 2026-07-04)

**Phases completed:** 5 phases, 26 plans, 53 tasks

**Key accomplishments:**

- Go module, imports, entrypoint dir, and Makefile target renamed kangent -> kamacu; cosmetic prose + the kangentSessionID identifier say Kamacu — while every runtime path/socket/session/config/auth literal stays kangent for Phase 21's gated flip.
- An abstract multi-tone ember-spark `KamacuMark` inline-SVG component, a spark-derived rounded-badge `favicon.svg`, and a `web/index.html` that titles the tab "Kamacu" and links the favicon.
- Shareable open-source Kamacu README with a post-rebrand board screenshot, plus a human-confirmed end-to-end verification that the ember-spark branding renders correctly and is distinct from the amber waiting dot.
- One-shot idempotent localStorage prefix-scan migration copies every kangent-prefixed value to the kamacu key on boot, and the sidebar / sessions-bar / review-collapse components now read the new keys — completing the rebrand's client-side carry-over (MIGRATE-04).
- `internal/migrate` package: a pure 5-branch Gate decision table plus `Prepare` — gate → preflight → atomic `os.Rename` of `~/.kangent`→`~/.kamacu` → checkpoint-first `kangent.db`→`kamacu.db` rename → idempotent `-L kangent` tmux retirement, failure-safe by construction.
- Wires the two-part `internal/migrate` one-shot into `cmd/kamacu/main.go` (Prepare before `store.Open`, Complete after `store.Migrate`, refuse-to-boot on either error) and flips every remaining runtime `kangent` literal — `--db` default, `-L kamacu` socket, `kamacu-<task>-<n>` session prefix, tmux config header/filename, and the `~/.kamacu` worktree + repos roots — so the running app switches atomically to `~/.kamacu`.
- Status:
- `migrate.repairWorktrees` now repairs each managed worktree PER PATH and tolerates a DB-referenced dir that exists on disk but is unregistered in `.git/worktrees/` — logging a terminal-visible `slog.Warn` and skipping it instead of aborting the whole migration — so the real `sched` repo's 4 stale dirs no longer make `Complete` return an error, `deleteOldTmuxRows` (MIGRATE-03) always runs, and the app boots (Gap 1 closed).
- Re-ran the MIGRATE-01..05 phase gate against a provably-isolated copy of the real 5.5 GB ~/.kangent install: the migration now completes and serves (21-06 stale-worktree fix proven live), the real install stayed byte-identical (Gap 2 closed), and the preserved human-verify checkpoint was approved.
- Broadened `~/.kangent` → `~/.kamacu` worktree repair to folder-pointed (managed=0) repos so their moved worktrees survive a later `git worktree prune`, and boundary-checked the LIKE-gated path rewrites so a `_`/`%` home-path metacharacter or a sibling `~/.kangent-backup` dir can never be corrupted.
- Server-authoritative sha256 `Hash` (and a `Viewed` field) on `diff.File`, computed via a pure length-prefixed `hashFile` helper over the rendered diff (Status/Binary/OldPath/Hunks), keying the per-file Viewed persistence and driving DIFF-04 auto-reset.
- Server-side keep-history per-file diff "Viewed" state: a SQLite table keyed by (task, path, rendered-diff hash) that survives restart (DIFF-03) and auto-resets when a file's rendered diff changes while restoring on revert (DIFF-04), merged onto GET diff and toggled via a validated PUT endpoint.
- Blue-500 "Viewed" checkbox wired to an optimistic useToggleViewed mutation, sitting as a sibling of the collapse trigger in a sticky DiffFileSection header — checking collapses+dims, unchecking re-expands, collapse stays independent, and a new content hash remounts the section un-viewed+expanded.
- GitHub "Files changed" two-pane diff view: a fixed 288px path-compressed file tree whose scroll-only clicks jump the right pane, with an IntersectionObserver scroll-spy that highlights the top file and a PanelLeft toggle to hide/show the tree.
- A `git worktree list --porcelain -z` parser (typed Entry records) plus two additive extensions to the shared removal core — orphan mode (taskID==0 skips the null-columns UPDATE) and a D-01 permission-blocked outcome (BlockedError carrying the offending path, never retrying --force).
- The panel's HTTP contract: one annotated cross-project GET (`/api/worktrees`) enumerating + classifying every non-main worktree with per-row dirty/unpushed/stash/session flags and PR-state display, plus force-remove / bulk clean-eligible / clear-pointer actions — the 3rd caller of the shared `CleanupWorktreeGated`, wired in main.go with the shared `ghSvc`.
- Settings worktree-cleanup panel: per-project grouped worktree list with classification/flag chips, type-gated force-remove, a copyable-but-never-run sudo hint for permission-blocked shells, and a non-destructive bulk clean-eligible preview — fetch-on-open with a spinning manual Refresh, no polling.
- shadcn `badge` primitive + the TanStack Query data layer (fetch-on-mount no-poll `useWorktreeList` + three settle-invalidating mutation hooks) and the full TypeScript type set encoding the Wave 2 worktree-cleanup endpoint contract
- Outcome:
- PATCH /api/sessions/{id} renames a session label in memory and (for tmux tabs) persists it to tmux_sessions.label so it survives a restart, plus a belt-and-braces D-04 fix that guarantees a restarted survivor never shows the "Bash ?" sentinel.
- Collapsed active-sessions bar drops the total count and now auto-collapses on an outside click via the shared collapse() helper; the To Do column's inline "+ New task" quick-add is removed and QuickAdd.tsx is deleted.
- The agent view's standalone Stop button is replaced by a single ⋯ dropdown (Insert description / Insert review prompt / Stop), and the PR-review seed's connect-time auto-paste is deleted so the seed enters only when the user picks "Insert review prompt".
- Double-clicking a bash/tmux tab label swaps it to an inline Input (Enter commits, Esc cancels, blur commits); committing PATCHes /api/sessions/{id} so the custom name persists (and, for tmux, survives a restart), while an empty commit resets the tab to its Bash N default — Agent/Description/Diff keep fixed labels.

---

## v1.7 Project Icons in Collapsed Sidebar (Shipped: 2026-06-19)

**Phases completed:** 2 phases (18–19), 6 plans, 12 tasks

**Delivered:** Every project now has a colored monogram avatar (two uppercase letters on a curated-palette background) that makes projects identifiable and switchable directly from the collapsed sidebar — which previously slid fully off-screen, leaving no project reference. The avatar renders both as a clickable collapsed-rail icon (active highlight, name tooltip, amber waiting badge) and inline beside the project name when expanded; letters and color auto-derive at creation (name initials + random palette pick) and are editable from Project settings. Backend was minimal — two new `projects` columns with migration/backfill and the PATCH path to edit them — the rest was frontend.

**Key accomplishments:**

- Backend data foundation (Phase 18): new `internal/api/icons.go` as the single source of truth — `projectPalette`, `deriveLetters` (name → ≤2 uppercase monogram), `pickColor` (random palette member), and server-side `validateIconLetters`/`validateIconColor`; migration 00009 adds `icon_letters` + `icon_color` columns with an idempotent post-`Migrate` `BackfillProjectIcons` so every pre-existing project gets non-blank letters + a stable color; both create paths auto-assign at INSERT and the partial-PATCH handler validates+persists both (ICON-01..04).
- Shared `<ProjectAvatar>` monogram primitive (rail|inline sizes, rail-only active state + static amber waiting dot) plus the `PROJECT_PALETTE` TS const mirroring the Go `projectPalette` byte-for-byte — the Wave-1 dependency root every Phase 19 surface consumes.
- The collapsed sidebar is now a 3rem icon rail of per-project circular monogram avatars (filled-row active highlight, side=right name tooltip, static amber waiting badge); the same avatar appears inline beside the name when expanded; the floating re-expand trigger is retired (ICON-05..10).
- Project settings gained a live `<ProjectAvatar>` preview, an "Initials" input (client-normalized to ≤2 uppercase alphanumerics, mirroring the server rule), and a 9-swatch `PROJECT_PALETTE` color grid with the current color ring+check-marked — wired into the existing conditional PATCH so `icon_letters`/`icon_color` are sent only when changed, with saved edits propagating to the rail + expanded sidebar (ICON-11/12).

**UAT revisions shipped as the new contract:** circular avatars (reversed the planned `rounded-md` square, D-04); rail-row filled-highlight active state instead of a ring (reversed D-06); and a **muted desaturated palette** replacing the bright Tailwind-600 hues — changing both the Go source of truth and the TS mirror, plus a follow-up migration `00010_muted_palette.sql` that remapped existing rows by palette position.

**Known deferred items at close:** 4 completed quick tasks (`260613-osu`, `260613-ph5`, `260616-8l7`, `260618-mlu`) from earlier milestones (v1.3/v1.5/v1.6) lingered in `.planning/quick/` and were flagged by the pre-close artifact audit; each has a `SUMMARY.md` with a completion date — verified done, not gaps (audit metadata false-positive).

---

## v1.6 Global Active Sessions Bar (Shipped: 2026-06-18)

**Phases completed:** 1 phases, 2 plans, 5 tasks

**Key accomplishments:**

- The agent-status feed now carries `taskTitle` + `projectName` on every entry (both the manager-derived and post-restart passes) via a `JOIN projects`, and the TS `AgentStatusEntry` type exposes them — the sole backend change for the Global Active Sessions Bar (SBAR-10), with no new endpoint and no new migration.
- A persistent, collapsible bottom bar (`ActiveSessionsBar.tsx`) mounted globally in `AppLayout` shows every LIVE Claude agent session across all projects — collapsed it renders per-state counts (working/waiting/idle) + total with the waiting count amber-pulsing only when > 0; expanded it floats a panel UP over content (a fixed overlay that never reflows the xterm terminals) listing live sessions attention-first (waiting → working → idle), each row click-through to that task's agent view including cross-project, with collapse state persisted in localStorage and ~5s freshness off the existing poll. Covers SBAR-01..SBAR-09.

---

## v1.5 Sharper Review Column (Shipped: 2026-06-17)

**Phases completed:** 1 phases, 2 plans, 6 tasks

**Delivered:** The Review column now carries two independent at-a-glance signals per PR — your agent's session state (a colored left rail: green working / pulsing-amber waiting / blue idle / gray exited) and the PR's CI state (a bare glyph: green check / red cross / static amber circle; no-checks renders nothing) — split onto separate visual channels so they never confuse, plus a "Recently reviewed" section that keeps PRs you've reviewed (`reviewed-by:@me`, approve or request-changes) visible until they merge or close. Audit: 9/9 requirements, 4/4 integration seams, 3/3 E2E flows. Milestone-time redesign: the originally-planned agent dot was replaced by the left rail at the human-verify gate (a dot beside the CI glyph clashed).

**Key accomplishments:**

- Second `reviewed-by:@me draft:false` gh search riding the existing per-repo TTL cache, a `PRLists` combined runner with server-side top-precedence dedup, a widened `{prs, reviewed, state, stale, fetchedAt}` Result, and `/api/agents/status` entries carrying `prNumber`/`source` — plus the matching frontend wire types — with zero visual changes.
- The Review column's two at-a-glance signals split onto separate channels: your agent's state as a 3px colored LEFT RAIL (working green / waiting amber-pulse / idle blue / exited gray) and the PR's CI as a bare lucide glyph (green Check / red X / static amber Circle, `none` renders nothing) — plus a quiet-omitted "Recently reviewed" subsection reusing the same PRCard, all server-deduped against the awaiting list.

---

## v1.4 Repo-First Projects (Shipped: 2026-06-15)

**Phases completed:** 2 phases, 7 plans, 13 tasks

**Delivered:** When GitHub integration is on, add a project by naming a GitHub repo — Kangent `gh repo clone`s and manages the checkout under `~/.kangent/repos/<owner>/<name>` on the default branch — with the folder path as the optional fallback (folder-only when GitHub is off). Task worktrees branch off the managed checkout; the clone is gated-removed on project delete; clone failures degrade-don't-break with no half-created project; existing folder-based projects are untouched. Audit: 10/10 requirements, 6/6 integration seams, 5/5 E2E flows.

**Key accomplishments:**

- **Managed Checkout Foundations (Phase 14):** migration 00008 adds the `managed` marker (existing folder rows backfill to never-touch); `github.Clone` wraps `gh repo clone` (exit-0-only, remove-on-failure, faked-runner test seam); a second `POST /api/projects` path gh-validates `owner/name` (RPROJ-05), clones into `~/.kangent/repos/<owner>/<name>`, and INSERTs `managed=1` + `github_repo` ONLY after exit 0 (atomic — no orphan row/partial dir), with origin-matched reattach (refuse-without-clobber on mismatch).
- **Fresh worktrees off the managed clone (Phase 14):** managed task/PR-review worktrees best-effort `git fetch origin <default>` before `ResolveBase` so new work starts from the freshest tip — gated on the `managed` marker so folder projects keep their no-network guarantee, and a failed fetch is discarded so it never blocks task creation.
- **All-or-nothing gated managed delete (Phase 14):** a two-pass delete gates every task/PR worktree AND the clone root on dirty/unpushed (`origin/<default>..HEAD`, no fetch)/stash/running-session, refuses with a 409 `{reasons:[…]}` list removing nothing, and on all-clear tears down linked worktrees → `os.RemoveAll` the clone → deletes the rows; folder (`managed=0`) delete stays byte-for-byte unchanged (never touches the dir).
- **Repo description auto-capture (Phase 15):** repo-first create auto-captures the GitHub repo description via a best-effort `gh repo view --json description` read and persists it into `projects.description` (degrade-don't-break, never blocks; `ValidateRepo`'s shared signature untouched).
- **Repo-first Add-project UI (Phase 15):** integration-gated "GitHub repo | Local folder" segmented toggle (repo default, reusing `ui/tabs.tsx`); the `owner/name` input prefills the editable Name; submit drives the Phase-14 atomic create with a blocking "Cloning <owner/name>…" spinner; clone failures surface inline (dialog open, values preserved, no half-created project); folder mode byte-for-byte and the whole repo-first UI vanishes when integration is off. Human-verify gate approved end-to-end.

---

## v1.3 GitHub PR Review (Shipped: 2026-06-14)

**Phases completed:** 4 phases, 19 plans, 46 tasks

**Delivered:** Surface the GitHub PRs that need your review on a linked project's board and open each as a full task-like review workspace — a worktree on the PR's branch with the same agent/bash/diff tabs — then auto-clean the worktree when the PR merges/closes. All via the host's already-authenticated `gh` (no tokens stored), best-effort and degrade-don't-break throughout.

**Key accomplishments:**

- **GitHub Foundations (Phase 10):** migration 00007 lands all five v1.3 schema columns (`projects.description`/`github_repo`, `tasks.source`/`pr_number`/`pr_base_ref`) so Phases 11–13 need no further migration; the degrade-don't-break `internal/github` leaf (`ParseRepoRef`/`ValidateRepo`/`Available`); a gh-gated `/settings` integration toggle (OFF + un-enableable when `gh` is absent, via always-200 `GET /api/github/status`) with a full OFF cascade; and a Project settings dialog editing an origin-prefilled, soft-validated `owner/name` link + description.
- **PR Review Column (Phase 11):** a self-gating, collapsible per-project "Review" column listing `user-review-requested:@me draft:false` open PRs via ONE cached `gh pr list` call (server-side `statusCheckRollup` → pass/fail/pending/none, no N+1), behind an always-200 `GET /api/projects/{id}/pull-requests` endpoint; 60s visibility-paused auto-poll + manual refresh, inline loading/empty/degraded states, default-collapsed, appended outside the dnd machinery so PR cards never enter the kanban.
- **Open-a-Review (Phase 12) — the milestone headline:** clicking a PR card find-or-creates a `source='github_pr'` task rendered through the same TaskPage/agent/bash/diff shell, backed by a worktree on the PR's REAL head branch (`fetch refs/pull/<n>/head` + `worktree add -b`, `pr/<n>` collision fallback — never `gh pr checkout`); reopening reattaches (no duplicates); the diff computes against the PR's own base merge-base (matches GitHub Files-changed); a 5-query `source='manual'` board-leak guard + `/move` 409 keeps PR reviews off the board; fork/colliding-branch PRs open with the primary checkout HEAD provably unchanged (GHREV-05).
- **PR Worktree Auto-Cleanup (Phase 13):** the Phase-9 reaper gains a second `reconcilePRsOnce` pass that reads each PR's state (`gh pr view --json state`) and gated-removes a merged/closed worktree ONLY when pristine + idle (dirty / unpushed via `rev-list FETCH_HEAD..HEAD` / stash / running session each skip), always keeping the branch; the gated logic was extracted byte-equivalent into a shared `CleanupWorktreeGated` (HTTP DELETE + reaper, `force=false`); manual cleanup re-adds a PR `⋯` "Clean up worktree" item + a merged/closed banner.
- **Cross-phase integrity:** one shared `github.Service` flows through all three downstream phases (list cache, PR detail/checkout, reaper PRState); audit confirmed 20/20 requirements satisfied, 6/6 integration seams wired, 4/4 E2E flows complete. Every blocking human-verify checkpoint (11/12/13) was approved by the user.

---

## v1.2 Quota & Resumable Shells (Shipped: 2026-06-13)

**Phases completed:** 3 phases, 12 plans, 29 tasks

**Key accomplishments:**

- Demand-driven token-keyed quota cache (`internal/quota`) proxying Anthropic's OAuth usage endpoint at always-200 `GET /api/usage`, with the full six-state degradation matrix proven by 20 unit tests and zero new dependencies
- "Claude 5h" trigger with width=5h/color=max-of-all threshold bar, server-driven hover popup with reset countdowns and manual refresh, mounted in both page headers polling /api/usage every 60s while visible — zero new npm dependencies
- Phase 7 verified end-to-end — whole-repo green, token-leak audit clean, live /api/usage 200 — plus three user-feedback polish passes on the quota popup (live ticking footer, compact icon-free reset column, equal-width comparable bars), human-approved
- Socket-isolated tmux Client (new-session -A / has-session / kill-session with =name exact match, 5s exec timeouts, idempotent kill) plus the identity-only tmux_sessions table — all later tmux work calls through this one package
- settings.AllowedShells is now a call-time LookPath function: the shell dropdown offers "tmux" only while the binary resolves on PATH, and save-time validation tracks the exact same truth — zero frontend changes
- tmux threaded through the session package as a spawn-time lifecycle property: SpawnOpts.TmuxName spawns `tmux new-session -A` under the existing PTY pipeline, Stop is killer-first (kill-session, signal fallback), and waitExit discriminates exit-vs-detach via has-session — with bash/agent stop paths proven byte-identical
- End-to-end invisible tmux: POST /api/sessions with shell=tmux mints and persists `kangent-<task>-<n>` from the tmux_sessions table, attaches it under creack/pty on the dedicated `-L kangent` socket, and surfaces honest 409 spawn errors verbatim in the tab header — with zero tmux markers reaching the UI or the wire (D-77).
- `done_session_ttl` global setting (Go-style duration, default 24h; empty/0/never disable) with a shared `ParseDoneSessionTTL` helper that is the single source of truth for both save-time validation and the 09-05 reaper, plus a Cleanup field on the settings page.
- A background ticker goroutine that kills bash + tmux + agent sessions of tasks left in Done past `done_session_ttl` (clocked from `done_at`), keeping every agent resumable and never touching worktrees — the codebase's first background goroutine.

---

## v1.1 Settings & Polish (Shipped: 2026-06-11)

**Phases completed:** 1 phases, 4 plans, 10 tasks

**Key accomplishments:**

- SQLite-backed settings KV store (migration 00004) with code defaults, per-key validation carrying the UI-SPEC canonical error copy, branch-template expansion + git ref validation, quote-aware extra-params tokenizer, and the GET/PUT REST surface
- All four settings wired into their v1.0 call sites: tokenized claude extra-params append to every agent spawn (fresh + resume), bash tabs run the LookPath-resolved settings shell, and provisionWorktree builds template-driven branches under the settings worktree base with a create-time ref-format defense — all read-at-use, managers DB-free.
- Full /settings page with per-field commit (blur/Enter, Esc revert, 2s Saved flash, Reset to default, verbatim inline server errors), sidebar gear with active state, API-driven shell select, and the UI-01 full-width task header
- Phase 6 gate passed: release binary built clean with the settings page embedded, full Go suite green across 8 packages, 8/8 verbatim copy audit, restart-persistence smoke proven, and all seven v1.1 success criteria approved live by the user.

---

## v1.0 MVP (Shipped: 2026-06-11)

**Phases completed:** 5 phases, 28 plans, 74 tasks

**Key accomplishments:**

- SQLite store with WAL/foreign_keys/MaxOpenConns(1) discipline, embedded goose migrations creating projects/tasks schema, and a runnable server binary serving /api/healthz at 127.0.0.1:7333
- All 10 JSON endpoints (projects CRUD with git rev-parse path validation, tasks CRUD, move) with server-computed fractional ordering proven stable under a 200-insert renormalization stress test
- Vite + React 19 + Tailwind 4 SPA with dark-only zinc shadcn theme, full typed TanStack Query API layer (10 hooks incl. optimistic move), and the 3-route shell feature plans build against
- Collapsible projects sidebar with add/rename/delete flows: mono-path Add dialog mirroring server validation errors inline, AlertDialog delete with repo-untouched copy, and localStorage-persisted collapse
- dnd-kit kanban board with four fixed columns, optimistic drag persistence through the move endpoint, click-to-open cards, and dual task creation (quick-add row + 560px dialog with n shortcut)
- Deep-linkable full-page task view with the TabDef[]-driven tab strip seam, GFM markdown edit/preview description, inline title editing, and confirmed hard delete
- One binary serves the full kanban app: //go:embed'd Vite build with SPA-fallback routing, Makefile pipeline, and a scripted kill/restart persistence proof — human-approved end to end
- Server-side bash PTY session engine with 1 MiB ring-buffer replay, atomic attach/detach, and session-wide SIGTERM→5s→SIGKILL teardown proven to leave zero orphaned subprocesses
- xterm 6 set pinned exactly, Vite /api proxy made WebSocket-capable (ws: true), UI-SPEC zinc theme encoded as xtermTheme.ts, and typed TanStack Query session hooks (list/spawn/stop/delete) built against the fixed REST contract
- ttyd-style binary WebSocket bridge (input/output/replay/resize-jiggle/exit frames) over coder/websocket, session REST endpoints, and the full TERM-07 model: loopback-bind refusal, Host middleware, exact-origin allowlist with --dev-origin escape hatch — all proven headlessly with httptest + WS dials
- useTerminalSocket hook implementing the locked 0x30/0x31/0x78 wire protocol with 0.5–8s ×5 backoff and 4404 short-circuit, plus a self-contained TerminalPane with WebGL/DOM xterm rendering, debounced fit/resize, D-17 clipboard, and the full UI-SPEC banner contract
- /terminal dev route composing a 220px session rail with attach-by-click TerminalPane remounts, plus a Go integration test driving spawn→echo→detach→replay-reattach→stop→delete through the production mux — all five phase success criteria human-verified
- `internal/worktree` git-CLI service: ref-safe slugs, no-network default-branch resolution, leak-safe worktree+branch creation, -uall dirty counts, and branch-preserving removal — all tested against real git 2.43 repos in t.TempDir()
- Spawn(SpawnOpts{Cwd, TaskID}) with pre-PTY cwd validation, per-task monotonic "Bash N" labels, ListByTask filtering, and concurrent StopAllForTask — the engine half of TERM-04
- Migration 00002 + task-create worktree provisioning (201 always, D-25) + POST/GET/DELETE /api/tasks/{id}/worktree with server-enforced stop/force gates and branch-preserving cleanup + task-scoped session spawn/filter — the whole phase is now exercisable with curl
- Task view is now the working surface: typed worktree/session API hooks, the branch/failed/absent meta line with Retry/Create, and live bash tabs (spawn via +, mirror server sessions, x-to-stop with left-neighbor activation) mounted through a controlled TaskTabs — verified against the real server end-to-end at the API level
- The GIT-02/GIT-03 user surface: one CleanupWorktreeDialog composing four variants from state fetched at open (clean / N-sessions-stopped / dirty type-to-confirm / both), wired to fire after every persisted move into Done and from the task-view ellipsis menu — never a silent kill, branch always kept
- Six-stage lifecycle integration test over the real REST surface (provision → task-scoped session cwd proof → dirty/session state → composed 409 gates → branch-keeping forced cleanup → kept-branch reuse), full build gates green, and human approval of all four Phase 3 success criteria against the built binary
- Agent session kind spawning the real claude CLI (--session-id + inline --settings hook overlay, inherit-all env) plus the D-47 working/idle/waiting state machine with settle-gated activity and hooks-dead BEL fallback, all TDD-driven against a fake-claude stub
- Full agent backend over REST and WS: kind-discriminated agent spawn with one-per-task 409 and claude_session_id persistence, a constant-time token-gated hook receiver driving the D-47 state machine, the 5s-pollable GET /api/agents/status board source, delete-stops-sessions, and D-45 attach-clears-waiting — all TDD against fake-claude stubs
- Live agent status on the kanban: 5s-polled useAgentStatuses hook, shared StatusDot with the exact UI-SPEC palette (pulsing amber waiting, gray-not-red stop exits), D-44 waiting card border, and D-49 sidebar waiting-count chips
- Permanent first Agent tab with Start/Start-again fresh-spawn lifecycle, Insert-description bracketed paste through a TerminalPane onReady handle, kind-aware session API, and the tab-label StatusDot with D-45 optimistic waiting clear
- 8-stage agent-lifecycle integration test against a fake-claude stub plus human-approved live verification of real claude v2.1.170, closing Phase 4 with one checkpoint feedback cycle (dimmed exited terminal, code-free banner, Reset session)
- `claude --resume <persisted uuid>` as a body-flag variant of the existing agent spawn endpoint, plus a transcript-glob `resumable` flag and DB-derived post-restart exited entries on `/api/agents/status` — RCVR-01 reconciled by architecture with zero schema change.
- REVW-01 backend: GET /api/tasks/{id}/diff returns everything the task changed vs the merge-base of its base branch (committed + staged + unstaged + untracked-as-additions, base movement excluded) as structured per-file-hunk JSON, parsed from git's -z/unified machine output with rename/binary/no-newline/mode-only edges handled.
- The D-57 muted-gray post-restart dot and the D-54 Reset/Resume button pair in both placements (resumable pre-start after a restart, exited banner within a run), driven entirely by the server's `resumable` flag from plan 05-01 — pure wiring, zero new dependencies.
- REVW-01 frontend: a read-only Diff tab (third, after Agent/Description, D-64) that renders plan 05-02's structured per-file-hunk JSON as collapsible unified diffs with a scoped green/red content palette, a totals bar with manual-refresh (no polling), >400-line collapse + binary header-only handling, and verbatim empty/loading/error states — disabled with an explanation when the task has no worktree.
- One deterministic 7-stage integration test (TestRecoveryLifecycle) locks in restart reconciliation, the full resume lifecycle, and the wired diff path against fake-claude; full build gates pass; and a human verified the entire v1 experience end-to-end against real claude v2.1.173 — the v1 milestone gate.

---
