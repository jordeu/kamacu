# S03 — Session resume and argv regression hardening — Research

**Date:** 2026-07-07
**Milestone:** M002 (opencode built-in agent engine)
**Depends:** S01 (opencode engine + KAMACU_* env + on-disk plugin), S02 (fetch-based plugin + real-binary e2e harness)

## Summary

S03 closes the two gaps S01/S02 explicitly deferred: (1) an opencode task's conversation must resume after a kamacu restart, and (2) a fake-opencode stub must lock the exact argv the spawn path builds (fresh vs resume), mirroring the fake-claude `--session-id`/`--resume` regression guard that has protected the claude path since Phase 5.

The core difference from claude is **who owns the session id**. For claude, kamacu *mints* the id (`--session-id <uuid>`) so it always knows it; resume is `claude --resume <stored-id>`. opencode **mints its own opaque id** (`ses_…`, stored in `~/.local/share/opencode/opencode.db`, keyed by the session's `directory` = the worktree path) and offers two resume flags: `-c/--continue` (last session) and `-s/--session <id>` (specific session). `-s` is the deterministic, claude-`--resume`-equivalent path; `-c` is ambiguous and was rejected. Verified empirically against opencode 1.17.15: `opencode -s ses_NONEXISTENT` prints `Error: Session not found` (exit 0) — i.e. a stale id errors rather than silently forking, the exact failure mode claude's `FAKE_CLAUDE_RESUME_FAIL` already models.

**Recommendation:** persist the opencode session id (new nullable `tasks.opencode_session_id`, migration 00016, mirroring 00003's `claude_session_id`), capture it at spawn time, and resume via `opencode -s <stored-id>`. The capture path has one empirical unknown (whether the plugin's `session.created` fires in TUI mode — it did not in `run` mode per MEM029), so T01 spikes that first and picks plugin-capture (primary) or subprocess-discovery (fallback); both converge on the same persist column and resume argv. The resumable flag is engine-gated (`transcriptExists` is claude-only — skipped for opencode, which has no on-disk transcript glob). The frontend needs **no change**: the Resume button already fires `{kind:"agent", resume:true}` engine-agnostically and is shown whenever the status endpoint reports `resumable`.

## Recommendation

**Persist + resume via `opencode -s <id>`, with a T01 empirical spike to fix the capture strategy.** This mirrors claude's persist-and-resume model most closely, is fully CI-testable without the real opencode binary (insert a fake `opencode_session_id` row, assert the resume argv — discover-at-resume would require mocking an `opencode session list` subprocess), and keeps the resumable flag precise and cheap (a DB read, not a per-poll subprocess).

Two capture strategies, chosen by the T01 spike (MEM017: verify before planning rides on it):

- **Strategy A — plugin capture (primary, if `session.created` fires in TUI):** The shipped plugin already captures the opencode ses_id into `rootSessionID` on `session.created`. Extend that one `SessionStart` POST to carry `session_id: <ses_id>`. The hook receiver *already decodes* `session_id` (`internal/api/hooks.go`, unused today) — on `SessionStart` with a non-empty `session_id`, set it on the live `Session` and persist it through a manager-registered hook (keeps `session` DB-free; main.go wires the writer).
- **Strategy B — subprocess discovery (fallback, if it does not):** run `opencode session list --format json`, filter by `directory == worktree`, take the most-recent by `time_updated`, persist that id. Make the discovery function an injectable package var so tests stub it (no real opencode).

Both converge on: `tasks.opencode_session_id` → resume spawns `["opencode", "-s", <id>]`. A missing/stale id degrades to a fresh spawn (the "Reset session" path) or an honest 409 — never a silent fork.

## Implementation Landscape

### Key Files

**Spawn / resume argv (where the argv is built):**
- `internal/api/sessions.go` — the `create` handler. **This is where the opencode resume argv is assembled** (today lines ~268–283 build `opts.AgentArgs = renderAgentCommand(agentCommand, …)` with a fresh uuid for `{{session_id}}`, and lines ~255–257 set `opts.ResumeSessionID` from `tasks.claude_session_id`). For opencode resume: read the persisted `opencode_session_id` from the same task query (line ~218 already SELECTs `claude_session_id` + `a.engine` + `a.command` — add the new column), and when `engine=="opencode" && req.Resume`, append `["-s", storedID]` to `AgentArgs` (and skip the resume-validation block that gates on `transcriptExists`, which is claude-only). Fresh opencode spawn stays `["opencode"]` (the seed command, no placeholders) — byte-for-byte unchanged.
- `internal/session/manager.go` — the opencode spawn branch (~L170–200) is **unchanged**: it already injects the D014 env and runs `AgentArgs` verbatim. `ResumeSessionID` is already permitted for `KindAgent`; the opencode arm simply ignores it (the handler bakes `-s` into `AgentArgs`). No manager change needed for resume.
- `internal/session/session.go` — add an `agentSessionID string` field + `SetAgentSessionID(id)` (called by the receiver on SessionStart) + expose via `Info()` only if the status handler needs the live value (it does not, once persisted). `ClaudeSessionID()`/`claudeSessionID` stay as-is for the claude path.

**Capture / persistence:**
- `internal/store/migrations/00016_opencode_session_id.sql` — new file: `ALTER TABLE tasks ADD COLUMN opencode_session_id TEXT;` (+ down). Mirrors `00003_agent_sessions.sql`. Plain DDL, goose-default transaction.
- `internal/opencode/kamacu-status.js` — (Strategy A only) the `session.created` branch already computes `rootSessionID = id`; add `session_id: id` to the `SessionStart` POST payload. Update `internal/opencode/plugin_test.go` content assertions.
- `internal/api/hooks.go` — on `case "SessionStart":`, if `payload.SessionID != ""`, call `sess.SetAgentSessionID(payload.SessionID)` and trigger persistence via a manager hook (`mgr.SetAgentSessionCapturedHook(func(kamacuID, agentID string))`, wired in `cmd/kamacu/main.go` to `UPDATE tasks SET opencode_session_id = ? WHERE id = (the session's taskID)`). Keeps the `session` package DB-free. claude's own `session_id` payload is now also stored but unused — harmless.
- `internal/api/sessions.go` (persist block ~L370) — for opencode the id is captured *asynchronously* (after opencode boots), so the synchronous `UPDATE tasks SET claude_session_id = sess.ClaudeSessionID()` pattern does NOT apply; persistence is the receiver-hook's job (Strategy A) or a post-discovery write (Strategy B). Do not persist an empty id here.

**Resumable flag (status endpoint):**
- `internal/api/agents.go` — the status handler derives `resumable := info.AgentStatus == "exited" && m.csid.Valid && m.wtp.Valid && transcriptExists(a.globRoot, m.csid.String)` (L122) and the post-restart survivor pass keys off `claude_session_id IS NOT NULL` + `transcriptExists` (L149/L168). Both are **claude-specific**. Add `a.engine` to both task queries (JOIN `agents` already present in the spawn query — mirror it), and engine-gate: for `engine=="opencode"`, `resumable = exited && opencode_session_id.Valid && worktree.Valid` (no `transcriptExists`). For claude, unchanged.
- `internal/api/resume.go` — `transcriptExists` stays claude-only; no change.

**argv regression guard (the second deliverable):**
- `internal/api/testdata/fake-opencode` — new stub, modeled on `fake-claude`: `printf '%s\n' "$@" > "${FAKE_OPENCODE_ARGS_FILE:-/dev/null}"`, enable bracketed paste, idle (`sleep 300 & wait`). Records the exact argv so tests assert fresh `[fake-opencode]` vs resume `[fake-opencode, -s, ses_test…]`.
- `internal/api/opencode_resume_test.go` (or extend `internal/api/opencode_hook_status_test.go`) — wires an opencode-engine agent row (`engine='opencode'`, `command=<abs path to fake-opencode>`), spawns fresh → assert recorded argv has NO `-s`; pre-inserts `tasks.opencode_session_id='ses_test123'`, spawns `resume:true` → assert argv is `[fake-opencode, "-s", "ses_test123"]`. Fully CI-testable: no real opencode, no real `session.created`. Mirrors `agent_integration_test.go`'s `testdataFakeClaude` + argv-file pattern but via the agent `command` field.

**Frontend:** none. `web/src/api/sessions.ts` `useResumeAgent` already sends `{task_id, kind:"agent", resume:true}` engine-agnostically; the Resume button is shown whenever `resumable` is true.

### Build Order

1. **T01 — empirical spike + capture strategy.** Verify whether `session.created` (a core event-bus event, `Bus.publish(Event.Created, {info})`) fires for the TUI spawn (`opencode` with no args) — it did NOT in `run` mode (MEM029). Adapt the S02 `e2e_test.go` harness to a TUI-mode probe, or read opencode source. This single fact picks Strategy A vs B and unblocks everything downstream. (Highest risk/unknown — do it first.)
2. **T02 — migration + capture + persist.** 00016 column; the chosen capture path (plugin SessionStart+id + receiver `SetAgentSessionID` + manager hook → DB, OR subprocess discovery → DB); `Session.SetAgentSessionID`. Prove the id lands in `tasks.opencode_session_id` for a live opencode session.
3. **T03 — resume argv + resumable flag.** Handler opencode-resume branch (`-s <id>` into `AgentArgs`); engine-gate the `agents.go` resumable derivation (skip `transcriptExists` for opencode). Prove `opencode -s <id>` resumes the same conversation end-to-end.
4. **T04 — fake-opencode argv regression guard.** The stub + the fresh-vs-resume argv test. This is the lock that prevents the argv from drifting (the slice title's "guards the exact argv we build").

### Verification Approach

- **Unit (CI, no real opencode):** `go test ./internal/api/ -run Opencode` (fresh argv no `-s`; resume argv `[fake-opencode, -s, ses_test…]]`; resumable true for exited opencode + stored id + worktree); `go test ./internal/store/` (00016 idempotent); `go test ./internal/session/ -run 'Opencode|Custom|Claude'` (env injection + R016/R017 regression); `go test ./internal/opencode/` (plugin content assertions).
- **Real-binary e2e (host-gated, `-tags opencode_e2e`):** extend S02 harness with a resume proof — run a turn, capture ses_id, spawn `opencode -s <id>`, assert prior conversation visible (MEM031 provider caveat).
- **Milestone UAT (human, deferred):** restart kamacu, open an opencode task, click Resume, confirm prior conversation restored in the browser terminal.
- **Regression guards stay green:** fake-claude suite (R016), custom-engine suite incl. `TestCustomEngineDoesNotGetHookEnv` (R017), opencode hook-status integration (S01 T04), real-binary e2e (S02).

## Constraints

- **opencode owns the session id.** Unlike claude (kamacu mints `--session-id`), opencode mints `ses_…` internally; kamacu must *discover* it. No argv flag seeds an opencode session id at creation.
- **opencode launch-directory caveat (issues #28581, #6697):** `-s`/`-c` resume runs in the *launch* directory, not the session's stored directory. Does NOT bite kamacu because the spawn always sets `cmd.Dir = worktree` and the discovered session's `directory == worktree` (stable across restarts).
- **`session.created` reliability is version/mode-dependent** (MEM029: absent in `run` mode). Capture design must not assume it fires in TUI unproven — T01 resolves; Strategy B is the safe fallback.
- **`session` package stays DB-free** (dependency direction `api → session`). Persistence of the captured id goes through a func injected from the api layer (manager hook), not a direct `*sql.DB` in `session`/`hooks`.
- **No `--fork`.** `opencode -s <id> --fork` forks a new session (orphans the stored id), exactly the sharp edge claude's `--fork-session` has. Resume uses bare `-s`.
- **`transcriptExists` is claude-only.** opencode stores sessions in its own sqlite db, not as transcript files. The resumable derivation must be engine-gated.

## Common Pitfalls

- **Async capture timing** — the opencode ses_id is unknown at spawn return (opencode boots, then fires `session.created` seconds later). Do NOT persist it in the synchronous post-spawn block like claude's `claude_session_id`; persist from the receiver hook (A) or after a discovery poll (B). Asserting `sess.AgentSessionID()` non-empty immediately after `Spawn` will race.
- **Resumable flag false-negative** — forgetting to engine-gate `transcriptExists` in `agents.go` makes opencode tasks never `resumable` (no transcript glob → always false), so the Resume button never appears. The fix is the engine branch, not "make transcriptExists tolerate opencode."
- **Stale id** — `opencode -s <gone-id>` prints `Error: Session not found` (exit 0). Surface honestly (session errors/exits; user clicks "Reset session") — do NOT silently fall back to a fresh spawn inside Resume. Mirror claude's `FAKE_CLAUDE_RESUME_FAIL`.
- **Two resume-id channels** — keep `SpawnOpts.ResumeSessionID` (claude's `--resume`) and opencode's `-s` (baked into `AgentArgs` by the handler) conceptually separate. Do not route opencode resume through the claude `--resume` argv builder (MEM027).
- **`run` vs TUI event differences** — a real-binary e2e built on `opencode run` may not exercise `session.created`/TUI resume faithfully. If T01 shows `session.created` is TUI-only, the e2e resume proof needs a TUI invocation.

## Open Risks

- **`session.created` may not fire in TUI mode** (Strategy A linchpin). Mitigation: T01 spikes first; Strategy B (subprocess discovery) is the always-available fallback.
- **"Most recent session in worktree directory" ambiguity** (Strategy B only): manual opencode in the worktree could be picked. Low impact (per-task worktree); mitigated by preferring Strategy A.
- **opencode CLI flag drift** (`-s`/`--session`). The fake-opencode argv test pins the shape against opencode 1.17.15; a future rename fails the real-binary e2e loudly.

## Don't Hand-Roll

| Problem | Existing Solution | Why Use It |
|---------|------------------|------------|
| opencode session discovery (Strategy B) | `opencode session list --format json` (filtered by `directory`, sorted by `time_updated`) | opencode's own db is the source of truth; parsing its stable JSON beats reverse-engineering opencode.db |
| opencode resume semantics | `opencode -s <id>` (TUI + run modes) | documented claude-`--resume`-equivalent; `--fork`/`-c` rejected |
| argv regression stub | `internal/api/testdata/fake-claude` pattern | proven since Phase 5 — record argv to a file, idle, assert exact tokens |
| real-binary lifecycle proof | S02 `internal/opencode/e2e_test.go` host-gated harness | reuse verbatim — extend with a resume case |

## Sources

- opencode 1.17.15 `--help` / `run --help` / `session list --help` (local binary) — `-s/--session <id>` resumes a specific session; `-c/--continue` resumes last; `--fork` forks (HIGH).
- `opencode session list --format json` against `~/.local/share/opencode/opencode.db` — sessions carry `id` (`ses_…`), `directory`, `time_created`, `time_updated` (HIGH).
- opencode event bus architecture (deepwiki.com/sst/opencode) — `session.created` is a core event, `Bus.publish(Event.Created, { info: Session })` (MEDIUM).
- opencode issues #28581, #6697 — `-s`/`-c` resume runs in launch directory, not stored session directory; does not affect kamacu (HIGH for applicability).
- MEM029 — `session.created` did NOT appear in `opencode run`-mode logs (T01 must confirm TUI) (HIGH, project-local).
- `internal/api/hooks.go` — receiver already decodes `payload.SessionID` (unused); capture seam partially present (HIGH, project-local).
- `internal/api/agents.go` L122/L168 — `transcriptExists`-gated resumable is claude-specific; needs engine-gating (HIGH, project-local).