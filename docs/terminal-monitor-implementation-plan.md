# Terminal monitoring implementation plan

Implementation plan incorporating the user decisions. This implements the terminal-only direction in [the investigation](proactive-inbox-investigation.md), including replacement of session status hooks. All requested product choices and notification semantics are settled. This records the agreed design. A first implementation now exists; see [the runtime guide](inbox.md) for delivered behavior and current limits.

## Agreed requirements

- Kamacu observes terminal output to infer session activity and publish inbox messages. Neither feature depends on agents calling tools or emitting hooks.
- One monitor result drives status indicators and inbox decisions for all agent engines, including custom commands and Scratchpad sessions.
- The monitor lives in Kamacu's Go server. Its model is configured independently of the coding agent and may be local or hosted.
- Prioritize installed Claude Code and Codex login reuse, including user subscription access. Also support direct API keys and local endpoints.
- Target 10–15 seconds from a visible question to updated UI under the evaluated workload. No spending cap in the first release; retain resource bounds and usage visibility.
- Require model setup with no legacy status/inbox mode. Each workspace can select a monitoring profile; an unset workspace inherits the global default. Scratchpad uses the global default.
- Suppress new inbox messages and alerts while the user is viewing the originating terminal. Reconsider unanswered requests when they leave.
- Ordinary text replies are in scope; permission menus/forms initially use Open terminal.
- Process exit remains a directly observed transport fact. Finished work does not mean the session process exited or that the task should move to Done.

## 1. State contract and observation infrastructure

Introduce `internal/monitor` for scheduling, classification, and state reduction, with a narrow observation interface from `internal/session`. Keep inference outside session locks and the PTY pump. Use a bounded queue with explicit overflow detection; lost bytes invalidate the renderer until resynchronized, never silently produce trustworthy state from partial input.

Maintain a generic VT renderer from session start, plus bounded rendered history. Cover split UTF-8/ANSI sequences, cursor rewrites, scrolling, alternate screens, resize, and terminal reset. Evaluate the existing vt10x dependency against recordings before extending it; it is not yet proven sufficient for every target TUI. Keep renderer state independent of browser connections and terminal view changes.

Evidence contains a session generation, output sequence, visible-content revision, screen, recent history, truncation flag, and immutable capture timestamp. Local input generation is a stale-result guard, not semantic evidence that an agent started work. No hook-fed status enters the monitor.

Acceptance: terminal rendering and browser output continue under stalled inference, queue overflow is detected, and replay fixtures produce useful screen/history snapshots without an attached browser.

## 2. Unified session state

Separate process state (`running`/`exited`) from inferred activity (`unknown`, `working`, `waiting`, `idle`, `error`). Add an observation reason (`question`, `permission`, `completion`, etc.), evidence revision, observed time, and monitor health (`ready`, `disabled`, `unavailable`).

Use `idle` for a completed response or a ready prompt; only affirmative completion text creates a completion inbox card. An empty initial prompt can be idle without completed work. `error` requires visible evidence that progress is blocked; a recoverable error printed during continuing work is not enough.

Keep waiting until output indicates it has been superseded or answered. Viewing a terminal or dismissing a card does not change agent activity. Input invalidates pending classification/reply operations and prompts re-observation; arbitrary keypresses do not prove a new turn. Silence does not turn working into completed. Retain the last observation with its age, and mark uncertainty/unavailability explicitly when appropriate.

Update `Session.Info`, `internal/api/agents.go`, `web/src/api/agents.ts`, `web/src/api/sessions.ts`, and `StatusDot.tsx` together. All status consumers use the same server result. Start by retaining the shared polling mechanism, counting its existing five-second interval in the latency budget; change refresh delivery if the chosen latency target requires it.

Acceptance: custom and built-in agents receive identical semantics; status, inbox, and process-exit presentation agree; dismissal/attach cannot falsely mark a waiting agent working or done.

## 3. Scheduling and model configuration

Inspect after meaningful content settles, with a maximum interval during ongoing changes so spinner animation cannot starve detection. The 10–15 second target includes scheduling, CLI startup/model inference, and UI refresh. Start with a 2-second content debounce, at most 5 seconds before inspecting ongoing changes, one in-flight call per session, and a bounded fair worker pool; tune from measurements rather than spending the full target on the timer alone. These are provisional, not service guarantees. Coalesce pending work; cancel stale calls where possible and always discard stale results.

Use a narrow classifier interface returning a validated decision, evidence references, optional question/visible choices, summary, and usage. Bound input and output, handle cancellation/refusals/malformed responses, and require evidence references to match supplied text. Prompts explicitly treat terminal contents as untrusted data. The model has no action tools.

