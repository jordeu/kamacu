# Phase 6: Settings & Polish - Research

**Researched:** 2026-06-11
**Domain:** SQLite-backed global settings wired into existing spawn/creation call sites (Go + React); claude flag composition; git branch-template validation
**Confidence:** HIGH — flag composition and ref validation verified empirically against the installed binaries (claude v2.1.173, git 2.43.0) via quota-free probes; all wiring points read directly from current source

<user_constraints>
## User Constraints (locked decisions — ROADMAP Phase 6 notes + approved 06-UI-SPEC; no CONTEXT.md exists for this phase)

### Locked Decisions
- **Global-only settings.** No per-project overrides (SET-FUT-01 deferred).
- **SQLite settings table, migration 00004.** Read/written over the API (SET-02).
- **Per-field commit save model** (UI-SPEC, discretion resolved): text inputs commit on blur/Enter, Esc reverts draft, select commits on change, `Saved` flash 2s, `Reset to default` per field, no page-level Save button, no dirty badges, no navigation guard.
- **Extra-params free text defaults to `--dangerously-skip-permissions`** — intentional D-51 reversal (AGENT-02). No warning chrome on the field; help line carries the explanation. Log the D-51 reversal in PROJECT.md Key Decisions at transition.
- **Settings apply at next spawn/creation** (SET-03). Running sessions and existing worktrees unaffected; no restart; communicated once by the page sub-line.
- **Worktree base default `~/.kangent/worktrees/`, new-creations-only** (WT-01/02). Existing worktrees keep stored absolute paths; never move live trees.
- **Shell dropdown bash-only** but the value is read from settings, not hardcoded (SHELL-01/02). Curated dropdown — never a free-text command field.
- **Branch template tokens `{slug}` `{id}` `{title}`, default `task/{slug}-{id}`** (BRANCH-01). Validated to a legal, collision-safe ref; invalid templates rejected with the UI-SPEC canonical error copy and never used (BRANCH-02).
- **UI-01 = remove the `max-w-[860px]` wrapper around the TaskPage header block** (becomes `w-full shrink-0 space-y-2`); Description prose island keeps its 860px; loading/error page states out of scope.
- **Gear control at the bottom of the projects sidebar** → dedicated full-page `/settings` route under AppLayout (SET-01); UI-SPEC defines exact footer-row layout, copy, and active state.
- **Out of scope:** validating that claude extra-params are individually correct flags (pass-through); migrating existing worktrees; shells beyond bash; settings sync/export.

### Claude's Discretion (resolved by this research, see patterns)
- Settings storage shape (KV vs typed columns), API shape (per-key PUT), defaults-in-code vs seeded rows
- Extra-params tokenization strategy
- Exact insertion point of extra args in the claude argv
- Settings read strategy (read-at-use vs cached)

### Deferred Ideas (OUT OF SCOPE)
- NOTF-01/02 browser notifications & In-Review suggestion, MAINT-01 stale-worktree purge, AGNT-01 MCP server, SET-FUT-01 per-project overrides, SHELL-FUT-01 additional shells
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SET-01 | Settings page via sidebar gear, full-page route | Pattern 7 (route + gear wiring; ProjectSidebar footer at lines 93–103, App.tsx routes verified) |
| SET-02 | Settings persist in SQLite table over the API | Pattern 1 (KV table, migration 00004), Pattern 2 (API shape) |
| SET-03 | Changes apply at next spawn/creation, no restart | Pattern 3 (read-at-use from DB in the API handlers — SET-03 falls out structurally) |
| SET-04 | Defaults + restore + inline errors without losing other edits | Pattern 1 (defaults in code), Pattern 2 (per-key PUT isolates failures), UI-SPEC save model |
| AGENT-01 | Free-text extra params appended to every claude spawn | Verified flag composition (§Verified Behaviors), Pattern 4 (tokenizer + argv insertion incl. resume path) |
| AGENT-02 | Default includes `--dangerously-skip-permissions`, removable | Verified on v2.1.173: composes with `--session-id` + `--settings`, no acceptance dialog, straight to prompt with bypass on |
| WT-01 | Configurable worktree base dir, default `~/.kangent/worktrees/` | Pattern 5 (PathFor/base injection, ~ expansion, save-time validation) |
| WT-02 | Only new worktrees affected; existing keep absolute paths | Tasks store absolute `worktree_path`; all readers use the stored path — nothing to change (verified in tasks.go/worktrees.go) |
| SHELL-01 | Shell dropdown (bash only) | Pattern 6 + UI-SPEC select block; options served as data |
| SHELL-02 | Shell read from settings, de-hardcoded | Pattern 6 — current hardcode is manager.go:147–151 (`$SHELL` fallback `/bin/bash`) |
| BRANCH-01 | Template with `{slug}` `{id}` `{title}`, default `task/{slug}-{id}` | Pattern 5 (expansion rules; current hardcode is tasks.go:75) |
| BRANCH-02 | Template validated legal + collision-safe, rejected with clear message | §git check-ref-format verification matrix, Pattern 5 validation algorithm, sample-expansion invariance argument |
| UI-01 | Task-view header spans full width | §UI-01 — exact wrapper located (TaskPage.tsx:381); tab strip already full-width |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Locked stack:** Go 1.26 + stdlib ServeMux, `creack/pty` v1, `coder/websocket`, `modernc.org/sqlite` + goose migrations, React 19 + Vite + Tailwind 4 + shadcn + TanStack Query. **No new Go modules, no new npm packages** — the only sanctioned additions are the two shadcn blocks `select` and `label` (copied in, not dependencies).
- Shell out to git with arg arrays — **never `sh -c`**; parse only `--porcelain`/plumbing output. Same discipline applies to claude argv construction (the existing spawn already execs arg arrays).
- REST under `/api/*` via route-registration functions; JSON respond helpers (`writeJSON`/`writeError`); table-driven Go tests with httptest.
- Frontend: typed TanStack Query hooks in `web/src/api/*`; components own their files; **verbatim UI-SPEC copy** in single template literals.
- GSD workflow enforcement: implementation happens through `/gsd:execute-phase`.
- User global rule: never mention/co-author Claude or happy-otter on commits/PRs.
- `.planning/config.json`: `nyquist_validation` is explicitly `false` → no Validation Architecture section; test posture = existing `go test ./...` table-driven suites (no frontend test framework exists; consistent with Phases 1–5).

