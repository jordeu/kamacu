# Phase 11: PR Review Column - Research

**Researched:** 2026-06-13
**Domain:** `gh` CLI shell-out (read-only PR review queue) + best-effort Go service proxy + TanStack-polled React column
**Confidence:** HIGH (every `gh` command, JSON field, exit code, and rollup shape verified against live `gh` 2.82.0 on this host; every reuse precedent read from the actual codebase)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

Pre-settled (locked going in — NOT re-litigated):
- **D-00a:** Qualifier is `user-review-requested:@me`, `--state open`, **drafts excluded** (GHCOL-02). Direct requests only — `review-requested:@me` floods with all team requests (~100 PRs). GitHub auto-removes the user from the set on review submit, so the column self-empties for free — **do not over-cache against it**.
- **D-00b:** Fetch via `gh pr list -R <owner/name> --search "user-review-requested:@me" --state open --json <rich set>` run with `cmd.Dir` = the project's repo path (selects the right host/account for multi-`gh`-account / enterprise). **Never** `gh search prs` (thin JSON, 30/min search budget). `gh pr list` lives on the core budget.
- **D-00c:** `internal/github` gains a best-effort `Service` modeled on `internal/quota`: per-repo cache with TTL/floor/backoff, typed degraded states, and an **always-200** endpoint `GET /api/projects/{id}/pull-requests[?refresh=1]`, **toggle-gated** (`github_integration`) AND **link-gated** (project has `github_repo`). `gh` is a soft dependency — degrade, don't break (GHSET-03). Do not trust `gh auth status` exit codes alone (cli/cli#8845) — prefer presence via `LookPath` + parse.
- **D-00d:** `statusCheckRollup` (raw CheckRun/StatusContext array) is reduced **server-side** to a single `pass | fail | pending | none` value. The raw array is never shipped to the browser.

Card content (GHCOL-03) — strict minimal:
- **D-01:** PR cards show exactly the GHCOL-03 set and nothing more: PR number, title, author, relative "updated X ago" time, and the checks pill (D-04). The card is a presentational variant of `TaskCard`.
- **D-02:** Diffstat (+/−), fork-PR indicator, head→base branch line are **NOT** shown in Phase 11 (GHCARD-01/02/03 Future). **Fetch the rich `--json` set anyway** (one call; Phase 12/13 consume `headRefName`/`headRefOid`/`baseRefName`/`isCrossRepository`); just don't render the deferred fields.

Checks pill (GHCOL-03) — small colored dot, "none" omitted:
- **D-03:** Render the CI rollup as a small colored dot using the card's existing dot visual language (`StatusDot`): green = pass, red = fail, amber = pending.
- **D-04:** When a PR has no checks (`none`), render **nothing** — no dot, no reserved gutter (the dotless-task-card rule: dotless cards must not shift layout).

Collapse + count persistence (GHCOL-01/06) — localStorage, per-project, default expanded:
- **D-05:** Collapse state persisted in `localStorage`, keyed per-project (project id). No SQLite, no new migration, no new endpoint.
- **D-06:** Column defaults to expanded.
- **D-07:** Collapsed column shows a PR count badge reflecting current GitHub state. Count source = the same query data that fills the expanded list.

Click behavior — inert body + ↗ to GitHub:
- **D-08:** Card body is **inert** in Phase 11 — clicking it does nothing. Whole-card click is reserved for Phase 12.
- **D-09:** A small external-link ↗ icon opens the PR on github.com in a new tab via the fetched `url` field.

Auto-poll & refresh (GHCOL-04):
- **D-10:** Mirror the Phase 7 quota pattern verbatim: TanStack `useQuery(['pull-requests', projectId])` with `refetchInterval: 60_000` and `refetchIntervalInBackground: false`, plus a manual refresh that hits `?refresh=1` and writes back to cache (`useRefreshQuota` precedent).
- **D-11:** Only the active/visible project's column polls. Combined with server-side per-repo cache + backoff, keeps `gh` call rate bounded. No explicit rate-limit polling needed.

Degraded / empty / loading (GHCOL-05):
- **D-12:** All states render inline in the column, never modal, never blocking the board. Loading affordance on first fetch; empty "you're all caught up"; distinct degraded states for `gh` missing / unauthenticated / error as a quiet inline note. Typed states from the quota model: `ok`/`no_gh`/`auth_required`/`disabled`/`error`.

Column placement & ordering:
- **D-13:** Review column is a **sibling** of the status columns, to the right of Done in the board's flex row. **Not** a `dnd-kit` droppable. Min-width/styling visually echoes task columns.
- **D-14:** List PRs most-recently-updated first (`updatedAt` desc).

### Claude's Discretion
- Auto-poll interval value (defaulted to 60s per D-10) and backoff/TTL constants (mirror quota).
- Exact degraded/empty/loading copy and placement (now pinned by UI-SPEC).
- Column min-width, header styling, count-badge styling (now pinned by UI-SPEC).
- The `internal/github.Service` cache shape and the `PRSummary` struct field set (fetch rich, render minimal).
- Whether the checks dot reuses `StatusDot` directly or a small sibling component (D-03).

