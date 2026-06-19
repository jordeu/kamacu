# Phase 18: Project Icon Data Foundation - Context

**Gathered:** 2026-06-19
**Status:** Ready for planning

<domain>
## Phase Boundary

Backend data layer only. Give every project two persisted properties — two
uppercase monogram letters (`icon_letters`) and a curated-palette background
color (`icon_color`) — that are sensible by default at creation, backfilled for
every pre-existing row by migration `00009`, persist across restarts (SQLite),
and are editable through the project PATCH path so Phase 19's editors have a wire
path.

**In scope (ICON-01..04):** the two new `projects` columns + migration/backfill;
default letter derivation from the project name; random curated-palette color
assignment at creation; extending the `Project` model/serialization, both create
paths, and the `update` (PATCH) handler; mirroring the two fields into the
frontend `Project` type + `useUpdateProjectSettings` so the wire path exists.

**Out of scope (Phase 19, ICON-05..12):** any avatar rendering, the collapsed
rail, tooltips, active/waiting badges, and the Project-settings letter/color
editor UI. This phase only establishes and exposes the data.

</domain>

<decisions>
## Implementation Decisions

### Curated Palette (ICON-03)
- **D-01:** Palette = **9 hues at Tailwind-600 weight**, with **fixed white
  (`#ffffff`) text** (chosen for AA-legible contrast on the zinc-900 dark theme):
  `#dc2626` (red), `#ea580c` (orange), `#d97706` (amber), `#16a34a` (green),
  `#0d9488` (teal), `#2563eb` (blue), `#4f46e5` (indigo), `#7c3aed` (violet),
  `#db2777` (pink).
- **D-02:** **Source of truth = a Go const slice** (lowercase hex). The backend
  random-picks from it at creation. Phase 19 **mirrors the same list in TS** for
  the swatch UI — **no palette endpoint** (simplest for a local single-user app).
  Keep the two lists trivially in sync (small, static).
- **D-03:** Color is assigned **pure-random** from the palette at creation (no
  avoidance of repeats / round-robin). Stored once → naturally stable across
  restarts (satisfies ICON-03 "stays stable").

### Letter Derivation (ICON-02)
- **D-04:** Word boundary = **whitespace AND the separators `-` `_` `.`**.
  Multi-token name → first letter of the first two tokens
  (`My Cool App`→`MC`, `nf-core`→`NC`, `my_cool_app`→`MC`). Single token →
  first two letters (`kangent`→`KA`). Always uppercased. This goes beyond the
  REQUIREMENTS' verbatim whitespace-only examples deliberately, because folder
  names default to the dir basename and managed repos to the repo name — both
  commonly hyphenated with no spaces.
- **D-05:** Edge-case fallbacks (Claude's discretion, default below): skip
  leading non-alphanumerics; take up to 2 alphanumeric chars; Unicode-aware
  uppercase; if only 1 usable char → a 1-letter monogram; if the name has **zero
  alphanumerics** (e.g. emoji-only) → fall back to `?` so the column is never
  empty.
- **D-06:** `deriveLetters(name)` is a **single Go function reused by both
  create paths AND the backfill** — one source of truth for the rule.

### Backfill Mechanism (ICON-04)
- **D-07:** Migration `00009_project_icons.sql` (pure SQL, goose) **only ADDs the
  two columns** with empty-string defaults (NOT NULL DEFAULT '' so SQLite
  ADD COLUMN is happy and existing rows are non-null-but-blank). Provide a
  `-- +goose Down` that `DROP COLUMN`s both (modernc/SQLite 3.53 supports
  DROP COLUMN directly — mirror `00007`/`00008`).
- **D-08:** A separate **idempotent Go backfill pass runs once right after
  `store.Migrate(db)`** at startup: `UPDATE` every row `WHERE icon_letters = ''
  OR icon_color = ''`, computing letters via `deriveLetters(name)` and color via
  `pickColor()` (the same functions the create paths use). Idempotent — a no-op
  cheap query on subsequent boots. Rationale: keeps letter logic out of gnarly
  SQL and guarantees backfill === create-time behavior (no divergence).

### PATCH / Create Validation (ICON-04, foundation for ICON-11/12)
- **D-09:** New fields wired into the **existing partial-pointer PATCH pattern**
  in `update()` (like `name`/`description`/`github_repo`): `*string` decode,
  omitted key = untouched.
- **D-10:** `icon_letters` validation (server is the enforcer): trim, uppercase,
  keep **up to 2 chars**, require **≥1** (reject empty — an avatar must render),
  allow **alphanumerics** (so digit-derived monograms like `7E` are valid).
- **D-11:** `icon_color` validation: must be a **member of the curated palette**
  (case-insensitive hex match against the Go const). Enforced at **both create
  and PATCH**. Off-palette / arbitrary hex is rejected (ICON-FUT-03 defers
  free-form color; no UI sets it this milestone anyway).

### Model / Serialization
- **D-12:** Extend `Project` struct, `projectColumns`, and `scanProject` in
  `internal/api/projects.go` with `icon_letters` / `icon_color` (both plain
  `string`, NOT NULL, never null on the wire). Both create paths (`create` folder
  + `createByRepo` repo-first) must set both fields on INSERT via the shared
  derive + pick helpers. Mirror the two fields into the TS `Project` interface
  (`web/src/api/types.ts`) and the `useUpdateProjectSettings` mutation payload
  (`web/src/api/mutations.ts`).

### Claude's Discretion
- Exact Go file/placement of `deriveLetters` / `pickColor` / the palette const
  and the backfill call site (likely `internal/api` or a small helper; backfill
  hooked wherever `store.Migrate` is invoked at startup).
- Whether `pickColor`'s randomness uses `math/rand/v2` directly (single-user,
  non-crypto is fine).