## Summary

This phase is integration, not infrastructure: four settings wire into four existing call sites, plus a one-line CSS fix. Research verified the two load-bearing unknowns empirically:

1. **Claude flag composition works (HIGH, verified on installed v2.1.173).** `claude --session-id <uuid> --settings '<the exact Phase-4 overlay JSON>' --dangerously-skip-permissions` launches straight to the prompt with "⏵⏵ bypass permissions on" in the footer — no flag conflict, no flag-order constraint, and **no one-time acceptance dialog** even though `bypassPermissionsModeAccepted` is unset in `~/.claude.json`. The `--resume <uuid> --dangerously-skip-permissions --settings` combination also parses cleanly (probe proceeded past flag parsing to the TUI flow). Extra params therefore append to the existing argv unchanged for both fresh and resume spawns.

2. **`git check-ref-format refs/heads/<candidate>` is the authoritative validator and runs without a repo (HIGH, verified on git 2.43.0).** A 27-case matrix (below) pins exactly what the canonical "Not a valid git branch name." error covers. Use the positional `refs/heads/<name>` form, NOT `--branch` (which has shorthand-expansion semantics, e.g. `@{-1}`). One extra app-side rule: reject expanded names starting with `-` (git accepts `refs/heads/-foo` but a leading dash is an argv hazard for `git worktree add -b`).

Everything else is established pattern: a 2-column KV settings table (defaults in code, absent row = default — new settings never need a migration), per-key `PUT /api/settings/{key}` matching the UI-SPEC per-field commit model, and read-at-use settings reads inside the API handlers so SET-03's next-spawn semantics fall out of the structure rather than being implemented.

**Primary recommendation:** Build a small `internal/settings` package (defaults, Get/Set, per-key validation, template expansion, params tokenizer, relocated `expandHome`); thread values through `SpawnOpts`/`provisionWorktree` from the handlers (managers/services stay DB-free); keep the frontend to one query + one per-key mutation and the UI-SPEC contract.

## Standard Stack

### Core — no new dependencies

| Asset | Where | Phase 6 use |
|-------|-------|-------------|
| goose migrations | `internal/store/migrations/` | `00004_settings.sql` — KV table (next number after 00003, verified) |
| `internal/session` Manager | `manager.go` | `SpawnOpts` gains `ExtraArgs []string` + `Shell string`; manager stays DB-free |
| `provisionWorktree` | `internal/api/tasks.go:73` | Single choke point for BOTH worktree-creation paths (task create + retry, verified) — branch template + base dir wire in here |
| `worktree.Slug` | `internal/worktree/worktree.go:51` | Reused for `{slug}`; same algorithm (uncapped variant) for `{title}` |
| `expandHome` | `cmd/kangent/main.go:183` | Relocate to `internal/settings` (or shared util); main.go + settings validation both need it |
| API respond helpers, route-registration pattern | `internal/api/respond.go`, `routes.go` | `SettingsRoutes(mux, db)` |
| `web/src/api/client.ts` | `get`/`put`-style helpers | client.ts has `get/post/patch/del` — add a `put` helper (4 lines, same shape) |
| TanStack Query hooks pattern | `web/src/api/agents.ts`, `worktrees.ts` | `settings.ts`: `useSettings()` + `useSaveSetting()` |
| shadcn blocks | `npx shadcn@latest add select label` | The ONLY new frontend assets (sanctioned by UI-SPEC; `components.json` verified: radix-nova, `registries: {}`) |
| lucide-react `Settings` icon | already a dependency | Sidebar gear (16px) |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| KV table (`key TEXT PRIMARY KEY, value TEXT`) | Typed columns (one row, one column per setting) | Typed columns need a migration per new setting and NULL-handling for defaults; KV + defaults-in-code means SHELL-FUT/new settings are pure code changes. KV wins for 4 string settings |
| Defaults in code (absent row = default) | Seeded default rows in migration | Seeded rows can't distinguish "user chose the default" from "never touched" and require migrations to change a default. Code defaults are the standard pattern; `Reset to default` still just PUTs the default value |
| Per-key `PUT /api/settings/{key}` | Whole-object PUT | Whole-object PUT couples fields: one invalid field 400s the batch, violating SET-04's isolation. Per-key maps 1:1 onto the UI-SPEC per-field commit model |
| `git check-ref-format refs/heads/<n>` subprocess | Reimplement ref rules in Go | git is guaranteed present (app premise); the rules have ~10 obscure cases (verified matrix below); a subprocess per settings-save and per task-create is negligible. Don't hand-roll |
| Minimal in-house quote-aware tokenizer | `google/shlex` / `kballard/go-shellquote` | Both are new Go modules — locked-stack violation for ~40 lines of code with zero expansion/globbing semantics needed (we exec arg arrays). Justified hand-roll, see Pattern 4 |
| Read-at-use settings (per spawn/creation) | Cached settings struct + invalidation | Caching adds an invalidation path and a staleness bug class for zero benefit at single-user load; read-at-use makes SET-03 true by construction |

## Verified Behaviors (empirical, this host, 2026-06-11)

### Claude v2.1.173 flag composition

The installed claude is now **2.1.173** (Phase 4 research ran against 2.1.170; Phase 5 already re-verified `--resume` semantics on 2.1.173 per manager.go comments). Probes were scripted PTY runs that never submitted a prompt (zero API turns).