### Deferred Ideas (OUT OF SCOPE)
- Diffstat (+/−), fork-PR pill, head→base branch line on cards — GHCARD-01/02/03. JSON is fetched; rendering deferred.
- `reviewDecision` pill, labels, comment-count, stale-aging sort — GHCARD-04 / GHFILT.
- Include team review requests (`review-requested:@me`) / show drafts — GHFILT-01/02.
- Cross-project "all repos" review inbox / author-side PR view — GHWIDE-01/02.
- SQLite/cross-browser persistence of collapse state — chose localStorage per-project (D-05).
- Opening a PR card into a worktree-backed review workspace — Phase 12 (GHREV-*).
- Auto-cleanup/reaper reconciliation of merged-closed PR worktrees — Phase 13 (GHCLN-*).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| GHCOL-01 | Collapsible "Review" column appears on the right when project is linked + integration on | Render gate = `useSettings().github_integration === 'on'` AND `project.github_repo != null` (both already exposed; §Frontend Integration). Column appended after Done in `Board.tsx` flex row, NOT a droppable (D-13, §Column Placement). |
| GHCOL-02 | Lists open PRs where review requested directly from user (`user-review-requested:@me`), excluding drafts | Verified literal command: `gh pr list -R <owner/name> --search "user-review-requested:@me" --state open --json …` (§gh Command #1). Draft exclusion: add `draft:false` to the search string (or filter `isDraft` server-side; both verified). |
| GHCOL-03 | Card with number, title, author, relative "updated X ago", pass/fail/pending checks pill | All fields confirmed valid `--json` fields with verified live shapes: `number`, `title`, `author.login`, `updatedAt`, `url`, `statusCheckRollup` (§JSON Field Table). Rollup reduced server-side to `pass\|fail\|pending\|none` (§Checks Rollup Algorithm). |
| GHCOL-04 | Auto-refresh on interval (paused when tab hidden), manual refresh, mirrors live GitHub state | TanStack `useQuery` + `refetchInterval: 60_000` + `refetchIntervalInBackground: false` (verbatim `useQuota`); manual refresh hits `?refresh=1` writes-back (verbatim `useRefreshQuota`). Self-emptying is automatic — server list is truth (§Frontend Polling, PITFALL 2). |
| GHCOL-05 | Loading, empty ("you're all caught up"), degraded (`gh` missing/unauth/error) states inline, non-blocking | Always-200 endpoint with `state` discriminator (`ok\|no_gh\|auth_required\|disabled\|error`). Exit-code + stderr classification verified live (§Degraded-State Detection). Frontend branches on `state`/`stale`, never HTTP status (verbatim quota model). |
| GHCOL-06 | Collapse/expand; collapsed shows PR count; collapse state remembered | localStorage key `kangent:review-collapsed:{projectId}`, default expanded (D-05/D-06, UI-SPEC). Count from the same query data (D-07). No backend work for persistence. |
</phase_requirements>

## Summary

This phase is a **pure read path** with three thin layers, each a near-clone of an in-repo precedent that already shipped:

1. **`internal/github` gains `ListReviewRequested` + a best-effort `Service`** modeled byte-for-byte on `internal/quota` (per-repo cache, TTL/attempt-floor/backoff, typed degraded states, drop-cache-on-N-consecutive-failures, in-flight dedup). The single new I/O primitive is one verified `gh pr list` invocation; everything else is the quota cache state machine retargeted from "token fingerprint" to "owner/name" keying.

2. **One always-200 endpoint** `GET /api/projects/{id}/pull-requests[?refresh=1]`, gated by the `github_integration` settings KV AND the project's `github_repo` link, returning `{ state, stale, fetchedAt, prs }` — the exact shape contract of `UsageRoutes` (one HandleFunc, `qs.Get(ctx, refresh=="1")`).

3. **A Review column** that is a presentational sibling of the existing `Column`/`TaskCard`, appended to the right of Done in `Board.tsx`'s flex row, **outside** the dnd machinery (no `useDroppable`/`useSortable`). Its data hook (`usePullRequests`/`useRefreshPullRequests`) is a clone of `useQuota`/`useRefreshQuota`.

The work is overwhelmingly **pattern-replication, not invention**. Zero new npm deps, zero new Go deps, zero migrations, zero new settings keys. The genuinely novel surface is exactly two things: (a) the verified `gh pr list` command + JSON parsing, and (b) the `statusCheckRollup` → `pass/fail/pending/none` reduction. Both are nailed down below with live-verified data.

**Primary recommendation:** Build `internal/github.Service` as a structural copy of `internal/quota.Service` (same fields, same Get() gate ladder, same `recordFailureLocked`/`resultLocked`), keyed per `owner/name` instead of per token fingerprint, fetching via `exec.CommandContext("gh", "pr", "list", "-R", repo, "--search", "user-review-requested:@me draft:false", "--state", "open", "--json", "<rich set>")` with `cmd.Dir` = the project's `repo_path`. Classify exit code + stderr into `no_gh`/`auth_required`/`error`. Reduce `statusCheckRollup` server-side. Then clone `usage.ts`→`pullRequests.ts` and the `QuotaIndicator` render-mapping into a `ReviewColumn` rendered after Done.

## Project Constraints (from CLAUDE.md)

These are authoritative directives from the project CLAUDE.md and the v1.3 decision log (STATE.md). The plan MUST NOT contradict them:

- **Tech stack is fixed**: Go backend (1.26), React 19 + Vite + TanStack Query frontend. No new frameworks.
- **Single binary, local-only**: API + embedded SPA at localhost. No external services.
- **Shell out, never reimplement**: the app spawns the system `gh` CLI (as it already spawns `git`); it **never reimplements GitHub auth** and **never stores credentials** (GHSET-03, Out-of-Scope table). Access is via the host's already-authenticated `gh`.
- **Arg-array exec only, never `sh -c`**: every existing shell-out (`validateRepoPath`, `githubOrigin`, `ValidateRepo`) uses `exec.Command`/`exec.CommandContext` with an arg array. Match this exactly — `gh … --json` output is parsed as machine output, never human text.
- **Degrade, don't break (GHSET-03)**: a missing/unauthenticated `gh` or any GitHub failure must never break the board or any existing feature. Endpoints are always-200 with a typed `state` discriminator; the frontend never branches on HTTP status.
- **OFF cascade (GHSET-02)**: when `github_integration` is off, NO GitHub UI appears and the app is byte-for-byte pre-v1.3. The Review column render gate must respect `useSettings().github_integration === 'on'`.
- **No new migration through Phase 12**: Phase 10 banked migration 00007 covering all v1.3 columns. Phase 11 adds NO schema (D-05 collapse state is localStorage).
- **GSD workflow enforcement**: file edits happen only through a GSD command — this is research only.

## Standard Stack

No new dependencies. Everything Phase 11 needs already exists in the repo. This table documents what to reuse.

### Backend (Go) — all stdlib + existing internal packages
| Package | Source | Purpose | Why |
|---------|--------|---------|-----|
| `os/exec` | stdlib | Shell out to `gh pr list` | Same pattern as `internal/github.ValidateRepo` and `projects.githubOrigin`. `exec.CommandContext` with arg array + `cmd.Dir`. |
| `encoding/json` | stdlib | Parse `gh --json` output | `statusCheckRollup` needs a lenient struct (two `__typename` variants). |
| `sync` + `time` | stdlib | The `Service` cache state machine | Verbatim from `internal/quota` (mutex, TTL, floor, backoff). |
| `log/slog` | stdlib | Degrade-path debug logging | Quota logs `slog.Debug("quota fetch degraded", …)`; mirror it. |
| `internal/github` (existing) | repo | `Available()` (LookPath), `ParseRepoRef` | Extend, don't replace. `Available()` is the `no_gh` gate. |

### Frontend (React) — all already installed
| Package | Version (installed) | Purpose | Reuse precedent |
|---------|---------------------|---------|-----------------|
| `@tanstack/react-query` | 5.x (in repo) | Poll + manual refresh + cache | `web/src/api/usage.ts` |
| `lucide-react` | ^1.17.0 | `ChevronDown`/`ChevronRight`, `RefreshCw`, `TriangleAlert`, `ExternalLink`/`ArrowUpRight` | All already imported elsewhere (verified: `RefreshCw`+`TriangleAlert` in QuotaIndicator, `ChevronRight` in DiffFileSection) |
| `@/components/ui/{button,collapsible,tooltip,skeleton}` | present | Refresh control, collapse, dot tooltip, loading | All four files confirmed present in `web/src/components/ui/` |
| `@/components/StatusDot` | present | Checks-dot visual language | `dotMeta` color classes are the exact pass/fail/pending hues |

**Installation:** NONE. `components.json` `"registries": {}`; UI-SPEC Registry Safety confirms zero new installs.

**Version verification:** Host `gh` is **2.82.0** (2025-10-15), meets the STACK.md floor of 2.82.0. Latest `gh` is **2.94.0** (2026-06-10) — the host is older but every command/field used here (`--search`, `statusCheckRollup`, `draft:false`) is verified working on 2.82.0, so 2.82.0 is the safe documented floor. Do not pin to 2.94 features.

## gh Command (verified live against `gh` 2.82.0)

### Command #1 — the review-queue list (GHCOL-02, GHCOL-03)

**Literal command (the one to put in `exec.CommandContext`):**
```
gh pr list \
  -R <owner/name> \
  --search "user-review-requested:@me draft:false" \
  --state open \
  --json number,title,author,updatedAt,url,isDraft,statusCheckRollup,headRefName,headRefOid,baseRefName,isCrossRepository,additions,deletions
```

As an arg array (the form the code must use — never `sh -c`):
```go
exec.CommandContext(ctx, "gh", "pr", "list",
    "-R", repo, // canonical owner/name from projects.github_repo
    "--search", "user-review-requested:@me draft:false",
    "--state", "open",
    "--json", "number,title,author,updatedAt,url,isDraft,statusCheckRollup,headRefName,headRefOid,baseRefName,isCrossRepository,additions,deletions",
)
// cmd.Dir = project.repo_path   ← selects the right gh host/account (D-00b)
```

**Verified facts:**
- `gh pr list` accepts `--search` (advanced issue/PR search syntax) AND `-R [HOST/]OWNER/REPO`. Confirmed in `gh pr list --help`.
- `user-review-requested:@me` is the correct qualifier (direct requests only). Returned `[]` cleanly on a repo where this user has no requests — proving the empty case is a clean empty array, not an error.
- **Draft exclusion: two equivalent verified options** — put `draft:false` in the `--search` string (verified valid), OR fetch `isDraft` and filter server-side. Recommendation: include `draft:false` in the search (matches GitHub's filtered page exactly per GHCOL-02) AND still fetch `isDraft` defensively. (`-is:draft` / `-draft:true` also work but `draft:false` is the clearest.)
- `--state open` is kept explicit even with `--search` (belt-and-suspenders; `--search` defaults to open but stating it documents intent).
- **`statusCheckRollup` IS a valid `--json` field on `gh pr list`** — confirmed in the JSON-fields list. **This means the CI rollup comes from the SAME list call — NO N+1 per-PR `gh pr checks` calls.** This is the single most important rate-limit win.
- Empty result = `[]` on stdout, exit 0.

**Why NOT `gh search prs`** (locked, re-confirmed): thinner JSON (no `statusCheckRollup`), and it bills the **search API budget (30 req/min)** rather than the GraphQL/core budget — far easier to exhaust under polling.

**Why NOT `gh pr checks <num>`** (the N+1 trap): it requires one call per PR. With `statusCheckRollup` on the list call, it is entirely unnecessary for Phase 11.

### JSON Field Table (live-verified shapes)

| `--json` field | Type / shape (verified) | Used in Phase 11? | Notes |
|----------------|-------------------------|-------------------|-------|
| `number` | int (e.g. `13642`) | YES (card #) | |
| `title` | string | YES (card title) | |
| `author` | object `{login, name, id, is_bot}` | YES — use `author.login` | snake_case `is_bot` inside the object (gh quirk); `login` is what GHCOL-03 needs |
| `updatedAt` | RFC3339 string `"2026-06-12T19:30:13Z"` | YES (relative time + D-14 sort) | parse to time for sort; pass ISO to frontend for `formatAgo` |
| `url` | string `"https://github.com/owner/repo/pull/N"` | YES (↗ link, D-09) | |
| `isDraft` | bool | filter only (drafts excluded) | belt-and-suspenders vs `draft:false` search |
| `statusCheckRollup` | array of CheckRun \| StatusContext (see below) | YES → reduced to pill | **reduce server-side, never ship raw (D-00d)** |
| `headRefName` | string | NO (fetch for Phase 12) | D-02: fetched, not rendered |
| `headRefOid` | string (SHA) | NO (fetch for Phase 12/13) | |
| `baseRefName` | string | NO (fetch for Phase 12 diff base) | |
| `isCrossRepository` | bool | NO (fetch for Phase 12 fork handling) | |
| `additions` / `deletions` | int | NO (fetch for GHCARD-01) | D-02 deferred render |

### `statusCheckRollup` array shape (live-verified — TWO variants)

The array mixes two `__typename`s. **Both observed live** (CheckRun in `cli/cli`, StatusContext in `kubernetes/kubernetes`).

**CheckRun** (GitHub Actions / Checks API):
```json
{"__typename":"CheckRun","status":"COMPLETED","conclusion":"SUCCESS","name":"lint","workflowName":"Lint","detailsUrl":"…","startedAt":"…","completedAt":"…"}
```
- `status`: `QUEUED` | `IN_PROGRESS` | `COMPLETED` | `WAITING` | `PENDING` | `REQUESTED`
- `conclusion` (only meaningful when `status==COMPLETED`): `SUCCESS` | `FAILURE` | `NEUTRAL` | `CANCELLED` | `SKIPPED` | `TIMED_OUT` | `ACTION_REQUIRED` | `STARTUP_FAILURE` | `STALE` | `null`

**StatusContext** (legacy commit statuses, e.g. Prow `tide`, `EasyCLA`):
```json
{"__typename":"StatusContext","context":"tide","state":"PENDING","targetUrl":"…","startedAt":"…"}
```
- `state`: `SUCCESS` | `FAILURE` | `PENDING` | `ERROR` | `EXPECTED`
- **no `conclusion` field** — uses `state` instead.

**Lenient Go struct** (decode both into one shape):
```go
type checkEntry struct {
    Typename   string `json:"__typename"`
    Status     string `json:"status"`     // CheckRun
    Conclusion string `json:"conclusion"` // CheckRun
    State      string `json:"state"`      // StatusContext
}
```

### Checks Rollup Algorithm (D-00d, D-03, D-04) — verified logic

Reduce the array to one of `pass` | `fail` | `pending` | `none`:

```
if len(rollup) == 0:            -> "none"   (render nothing, D-04)

per-entry "signal":
  if entry.Typename == "StatusContext":
     SUCCESS                    -> ok
     PENDING | EXPECTED         -> pending
     FAILURE | ERROR            -> fail
  else (CheckRun):
     if status != COMPLETED     -> pending     (QUEUED/IN_PROGRESS/WAITING/PENDING/REQUESTED)
     else by conclusion:
       SUCCESS                  -> ok
       NEUTRAL|SKIPPED|STALE    -> ok   (non-failing; do NOT turn the dot red)
       FAILURE|TIMED_OUT|CANCELLED|ACTION_REQUIRED|STARTUP_FAILURE -> fail
       "" / null                -> pending   (defensive: completed-but-no-conclusion)

aggregate (precedence: fail > pending > ok):
  any fail    -> "fail"   (red, D-03)
  else any pending -> "pending"  (amber, D-03)
  else        -> "pass"   (green, D-03)
```

**Critical detail (verified live):** real PRs are full of `SKIPPED` and `NEUTRAL` conclusions (the `cli/cli` sample was ~30% SKIPPED). Treating SKIPPED/NEUTRAL/STALE as failing would paint almost every PR red. Map them to **ok** (non-failing). This is how GitHub's own merge-status UI behaves.

The reduced value is the ONLY checks data shipped to the browser. Suggested wire field: `checks: "pass"|"fail"|"pending"|"none"` on each `PRSummary`.

## Degraded-State Detection (GHCOL-05, D-00c) — exit codes + stderr, verified live

The backend must classify a `gh` failure into a typed state. **All of the following were probed directly on this host.** Note: `gh`'s documented exit-code-4-for-auth is **unreliable** (cli/cli#9338: 401 can exit 1; cli/cli#8845: `gh auth status` returns wrong codes) — so classification combines exit code AND stderr substring sniffing, exactly as CONTEXT D-00c warns.

| Scenario | How to detect (verified) | Typed `state` |
|----------|--------------------------|---------------|
| `gh` binary absent | `github.Available()` (`exec.LookPath("gh")`) returns false — **don't even run the command** | `no_gh` |
| No GitHub account configured at all | exit code **4**, stderr: `To get started with GitHub CLI, please run: gh auth login` | `auth_required` |
| Bad/expired token | exit **1**, stderr contains `401` / `Bad credentials` (real call), or `gh auth status` shows `Failed to log in` | `auth_required` |
| Repo not found / no access | exit **1**, stderr: `GraphQL: Could not resolve to a Repository with the name '…'` | `error` |
| Malformed `-R` arg | exit **1**, stderr: `expected the "[HOST/]OWNER/REPO" format` (should never happen — we store canonical owner/name) | `error` |
| Network failure / timeout | non-nil `exec` err / context deadline / non-zero exit, stderr has no auth markers | `error` |
| Success | exit **0**, stdout is valid JSON array (possibly `[]`) | `ok` |

**Recommended classification (server-side):**
```
if !github.Available():                    -> no_gh   (never spawn)
run gh; capture stdout, stderr, exit err
if err == nil and json parses:             -> ok (prs = parsed)
exitCode == 4
  OR stderr contains "gh auth login"
  OR stderr contains "401"
  OR stderr contains "Bad credentials":    -> auth_required
otherwise:                                 -> error
```
- Capture stderr via `cmd.Stderr = &buf` (or `CombinedOutput` and split) — `exec.Output()` already returns stderr inside `*exec.ExitError.Stderr`, but a dedicated `bytes.Buffer` on `cmd.Stderr` is cleaner for substring matching.
- **Never log the token or full stderr at info level** — quota's precedent logs only `slog.Debug` with a coarse kind. Match it.
- `disabled` state never reaches the wire on a normal request — it is the gate result (endpoint returns it / 404s when `github_integration` is off or project not linked). The frontend's render gate (`useSettings`) means `disabled` is never even fetched in practice (UI-SPEC: "should not surface as copy").

## Architecture Patterns

### Recommended structure (extends existing `internal/github`)
```
internal/github/
├── github.go        # EXISTING: Available, ParseRepoRef, ValidateRepo (untouched)
├── service.go       # NEW: Service (cache/TTL/floor/backoff/typed-state) — clone of quota.Service
├── prlist.go        # NEW: ListReviewRequested(ctx, repoDir, repo) + statusCheckRollup reduction
└── *_test.go        # NEW: rollup-reduction table tests + classification tests (inject a fake gh via Config seam)

internal/api/
└── github.go        # EXISTING githubStatus + ADD PullRequestRoutes / a handler for
                     #   GET /api/projects/{id}/pull-requests[?refresh=1]

web/src/api/
└── pullRequests.ts  # NEW: clone of usage.ts (usePullRequests / useRefreshPullRequests + types)

web/src/components/board/
├── ReviewColumn.tsx # NEW: column shell (collapse, header, refresh, state rendering)
├── PRCard.tsx       # NEW: presentational variant of TaskCard (CardRow-style)
└── Board.tsx        # EDIT: append <ReviewColumn> after the STATUSES.map, outside dnd droppables
```

### Pattern 1: The best-effort `Service` (clone of `internal/quota.Service`)
**What:** A per-repo-keyed cache with the exact gate ladder quota uses.
**When:** This IS the backend of Phase 11.
**Map quota → github:**

| quota.Service | github.Service equivalent |
|---------------|---------------------------|
| keyed by token fingerprint (`tokenFP`) | keyed by `owner/name` (`map[string]*repoEntry` OR a single-entry cache per Service instance keyed by repo) |
| `readCredentials` every call | nothing — repo is the key, no per-call secret read |
| `fetchUpstream` (HTTP GET) | `runGH` (`exec.CommandContext` + JSON parse) |
| states `ok`/`no_credentials`/`auth_expired`/`error` | states `ok`/`no_gh`/`auth_required`/`disabled`/`error` |
| `cacheTTL=60s`, `attemptFloor=10s`, `minBackoff429=30s`, `maxFailures=3` | reuse the SAME constants (60s TTL matches the 60s poll; 10s floor protects `?refresh=1` spam; backoff on repeated `gh` failure; drop cache after 3 fails) |
| `inflight` dedup | same |
| `Result{State,Stale,FetchedAt,Windows}` | `Result{State,Stale,FetchedAt,PRs}` |

**Key difference — multi-repo:** quota has one global cache (one Claude account). Here, each linked project is a distinct repo, so the Service holds a `map[repo]*entry` guarded by the mutex (or instantiate per-request keyed by repo). The 60s-TTL / backoff state is **per repo**. D-11 (only the visible project polls) keeps the live key-set to ~1.

**Example (the Get gate ladder, adapted from quota.go:220-303):**
```go
// Source: internal/quota/quota.go Get() — replicate the gate order verbatim
func (s *Service) Get(ctx context.Context, repo, repoDir string, force bool) Result {
    s.mu.Lock()
    e := s.entry(repo) // get-or-create per-repo cache entry
    now := s.now()
    if (!force && e.cached != nil && now.Sub(e.fetchedAt) < cacheTTL) ||
        now.Before(e.backoffUntil) ||
        now.Sub(e.lastAttempt) < attemptFloor ||
        e.inflight {
        res := e.resultLocked()
        s.mu.Unlock()
        return res
    }
    e.inflight = true
    e.lastAttempt = now
    s.mu.Unlock()

    prs, state, runErr := s.runGH(ctx, repo, repoDir) // exec + classify

    s.mu.Lock()
    defer s.mu.Unlock()
    e.inflight = false
    switch state {
    case "ok":
        e.cached = prs; e.fetchedAt = s.now(); e.failures = 0; e.lastErr = ""
    default: // no_gh / auth_required / error
        e.recordFailureLocked(state)
    }
    return e.resultLocked()
}
```

### Pattern 2: The always-200 endpoint (clone of `UsageRoutes`)
**What:** One HandleFunc, gated, always 200, `state` carries degradation.
**Example (adapted from internal/api/usage.go + the gate from projects.go):**
```go
// GET /api/projects/{id}/pull-requests[?refresh=1]
mux.HandleFunc("GET /api/projects/{id}/pull-requests", func(w http.ResponseWriter, r *http.Request) {
    id, ok := pathID(w, r)
    if !ok { return }
    // GATE 1: integration toggle off -> disabled (read settings KV; mirror Phase 10 read-at-use)
    if !githubIntegrationOn(db) {
        writeJSON(w, http.StatusOK, github.Result{State: "disabled"})
        return
    }
    // GATE 2: project not linked -> disabled (look up github_repo + repo_path)
    repo, repoDir, linked := lookupProjectRepo(db, id) // SELECT github_repo, repo_path
    if !linked {
        writeJSON(w, http.StatusOK, github.Result{State: "disabled"})
        return
    }
    writeJSON(w, http.StatusOK, svc.Get(r.Context(), repo, repoDir, r.URL.Query().Get("refresh") == "1"))
})
```
- The settings read: Phase 10 established `github_integration` as a settings KV with code-default `'on'` read-at-use. Reuse the same read path the existing settings handlers use (see `internal/settings` + `internal/api/settings.go`). The frontend already gates via `useSettings()`, but the backend gate is the GHSET-02 enforcement point — do not skip it.
- Wire the `Service` in `main.go` exactly like quota: `ghSvc := github.New(github.Config{...}); api.PullRequestRoutes(mux, db, ghSvc)` near lines 128-129.

### Pattern 3: Frontend data hook (clone of `usage.ts`)
**What:** `usePullRequests(projectId)` + `useRefreshPullRequests(projectId)`.
**Example (adapted verbatim from web/src/api/usage.ts):**
```ts
export interface PRSummary {
  number: number;
  title: string;
  author: string;        // author.login
  updatedAt: string;     // ISO
  url: string;
  checks: "pass" | "fail" | "pending" | "none";
  // fetched-but-unrendered (Phase 12/13): headRefName, headRefOid, baseRefName, isCrossRepository
}
export interface PullRequestsResponse {
  state: "ok" | "no_gh" | "auth_required" | "disabled" | "error";
  stale: boolean;
  fetchedAt: string | null;
  prs: PRSummary[] | null;
}
export function usePullRequests(projectId: number) {
  return useQuery({
    queryKey: ["pull-requests", projectId],
    queryFn: () => get<PullRequestsResponse>(`/api/projects/${projectId}/pull-requests`),
    refetchInterval: 60_000,
    refetchIntervalInBackground: false, // GHCOL-04: visibility-paused
  });
}
export function useRefreshPullRequests(projectId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => get<PullRequestsResponse>(`/api/projects/${projectId}/pull-requests?refresh=1`),
    onSuccess: (data) => qc.setQueryData(["pull-requests", projectId], data),
  });
}
```

### Pattern 4: Column placement (D-13) — sibling, NOT a droppable
**What:** Append the Review column after `STATUSES.map(...)` inside the same flex row in `Board.tsx`, outside any `useDroppable`/`SortableContext`.
**Where (Board.tsx:198-207):**
```tsx
<div className="flex h-full min-h-0 flex-1 gap-4 overflow-x-auto px-6 pb-6">
  {STATUSES.map((status) => (
    <Column key={status} status={status} tasks={columns[status]} projectId={projectId} />
  ))}
  {/* NEW — sibling of the status columns; never a dnd droppable (D-13) */}
  <ReviewColumn projectId={projectId} repo={project.github_repo} />
</div>
```
- `Board` currently receives only `tasks` + `projectId`. It will need the project's `github_repo` and the `github_integration` setting to render-gate the column. Two clean options: (a) pass `project` into `Board` from `BoardPage` (which already has `projects` via `useProjects`), or (b) have `ReviewColumn` self-gate internally using `useProjects()` + `useSettings()` and render `null` when the gate is closed. **Recommendation: (b)** — keeps `Board`'s props untouched and matches how `QuotaIndicator` self-gates (renders `null` on its own). The column being `null` when gated means the board row is byte-for-byte unchanged (GHSET-02).
- The `ReviewColumn` outer wrapper copies `Column`'s `flex min-h-0 min-w-[260px] flex-1 flex-col` when expanded; collapses to a `w-10` rail (UI-SPEC). Card list copies `Column`'s list container **minus** the `isOver` ring (it has no `useDroppable`).

### Pattern 5: PR card (presentational variant of TaskCard's `CardRow`)
**What:** A two-row card; row 1 = title (`line-clamp-2 flex-1 text-sm font-medium`) + checks dot (`mt-[6px]`, only when not `none`) + ↗ link; row 2 = `#{number} · @{author} · updated {ago}` meta (`text-xs text-muted-foreground`).
- Reuse `StatusDot`'s exact dot markup (`size-2 rounded-full shrink-0` + the `bg-green-500`/`bg-red-500`/`bg-amber-400` classes from `dotMeta`) for the checks dot. Either import a tiny mapper or hand-write the three-class switch — UI-SPEC says either is fine (D-03).
- The card body is **inert** (D-08): no `onClick` navigate, no `cursor-grab`, no `hover:bg`. The ONLY interactive element is the `<a href={url} target="_blank" rel="noreferrer">` ↗ icon (D-09).
- Relative time: reuse the `formatAgo` shape from `QuotaIndicator.tsx:40-47` ("2h", "3d"). Note GHCOL-03 wants "updated Xh ago" phrasing — wrap `formatAgo` output as `updated {ago} ago`. `formatAgo` currently lives inside `QuotaIndicator.tsx`; consider lifting it to a shared util OR re-implementing the same 3-tier logic locally (it is ~8 lines).

### Anti-Patterns to Avoid
- **N+1 `gh pr checks` per PR** — unnecessary; `statusCheckRollup` is on the list call. Avoid entirely.
- **Making the Review column a `useDroppable`** — PR cards must never enter the kanban (D-13, GHREV-04 forward-looking). No `SortableContext`, no `useSortable` on PR cards.
- **Shipping the raw `statusCheckRollup` array to the browser** — reduce server-side (D-00d); the raw array is dozens of entries per PR.
- **Branching on HTTP status in the frontend** — endpoint is always 200; branch on `state`/`stale` (quota precedent).
- **Caching against the self-emptying set** — when the user submits a review, the PR leaves `user-review-requested:@me` and the card SHOULD vanish on next poll. Do not add local "keep showing it" logic (PITFALL 2).
- **Trusting `gh` exit code 4 alone for auth** — it is documented but unreliable (cli/cli#9338); combine exit code with stderr sniffing.
- **`sh -c` / string-interpolated commands** — always arg arrays (project convention).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| GitHub auth | Any token handling / OAuth | Host `gh` CLI (spawn it) | Out-of-Scope table forbids storing credentials; `gh` already authenticated on host |
| PR review-queue query | REST/GraphQL client to GitHub | `gh pr list --search "user-review-requested:@me"` | `gh` encapsulates the qualifier, host/account selection (via `cmd.Dir`), pagination, and the `@me` resolution |
| CI rollup per PR | Walk Checks API + Statuses API and merge | `statusCheckRollup` field on the same list call | GitHub already merges CheckRun + StatusContext into one rollup array; one call, no N+1 |
| Best-effort cache/backoff/degrade | A new caching/backoff scheme | Clone `internal/quota.Service` | Proven, tested, already handles TTL/floor/backoff/in-flight-dedup/drop-on-N-failures |
| Poll + visibility-pause + manual refresh | Custom `setInterval` + Page Visibility wiring | TanStack `refetchInterval` + `refetchIntervalInBackground:false` | `useQuota` already does exactly this; React Query handles visibility internally |
| Collapse persistence | New settings KV + migration + endpoint | `localStorage` per-project (D-05) | Single-user localhost; no cross-device need; Phase 10 banked "no further migration" |
| Relative time | A date library (dayjs/date-fns) | The existing `formatAgo` 3-tier logic | ~8 lines already in QuotaIndicator; no new dep (UI-SPEC: zero new npm deps) |

**Key insight:** Phase 11 has almost no genuinely new logic. The only two pieces that didn't exist before are (1) the `gh pr list` arg array + classification, and (2) the rollup reduction — both fully specified above. Everything else is copy-adapt-test from quota.

## Common Pitfalls

### Pitfall 1: SKIPPED/NEUTRAL checks painting every PR red
**What goes wrong:** Naively mapping any non-SUCCESS conclusion to `fail` turns ~30% of real checks (SKIPPED conditional jobs, NEUTRAL informational checks) into red dots.
**Why:** Modern GitHub Actions configs skip many jobs per PR (path filters, conditionals). The `cli/cli` live sample was full of `SKIPPED`.
**How to avoid:** Map `SKIPPED`/`NEUTRAL`/`STALE` → ok (non-failing); only `FAILURE`/`ERROR`/`TIMED_OUT`/`CANCELLED`/`ACTION_REQUIRED`/`STARTUP_FAILURE` → fail. Precedence fail > pending > pass.
**Warning sign:** Almost every card shows a red dot in testing.

### Pitfall 2: Caching against the self-emptying column
**What goes wrong:** Adding logic to "keep" a card after the user reviews it, thinking the disappearance is a bug.
**Why:** GitHub removes the user from `user-review-requested:@me` the instant they submit a review. The card vanishing on next poll IS correct (GHCOL-04: "card disappears once the user submits its review").
**How to avoid:** Let the server list be the single truth. No local merge/retain. The 60s TTL means at most a 60s lag (or instant via manual refresh).
**Warning sign:** Reviewed PRs lingering in the column.

### Pitfall 3: TTL vs attempt-floor conflation under polling
**What goes wrong:** Using one timestamp for both "fetched successfully" and "last attempted" lets a failing `gh` get hammered by the 60s poll, or lets `?refresh=1` spam `gh`.
**Why:** Quota.go documents this exact bug (Pitfall 3 there): separate `fetchedAt` (gates the 60s success-TTL) from `lastAttempt` (gates the 10s hard floor, binds even `force`).
**How to avoid:** Copy quota's two-field design verbatim. `?refresh=1` bypasses the 60s TTL but still respects the 10s floor and any active backoff.
**Warning sign:** Rapid repeated `gh` spawns in logs when GitHub is down, or when the user mashes the refresh button.

### Pitfall 4: Unreliable `gh` auth exit codes
**What goes wrong:** Classifying auth purely on exit code 4 misses bad-token cases (which exit 1 with a 401 body).
**Why:** cli/cli#9338 — `gh` can return HTTP 401 without exiting 4; cli/cli#8845 — `gh auth status` returns wrong codes. Verified live: bad `GH_TOKEN` → exit 1 + `401 Bad credentials`, NOT exit 4.
**How to avoid:** Combine exit code (4) with stderr substring sniffing (`gh auth login`, `401`, `Bad credentials`) for `auth_required`. See the classification block above.
**Warning sign:** A bad-token user shown the generic `error` state instead of `auth_required` "GitHub not connected".

### Pitfall 5: GraphQL point budget under multi-project polling
**What goes wrong:** `gh pr list` with `statusCheckRollup` is a GraphQL query and bills the **5000-points/hour GraphQL budget** (not REST request count). Polling many repos concurrently can exhaust it (cli/cli#13433, manaflow-ai/cmux#2746 observed this).
**Why:** Each `gh pr list --json statusCheckRollup` is a non-trivial GraphQL query; multiple concurrent pollers compound.
**How to avoid:** D-11 already bounds this — only the **visible** project polls (React Query pauses hidden tabs and the column is scoped to the active board), 60s interval, server-side cache + backoff. Do NOT poll every linked project in the background. The quota-style "serve stale, back off on failure" handles a rate-limit hit gracefully (it surfaces as `error` with stale data, never a crash). No explicit rate-limit polling needed.
**Warning sign:** `error` state appearing under heavy multi-project use; `gh api rate_limit` showing GraphQL `remaining: 0`.

### Pitfall 6: `cmd.Dir` not set → wrong account/host
**What goes wrong:** Running `gh` without `cmd.Dir = repo_path` resolves `@me` and the host against the default account, not the one for that repo (breaks multi-account / GitHub Enterprise).
**Why:** `gh` selects host/account from the working directory's git context (D-00b).
**How to avoid:** Always set `cmd.Dir = project.repo_path` (the existing `githubOrigin` handler already does `git -C repoPath` for the analogous reason). The endpoint must `SELECT repo_path` alongside `github_repo`.
**Warning sign:** Empty/wrong PR list for enterprise or secondary-account repos.

## Code Examples

### Example: parse + reduce statusCheckRollup (Go)
```go
// Source: live-verified shapes from gh pr list --json statusCheckRollup (cli/cli, kubernetes/kubernetes)
type checkEntry struct {
    Typename   string `json:"__typename"`
    Status     string `json:"status"`
    Conclusion string `json:"conclusion"`
    State      string `json:"state"`
}

func reduceChecks(rollup []checkEntry) string {
    if len(rollup) == 0 {
        return "none"
    }
    anyFail, anyPending := false, false
    for _, c := range rollup {
        var sig string // "ok" | "pending" | "fail"
        if c.Typename == "StatusContext" {
            switch c.State {
            case "SUCCESS":
                sig = "ok"
            case "PENDING", "EXPECTED":
                sig = "pending"
            default: // FAILURE, ERROR
                sig = "fail"
            }
        } else { // CheckRun
            if c.Status != "COMPLETED" {
                sig = "pending"
            } else {
                switch c.Conclusion {
                case "SUCCESS", "NEUTRAL", "SKIPPED", "STALE":
                    sig = "ok"
                case "":
                    sig = "pending"
                default: // FAILURE, TIMED_OUT, CANCELLED, ACTION_REQUIRED, STARTUP_FAILURE
                    sig = "fail"
                }
            }
        }
        switch sig {
        case "fail":
            anyFail = true
        case "pending":
            anyPending = true
        }
    }
    switch {
    case anyFail:
        return "fail"
    case anyPending:
        return "pending"
    default:
        return "pass"
    }
}
```

### Example: run gh + classify degraded state (Go)
```go
// Source: pattern from internal/github.ValidateRepo + live-verified exit codes/stderr
func (s *Service) runGH(ctx context.Context, repo, repoDir string) (prs []PRSummary, state string, err error) {
    if !Available() {
        return nil, "no_gh", nil
    }
    cmd := exec.CommandContext(ctx, "gh", "pr", "list",
        "-R", repo,
        "--search", "user-review-requested:@me draft:false",
        "--state", "open",
        "--json", "number,title,author,updatedAt,url,isDraft,statusCheckRollup,headRefName,headRefOid,baseRefName,isCrossRepository,additions,deletions",
    )
    cmd.Dir = repoDir // D-00b / Pitfall 6
    var stdout, stderr bytes.Buffer
    cmd.Stdout, cmd.Stderr = &stdout, &stderr
    runErr := cmd.Run()
    if runErr != nil {
        es := stderr.String()
        var exitErr *exec.ExitError
        code := -1
        if errors.As(runErr, &exitErr) {
            code = exitErr.ExitCode()
        }
        if code == 4 ||
            strings.Contains(es, "gh auth login") ||
            strings.Contains(es, "401") ||
            strings.Contains(es, "Bad credentials") {
            return nil, "auth_required", nil
        }
        return nil, "error", nil
    }
    // parse stdout into raw shape, reduce rollup -> PRSummary, sort by updatedAt desc (D-14)
    // ...
    return prs, "ok", nil
}
```

### Example: visibility-paused poll (TS) — verbatim quota shape
See Pattern 3 above. The `refetchIntervalInBackground: false` is the GHCOL-04 visibility pause; React Query handles the Page Visibility API internally — no manual wiring.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `gh pr checks <num>` per PR for CI status | `statusCheckRollup` field on `gh pr list` | long-standing | One call, no N+1, no per-PR rate cost |
| `gh search prs` (search API, 30/min) | `gh pr list --search` (GraphQL/core budget) | locked decision | Larger budget, richer JSON incl. rollup |
| Trust `gh` exit code for auth | Exit code + stderr sniffing | cli/cli#8845, #9338 | Reliable `auth_required` classification |

**Deprecated/outdated:** none relevant — `gh pr list --search` + `statusCheckRollup` is the current, stable surface (verified on 2.82.0; unchanged through 2.94.0).

## Open Questions

1. **Where does the backend read the `github_integration` toggle?**
   - What we know: Phase 10 established it as a settings KV (code-default `'on'`, read-at-use); `internal/api/settings.go` + `internal/settings` own the read path.
   - What's unclear: the exact helper to call from the new PR handler (a `settings.Get(db, "github_integration")`-style call).
   - Recommendation: planner reads `internal/settings/settings.go` + `internal/api/settings.go` to find the existing single-key read and reuse it for the backend gate. (Frontend gate via `useSettings()` is already specified; the backend gate is the GHSET-02 enforcement and must exist too.)

2. **`formatAgo` reuse vs re-implement.**
   - What we know: it lives inside `QuotaIndicator.tsx` (not exported).
   - What's unclear: whether to lift it to `web/src/lib/utils.ts` (shared) or copy the ~8 lines into the PR card.
   - Recommendation: lift to a shared util (DRY, one tier-logic) OR copy locally — both are fine; lifting is cleaner if a quick refactor is acceptable. Either way, no new dependency.

3. **Service instance: single multi-repo Service vs per-project.**
   - What we know: quota has one global Service (one account). Here there are N repos.
   - Recommendation: one `Service` holding a `map[repo]*entry` guarded by the existing mutex (cleanest, matches the single-wire-in-main.go pattern). Per-repo TTL/backoff state lives in the entry. D-11 keeps the live key-set small.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `gh` CLI | The entire PR list path | ✓ (this host) | 2.82.0 (floor met; latest 2.94.0) | **Degrade to `no_gh` state** — the whole point of GHSET-03; the app stays fully usable |
| `gh` authenticated | Live PR data | ✓ (account `jordeu`) | — | **Degrade to `auth_required` state** inline |
| `git` CLI | `cmd.Dir` repo context (already used project-wide) | ✓ | system git | — (already a hard premise of the app) |
| Go toolchain | Backend build | ✓ | 1.26 (go.mod) | — |
| Node/Vite | Frontend build | ✓ (assumed; prior phases built) | — | — |

**Missing dependencies with no fallback:** none — `git` and Go are pre-existing premises.

**Missing dependencies with fallback:** `gh` itself is the soft dependency by design; absence/unauth are first-class typed states (`no_gh`/`auth_required`), never blocking. This is the core GHSET-03 contract and is fully handled by the Service.

## Sources

### Primary (HIGH confidence — verified live on this host, `gh` 2.82.0)
- `gh pr list --help` and `gh pr list --json` (field list) — confirmed `--search`, `-R`, `--state`, `-d/--draft`, and all `--json` fields incl. `statusCheckRollup`.
- Live `gh pr list -R cli/cli --json …` — verified `author{login,name,id,is_bot}`, `updatedAt` (RFC3339), `url`, CheckRun shape (`status`/`conclusion`).
- Live `gh pr list -R kubernetes/kubernetes --json statusCheckRollup` — verified StatusContext shape (`state`, no `conclusion`).
- Live `gh pr list --search "user-review-requested:@me draft:false"` — returns `[]` cleanly (empty case).
- Live degraded probes: nonexistent repo → exit 1 + `Could not resolve to a Repository`; bad `GH_TOKEN` → exit 1 + `401 Bad credentials`; empty config → exit **4** + `gh auth login`; missing binary → `LookPath` fails.
- Repo files read directly: `internal/quota/quota.go`, `internal/github/github.go`, `internal/api/{usage,github,projects,routes,respond}.go`, `cmd/kangent/main.go`, `web/src/api/{usage,settings,client,types}.ts`, `web/src/components/{StatusDot,quota/QuotaIndicator}.tsx`, `web/src/components/board/{Board,Column,TaskCard}.tsx`, `web/src/pages/BoardPage.tsx`.

### Secondary (MEDIUM-HIGH — official docs / verified issues)
- [cli.github.com/manual/gh_help_exit-codes](https://cli.github.com/manual/gh_help_exit-codes) — exit 0 success, 1 failure, 2 cancelled, **4 auth required**.
- [cli/cli#9338](https://github.com/cli/cli/issues/9338) — 401 can exit without code 4 (exit-code unreliability; corroborates the stderr-sniffing approach).
- [cli/cli#8845](https://github.com/cli/cli/issues/8845) — `gh auth status` wrong exit code (cited in CONTEXT D-00c).
- [cli/cli#13433](https://github.com/cli/cli/issues/13433) + [manaflow-ai/cmux#2746](https://github.com/manaflow-ai/cmux/issues/2746) — `gh pr list`/`gh pr checks` bill the GraphQL 5000-points/hour budget; background polling can exhaust it (Pitfall 5).

### Tertiary (LOW — none load-bearing)
- General GitHub search-qualifier docs (the `@me`/`draft:false` semantics) — corroborated directly by the live probes above, so not relied upon alone.

## Metadata

**Confidence breakdown:**
- gh command + JSON fields: **HIGH** — every flag and field verified against live `gh` 2.82.0.
- statusCheckRollup reduction: **HIGH** — both `__typename` variants observed live; SKIPPED/NEUTRAL prevalence confirmed in real data.
- Degraded-state classification: **HIGH** — all four scenarios (missing/no-auth/bad-token/repo-error) probed directly with captured exit codes + stderr.
- Architecture / reuse mapping: **HIGH** — every precedent file read in full; the clone targets are concrete line ranges.
- Rate-limit mitigation: **MEDIUM-HIGH** — budget mechanics from official issues; the D-11 mitigation is sound but real-world multi-project behavior is bounded by design, not yet UAT-observed.

**Research date:** 2026-06-13
**Valid until:** 2026-07-13 for the architecture/reuse mapping (stable); ~2026-06-27 for the exact `gh` JSON field set / exit codes (re-verify if `gh` is upgraded past 2.94 or if cli/cli changes rollup typenames — fast-moving CLI).

*Validation Architecture section omitted: `workflow.nyquist_validation` is `false` in .planning/config.json.*
