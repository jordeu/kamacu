---
status: testing
phase: 10-activity-data-api
source: [10-VERIFICATION.md]
started: 2026-07-30T14:01:13Z
updated: 2026-07-30T14:01:13Z
---

## Current Test

number: 1
name: Live gh integration smoke against a real linked repo with reviewed-then-closed PRs
expected: |
  curl -s 'http://localhost:<port>/api/activity?scope=global&window=month' returns HTTP 200
  with non-empty reviews.prs (each carrying number/title/completedAt/url +
  projectName/projectId/repo), reviews.state in {ok,partial}; confirms the verified gh search
  'reviewed-by:@me draft:false is:closed' + --limit 100 + closedAt→completedAt mapping returns
  real merged+closed data end-to-end.
awaiting: user response

## Tests

### 1. Live gh integration smoke against a real linked repo with reviewed-then-closed PRs

**Prerequisites:**
- `gh` CLI installed and authenticated (`gh auth status`).
- The `github_integration` setting is `"on"` (toggle in Settings, backed by migration 00007).
- At least one project linked to a GitHub repo where you have reviewed PRs that were later merged/closed.

**Steps:**
1. Start the server: `go run ./cmd/kamacu/serve.go` (or the built binary).
2. `curl -s -o /dev/null -w "%{http_code}\n" 'http://localhost:<port>/api/activity?scope=global&window=month'` → expect `200`.
3. `curl -s 'http://localhost:<port>/api/activity?scope=global&window=month' | jq '.reviews'` → expect `state` in `{ok, partial}` and a non-empty `prs[]` with each entry carrying `number`, `title`, `completedAt`, `url`, `projectName`, `projectId`, `repo`.
4. (Optional) Confirm window filter: `?window=week` drops PRs closed >7d ago; `?window=month` keeps PRs closed within 30d.

expected: HTTP 200 always; reviews.prs non-empty for a repo with reviewed-then-closed PRs; degradation (state=partial/no_gh) does not break tasks-done or stats when gh is absent.
result: [pending]

## Summary

total: 1
passed: 0
issues: 0
pending: 1
skipped: 0
blocked: 0

## Gaps