| # | Question | Verified Answer | Evidence |
|---|----------|-----------------|----------|
| 1 | `--dangerously-skip-permissions` + `--settings '<overlay>'` + `--session-id <uuid>` together? | **YES — composes cleanly.** Launched in the trusted kangent dir with the exact Phase-4 overlay shape: straight to the prompt, footer shows "⏵⏵ bypass permissions on (shift+tab to cycle)", no settings-validation errors | `/tmp/claude-probe6/compose.raw` |
| 2 | One-time bypass acceptance dialog? | **NO dialog on v2.1.173**, even with `bypassPermissionsModeAccepted` absent from `~/.claude.json` (verified absent). Session went directly to the prompt | same probe + config inspection |
| 3 | Flag-order constraints? | **None observed.** Flags are independent commander options; probe placed `--dangerously-skip-permissions` last after `--settings`. Recommend appending user extras after the fixed flags (Pattern 4) | same probe |
| 4 | `--resume <uuid>` + `--dangerously-skip-permissions` + `--settings`? | **Parses cleanly** — probe with a nonexistent uuid proceeded past flag parsing into the TUI flow (trust dialog for the untrusted /tmp cwd), proving no flag conflict. `--settings`+`--resume` was already verified end-to-end in Phase 5 (overlay applies on resume; SessionStart fires with source "resume") | `/tmp/claude-probe6/resume.raw` |
| 5 | `--help` inventory | `--dangerously-skip-permissions` ("Bypass all permission checks"), `--permission-mode` (choices incl. `bypassPermissions`), `--allow-dangerously-skip-permissions` (enables bypass as an OPTION without default-on — not what AGENT-02 specifies), `--settings <file-or-json>`, `--session-id <uuid>`, `-r/--resume [value]` all present | `claude --help` |
| 6 | Hook behavior under bypass | **Logical consequence, not separately probed:** with permissions bypassed there are no permission prompts, so `Notification(permission_prompt)` hooks never fire. See §Status-dot implication below | docs + Phase 4 hook semantics |

**Status-dot implication of the default-on flag (document for the planner/verifier):** with `--dangerously-skip-permissions` active, the amber *waiting* state effectively disappears for permission prompts (the dominant source). `elicitation_dialog` (MCP input requests) can still fire, Stop→idle and the output-activity working heuristic are unaffected, and exited detection is unchanged. Net: dots are green/gray in normal operation; the loud amber board behavior returns when the user removes the flag. This is acceptable specified behavior, not a regression — the Phase 4 detection machinery needs **zero changes**.

**One probe side-effect:** the composition probe minted session `172029bd-d38e-41aa-9ee1-6d2b6c818a81`; its (empty) transcript remains at `~/.claude/projects/-home-jordi-workspace-github-kangent/` — harmless, removal was outside sandbox scope.

### git 2.43.0 — `check-ref-format` verification matrix

`git check-ref-format refs/heads/<candidate>` **works outside any repo** (verified from /tmp) — settings-save validation needs no repo context. Exit 0 = legal, nonzero = illegal.

| Candidate | Result | Candidate | Result |
|-----------|--------|-----------|--------|
| `task/fix-login-42` | OK | `a:b` | REJECT |
| `a b` (space) | REJECT | `a?b` | REJECT |
| `a..b` | REJECT | `a*b` | REJECT |
| `a.lock` | REJECT | `a[b` | REJECT |
| `a/` (trailing slash) | REJECT | `a\b` | REJECT |
| `/a` (leading slash) | REJECT | `a~b` | REJECT |
| `a//b` | REJECT | `a^b` | REJECT |
| `a.` (trailing dot) | REJECT | control char | REJECT |
| `.a` / `a/.b` (dot-leading component) | REJECT | empty | REJECT |
| `@` | OK | `@{x` | REJECT |
| **`-foo` / `task/-x`** | **OK (git-legal — app must reject; argv hazard)** | `a@b` | OK |
| **`{a}` / `a{b}c`** | **OK (braces are git-legal — unknown-token detection must be an explicit app check)** | `café` (unicode) | OK |
| `TASK/Fix` (uppercase) | OK | | |

**Do NOT use `git check-ref-format --branch`:** it applies "branch shorthand" interpretation (documented `@{-1}` previous-branch expansion — a candidate could be *rewritten* rather than validated). The positional `refs/heads/<name>` form has no expansion semantics. (Outside-repo `--branch` happened to reject `@{-1}` here, but the positional form is categorically safe.)

## Architecture Patterns

### Recommended structure

```
internal/settings/            # NEW package
├── settings.go               # keys, Defaults map, Get/GetAll/Set (database/sql)
├── validate.go               # per-key validation; canonical error strings (UI mirrors verbatim)
├── template.go               # branch-template token scan + ExpandTemplate + sanitizeTitle
├── tokenize.go               # extra-params tokenizer
├── home.go                   # expandHome (moved from cmd/kangent/main.go; main imports it)
└── *_test.go                 # table-driven
internal/api/settings.go      # SettingsRoutes: GET /api/settings, PUT /api/settings/{key}
internal/store/migrations/00004_settings.sql
web/src/api/settings.ts       # useSettings + useSaveSetting
web/src/pages/SettingsPage.tsx
web/src/components/settings/SettingsField.tsx   # label row + Saved flash + Reset + input + help/error
web/src/components/ui/{select,label}.tsx        # via shadcn CLI
```

### Pattern 1: Storage — KV table, defaults in code

```sql
-- 00004_settings.sql
-- +goose Up
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
-- +goose Down
DROP TABLE settings;
```