- Exact validation error copy / status codes (reuse the `update()` 400 pattern).
- D-05 letter fallbacks above.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & Roadmap
- `.planning/REQUIREMENTS.md` — ICON-01..04 (this phase) + ICON-05..12 (Phase 19
  consumes this data); Out-of-Scope table (free-form hex, image/emoji icons,
  >2 letters all deferred) and Future ICON-FUT-01..05.
- `.planning/ROADMAP.md` § "Phase 18: Project Icon Data Foundation" — goal +
  4 success criteria (the letter rule + backfill-all-rows acceptance).
- `.planning/STATE.md` — v1.7 codebase grounding block (orchestrator-verified;
  treat as fact) and the two Phase 18 plan-time todos now resolved here.

### Backend code to modify
- `internal/api/projects.go` — `Project` struct, `projectColumns`,
  `scanProject`, `create` (folder path), `createByRepo` (repo-first path),
  `update` (partial PATCH). All five touch points.
- `internal/store/migrations/00008_managed_checkout.sql` — pattern reference for
  the new `00009_project_icons.sql` (ADD COLUMN Up / DROP COLUMN Down idiom).
- `internal/store/migrate.go` + the startup call site of `store.Migrate(db)` —
  where the idempotent Go backfill pass hooks in (after Migrate succeeds).

### Frontend wire path (extend only — no UI this phase)
- `web/src/api/types.ts` — `Project` interface (add `icon_letters`,
  `icon_color`).
- `web/src/api/mutations.ts` — `useUpdateProjectSettings` (carry the two new
  optional fields in the PATCH payload).

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `scanProject` / `projectColumns` (`internal/api/projects.go`): single helper +
  column const used by list/create/update/RETURNING — adding two columns is a
  localized change touching both plus every `INSERT ... RETURNING`.
- `update()` partial-pointer PATCH pattern: `*string` fields, "omitted =
  untouched", build `sets`/`args`, append `updated_at`, `RETURNING projectColumns`.
  The new fields slot directly into this shape.
- goose embedded-SQL migration pattern (`migrate.go` `//go:embed migrations/*.sql`
  + `goose.Up`): `00009` follows it; the embed stays SQL-only (Go backfill lives
  outside goose, per D-07/D-08).

### Established Patterns
- SQLite booleans/ints scanned via intermediate var (`managedInt`) — not needed
  for the two TEXT columns, but confirms the scanProject column-order discipline
  ("order must match projectColumns").
- Both create paths (`create`, `createByRepo`) do `INSERT ... RETURNING
  projectColumns` then `scanProject` — both must populate the two new columns.
- Validation/error idiom: `writeError(w, http.StatusBadRequest, msg)`, early
  return, row left untouched on reject (see `github_repo` mandatory-verify block).

### Integration Points
- Startup: `store.Migrate(db)` → then run the one-shot idempotent Go backfill
  (new) → then serve. Backfill must be safe to run on every boot.
- Phase 19 reads `icon_letters` + `icon_color` off the project list/GET and
  writes them via the PATCH path established here.

</code_context>

<specifics>
## Specific Ideas

- Palette is the exact 9-hex Tailwind-600 set in D-01 with fixed white text.
- Concrete derivation expectations to encode as tests:
  `My Cool App`→`MC`, `kangent`→`KA`, `nf-core`→`NC`, `my_cool_app`→`MC`,
  `7 Eleven`→`7E`, single-char `X`→`X`, emoji-only name → `?`.

</specifics>

<deferred>
## Deferred Ideas

- **Free-form / hex color picker** beyond the curated swatches — ICON-FUT-03,
  explicitly out of scope; drives the D-11 "palette membership" validation.
- **Image / logo / emoji project icons** — ICON-FUT-01/02, future.
- **>2-letter monograms** — out of scope (fixed two-char for legibility).
- **Palette endpoint / dynamic palette** — not built; Go const + TS mirror is the
  chosen source of truth (D-02). Revisit only if the palette needs to be
  user-configurable.

None of these are Phase 18 work — captured so they aren't lost.

</deferred>

---

*Phase: 18-project-icon-data-foundation*
*Context gathered: 2026-06-19*
