# Phase 18: Project Icon Data Foundation - Pattern Map

**Mapped:** 2026-06-19
**Files analyzed:** 6 (2 new, 4 modified) + 1 new test
**Analogs found:** 7 / 7

This is a backend data-foundation phase: Go + modernc SQLite + goose migration,
plus a thin React/TS wire-path extension (type + mutation payload only — no UI).
Every file in scope has a direct, recent in-repo analog. Prefer these analogs
over RESEARCH.md examples — they encode this codebase's exact conventions
(column-order discipline, partial-pointer PATCH, goose ADD/DROP idiom).

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/store/migrations/00009_project_icons.sql` | migration | schema | `internal/store/migrations/00008_managed_checkout.sql` | exact |
| `internal/api/icons.go` (NEW — palette + `deriveLetters` + `pickColor` + backfill) | utility | transform / batch | `internal/settings/tokenize.go` (pure string helper) + `internal/github/github.go` `ParseRepoRef` (pure helper + canonical errors) | role-match |
| `internal/api/icons_test.go` (NEW) | test | transform | `internal/github/github_test.go` `TestParseRepoRef` (table test) | exact |
| `internal/api/projects.go` (MODIFY — struct, columns, scan, both creates, PATCH) | model + controller | CRUD / request-response | itself (existing `managed` column, `github_repo` PATCH block) | exact (self-analog) |
| `internal/store/migrate.go` + caller (`cmd/kangent/main.go`) (MODIFY — invoke backfill after Migrate) | config | batch | `cmd/kangent/main.go` startup sequence around `store.Migrate(db)` | exact (self-analog) |
| `web/src/api/types.ts` (MODIFY — `Project` interface) | model | — | itself (`managed` field added v1.4) | exact (self-analog) |
| `web/src/api/mutations.ts` (MODIFY — `useUpdateProjectSettings`) | hook | request-response | itself (`github_repo` optional-field payload branch) | exact (self-analog) |

**Helper placement note (Claude's discretion, D-12 / CONTEXT lines 96-98):** the
palette const + `deriveLetters` + `pickColor` + backfill belong in **package
`api`** (proposed file `internal/api/icons.go`), NOT in `internal/store`. Reasons:
(1) the create paths (`create`, `createByRepo`) and `update` in `projects.go` are
package `api` and call the helpers directly — same-package, no import; (2) the
backfill runs `UPDATE projects ... ` which mirrors the existing `projects.go`
SQL idiom; (3) keeping it out of `internal/store` honors D-07/D-08 ("Go backfill
lives outside goose / the embed stays SQL-only"). The backfill is *invoked* from
`cmd/kangent/main.go` right after `store.Migrate(db)` (a new exported func like
`api.BackfillProjectIcons(db)`).

---

## Pattern Assignments

### `internal/store/migrations/00009_project_icons.sql` (migration, schema)

**Analog:** `internal/store/migrations/00008_managed_checkout.sql` (and the
multi-column `00007_github_foundations.sql` for the ADD/DROP reverse-order idiom).

**Full ADD-Up / DROP-Down idiom** (`00008` lines 1-14, the single-column shape;
copy structure exactly, swap to two TEXT columns per D-07):
```sql
-- +goose Up
ALTER TABLE projects ADD COLUMN managed INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- modernc.org/sqlite tracks SQLite 3.53, which supports DROP COLUMN directly
-- (no table rebuild) — same as 00007's Down.
ALTER TABLE projects DROP COLUMN managed;
```

**Two-column reverse-order Down** (`00007` lines 9-22 — one ADD per statement Up,
one DROP per statement Down in reverse order):
```sql
-- +goose Up
ALTER TABLE projects ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN github_repo TEXT;
...
-- +goose Down
-- One DROP per statement, reverse order of the adds.
ALTER TABLE tasks DROP COLUMN pr_base_ref;
...
ALTER TABLE projects DROP COLUMN description;
```

**What `00009` must do (per D-07):** ADD both columns `NOT NULL DEFAULT ''`
(empty-string default so SQLite ADD COLUMN is happy and pre-existing rows are
non-null-but-blank — the Go backfill then fills them). Down DROPs both in reverse
order.
```sql
-- +goose Up
ALTER TABLE projects ADD COLUMN icon_letters TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN icon_color TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE projects DROP COLUMN icon_color;
ALTER TABLE projects DROP COLUMN icon_letters;
```
Critical: the migration is **SQL-only** (no letter logic in SQL — D-08). The Go
backfill does the derivation in a separate pass.

---

### `internal/api/icons.go` (NEW — utility: palette + derive + pick + backfill)

This file has no single analog because it bundles four concerns. Each maps to a
distinct in-repo pattern:

#### (a) Palette const — pure data (D-01/D-02)

**Pattern source:** package-level const block in `internal/api/projects.go`
(lines 83, 184, 307-310) — top-level `const` / grouped `const ( ... )`. The
palette is a `[]string` package var of the 9 lowercase Tailwind-600 hexes:
```go
// projectPalette is the curated dark-theme avatar palette (D-01): 9 Tailwind-600
// hues, fixed #ffffff text. Source of truth; Phase 19 mirrors it in TS (D-02).
var projectPalette = []string{
	"#dc2626", "#ea580c", "#d97706", "#16a34a", "#0d9488",
	"#2563eb", "#4f46e5", "#7c3aed", "#db2777",
}
```

#### (b) `deriveLetters(name string) string` — pure string helper (D-04/D-05/D-06)

**Analog:** `internal/settings/tokenize.go` (whole file, lines 1-50) — a pure
`strings` + `unicode` function with a doc comment enumerating the rule and its
edge cases. Copy this *shape*: a `for _, r := range s` loop, `unicode.IsSpace`,
`strings.Builder` accumulation. Note `tokenize.go` already treats whitespace as
the separator; D-04 extends the separator set to also include `- _ .`.

Tokenize's loop idiom to adapt (lines 32-49):
```go
for _, r := range s {
	switch {
	...
	case unicode.IsSpace(r):
		flush()
	default:
		cur.WriteRune(r)
	}
}
flush()
```
For `deriveLetters` the rule (D-04/D-05): split on whitespace AND `- _ .`; skip
leading non-alphanumerics; multi-token → first letter of first two tokens; single
token → first two letters; `unicode.ToUpper`; ≥1 usable char → 1-letter monogram;
zero alphanumerics → `"?"`. Encode the D-04/D-05 examples as the test (below):
`My Cool App`→`MC`, `kangent`→`KA`, `nf-core`→`NC`, `my_cool_app`→`MC`,
`7 Eleven`→`7E`, `X`→`X`, emoji-only→`?`.

#### (c) `pickColor() string` — random selection (D-03)

**Pattern note:** no existing `math/rand` usage in the repo (only
`crypto/rand` in `main.go` line 88 for the hook token). Per CONTEXT line 99,
`math/rand/v2` is fine (single-user, non-crypto). Pure-random index into
`projectPalette`:
```go
func pickColor() string {
	return projectPalette[rand.IntN(len(projectPalette))] // math/rand/v2
}
```

#### (d) `BackfillProjectIcons(db *sql.DB) error` — idempotent batch UPDATE (D-08)

**Analog:** the `database/sql` query idiom in `internal/api/projects.go`
`list` (lines 102-122) for the SELECT-loop-scan, and `update` (lines 387-399)
for the parameterized `UPDATE projects ... WHERE`. Backfill: SELECT every row
needing fill, derive+pick per row, UPDATE. Idempotent via the `WHERE` clause
(no-op on subsequent boots, D-08):
```go
// BackfillProjectIcons fills icon_letters/icon_color for any row left blank by
// migration 00009 (D-08). Idempotent: WHERE icon_letters='' OR icon_color=''
// makes it a cheap no-op on subsequent boots. Same derive+pick the create paths
// use → backfill === create-time behavior (no divergence).
func BackfillProjectIcons(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, name FROM projects WHERE icon_letters = '' OR icon_color = ''`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type fill struct {
		id      int64
		letters string
		color   string
	}
	var fills []fill
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		fills = append(fills, fill{id, deriveLetters(name), pickColor()})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, f := range fills {
		if _, err := db.Exec(
			`UPDATE projects SET icon_letters = ?, icon_color = ? WHERE id = ?`,
			f.letters, f.color, f.id); err != nil {
			return err
		}
	}
	return nil
}
```
(Collect-then-update avoids holding the SELECT cursor open while writing on the
single-writer connection — `db.SetMaxOpenConns(1)` in `store.go` line 23.)

#### (e) Validators — `validateIconLetters` / `validateIconColor` (D-10/D-11)

**Analog:** the canonical-error helper style in `internal/github/github.go`
`ParseRepoRef` (lines 40-65 — return `(canonical, error)`, trim, validate) and
the inline reject idiom in `projects.go` `update` (lines 341-385:
`writeError(w, http.StatusBadRequest, msg)` + early return, row untouched).
- `icon_letters` (D-10): trim, `unicode.ToUpper`, keep up to 2 chars, require ≥1,
  allow alphanumerics (digit monograms like `7E` valid). Reject empty.
- `icon_color` (D-11): case-insensitive membership test against `projectPalette`
  (`strings.EqualFold` against each hex — same pattern `reattachManaged` uses for
  origin compare, line 297). Off-palette → reject.

---

### `internal/api/icons_test.go` (NEW — test, transform)

**Analog:** `internal/github/github_test.go` `TestParseRepoRef` (lines 13-56) —
the exact table-driven test idiom this repo uses for pure helpers.

**Table-test skeleton to copy** (lines 13-55):
```go
func TestParseRepoRef(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"shorthand", "cli/cli", "cli/cli", false},
		...
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRepoRef(tt.in)
			...
			if got != tt.want {
				t.Errorf("ParseRepoRef(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
```
Apply to `deriveLetters` with the D-04/D-05/specifics cases (CONTEXT lines 172-174):
`{"multi-word","My Cool App","MC"}`, `{"single","kangent","KA"}`,
`{"hyphen","nf-core","NC"}`, `{"underscore","my_cool_app","MC"}`,
`{"digit","7 Eleven","7E"}`, `{"single-char","X","X"}`, `{"emoji-only","🎉","?"}`.
Also table-test `validateIconColor` (palette member vs off-palette) and
`validateIconLetters` (trim/upper/≤2/≥1). Backend verifies via `go test` (no
frontend test framework — CLAUDE.md).

---

### `internal/api/projects.go` (MODIFY — 5 touch points, self-analog)

Every change has an existing in-file pattern to mirror. **Column-order discipline
is load-bearing** (scanProject comment, lines 89-91): `scanProject` Scan order
MUST match `projectColumns` order.

**Touch point 1 — `Project` struct** (lines 26-40). Mirror `managed`/`description`
(plain non-null `string` fields with `json` tags; CONTEXT D-12 says both new
fields are plain `string`, NOT NULL, never null on the wire):
```go
type Project struct {
	...
	Managed   bool   `json:"managed"`
	// add (D-12): both plain string, NOT NULL, never null on the wire.
	IconLetters string `json:"icon_letters"`
	IconColor   string `json:"icon_color"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
```
Placement matters for column order — see touch points 2 & 3.

**Touch point 2 — `projectColumns`** (line 83). Append the two columns. Decide
ONE canonical position and keep struct/scan/INSERT all consistent:
```go
const projectColumns = `id, name, repo_path, description, github_repo, managed, icon_letters, icon_color, created_at, updated_at`
```

**Touch point 3 — `scanProject`** (lines 85-99). Add the two `string` dests in
the SAME order as `projectColumns` (between `managedInt` and the timestamps).
TEXT columns scan straight into `string` — no `sql.NullString` / intermediate var
needed (D-12: never null). Existing shape to extend (lines 92-97):
```go
var managedInt int
err := row.Scan(&p.ID, &p.Name, &p.RepoPath, &p.Description, &repo, &managedInt,
	&p.IconLetters, &p.IconColor, // <- add, matching projectColumns order
	&p.CreatedAt, &p.UpdatedAt)
```

**Touch point 4 — `create` (folder path)** (lines 169-179). The INSERT must
populate both new columns from the shared helpers. Existing INSERT (lines 173-174):
```go
p, err := scanProject(h.db.QueryRow(
	`INSERT INTO projects (name, repo_path) VALUES (?, ?) RETURNING `+projectColumns, name, abs))
```
becomes (set letters from name, color from palette):
```go
p, err := scanProject(h.db.QueryRow(
	`INSERT INTO projects (name, repo_path, icon_letters, icon_color) VALUES (?, ?, ?, ?) RETURNING `+projectColumns,
	name, abs, deriveLetters(name), pickColor()))
```

**Touch point 5 — `createByRepo` (repo-first path)** (lines 255-271). Same: add
the two columns to its INSERT (line 266) sourced from the same helpers. Existing:
```go
p, err := scanProject(h.db.QueryRow(
	`INSERT INTO projects (name, repo_path, github_repo, managed, description) VALUES (?, ?, ?, 1, ?) RETURNING `+projectColumns,
	name, dest, canonical, desc))
```
→ add `, icon_letters, icon_color` to the column list and `, deriveLetters(name), pickColor()` to the args.

**Touch point 6 — `update` (partial PATCH)** (lines 320-400). THE key analog is
the partial-pointer block. Add two `*string` request fields and slot them into
the `sets`/`args` builder exactly like `name`/`github_repo`, WITH validation
(D-09/D-10/D-11). Request struct (lines 325-329):
```go
var req struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	GithubRepo  *string `json:"github_repo"`
	IconLetters *string `json:"icon_letters"` // add
	IconColor   *string `json:"icon_color"`   // add
}
```
The "nothing to update" guard (lines 334-337) must include the new pointers.
Per-field validate-then-append idiom to mirror — the `name` required-field reject
(lines 341-349) for `icon_letters`, and the `github_repo` membership-style reject
(lines 358-385) for `icon_color`:
```go
if req.Name != nil {
	name := strings.TrimSpace(*req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	sets = append(sets, "name = ?")
	args = append(args, name)
}
```
Apply: `icon_letters` → validate (trim/upper/≤2/≥1, D-10), reject empty with
`writeError(w, http.StatusBadRequest, ...)` + early return (row untouched), else
`sets = append(sets, "icon_letters = ?")`. `icon_color` → palette-membership
check (D-11), reject off-palette, else append. Then the existing
`updated_at` + `RETURNING projectColumns` tail (lines 387-399) is unchanged and
returns the updated row via `scanProject`.

---

### `internal/store/migrate.go` + caller `cmd/kangent/main.go` (MODIFY, self-analog)

**Pattern:** `migrate.go` (lines 13-21) stays UNCHANGED — the embed stays
SQL-only (D-07). The new work is the *call site*: invoke the backfill right after
`store.Migrate(db)` succeeds.

**Startup sequence analog** (`cmd/kangent/main.go` lines 60-70 — open → migrate →
fatal-on-error). Insert the backfill between Migrate and the next step:
```go
db, err := store.Open(dbPath)
...
if err := store.Migrate(db); err != nil {
	slog.Error("running migrations", "error", err)
	os.Exit(1)
}
// NEW: one-shot idempotent icon backfill (D-08). Safe to run every boot.
if err := api.BackfillProjectIcons(db); err != nil {
	slog.Error("backfilling project icons", "error", err)
	os.Exit(1)
}
```
Mirror the exact `slog.Error(...) + os.Exit(1)` fatal idiom used for every other
startup step. `main.go` already imports the `api` package (lines 122-132).

---

### `web/src/api/types.ts` (MODIFY — `Project` interface, self-analog)

**Analog:** the `managed: boolean` field added in v1.4 (lines 18-21) — add two
non-null `string` fields with a brief comment, alongside `github_repo`:
```ts
export interface Project {
  ...
  github_repo: string | null;
  managed: boolean;
  // v1.7 icon identity (Phase 18): two uppercase letters + a curated-palette
  // hex (#rrggbb). Both NOT NULL on the wire (never null).
  icon_letters: string;
  icon_color: string;
  created_at: string;
  updated_at: string;
}
```

---

### `web/src/api/mutations.ts` (MODIFY — `useUpdateProjectSettings`, self-analog)

**Analog:** the `github_repo` optional-field handling already in
`useUpdateProjectSettings` (lines 32-54) — an `undefined`-means-omit optional
field that conditionally builds the PATCH body. Extend the same shape for the two
new optional fields (D-09: omitted = untouched). Existing (lines 35-49):
```ts
mutationFn: ({ id, description, github_repo }: {
  id: number;
  description: string;
  github_repo?: string;
}) =>
  patch<Project>(
    `/api/projects/${id}`,
    github_repo === undefined ? { description } : { description, github_repo },
  ),
```
Carry `icon_letters?: string` and `icon_color?: string` in the args and spread
them into the body only when defined (build the payload object conditionally so an
omitted field is never sent — matching the backend partial-PATCH contract). Phase
19's editors will pass these; this phase only establishes the wire path.

---

## Shared Patterns

### Single derive+pick source of truth (D-06/D-08)
**Source:** `internal/api/icons.go` `deriveLetters` / `pickColor` (NEW).
**Apply to:** both create paths (`create`, `createByRepo`) AND `BackfillProjectIcons`.
Same functions everywhere → backfill behavior === create-time behavior, no divergence.

### Column-order discipline
**Source:** `internal/api/projects.go` `scanProject` comment (lines 89-91:
"Column ORDER must match projectColumns").
**Apply to:** every `projectColumns` / `scanProject` / `INSERT ... RETURNING`
edit. Add the two columns in ONE canonical position and keep struct field order,
scan dest order, and INSERT column order consistent.

### Partial-pointer PATCH with validate-then-append
**Source:** `internal/api/projects.go` `update` (lines 320-400).
**Apply to:** the two new PATCH fields. `*string` decode → omitted key untouched;
trim/validate → on reject `writeError(w, http.StatusBadRequest, msg)` + early
return (row left untouched, see the `github_repo` mandatory-verify block,
lines 358-385); on pass `sets = append(...)` / `args = append(...)`.

### Validation / error idiom
**Source:** `internal/api/respond.go` `writeError` (lines 16-18) + the inline
reject pattern in `update`. **Apply to:** `icon_letters` empty/over-length reject,
`icon_color` off-palette reject (both create and PATCH, D-11). Exact status/copy
is Claude's discretion (CONTEXT line 101): reuse the 400 pattern.

### Idempotent startup pass
**Source:** `cmd/kangent/main.go` startup sequence (lines 60-70).
**Apply to:** the backfill call after `store.Migrate(db)` — `slog.Error + os.Exit(1)`
fatal idiom; safe to re-run every boot (WHERE-clause no-op).

### Table-driven pure-helper test
**Source:** `internal/github/github_test.go` `TestParseRepoRef` (lines 13-56).
**Apply to:** `deriveLetters`, `validateIconLetters`, `validateIconColor` tests.

---

## No Analog Found

None. Every file in scope has a direct in-repo analog (most are self-analogs in
`projects.go` / `migrations/` / `main.go`). The only genuinely new code is the
icons helper file, and each of its four concerns maps to an existing pattern
(`tokenize.go` pure helper, `ParseRepoRef` canonical-error helper,
`projects.go` SQL idiom, `github_test.go` table test).

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| — | — | — | No file lacks an analog. |

---

## Metadata

**Analog search scope:** `internal/api/`, `internal/store/`, `internal/store/migrations/`,
`internal/settings/`, `internal/github/`, `cmd/kangent/`, `web/src/api/`
**Files scanned:** projects.go, migrate.go, store.go, respond.go,
00007/00008 migrations, tokenize.go, github.go, github_test.go, projects_test.go,
main.go, types.ts, mutations.ts
**Pattern extraction date:** 2026-06-19
</content>
</invoke>