```go
// internal/settings/settings.go
const (
    KeyAgentExtraParams = "agent_extra_params"
    KeyWorktreeBase     = "worktree_base"
    KeyShell            = "shell"
    KeyBranchTemplate   = "branch_template"
)

var Defaults = map[string]string{
    KeyAgentExtraParams: "--dangerously-skip-permissions", // AGENT-02, D-51 reversal
    KeyWorktreeBase:     "~/.kangent/worktrees/",          // WT-01; stored unexpanded, expanded at use
    KeyShell:            "bash",                            // SHELL-01
    KeyBranchTemplate:   "task/{slug}-{id}",                // BRANCH-01
}

// Get returns the stored value, or the code default when no row exists.
// Absent row == default → new settings never need a migration (SET-04).
func Get(db *sql.DB, key string) (string, error) {
    var v string
    err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
    if errors.Is(err, sql.ErrNoRows) {
        return Defaults[key], nil
    }
    return v, err
}

// Set validates then upserts (validate BEFORE write — an invalid template is
// never stored, BRANCH-02).
//   INSERT INTO settings(key,value) VALUES(?,?)
//     ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=...
```

- `Reset to default` is just a PUT of the default value from the client (per UI-SPEC: "Click → PUT the default immediately"). Whether the row then holds the default or could be deleted is behaviorally identical — store it, keep it simple.
- Empty string is a VALID value for `agent_extra_params` ("no extra parameters" — UI-SPEC: no placeholder, empty is valid). Don't conflate empty with absent: the upsert stores `""`, and `Get` returns it. This is the one key where the absent-vs-stored distinction matters.

### Pattern 2: API shape — per-key PUT mirroring the per-field commit model

```
GET /api/settings → 200
{
  "agent_extra_params": {"value": "--dangerously-skip-permissions", "default": "--dangerously-skip-permissions"},
  "worktree_base":      {"value": "~/.kangent/worktrees/",          "default": "~/.kangent/worktrees/"},
  "shell":              {"value": "bash", "default": "bash", "options": ["bash"]},
  "branch_template":    {"value": "task/{slug}-{id}",               "default": "task/{slug}-{id}"}
}

PUT /api/settings/{key}  body {"value": "..."} →
  200 {"value": ..., "default": ...}     (stored)
  400 {"error": "<canonical message>"}   (validation failed; nothing stored)
  404                                     (unknown key)
```

- Returning `default` per key lets the client render `Reset to default` only when `value !== default` (UI-SPEC) without hardcoding defaults frontend-side.
- `shell.options` served as data is the SHELL-FUT-01 seam: additional shells later are a one-line server change, zero frontend changes (the select maps options).
- Existing `writeError` produces `{"error": msg}` — the UI-SPEC "mirror server messages verbatim" contract rides the established error pipe (client.ts already extracts `body.error`).

### Pattern 3: Wiring — read-at-use in handlers; managers/services stay DB-free

**SET-03 is structural:** every consumer reads settings from the DB at the moment of use, inside API handlers that already hold `*sql.DB`. No caching, no invalidation, no restart semantics to build.

| Setting | Read where | Threaded how | Current hardcode |
|---------|-----------|--------------|------------------|
| agent_extra_params | `sessions.go create` (agent branch, has db) | `SpawnOpts.ExtraArgs []string` (tokenized) | — (new) |
| shell | `sessions.go create` (bash branch — task tabs AND the /terminal dev spawn, one code path) | `SpawnOpts.Shell string` | `manager.go:147-151` — `os.Getenv("SHELL")` fallback `/bin/bash` |
| worktree_base | `provisionWorktree` (tasks.go:73, has db; serves both create + retry paths) | expanded base passed into path construction | `cmd/kangent/main.go:83` fixed `wtRoot` → `worktree.NewService(wtRoot)`; `Service.PathFor` uses `s.Root` |
| branch_template | `provisionWorktree` | expanded branch replaces the literal | `tasks.go:75` — `branch = "task/" + slug + "-" + id` |

**Manager changes (`internal/session/manager.go`):**
```go
type SpawnOpts struct {
    Cwd             string
    TaskID          int64
    Kind            Kind
    ResumeSessionID string
    ExtraArgs       []string // agent-only: tokenized settings extras, appended after fixed flags
    Shell           string   // bash-only: resolved from settings; "" keeps current $SHELL fallback (back-compat for tests)
}

// agent branch — extras append AFTER the fixed flags:
args := []string{idFlag, claudeSessionID, "--settings", buildOverlayJSON(...)}
args = append(args, opts.ExtraArgs...)
cmd = exec.Command(bin, args...)

// bash branch — settings-driven shell:
shell := opts.Shell                    // e.g. "bash"
if shell == "" { /* existing $SHELL fallback path, keeps Phase 2 tests green */ }
path, err := exec.LookPath(shell)      // no slash → PATH resolution; clean error → existing "couldn't start a session"
...
cmd.Env = []string{..., "SHELL=" + path, ...}  // SHELL env now reflects the resolved shell
```

- Extras apply to **resume spawns too** (AGENT-01 "every claude spawn"; composition verified). The handler reads settings once and sets `ExtraArgs` for both the fresh and resume variants.
- Behavior change to call out in the plan: bash tabs previously ran `$SHELL` (possibly zsh); v1.1 runs the settings value (`bash`) — that IS SHELL-01/02 as specified.
- Settings read failure (real DB error, not absent row) → 500 from the handler ("couldn't start a session" path) — local SQLite errors are exceptional; don't silently fall back.

**Worktree base injection — recommended shape:** demote path construction to a pure function and drop `Service.Root`:

```go
// internal/worktree — replaces the method; Service keeps the mutex + git ops only.
func PathUnder(base, repoPath, slug string, taskID int64) string {
    return filepath.Join(base, filepath.Base(repoPath), slug+"-"+strconv.FormatInt(taskID, 10))
}
```
`provisionWorktree` reads `worktree_base`, expands `~`, and calls `PathUnder`. The repo-basename subdirectory structure is **kept under the new base** (locked: "PathFor currently uses repo basename subdirs — keep that structure"). `Create` already does `MkdirAll(filepath.Dir(path))`, so a not-yet-existing base is created on first use — no creation needed at save time. main.go's `wtRoot` computation and the `NewService(root)` root param go away (or `Root` stays as an unused-default during transition — planner's call; tests currently inject `t.TempDir()` via `NewService` and would switch to passing base explicitly).

