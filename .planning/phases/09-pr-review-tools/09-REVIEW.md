---
phase: 09-pr-review-tools
depth: standard
files_reviewed: 3
status: issues
critical: 0
warning: 1
info: 2
total: 3
---

# Code Review: Phase 09 — pr-review-tools

## Summary

The phase is a clean, well-documented mechanical bridge: 3 thin MCP tools over existing endpoints, correct error wrapping, no injection surface (int64 + `%d`), and a load-bearing Pitfall-1 regression test that genuinely pins the stale-cache invariant. All 6 tests pass, vet/build are clean, and the clean-import streak is intact. One Warning and two Info findings — none are blocking. The Warning concerns a realistic edge case (zero PRs on `state:"ok"`) where the success path emits the literal string `"null"` instead of `"[]"`, which is a degraded-but-not-broken experience for the consuming agent.

## Findings

### WR-01: Success path emits `"null"` (not `"[]"`) when the chosen queue is empty

**File:** `internal/mcp/reviews.go:263-271`
**Severity:** Warning

D-03 specifies that on `state:"ok"` the success branch returns "the `prs` array (re-marshalled)" / "the `reviewed` array (re-marshalled)". The implementation projects the chosen `json.RawMessage` verbatim:

```go
raw := res.PRs
if queue == "reviewed" {
    raw = res.Reviewed
}
return &mcp.CallToolResult{
    Content: []mcp.Content{
        &mcp.TextContent{Text: string(raw)},
    },
}, nil
```

This trusts the upstream field to be a JSON array, but `internal/github/service.go:165-178` (`resultLocked`) assigns `res.PRs = e.cachedAwaiting` directly from the cache, and `e.cachedAwaiting` is a **nil** `[]PRSummary` when a successful `gh` fetch returns zero awaiting PRs (service.go:136 — `e.cachedAwaiting = lists.Awaiting`, nil for an empty result). Go marshals a nil slice as JSON `null`, not `[]`. So a user with **zero PRs awaiting review** — a common, healthy state — produces `{"state":"ok","prs":null,...}` from Kamacu, and this branch returns `TextContent{Text: "null"}` to the agent.

That is ambiguous: `IsError=false` + `"null"` is not an array representation and is indistinguishable from a missing field. The degrade path correctly preserves the raw body verbatim (D-02), but the **success** path is where D-03 wants an array. The fix is to coerce a null/nil queue to `"[]"` on the success branch only:

```go
raw := res.PRs
if queue == "reviewed" {
    raw = res.Reviewed
}
if len(raw) == 0 || string(raw) == "null" {
    raw = []byte("[]")
}
return &mcp.CallToolResult{
    Content: []mcp.Content{
        &mcp.TextContent{Text: string(raw)},
    },
}, nil
```

Note: the degrade branch (`reviews.go:249-259`) must keep returning the raw body verbatim per D-02 — do not normalize there. The `TestBridge_ListPendingReviews_Happy_StateOk_ProjectsPrsArray` test uses a non-empty array, so it does not exercise this edge; a test with `{"state":"ok","prs":null}` asserting `tc.Text == "[]"` would pin the fix.

### IN-01: `queue` discriminator silently falls through to PRs

**File:** `internal/mcp/reviews.go:264`

```go
raw := res.PRs
if queue == "reviewed" {
    raw = res.Reviewed
}
```

Any `queue` value other than the literal `"reviewed"` (including a future typo like `"reveiwed"`) silently projects the `prs` queue with no error. Both internal callers (`listPendingReviews` → `"prs"`, `listRecentlyReviewed` → `"reviewed"`) are correct, so this is purely defensive. A `switch` with a `default` error branch, or an explicit `queue == "prs"` check, would surface a programming mistake loudly. Low priority since the discriminator is a private helper's parameter, not agent input.

### IN-02: Tests index `res.Content[0]` without a length guard

**File:** `internal/mcp/reviews_test.go:51`, `:91`, `:165`, `:214`

```go
tc, ok := res.Content[0].(*mcpsdk.TextContent)
```

If a handler ever returned a `CallToolResult` with an empty `Content` slice, this would panic instead of failing with a readable `t.Errorf`. The review handlers always populate `Content` on every return path (both the success and degrade branches set it), so this is safe today and matches the existing `workspaces_test.go` convention. A defensive `if len(res.Content) == 0 { t.Fatal(...) }` before indexing would make test failures more diagnosable. Consistent with the codebase, so optional.
