---
status: partial
phase: 10-github-foundations
source: [10-VERIFICATION.md]
started: 2026-06-13T00:00:00.000Z
updated: 2026-06-13T00:00:00.000Z
---

## Current Test

[awaiting human testing]

## Tests

### 1. OFF cascade click-through (GHSET-02)
expected: Turn the GitHub integration Switch OFF on /settings; open a project's ⋯ → Project settings: the GitHub repository field is gone, Description remains; no Review column or other GitHub UI appears anywhere; board/tasks/sessions behave exactly as before v1.3.
result: [pending]

### 2. Live gh-removed degrade test (GHSET-03)
expected: Rename/remove the `gh` binary (or de-authenticate it), then link a project to a syntactically valid repo (e.g. owner/name): the PATCH still 200s and stores the syntactic owner/name, the dialog closes, and nothing in core flows breaks.
result: [pending]

### 3. Origin prefill on dialog open (GHPRJ-03 / D-08)
expected: Open Project settings for an unlinked project whose git origin is a GitHub remote: the repository field prefills with the canonicalized owner/name; for a non-GitHub or absent origin the field stays empty; the origin endpoint is hit only on open, never on the project list.
result: [pending]

## Summary

total: 3
passed: 0
issues: 0
pending: 3
skipped: 0
blocked: 0

## Gaps