### Pattern 4: Extra-params tokenizer — minimal quote-aware split, no shell

We exec arg arrays (never `sh -c`), so the tokenizer defines the entire grammar. Whitespace-only splitting breaks legitimate values like `--append-system-prompt "be terse"`. The locked stack forbids new modules, and the needed grammar is tiny — a justified hand-roll:

```go
// Tokenize splits s into argv tokens: runs of non-space, with '...' and "..."
// preserving spaces. No escapes, no expansion, no globbing — tokens pass to
// exec.Command verbatim. An unclosed quote consumes the rest of the string as
// one token (lenient; the field is pass-through per REQUIREMENTS Out of Scope).
func Tokenize(s string) []string {
    var tokens []string
    var cur strings.Builder
    var quote rune // 0 = unquoted
    flush := func() { if cur.Len() > 0 { tokens = append(tokens, cur.String()); cur.Reset() } }
    for _, r := range s {
        switch {
        case quote != 0:
            if r == quote { quote = 0 } else { cur.WriteRune(r) }
        case r == '\'' || r == '"':
            quote = r
        case unicode.IsSpace(r):
            flush()
        default:
            cur.WriteRune(r)
        }
    }
    flush()
    return tokens
}
```

- **Lenient on unclosed quotes** (rest-of-string becomes one token): the UI-SPEC defines NO canonical error copy for the agent field, and REQUIREMENTS explicitly scope out policing the field. Rejecting unclosed quotes at save would invent copy outside the approved contract. (If the planner prefers strictness, a save-time 400 is permitted by the generic "mirror server messages" rule — but lenient is the spec-minimal reading.)
- Note: a quirk worth one comment in code — quote handling is mid-token (`a"b c"d` → `ab cd`), matching POSIX-ish expectations.
- Empty/whitespace-only value → `nil` → no extra args (valid per UI-SPEC).
- **Known sharp edge (document, don't fix):** a user adding their own `--settings` would override Kangent's hook overlay (commander last-value-wins for scalar options) and silently kill status hooks. The Phase 4 SessionStart canary already detects this (hooks-dead → bell fallback + log warning). Policing it is explicitly out of scope.

### Pattern 5: Branch template — validation algorithm + expansion rules

**Token expansion (all tokens expand to non-empty `[a-z0-9-]` strings, never leading/trailing `-`):**
- `{slug}` → existing `worktree.Slug(title)` (lowercase, collapse to `-`, trim, 40-char cap, fallback `"task"`)
- `{id}` → decimal task ID (digits)
- `{title}` → same sanitizer as Slug but a higher cap (recommend 100 chars; rationale: "full title" semantics while keeping refs filesystem-sane); fallback `"task"` — so an **empty title can never produce an empty token**
- Expansion is plain string replacement of the three exact tokens; no recursion (replace once, left-to-right — expanded values contain no `{`).

**Validation at save (`PUT /api/settings/branch_template`), in this order, with the UI-SPEC canonical copy verbatim:**

```go
// 1. strings.TrimSpace(tpl) == ""        → "Template can't be empty."
// 2. scan regexp `\{[^}]*\}`: any group not in {slug|id|title}
//                                         → "Unknown token {x}. Use {slug}, {id}, or {title}."
//    (REQUIRED as an explicit check: braces are git-legal — verified `{a}` passes
//     check-ref-format. An unmatched lone `{` matches no group → literal, git-legal.)
// 3. !strings.Contains(tpl, "{id}")      → "Template must include {id} so branch names never collide."
//    (Recommend REQUIRE — research focus resolved YES: {id} is the uniqueness
//     guarantee; slugs/titles collide trivially and Create() reuses existing
//     branches, which would silently entangle two tasks.)
// 4. expand with samples (slug "fix-login", id 42, title-sanitized "fix-login")
//    + reject leading '-' on the result, then:
//      git check-ref-format refs/heads/<expanded>   (exec arg array, no repo needed — verified)
//    nonzero exit                         → "Not a valid git branch name."
```

**Why sample-expansion validation is sound (invariance argument):** every token expands to a non-empty string over `[a-z0-9]` with internal `-` only. Therefore all git-illegal constructs (`..`, `.lock` suffix, leading/trailing `/` or `.`, `@{`, spaces, `~^:?*[\`, control chars, empty components, leading `-`) can only come from the template's **literal characters and token boundaries**, which are identical for every real task. A template that validates with sample values is legal for ALL task values, and vice versa. (This is why the existing `Slug` deliberately uses a constructive charset — same trick, extended.)

**Defense in depth at creation time:** `provisionWorktree` re-runs `git check-ref-format refs/heads/<real-expanded>` before `wt.Create`. Cost: one subprocess. If it somehow fails (e.g. settings row edited by hand in SQLite), the error lands in `worktree_error` via the existing D-25 failed-state path — never a crashed create.

### Pattern 6: Shell validation

`PUT /api/settings/shell`: value must be in the allowed set (`{"bash"}` in v1.1) → else 400 (generic message, e.g. `Unknown shell.` — the select UI makes this unreachable except via direct API use). The allowed set and the GET `options` array are the same slice — one source of truth, future shells are data (SHELL-02/SHELL-FUT-01).

### Pattern 7: Frontend — hooks, page, route, gear

```ts
// web/src/api/settings.ts
export interface SettingEntry { value: string; default: string; options?: string[] }
export type Settings = Record<string, SettingEntry>;

export function useSettings() {
  return useQuery({ queryKey: ["settings"], queryFn: () => get<Settings>("/api/settings") });
}

export function useSaveSetting(key: string) {
  const qc = useQueryClient();
  return useMutation<SettingEntry, ApiError, string>({
    mutationFn: (value) => put<SettingEntry>(`/api/settings/${key}`, { value }),
    onSuccess: (entry) =>
      qc.setQueryData<Settings>(["settings"], (old) => old && { ...old, [key]: entry }),
  });
}
```

- **No optimistic update needed:** the per-field commit model holds the draft in local component state until the server confirms; `setQueryData` on success makes the saved value the new revert target (UI-SPEC). A 400 leaves the cache untouched and surfaces `error.message` (the canonical server copy) inline. This is simpler and exactly matches the spec's failure isolation.
- `SettingsField` owns: draft state, commit-on-blur/Enter (skip request when draft === saved value), Esc revert, 2s `Saved` flash timer, inline error display (error replaces help text while present, clears on edit or next success), `Reset to default` (PUT default, disabled while in flight).
- **Route:** add `<Route path="/settings" element={<SettingsPage />} />` as a sibling inside the `AppLayout` route in `App.tsx` (sidebar stays visible — verified route structure).
- **Gear:** `ProjectSidebar.tsx` `SidebarFooter` (lines 93–103) becomes one row per UI-SPEC: existing Add-project button gains `flex-1`, plus a Link-wrapped 28px ghost icon button (`<Settings className="size-4" />`, tooltip `Settings`, `aria-label="Settings"`); active state via `useLocation().pathname === "/settings"`.
- **Esc safety:** verified — the only global Escape→board handler lives in `TaskPage.tsx` (plus QuickAdd's local handler). No global listener affects `/settings`; the spec's "Esc never navigates on this page" is already true. Field-level Esc revert needs no stopPropagation gymnastics.
- Loading: 4 group-shaped skeletons; load failure: `Couldn't load settings.` + `Retry loading` (UI-SPEC, mirrors board-failure pattern).

### UI-01 — the exact fix

Verified in `web/src/pages/TaskPage.tsx`:
- **Line 381:** `<div className="max-w-[860px] shrink-0 space-y-2">` wraps the header row (back button → title flex-1 → ellipsis trigger, `align="end"` already set) and `WorktreeMetaLine`. **Fix: `max-w-[860px]` → `w-full`** (per UI-SPEC: `w-full shrink-0 space-y-2`).
- The outer container (line 380, `flex h-full w-full flex-col gap-6 p-4`) and `TaskTabs` (line 458) are already full-width — the trigger will land flush at the same 16px right padding edge where the tab strip ends. Nothing else constrains the header.
- **Keep:** Description tab's `max-w-[860px]` (line 287, prose island), loading skeleton (line 186) and error state (line 198) `max-w-[860px]` — explicitly out of UI-01 scope.

### Anti-Patterns to Avoid

- **Caching settings in the Manager/Service at startup** — recreates the restart-to-apply problem SET-03 forbids; read-at-use in handlers.
- **Validating the branch template with `git check-ref-format --branch`** — shorthand expansion can rewrite the candidate; use positional `refs/heads/<name>`.
- **Whitespace-only tokenization of extra params** — breaks quoted values; use the quote-aware splitter.
- **Seeding default rows in the migration** — couples defaults to migrations; absent row = default.
- **Whole-page Save / whole-object PUT** — violates SET-04 field isolation and the approved save model.
- **Validating claude flags individually** — explicitly out of scope; the field is pass-through.
- **Moving/migrating existing worktrees on base change** — explicitly out of scope (WT-02); tasks' stored absolute paths are already the only thing readers use.
- **Free-text shell field** — curated dropdown only (Out of Scope table).
- **New colors/warning chrome on the skip-permissions field** — UI-SPEC: renders like every other field; amber is reserved for agent waiting.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Branch-name legality | Go reimplementation of git ref rules | `git check-ref-format refs/heads/<n>` subprocess | ~10 obscure rules (verified matrix); git is authoritative and guaranteed present; works without a repo |
| Ref-safe token values | Regex over git's rules | Existing constructive `Slug` charset (`[a-z0-9-]`) | Already proven in Phase 3; sidesteps every edge case by construction |
| `~` expansion | New parsing | Relocated `expandHome` from main.go | Exists, tested in production since Phase 1 |
| Settings upsert semantics | App-level SELECT-then-INSERT/UPDATE | SQLite `ON CONFLICT(key) DO UPDATE` | Atomic, one statement |
| Per-field save UX state machine | Global form library | Local component state + one mutation per field | The commit-on-blur model is ~30 lines per field; a form lib is a new dep and wrong shape |

**Exception (justified hand-roll):** the extra-params tokenizer — external tokenizer packages are new Go modules (locked-stack violation) for a ~40-line grammar with no expansion semantics. Cover it with table-driven tests (quoted spans, mixed quotes, unclosed quote, empty, unicode).

## Common Pitfalls

### Pitfall 1: Treating empty-string `agent_extra_params` as "absent → default"
**What goes wrong:** user clears the field to restore interactive prompts (AGENT-02's whole point); a naive `if v == "" { v = default }` silently re-adds `--dangerously-skip-permissions`.
**How to avoid:** absent ROW = default; stored `""` = "no extra parameters". The upsert must store empty strings; `Get` must only default on `sql.ErrNoRows`.
**Warning signs:** removing the flag in the UI doesn't restore permission prompts on next Start.

### Pitfall 2: Unknown-token detection skipped because "git will catch it"
**What goes wrong:** `{slugg}` is git-LEGAL (braces verified OK) — without the explicit token scan, the literal string `{slugg}` becomes part of every branch name and the canonical "Unknown token" error never fires.
**How to avoid:** token scan (step 2 of the validation order) runs BEFORE check-ref-format.
**Warning signs:** branches named `task/{slugg}-42` appearing in repos.

### Pitfall 3: Leading-dash branch names pass git validation but poison argv
**What goes wrong:** template `-{id}` expands to `-42`; `git check-ref-format refs/heads/-42` PASSES (verified), but `git worktree add -b -42 ...` misparses.
**How to avoid:** explicit leading-`-` reject in validation, mapped to "Not a valid git branch name."
**Warning signs:** cryptic git option-parsing errors in `worktree_error`.

### Pitfall 4: Settings select/Saved flash drifting from UI-SPEC copy
**What goes wrong:** the checker audits verbatim copy; improvised help text, error text, or green confirmation states fail the gate.
**How to avoid:** every user-facing string in the settings page comes from the UI-SPEC tables (page sub-line, four help lines, four canonical branch errors, two worktree errors, `Saved`, `Reset to default`, `Couldn't save. Try again.`). `Saved` is muted zinc — never green.
**Warning signs:** any settings string not findable in 06-UI-SPEC.md.

### Pitfall 5: Manager grows a `*sql.DB`
**What goes wrong:** threading the DB into `session.Manager` (or `worktree.Service`) to read settings couples the engine to storage, breaks the existing test seams (fake-claude stub, t.TempDir roots), and invites caching.
**How to avoid:** handlers read settings; values travel via `SpawnOpts` fields / `provisionWorktree` locals. Managers stay pure.
**Warning signs:** `import "database/sql"` appearing in internal/session or internal/worktree.

### Pitfall 6: Worktree-base trailing slash / `~` stored-vs-expanded confusion
**What goes wrong:** default is `~/.kangent/worktrees/` (with trailing slash, with tilde). Comparing expanded values against the stored default breaks `Reset to default` visibility; expanding at save loses the user's `~` display form.
**How to avoid:** store and compare the RAW value (as entered / as default); expand `~` only at use (`provisionWorktree`) and during the exists-check in validation. `filepath.Join` neutralizes the trailing slash.
**Warning signs:** `Reset to default` showing on an untouched field, or paths with literal `~` directories on disk.

### Pitfall 7: Forgetting the retry path when wiring the branch template
**What goes wrong:** wiring template/base only into task-create; the worktree Retry endpoint (`POST /api/tasks/{id}/worktree`) builds a stale `task/<slug>-<id>` branch.
**How to avoid:** both paths already flow through `provisionWorktree` (verified: tasks.go:180 and worktrees.go:85) — wire there and only there. Note: a branch leaked by a failed create under template A is intentionally NOT reused after switching to template B; `Create`'s existing-branch reuse keys on the exact expanded name. Acceptable; D-34 keeps branches anyway.
**Warning signs:** retry after a template change producing the old-style branch name.

### Pitfall 8: New `put` helper diverging from client.ts conventions
**What goes wrong:** ad-hoc fetch in settings.ts bypasses ApiError extraction; canonical server error copy never reaches the inline error.
**How to avoid:** add `put` to client.ts exactly like `patch` (4 lines); the `{"error": msg}` pipe then works untouched.
**Warning signs:** inline errors showing "Bad Request" instead of "Template must include {id}…".

## Code Examples

### Verified spawn composition (what the manager will exec)

```go
// All flags verified composing on claude v2.1.173 (probe 2026-06-11):
// straight to prompt, "bypass permissions on" footer, no acceptance dialog,
// overlay parse-clean. Extras append AFTER fixed flags; same for --resume.
args := []string{
    "--session-id", claudeSessionID,            // or "--resume", storedID
    "--settings", buildOverlayJSON(cfg.BaseURL, cfg.Token, id),
}
args = append(args, opts.ExtraArgs...)          // e.g. ["--dangerously-skip-permissions"]
cmd := exec.Command(bin, args...)
```

### Branch validation core (save-time and create-time share it)

```go
// CheckRefFormat: positional form, no repo needed (verified git 2.43.0).
func CheckRefFormat(ctx context.Context, name string) error {
    if strings.HasPrefix(name, "-") {
        return errors.New("Not a valid git branch name.")
    }
    if exec.CommandContext(ctx, "git", "check-ref-format", "refs/heads/"+name).Run() != nil {
        return errors.New("Not a valid git branch name.")
    }
    return nil
}
```

### provisionWorktree delta (the single choke point)

```go
// Before (tasks.go:73-76):
//   slug := worktree.Slug(title)
//   branch = "task/" + slug + "-" + strconv.FormatInt(taskID, 10)
//   path = wt.PathFor(repoPath, slug, taskID)
// After:
slug := worktree.Slug(title)
tpl, _ := settings.Get(db, settings.KeyBranchTemplate)      // absent → default
branch = settings.ExpandTemplate(tpl, slug, taskID, title)  // {slug} {id} {title}
if err := settings.CheckRefFormat(ctx, branch); err != nil { /* → worktree_error, D-25 */ }
baseRaw, _ := settings.Get(db, settings.KeyWorktreeBase)
baseDir, err := settings.ExpandHome(baseRaw)                 // raw stored; expanded at use
path = worktree.PathUnder(baseDir, repoPath, slug, taskID)   // repo-basename structure kept
```

### Worktree-base save-time validation (canonical copy)

```go
v := strings.TrimSpace(value)
if v != "~" && !strings.HasPrefix(v, "~/") && !strings.HasPrefix(v, "/") {
    return errors.New("Path must be absolute or start with ~/")
}
abs, _ := ExpandHome(v)
if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
    return fmt.Errorf("Not a directory: %s", v)   // exists-but-file only; nonexistent is fine (MkdirAll at use)
}
```
No inside-a-repo check: the UI-SPEC error set is the contract (two messages only); anything more is unsanctioned copy. (Single-user app; `Create` would surface any real git conflict into `worktree_error` anyway.)

## State of the Art

| Old (Phase 4/5 state) | Current (this phase) | Impact |
|---|---|---|
| claude v2.1.170 verified (Phase 4); v2.1.173 partially re-verified (Phase 5 resume) | v2.1.173 installed; full flag composition re-verified today incl. `--dangerously-skip-permissions` | Overlay + session-id behavior unchanged; bypass composes; no acceptance dialog |
| D-51: interactive permission posture, no skip flag | AGENT-02 default-on `--dangerously-skip-permissions` (deliberate reversal) | Log in PROJECT.md Key Decisions at transition; amber waiting mostly disappears under the default (documented above) |
| Worktree root fixed at startup (`main.go` → `NewService`) | Settings-driven base, read per creation | `Service.Root`/`PathFor` give way to `PathUnder(base, ...)`; main.go drops `wtRoot` |
| Branch name literal `task/<slug>-<id>` | Template-driven with validation | Same default output — zero behavior change until the user edits the template |
| Bash tabs run `$SHELL` (user's login shell) | Settings value `bash` always | Specified change (SHELL-01/02); note in plan verification |

## Open Questions

1. **Does the bypass acceptance dialog exist on other machines / future claude versions?**
   - What we know: absent on this host's v2.1.173 with `bypassPermissionsModeAccepted` unset (verified — straight to prompt).
   - What's unclear: whether some environments (e.g. root, managed settings) re-introduce a gate.
   - Recommendation: nothing to build; if a dialog ever appears it renders fine in the PTY and the user answers once (same posture as the Phase 4 trust-dialog finding). Not a blocker.
2. **Exact amber-waiting behavior under bypass with MCP elicitation in play.**
   - What we know: `permission_prompt` notifications logically cannot fire under bypass; `elicitation_dialog` should still fire.
   - Recommendation: confirm during phase verification with a real session that the dot stays green/gray through a tool-heavy turn under the default settings; no code depends on the answer.
3. **`{title}` cap value (100 recommended).**
   - Pure discretion; any cap that keeps tokens in the constructive charset preserves the validation invariance. Planner picks the constant.
4. **Strict vs lenient unclosed-quote handling in extra params** (lenient recommended — see Pattern 4; the UI-SPEC defines no error copy for this field).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| claude CLI | extra-params composition (AGENT-01/02) | ✓ | 2.1.173 (`~/.local/bin/claude`) — verified today | — |
| git | `check-ref-format` validation + worktree ops | ✓ | 2.43.0 — verified today; check-ref-format works repo-less | — |
| Go toolchain | backend build/tests | ✓ | 1.26.0 (Phase 4 verified; `make build` shipped v1.0 yesterday) | — |
| Node | shadcn CLI (`select`, `label`) + Vite build | ✓ | 24.4.1 (Phase 4 verified) | — |
| shadcn registry access | the two new blocks | ✓ (components.json present, radix-nova, `registries: {}`) | CLI latest | blocks can be vendored by hand if offline |

**Missing dependencies with no fallback:** none.

## Sources

### Primary (HIGH confidence)
- **Empirical probes, this host, 2026-06-11:** `claude --help` (v2.1.173 flag inventory); scripted PTY composition probe (`--session-id` + exact Phase-4 `--settings` overlay + `--dangerously-skip-permissions` → prompt with bypass on, raw capture `/tmp/claude-probe6/compose.raw`); `--resume` composition probe (`/tmp/claude-probe6/resume.raw`); `~/.claude.json` inspection (`bypassPermissionsModeAccepted` absent); 27-case `git check-ref-format refs/heads/<n>` matrix + `--branch` expansion-semantics check on git 2.43.0, run outside a repo
- **Repo inspection (all current source):** `internal/session/{manager,agent}.go` (spawn argv construction, shell hardcode lines 147–151, AgentConfig), `internal/api/{tasks,worktrees,sessions,routes}.go` (provisionWorktree choke point, both creation paths, handler/DB layering), `internal/worktree/worktree.go` (Slug, PathFor, Create/MkdirAll), `internal/store/{migrate.go,migrations/}` (goose embed, next number 00004), `cmd/kangent/main.go` (expandHome, wtRoot, AgentConfig wiring), `web/src/pages/TaskPage.tsx` (UI-01 wrapper line 381; tab strip full-width verified), `web/src/App.tsx` (route shell), `web/src/components/sidebar/ProjectSidebar.tsx` (footer lines 93–103), `web/src/api/{client,agents,worktrees}.ts` (helper + hook patterns; `put` missing), `web/components.json`, `web/src/components/ui/` inventory (no select/label yet), Escape-handler grep (TaskPage + QuickAdd only)
- 04-RESEARCH.md (verified overlay shape, hook semantics, SessionStart canary, Phase 5 resume verification carried in manager.go comments)
- 06-UI-SPEC.md (approved contract: save model, canonical error copy, field definitions, gear placement, UI-01 contract)

### Secondary (MEDIUM confidence)
- Notification-hook silence under bypassed permissions (logical consequence of bypass + Phase 4 verified hook semantics; not separately probed — Open Question 2)
- Commander last-value-wins for duplicate scalar options (consistent with observed CLI behavior; the SessionStart canary covers the failure mode regardless)

### Tertiary (LOW confidence)
- Bypass acceptance-dialog behavior on other machines/claude versions (Open Question 1; no code depends on it)

## Metadata

**Confidence breakdown:**
- Claude flag composition: HIGH — verified on the exact installed binary with the exact production overlay
- Branch validation semantics: HIGH — full matrix verified on git 2.43.0, positional form, repo-less
- Wiring points: HIGH — every insertion site read from current source with line numbers
- Storage/API design: HIGH — standard pattern, maps 1:1 onto the approved UI-SPEC save model
- Tokenizer grammar choice & {title} cap: MEDIUM — discretion areas with recommendations, no external dependency
- Status-dot behavior under bypass: MEDIUM — logical inference, flagged for phase verification

**Research date:** 2026-06-11
**Valid until:** ~2026-07-11 — re-verify the flag-composition probe if the installed claude moves past 2.1.x; git 2.43 ref rules are stable
