---
status: diagnosed
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
result: issue — cascade behavior OK, but the toggle help copy over-claims ("— the app behaves exactly as it did before."); user wants it removed. See Gap 2.

### 2. Live gh-removed degrade test (GHSET-03)
expected: Rename/remove the `gh` binary (or de-authenticate it), then link a project to a syntactically valid repo (e.g. owner/name): the PATCH still 200s and stores the syntactic owner/name, the dialog closes, and nothing in core flows breaks.
result: issue — user renamed `gh` (now unavailable). Requested behavior change: when `gh` is unavailable, GitHub integration must default OFF and refuse enabling, telling the user to install `gh` first. See Gap 1.

### 3. Origin prefill on dialog open (GHPRJ-03 / D-08)
expected: Open Project settings for an unlinked project whose git origin is a GitHub remote: the repository field prefills with the canonicalized owner/name; for a non-GitHub or absent origin the field stays empty; the origin endpoint is hit only on open, never on the project list.
result: [pending]

## Summary

total: 3
passed: 0
issues: 2
pending: 1
skipped: 0
blocked: 0

## Gaps

- **Gap 1 — gh-gated default + enable guard (GHSET-01/GHSET-03):** When `gh` is unavailable, integration defaults OFF; enabling checks `gh` and, if absent, blocks and tells the user to install `gh` first. status: open. Detail in 10-VERIFICATION.md `## Gaps`.
- **Gap 2 — toggle copy (GHSET-02):** Remove "— the app behaves exactly as it did before." from the GitHub integration help text. status: open. Detail in 10-VERIFICATION.md `## Gaps`.
