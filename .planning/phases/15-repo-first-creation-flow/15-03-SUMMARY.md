---
phase: 15-repo-first-creation-flow
plan: 03
subsystem: verification
tags: [human-verify, checkpoint, ui, github, create-flow]

# Dependency graph
requires:
  - phase: 15-repo-first-creation-flow
    provides: "Plan 01 (backend description capture + widened mutation/type) and Plan 02 (repo-first AddProjectDialog + rebuilt web/dist) — the running-app behavior verified here"
provides:
  - "Human-verify sign-off on the repo-first Add-project flow in the running binary"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns: []
key-files:
  created: []
  modified: []

requirements-completed: [RPROJ-01, RPROJ-02, RPROJ-03, RPROJ-04, CKOUT-04]

one_liner: "Blocking human-verify gate APPROVED 2026-06-15 — the user exercised the repo-first Add-project flow end-to-end in the running app (segmented toggle repo-default, name prefill, Cloning spinner, auto-captured description, folder fallback, inline clone-failure with no orphan project, integration-off folder-only) and confirmed every CONFIRM step."
---

# 15-03 Summary — Human-Verify Gate (Repo-First Creation Flow)

**Type:** `checkpoint:human-verify` (blocking, no code).
**Outcome:** **APPROVED by the user on 2026-06-15.**

## What was verified (running binary, embedded `web/dist`)

The user built and ran the single binary and confirmed all nine CONFIRM steps from the plan:

**GitHub integration ON:**
1. "Add project" shows a segmented "GitHub repo | Local folder" toggle, **GitHub repo default**; repo input has the mono `owner/name` placeholder (RPROJ-01).
2. Typing `owner/name` **prefills the editable Name** from the name segment; an edited Name is **not clobbered** by further repo typing (RPROJ-02).
3. Submit → inputs disable, **"Cloning <owner/name>…" spinner**, dialog stays open; on success it closes and **navigates** to the new project (D-06/D-08).
4. The new project's Settings shows the **auto-captured GitHub description** (or empty if none) (RPROJ-02/D-05).
5. Toggle → "Local folder" swaps to the folder-path input; **folder create still works** (RPROJ-03).
6. A bad/inaccessible repo → **inline destructive-alert, dialog stays open, values preserved, no "half-created" copy**, and **no orphan project / no stray `~/.kangent/repos/` dir** (CKOUT-04/D-07).
7. Re-adding an already-added repo → inline "…already added." (409, D-09).

**GitHub integration OFF:**
8. "Add project" is the **folder-only form with no toggle/repo field** — byte-for-byte pre-v1.4; folder create still works (RPROJ-04/D-02).

## Evidence

- Automated gate (orchestrator, pre-checkpoint): `go build ./...`, `go vet ./...`, full `go test ./...`, and `cd web && npx tsc --noEmit` all green; the rebuilt embedded bundle contains the new "Cloning" string (so the running binary ships the dialog).
- Human sign-off: "approved" — every CONFIRM step passed.

## Requirements

RPROJ-01, RPROJ-02, RPROJ-03, RPROJ-04, CKOUT-04 — all confirmed end-to-end in the running app. This is the final plan of Phase 15 (and of milestone v1.4).
