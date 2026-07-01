# Phase 20: Kamacu Rebrand & Brand - Context

**Gathered:** 2026-07-01
**Status:** Ready for planning

<domain>
## Phase Boundary

The product presents itself as **Kamacu** everywhere in **code identity and UI** — product name, `kamacu` binary, `module kamacu` (all imports updated), a new logo (SVG), a favicon, and a root `README.md`.

**Runtime data paths, the tmux socket, and browser storage keys are deliberately left as `kangent`** — that flip is Phase 21's gated migration. A `kamacu`-named binary that still reads `~/.kangent` at runtime is an accepted intermediate state within this milestone.

Covers requirements: REBRAND-01, REBRAND-02, REBRAND-03, BRAND-01, BRAND-02, BRAND-03.

</domain>

<decisions>
## Implementation Decisions

### Logo / brand mark (BRAND-01)
- **D-01:** The logo is an **abstract symbol, not a wordmark or a 'K' monogram.** It is a **creature/mascot-class mark** — but rendered as a **pure elemental "spark/wisp"**, *not* a literal animal and *not* a face (no eyes/character). Think glowing spark / wisp / comet with motion and glow.
- **D-02:** **Concept / naming story:** "Kamacu" is read as the Quechua **_kamaq / kamacu_** — "the one who animates / gives life-force." The mark is the visual of that animating force that drives the user's agents. Capture this story; it's the *why* behind the abstract spark.
- **D-03:** **Color:** a **warm ember** (amber/orange) glow on the dark UI.
- **D-04 (design constraint, MUST honor):** the ember mark must stay **clearly distinct from the existing amber "agent waiting" status dot.** Differentiate by being a fuller multi-tone glow/shape with motion — never a plain amber dot. Do a side-by-side check against the waiting indicator during implementation.
- **D-05:** Deliver as **SVG**. It must read cleanly at brand size AND tiny (it doubles as the favicon basis — see D-10).

### Favicon (BRAND-02)
- **D-06:** The favicon is **derived from the spark mark** (same ember spark, simplified for 16–32px). No `web/public/` or favicon exists today — this is net-new. Add the `<link rel="icon">` to `web/index.html` (which currently has none). Ensure it stays legible on both light and dark browser-tab chrome.

### Sidebar brand lockup (REBRAND-01 + BRAND-01)
- **D-07:** **Expanded sidebar header:** show the **ember spark mark + "Kamacu" wordmark** (capital K), replacing today's lowercase text `kangent` at `web/src/components/sidebar/ProjectSidebar.tsx:49`.
- **D-08:** **Collapsed icon rail:** show the **spark mark at the top of the rail**, above the project avatars (today the brand is hidden when collapsed — only the toggle shows). The mark was chosen to work tiny, so it fits the ~3rem rail. This is the primary reason the mark must be compact/legible small.
- **D-09:** Wordmark casing is **"Kamacu"** (title case) per REBRAND-01, everywhere the brand appears.

### README (BRAND-03)
- **D-10:** **Audience: shareable open-source** — written so a stranger could discover, build, and run it. Include a short "what it is / why it exists", prerequisites (**Claude Code CLI, git, Go 1.26, Node 20.19+/22.12+, tmux**), and build/run steps (`make build` → single binary).
- **D-11:** **Depth: concise + one screenshot/GIF** of the board + a task's agent terminal, plus a short **project → task → agent → review** workflow walkthrough. High signal, low maintenance (not a full architecture doc).
- **D-12 (execution note):** the screenshot must be captured **after** the rebrand lands so it shows Kamacu branding (new title, sidebar lockup, logo) — not the old kangent UI. Likely a human-verify / capture step at the end of the phase.

### Rename thoroughness & boundary (REBRAND-02 / REBRAND-03)
- **D-13:** **Full cosmetic sweep.** Beyond the required module/binary/imports/UI changes, also:
  - rename the **`cmd/kangent/` package directory → `cmd/kamacu/`**,
  - update **internal comments and log-line *wording*** that say "kangent" → "kamacu",
  - update non-runtime code identifiers,
  so no *cosmetic* "kangent" remains anywhere it's safe to change.
- **D-14 (the boundary — MUST hold, keeps Phase 20 provably runtime-safe):** Do **NOT** touch anything that *is* or *constructs* a real runtime path / socket / key / data-dir filename. Explicitly leave for Phase 21:
  - `~/.kangent` path strings (data dir, `~/.kangent/repos/`, `~/.kangent/worktrees/`, `~/.kangent/kangent.db`),
  - `DefaultSocket = "kangent"` / the `-L kangent` tmux socket (`internal/tmux/tmux.go`),
  - the `kangent-<task>-<n>` tmux **session-name prefix**,
  - the regenerated `kangent-tmux.conf` filename (it lives in the data dir),
  - browser `localStorage` keys `kangent.*` / `kangent:*` (e.g. `kangent.sidebar`, `kangent:sessions-bar-collapsed`, `kangent:review-collapsed:*`).
- **D-15 (reconciliation of D-13 + D-14 — prevents a green-test regression):** In the full sweep, rename the product-*name* word inside log/comment **prose**, but **never** alter a string literal that is (or builds) an actual `~/.kangent` path/socket/key/filename. Tests assert real paths (e.g. `internal/api/projects_test.go` expects `.kangent/repos`); changing those literals would break `go test ./...`, which REBRAND-02 requires green.