Add settings for endpoint, model, credential reference, concurrency, and usage reporting. No spending-limit setting is required in this release. Credentials must not be returned through the ordinary settings GET response. Add a test-connection operation with capability checks and understandable errors. Implement installed Claude Code and Codex adapters first, then an OpenAI-compatible HTTP adapter for API keys and local endpoints plus a native Anthropic API adapter. Preserve an extension interface for other providers; an endpoint compatibility label alone is not sufficient.

Track calls, tokens where reported, timeouts, queue delay, and model latency. Do not implement spending cutoffs in this release. Bound request size, output size, concurrency, and retry count to protect operation; these are resource controls, not a monetary budget. Provider quota failures enter backoff and show monitoring unavailable, without silently switching to paid API access. Persist minimal operational metadata rather than terminal contents by default.

Acceptance: multiple sessions remain fair, unchanged evidence does not create unlimited calls, model failure leaves terminals usable, and configuration/secrets are handled correctly.

## Installed login adapters and workspace configuration

Use the installed, unmodified binaries through subprocesses with stdin evidence and structured stdout. Never read/copy OAuth tokens or send them through Kamacu HTTP clients. Account sign-in remains in each vendor CLI; expose connection status and a test action in Kamacu. Do not spawn the classifier as a normal task/Scratchpad agent: it must not be monitored, post inbox messages, or recursively invoke itself.

