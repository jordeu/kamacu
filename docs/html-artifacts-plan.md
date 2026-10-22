# HTML artifacts implementation plan

## Goal and agreed behavior

Agents publish interactive HTML previews to tasks without depending on a shared filesystem. Each artifact appears as its own named tab immediately after Diff, before terminal tabs. Users can rename or remove artifacts; artifact creation and content replacement happen through MCP.

- Accept a self-contained `.html` file or `.tar.gz` with a regular `index.html` at the archive root.
- Execute JavaScript in an isolated iframe. Only uploaded resources may load; external resources and network requests are blocked.
- MCP supports create, list, and replace content, using stable artifact IDs. Replacement preserves the user-visible title.
- New artifacts do not change the selected tab. A visible artifact receiving new content shows an “Updated — Reload” action instead of reloading automatically.
- Switching tabs preserves iframe interaction state. Task reload or accepting an update resets it. No persistent browser storage.
- Uploads survive worktree cleanup and service restart. Deleting the task also deletes its artifacts; project deletion must handle cascading task deletion.
- Maximum 20 artifacts per task, 20 MB uploaded, 100 MB extracted, and 1,000 files per artifact. Implement byte limits explicitly as 20,000,000 and 100,000,000 bytes.
- Failed uploads or replacements leave the published version intact.

## Repository integration

- `internal/mcp/server.go` registers resource-specific tools. `internal/mcp/bridge.go` forwards tool requests over HTTP with `X-Kamacu-Token`; payloads should remain small JSON rather than file bytes.
- `cmd/kamacu/serve.go` owns API registration, the database path, and the instance hook token. Construct and inject the artifact service here, using a `<database filename>.artifacts` directory beside the database, isolating custom databases.
- Add the next store migration after `00022_recurrent_tasks.sql` for artifact metadata, immutable revisions, and pending uploads.
- `internal/api/tasks.go` and `internal/api/projects.go` own task and project deletion. Both need file cleanup integration; cascading SQL deletion alone does not remove files.
- `web/src/pages/TaskPage.tsx` builds the tabs and repairs selection when a tab disappears. Insert artifact IDs into both the rendered tabs and selection ordering, independent of worktree availability.
- `web/src/components/task/TaskTabs.tsx` already supports rename and kept-mounted contents. Make close labels configurable: artifacts say “Remove artifact,” terminals retain their existing wording.
- `GlobalTaskPage.tsx` currently has only agent and terminal tabs. Initial UI scope is project task views, where Diff exists. Do not silently add artifacts to the global Scratchpad without an explicit scope decision.

## 1. Persistence and artifact service

Create `internal/artifact` for bounded upload streaming, archive validation, revision publishing, and garbage collection. Store metadata in SQLite; keep file bytes on disk outside worktrees.

Use server-generated opaque artifact, revision, and upload identifiers. An artifact records task ID, title, current revision, creation time, and update time. Revisions contain file manifests and byte counts. Pending uploads record target task/artifact, expected current revision for replacements, format, declared bytes, expiration, token hash, and status.

Write incoming bytes to staging; validate and extract into a fresh revision directory. Rename the completed directory into its immutable location before committing the published revision pointer. A failed database transaction leaves only an unreferenced directory for cleanup, never a published pointer to incomplete content. Recheck task existence and the per-task count within the publishing transaction. Conflicting replacement finalizations return a conflict instead of overwriting newer content.

Retain replaced revisions while the task exists, so a kept-mounted old iframe can still load its own assets until the user accepts the update. Track this storage cost explicitly; old revision retention needs a later quota policy if disk growth becomes a concern.

Remove artifact files on artifact/task/project deletion after metadata removal; retry failed filesystem cleanup through a startup and periodic orphan sweep. Use opaque IDs that are never reused so delayed cleanup cannot delete a newly created artifact. Expired upload staging is also swept. Coordinate sweeps with active publications to avoid deleting a revision between its filesystem rename and database commit.

## 2. Remote upload protocol and MCP

Proposed MCP tools:

- `list_artifacts(task_id)` returns published artifacts, titles, IDs, revisions, and timestamps.
- `begin_artifact_upload(task_id, title, filename, size_bytes)` starts creation.
- `begin_artifact_replacement(task_id, artifact_id, filename, size_bytes)` starts replacement, capturing the current revision.
- `finalize_artifact_upload(task_id, upload_id)` validates and publishes uploaded content; retrying successful finalization returns the same result.

Begin returns an upload ID, absolute upload URL, expiration, required method (`PUT`), and headers. The remote agent sends raw file bytes using HTTP, then finalizes through MCP. URLs must derive from the configured agent-reachable service base, never an assumed localhost address or unvalidated request Host. Existing Kamacu access still requires a reachable base URL/tunnel; this feature does not change deployment networking.