### Claude's Discretion
- Exact spark geometry/animation, stroke vs fill, the precise ember hue and glow radius — open, within D-01..D-05. (A `/gsd:ui-phase 20` could produce a tighter visual contract if desired; not required.)
- Exact README section ordering and prose.
- Whether the sidebar mark and favicon share one SVG source or are two optimized variants — implementer's call, as long as both render well at their sizes.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope & requirements
- `.planning/ROADMAP.md` § "Phase 20: Kamacu Rebrand & Brand" — goal, success criteria, the explicit "runtime data paths left unchanged" boundary.
- `.planning/REQUIREMENTS.md` § "Rebrand" (REBRAND-01/02/03) and § "Brand" (BRAND-01/02/03); also § "Out of Scope" (do NOT rename the GitHub repo/remote; no permanent dual-path support).

### Grounding facts (rebrand seams + verification model)
- `.planning/STATE.md` § "Planning grounding for v1.8" → "Rebrand seams (Phase 20)" and § "Accumulated Context → Decisions → v1.8 roadmap-time decisions" (REBRAND vs MIGRATE split; BRAND rides Phase 20). Also the verification model (backend `go build`/`vet`/`test ./...`; frontend `cd web && npm run build` + `npm run lint` + human-verify; **no** frontend test framework).

No external ADRs/specs beyond the planning docs above — decisions are fully captured in this file.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets / rename sites (verified this session)
- **`go.mod`** — `module kangent` (a bare module name, not a URL). → `module kamacu`; ~53 Go files import `kangent/…` and must update to `kamacu/…`.
- **`web/index.html`** — `<title>Kangent</title>`; **no favicon `<link>` and no `web/public/` dir exist.** BRAND-02 adds both.
- **`web/src/components/sidebar/ProjectSidebar.tsx:49`** — the `kangent` brand `<span>` (hidden in collapsed/icon mode via `group-data-[collapsible=icon]:hidden`). D-07/D-08 rework this header (`SidebarHeader`) for the mark + wordmark and the collapsed-rail mark.
- **User-facing "Kangent" copy to rename (REBRAND-01):** `web/src/App.tsx:27` ("Point Kangent at a local git repository…"), `web/src/components/sidebar/ProjectMenu.tsx:95` ("removed from Kangent"), `web/src/pages/SettingsPage.tsx:43` ("across Kangent"), plus code comments in `web/src/api/types.ts`, `web/src/api/sessions.ts`.
- **`Makefile`** — build target/output binary → `kamacu` (REBRAND-02, `make build` produces `kamacu`).
- **`cmd/kangent/main.go`** — package dir renamed to `cmd/kamacu/` (D-13); note it also contains **keep-for-Phase-21** literals: `~/.kangent/kangent.db` default, `kangent-tmux.conf`, `~/.kangent/worktrees`, `sweepOrphanTmux` / `kangent-*` session references.

### Established patterns / constraints
- **Aesthetic to fit:** dark theme (`<html class="dark">`), `rounded-full` monogram project avatars, a curated muted desaturated palette, a `collapsible="icon"` sidebar rail. The ember spark should sit comfortably in this muted-dark environment (a reason a warm ember reads well — but D-04's dot-distinction caveat applies).
- **Amber is already load-bearing:** the "agent waiting" status dot is amber (per-project waiting badge overlay). D-04 exists because the ember logo shares that hue family.
- **Keep-out constants (Phase 21 territory):** `internal/tmux/tmux.go` `DefaultSocket = "kangent"`; `internal/settings/settings.go` `KeyWorktreeBase = "~/.kangent/worktrees/"`; `internal/api/projects.go` `~/.kangent/repos/`; localStorage keys in `web/src/components/layout/AppLayout.tsx` (`kangent.sidebar`), `ActiveSessionsBar.tsx` (`kangent:sessions-bar-collapsed`), `ReviewColumn.tsx` (`kangent:review-collapsed:*`).

### Integration points
- Sidebar header (`ProjectSidebar.tsx` `SidebarHeader`) is THE brand area (expanded + collapsed).
- `web/index.html` `<head>` for `<title>` + favicon link.
- Repo root for the net-new `README.md` and the logo SVG asset location (new `web/public/` or `web/src/assets/` — implementer's call).

</code_context>

<specifics>
## Specific Ideas

- **Naming story to bake in:** Kamacu = Quechua *kamaq* ("the animating life-force"); the abstract ember spark is that force made visible, driving the user's parallel agents. Use this thread in the mark's rationale and optionally a line in the README.
- **"Like an octocat/Rust-crab but abstract":** the user wants a mascot-class brand mark, but resolved to a *pure elemental spark* rather than a literal animal/face.
- **Warm ember on dark**, explicitly chosen over a cool/neutral accent — with the standing caveat to keep it visually separate from the amber waiting dot.

</specifics>

<deferred>
## Deferred Ideas

- **BRAND-FUT-01** — light/dark or animated logo variants. Already parked in `REQUIREMENTS.md` § Future. If the spark's "motion/glow" tempts an animated SVG, keep Phase 20's shipped asset static and note animation as the future variant.
- **Renaming the GitHub repo/remote** (kangent → kamacu) — explicitly Out of Scope (REQUIREMENTS.md); the local product rename does not touch the remote.

None of the discussion strayed into other phases' scope (data migration = Phase 21; diff/cleanup/polish = Phases 22–24).

</deferred>

---

*Phase: 20-kamacu-rebrand-brand*
*Context gathered: 2026-07-01*