Codex documents that `codex exec` reuses saved CLI authentication; its authentication docs distinguish subscription sign-in from API billing. Use ephemeral runs, schema-constrained output, a dedicated empty working directory, and a version-tested configuration that disables unnecessary tools, MCP, and hooks while preserving auth. A read-only sandbox alone still allows tools, so verifying a genuinely restricted classifier profile is an explicit adapter acceptance test. See [non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode) and [authentication](https://learn.chatgpt.com/docs/auth).

Claude supports `claude -p`, JSON/schema output, and disabled built-in tools. Keep its normal authentication path; `--bare` explicitly skips subscription login. Use supported settings/tool/MCP controls to isolate the classifier, and disable conversation persistence. Version-test these combinations rather than assuming flags compose. See [programmatic execution](https://code.claude.com/docs/en/headless) and [authentication](https://code.claude.com/docs/en/authentication). Anthropic distinguishes running its unmodified binary with the user's own subscription from collecting/intermediating credentials; this design keeps sign-in and requests inside that binary. See [binary integration and credential guidance](https://code.claude.com/docs/en/legal-and-compliance).

For both adapters: remove inherited Kamacu task/hook identity, launch without shell interpolation, terminate the process group on cancellation, cap stderr/stdout, parse refusal/auth/quota errors separately from valid classification, and prevent project instructions from contaminating the prompt. Detect auth overrides and display the active method; do not claim subscription billing merely because the CLI exists. Do not modify the user's login or authentication options. Subscription quotas and available models remain controlled by the provider.

Use a fresh classifier request initially. Benchmark CLI startup and authentication refresh contention with multiple sessions; optimize transport reuse only if needed and without mixing workspace evidence. No real subscription inference has been tested during this planning work.

Store reusable `monitor_profiles` with backend (`claude_cli`, `codex_cli`, `openai_compatible`, `anthropic_api`), model, executable/endpoint, nonsecret adapter options, and credential reference. A global setting references the default profile; a nullable workspace profile reference overrides it. Resolve workspace from the task's project ownership, never from the browser's currently selected workspace. Scratchpad resolves globally.

Resolve the entire profile rather than mixing credentials/model/endpoint fields across scopes. No override means inherit. A configured but failing override stays unavailable; it must not silently send workspace text to the global provider. Profile edits and workspace moves increment a configuration generation, cancel affected in-flight calls, and reject old results. Profile deletion must require replacing live references or explicitly resetting them to inheritance.

UI: Global Settings defines the default and profiles; Workspace Settings offers Use global default or a chosen profile and shows the effective provider/model. API responses expose configured credential status, not secret values. Setting up a valid default is required before starting new agent work; existing terminals remain accessible during upgrade, auth failure, or quota exhaustion, with honest unknown/unavailable status and no hook fallback. Provider failures do not stop running coding agents.

## 4. Inbox publication and persistence

Extend `internal/inbox/service.go` and migrations with observer origin, attention episode ID, evidence excerpt/revision, classification version, and delivery mode. Store monitor episode state sufficient to suppress duplicate/dismissed observations across restarts. Use a uniqueness constraint and transactional publication rather than relying only on in-memory checks.

Group related questions in one current episode per session to match the existing pending-message constraint. Update a pending card when its evidence changes and increment its revision so old answers cannot apply. Archive a superseded card explicitly, without fabricating a user answer. A later genuine recurrence of the same question gets a new episode after observed intervening progress; uncertainty must not cause repeated notifications.

Apply the presence gate below immediately before publication. Generate existing abstract/summary/context fields from the evidence, respecting current limits. Publish questions and completions; represent blocking errors or unsupported native dialogs as attention cards with Open terminal. During migration, prevent explicit tool messages from producing duplicate observer cards.

Acceptance: one visible situation creates one card, dismissal survives replay/restart, changed questions reject old replies, and simultaneous sessions remain isolated.

## Presence and deferred notifications

Reuse `internal/presence` and `web/src/presence`, which already identify terminal sessions, browser visibility, and five-second heartbeats with a fifteen-second expiry. Add explicit terminal selection/unselection, route changes, browser focus/blur, and visibility changes. Define viewing per exact session, not task route or an open websocket. Use all fresh clients; another active window showing that session still counts. Proposed default: a selected terminal in a visible, focused browser window is viewed, even when the user is not typing.

Always compute session status while viewed. Store an attention episode internally, but do not create an inbox row, increment unread count, change the favicon for it, or send an alert. Presence is a publication decision only; it cannot change what the agent is inferred to be doing.

On leaving, debounce briefly (about one second) and reevaluate publication even if no new terminal bytes arrive. If an unanswered question/permission request or blocking error still exists, publish it once. If the screen/input revision changed, reclassify before deciding. If output confirms resumed work, retire the episode without notification. Mere typing or Enter is insufficient evidence that the request was answered. If uncertainty remains after terminal interaction, keep the episode pending re-evaluation rather than falsely marking it answered.

Recheck presence and episode revision at the publication boundary. Returning to a terminal cancels a pending publication timer. Abrupt disconnect becomes away when its presence lease expires; detection may take longer than explicit navigation. Persist deferred episodes so a restart does not lose an unanswered request. On restart, allow a short presence-recovery window before publishing replayed episodes.

Agreed completion rule: completion observed while viewed is considered seen, and does not generate a delayed notification after leaving. Agreed existing-card rule: viewing hides/suppresses attention indicators for that session while preserving the unresolved episode; leaving restores the same card if unanswered, never creates a duplicate. Explicit dismissal remains durable and overrides later presence changes.

Acceptance: no new notification while watching; unanswered questions surface after departure without needing more output; terminal answers that resume work suppress stale alerts; hidden tabs, multiple windows, crashed browsers, races, and restart behave consistently.

## 5. Replies and delivery recovery

For verified ordinary chat-input situations, deliver the user's answer as plain text through the terminal sender; remove MCP acknowledgement instructions for observer messages. Acceptance, delivery attempt, successful PTY write, and uncertain/failed delivery are distinct recorded states.

Serialize terminal writes and coordinate the evidence/input-generation check with delivery initiation. Inspect `sendTerminalMessage` in `internal/api/sessions.go` for paste/Enter interleaving and terminal-control sanitization. Reject a stale reply before writing; preserve the user's text so they can recover it.

A PTY write cannot prove the agent processed the answer. Never blindly retry a write whose result is uncertain. On process exit, disable replies and archive pending questions with an explicit reason.

Agreed initial boundary: native approval menus/forms offer Open terminal. Direct actions require separately tested terminal interaction profiles and fresh screen validation; the model never invents keystrokes or grants approval autonomously. Direct menu actions are deferred beyond this first release.

Acceptance: browser answer versus terminal typing races cannot silently apply an obsolete answer; crash windows expose uncertainty; ordinary replies arrive as plain terminal input.

## 6. Restart and hook removal

Reattach observers to surviving tmux sessions and reconstruct a complete terminal snapshot where possible. Incomplete replay remains uncertain; persisted handled episodes suppress old notifications. New process generations cannot inherit pending decisions from unrelated sessions.

Audit `internal/api/hooks.go`, `internal/session/agent.go`, engine launch overlays/profiles, installers, status reducers, and tests. Remove status dependence first, then retire obsolete hook installation/configuration without offering a legacy mode. Shadow mode is for development validation only, not a production fallback. Preserve unrelated user configuration and shared credentials used by other Kamacu APIs.

Audit conversation-ID capture and resume separately: Codex currently captures identity through hooks as well as terminal-footer handling, and engine resume tests depend on saved IDs. Hook removal must not silently regress restart/resume. Validate non-hook identity paths; if an engine cannot expose an ID without integration, document that concrete limitation and resolve it before removal. Status and inbox must remain terminal-only regardless.

Acceptance: supported launch/reconnect/restart/resume paths pass with status hooks disabled, including custom engines and Scratchpad. Existing inbox history survives migration.

## 7. Evaluation and rollout

Build deterministic recording/replay tests and a classifier stub before live inference. Fixtures cover questions in tool logs, unfinished sentences, silent commands, spinners, menus, recoverable errors, repeated identical questions, resize, alternate screens, output loss, multiple sessions, restart, and dismissal. Keep real recordings opt-in and redacted.

Run the actual selected model in shadow mode on a labeled corpus. Report precision/recall by state and engine, visible-question-to-card latency (including UI refresh), duplicates, stale results, calls/tokens per active session-hour, and local resource use. Use a provisional load matrix of 1, 5, and 10 concurrent coding sessions across two workspaces; measure rather than promise support for an unspecified maximum. Target p95 question-to-visible-card latency within 15 seconds on the selected test models with healthy providers. Treat this as a release target, not a guarantee under outages or quota exhaustion; do not claim reliability from synthetic tests alone.

Implement in reviewable slices: observation/replay → installed-login adapters and global/workspace configuration → unified status → presence-aware inbox → reply delivery → recovery/hook cleanup → HTTP provider adapters and end-to-end validation. Enable publication only after shadow-mode results are reviewed. This is an implementation validation step, not an ongoing dependency on hooks.

Run focused Go tests and race tests for observer/inbox concurrency, migration/recovery tests, frontend type checks and status/inbox tests, then browser scenarios for task, board, sidebar, and Scratchpad. The release must demonstrate a complete question → inbox → reply → resumed output flow without hooks or explicit feedback tools.

## Final notification rules

- A completion produced while its terminal is viewed counts as seen and never creates a delayed notification on departure.
- An already-published unanswered card and its attention indicators hide while the originating terminal is viewed. The same card returns if the user leaves without answering; it is not recreated or marked answered.
- A new question detected while viewed is retained internally and published only after departure if still unanswered.
- An actual answer followed by resumed output retires the episode; explicit dismissal remains durable. Viewing alone never resolves a question.

Record presence intervals against output sequence/timestamps so a classification that finishes after departure still recognizes a completion produced while the user was watching. Use the evidence timestamp, not the inference completion time, for this rule. If presence or replay history is incomplete, do not invent a seen/answered event.

Implementation should distinguish episode resolution from presentation visibility: hiding a card is not dismissal. Exclude hidden cards from attention counts and restore visibility without a new notification identity. Add replay/browser tests for delayed classification across departure, repeated enter/leave, completed work seen live, and terminal replies while a card is hidden.

## Implementation status

The first working implementation includes generic terminal rendering, installed Claude/Codex login adapters, OpenAI-compatible and Anthropic HTTP adapters, required model configuration, global/workspace selection, terminal-derived status, presence-gated inbox episodes, plain-text reply delivery, usage counters, and removal of production status-hook wiring. Optional artifact/MCP tools remain available. Legacy creation tools cannot bypass monitoring in the running server.

The implementation uses synchronous bounded VT rendering instead of a lossy asynchronous byte queue, keeping inference outside session locks. Agent sessions are direct PTYs; only shell tabs use persistent tmux. Consequently server restart archives agent questions and restores conversation resume metadata, rather than reattaching a surviving agent renderer.

Validation includes the full Go suite, 73 frontend tests, browser settings/inbox checks, a fragmented recorded Codex terminal replay, a deterministic terminal → question → inbox reply → completion flow, evidence-time presence tests, and twelve live synthetic Claude/Codex classifications. Initial calls were roughly 4–5 seconds. This is not a production accuracy or concurrent-session latency benchmark.

Remaining release validation: broad real-terminal corpus, the 1/5/10-session load matrix and p95 measurements, and race-detector execution on a machine with a C compiler. A fresh Codex process killed before emitting its normal resume footer may have no saved conversation ID; normal exit/resume and already persisted IDs work without hooks. That abrupt-exit recovery gap remains open. Token counters are available where the adapter receives usage data; the Codex final-text adapter does not yet report tokens. Concurrency is currently fixed at four workers.


Review fixes add ordered, per-document presence beacons; atomic card restoration against presence changes; strict classification validation; provider backoff despite repaints; cancellation on session exit and profile changes; serialized paste/submit delivery; recovery of uncertain writes; accurate small-terminal rendering with explicit oversized-terminal unavailability; and stable episode identity across model rewording and subsequent repeated requests. Regression tests exercise these failure paths, including persistence errors and restored episodes.