Use a random, short-lived upload capability scoped to one pending upload (initial expiration: 15 minutes). Store its hash. PUT accepts only that capability, enforces declared and actual sizes, streams to disk, and prevents concurrent writers. A failed transfer can retry before expiry; a completed transfer is immutable. Finalization requires the MCP instance token and matching task/upload association. Do not expose rename/delete as MCP tools or include capabilities in list responses or logs.

Proposed HTTP routes:

- `GET /api/tasks/{taskID}/artifacts`
- `POST /api/tasks/{taskID}/artifact-uploads`
- `POST /api/tasks/{taskID}/artifacts/{artifactID}/uploads`
- `PUT /api/artifact-uploads/{uploadID}` (upload capability)
- `POST /api/tasks/{taskID}/artifact-uploads/{uploadID}/finalize`
- `PATCH /api/tasks/{taskID}/artifacts/{artifactID}` (title only)
- `DELETE /api/tasks/{taskID}/artifacts/{artifactID}`
- A dedicated immutable preview file route scoped to artifact and revision.

UI rename/delete follow existing UI request conventions and origin protections. Upload begin/finalize additionally require `X-Kamacu-Token`. MCP omission alone is not an authorization boundary against a fully privileged agent.

## 3. Validation and preview isolation

Reject malformed/truncated gzip and tar content, absent root entry points, symlinks, hardlinks, devices, FIFOs, absolute paths, traversal, duplicate normalized paths, and file/directory collisions. Do not follow filesystem links. Count streamed output against aggregate and per-entry limits, not just declared tar sizes. Bound entry count, path lengths, and decompression overhead, including archives dominated by directory or metadata entries. Serve only manifest-listed files with explicit MIME types and `nosniff`; never directory listings or SPA fallback pages.

Use iframe `sandbox="allow-scripts"` without `allow-same-origin`, plus response-header CSP sandboxing on all renderable artifact documents so direct navigation does not remove the sandbox. Disable forms, popups, top navigation, downloads, frames, workers, and persistent storage. Scope allowed asset URLs to the exact immutable revision path; a broad application-origin allowance could expose Kamacu API routes. Allow embedded data/blob resources only for the resource types that need them. Keep connection APIs blocked initially; any local-data fetch support must remain restricted to manifest-backed revision assets.

Before implementing the final delivery scheme, verify two browser behaviors with a focused prototype:

1. Opaque-origin sandbox behavior for bundled scripts, ES modules, fonts, CSS, and local assets. Any CORS exception must apply only to immutable artifact assets, never application APIs.
2. Blocking external navigation, including iframe self-navigation, links, meta refresh, and script location changes. Sandboxing and `connect-src 'none'` alone do not establish the complete “no external network” contract. Do not claim the requirement is met without browser evidence; choose a stronger delivery/filtering scheme if needed and document any unavoidable limitation before shipping.

References:
- https://html.spec.whatwg.org/dev/iframe-embed-object.html
- https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Security-Policy/sandbox
- https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Security-Policy/connect-src

## 4. Task UI

Add artifact API types, queries, and rename/delete mutations. Poll the artifact list while the task page is open using existing query conventions. Keep ordering stable by creation time and ID. Prefix tab IDs with `artifact:` to avoid session ID collisions.

Add an `ArtifactTab` component that pins the displayed immutable revision separately from the latest query revision. Mount its iframe lazily on first selection, then keep it mounted across tab switches. If updated while visible, retain the old iframe and show “Updated — Reload”; reload switches to the newest revision. An inactive previously visited artifact may adopt the newest revision when next selected. New artifacts never alter active selection.

Reuse inline tab rename, reject empty titles, and enforce a short server-side title limit (proposed: 100 characters). Remove uses the existing confirmation dialog pattern. On successful removal, choose the nearest surviving left tab using the current TaskPage selection repair. Show mutation errors without discarding the working preview. Display a loading/error state for preview availability.

## 5. Verification and delivery

Implement in this order:

1. Prototype and verify preview isolation and bundled asset loading in real browsers.
2. Add migration, artifact service, extraction validation, atomic publishing, and lifecycle cleanup.
3. Add HTTP upload/management/preview routes and MCP tools with end-to-end protocol tests.
4. Add task tabs, rename/remove, update notice, and mounted iframe behavior.
5. Document the remote agent upload sequence and supported HTML packaging in `docs/install-mcp.md`.

Backend tests must cover both input formats, boundaries, unsafe archives, token expiry/replay, wrong-task access, interrupted upload, idempotent finalization, concurrent creation/replacement/deletion, title preservation, restart persistence, cleanup, and preservation of old revisions on failures.

Browser verification must exercise a remote-style upload with no shared path; bundled CSS, classic scripts, modules, fonts, and images; isolation from parent DOM/session/storage; blocked external asset/API/WebSocket/form/navigation attempts; no focus stealing; update/reload behavior; state retention on tab switching; rename; removal; and survival after worktree cleanup.

Run the affected Go package tests, then `go test ./...`; run frontend tests, lint, and production build. Record browser evidence for the isolation contract. Any eventual PR must be a draft and start its description with a short abstract explaining the problem and resulting behavior.


## Implementation verification

Implemented storage, migration, HTTP upload/finalization, MCP tools, API browser-origin protection, task tabs, rename/remove, revision reload notices, and periodic cleanup. Storage is namespaced per database at `<database path>.artifacts`. The implementation accepts conventional `./index.html` tar paths, limits regular files to 1,000 and total archive entries to 4,000, and bounds decompressed header/padding overhead.

Verified:

- Full `go test ./...` regression suite passed.
- Artifact service and MCP race checks passed; artifact HTTP race checks passed.
- Frontend production build and all 56 existing frontend tests passed.
- Lint on the artifact API/components and shared tabs passed. Repository-wide lint still reports pre-existing errors, including TaskPage's existing session-state effect.
- Real Chrome acceptance passed for remote-style HTTP transfers, both formats, bundled CSS/classic scripts/modules/local JSON, parent and storage isolation, external asset/API/WebSocket blocking, focus, tab state, rename, replacement, removal, and page reload. Desktop and mobile screenshots were inspected; selected tabs remain visible after resizing.
- An initial review reproduced external self-navigation and WebRTC/STUN bypasses under CSP alone. The renderer now adds a native Connection Allowlist, verifies WebRTC blocking before parsing any uploaded markup, and refuses unsupported browsers. Browser checks verify zero STUN packets and blocked external navigation in the supported configuration. The browser-origin guard independently rejects opaque/external-origin API writes.

Security compatibility requirement: interactive previews execute only when the browser demonstrates native WebRTC blocking under its Connection Allowlist. CSP alone is insufficient. The full browser flow was verified on Chrome 149 with its native Connection Allowlists local-test feature switches; Chrome 134 was verified to fail closed, including direct HTML, SVG, and XHTML URLs. No user-agent version sniffing or JavaScript API replacement is used. Production browsers must have the native policy enabled; otherwise the UI reports an unsupported browser.

To repeat browser acceptance, build the frontend first, then run:

```sh
KAMACU_BROWSER_TEST=1 KAMACU_PLAYWRIGHT_PACKAGE=/path/to/playwright \
KAMACU_BROWSER_EXECUTABLE=/path/to/chrome \
go test ./internal/api -run '^TestArtifactsBrowser$' -count=1 -v
```

The browser test creates its own disposable database and HTTP server and never uses the running Kamacu service. `KAMACU_ARTIFACT_SCREENSHOTS` optionally specifies a directory for desktop/mobile captures.


### Review fixes

- WebRTC: `Connection-Allowlist` restricts network destinations to the immutable revision and explicitly blocks WebRTC and redirects. HTML is streamed as inert base64 through a trusted renderer, and uploaded markup reaches the parser only after checking native enforcement. Non-HTML responses use a script-disabled CSP sandbox and download on document navigation, preventing SVG/XML/PDF link-navigation bypasses while preserving subresource loading. This closes direct-navigation and fresh-realm routes on browsers that ignore the new header instead of silently weakening the policy.
- Availability: the viewer checks HTTP availability with HEAD, waits for both iframe load and a source/token-bound trusted renderer readiness message, and times out if readiness never arrives. Retry remounts even an unchanged revision. Connection/404 recovery is covered by the browser harness.
- To exercise native policy on the installed Chrome 149 test runtime, set `KAMACU_BROWSER_ARGS='["--enable-features=ConnectionAllowlists,OverrideConnectionAllowlistOriginTrial"]'`. These are local verification switches; the application does not modify browser settings. Run the same harness on an older browser with `KAMACU_EXPECT_UNSUPPORTED=1` to verify fail-closed behavior.


Review-fix verification passed: the full Go suite, focused renderer/header tests, Go vet, all 56 frontend tests, the production frontend build, and lint on the changed preview component. Supported-engine Chrome acceptance verified zero actual UDP/STUN packets, blocked external navigation, working bundled scripts/modules/CSS/JSON/SVG images, and retries after HEAD failures and after iframe-only 404/connection failures. Unsupported-engine Chrome acceptance verified that uploaded HTML never parses or executes and direct SVG/XHTML document requests become downloads. Desktop/mobile retry error states were inspected. Astra's targeted follow-up confirmed the original WebRTC and availability findings were addressed; its additional direct SVG/XHTML link-navigation finding was fixed with download disposition and covered by these browser tests.
