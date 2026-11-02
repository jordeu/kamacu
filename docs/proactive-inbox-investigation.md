# Proactive inbox monitoring

Kamacu should own a terminal-output monitor inside its Go server. Terminal output is the sole source for detecting questions, permission prompts, completion, and errors. Call a user-selected LLM to interpret rendered output and produce inbox summaries. The monitor must work without agent hooks, transcript files, plugins, or engine-specific server protocols. Keep Scratchpad as a user conversation. A continuously reasoning supervisor or a full agent SDK is unnecessary for the initial feature.

This is an architecture proposal based on repository inspection and official integration documentation, checked on 2026-10-29. Detection accuracy, latency, and local-model quality still need measurement against real session recordings.

## Accepted implementation direction

The [implementation plan](terminal-monitor-implementation-plan.md) incorporates subsequent decisions: prioritize installed Claude Code/Codex login reuse while supporting API/local endpoints; allow workspace model overrides with a global default; require model setup with no legacy mode; target 10–15 seconds; omit spending caps in the first release; suppress notifications while viewing the terminal and reconsider unanswered requests on leaving. Ordinary replies are supported first; permission menus open the terminal. These choices supersede the initial alternatives below.

## Existing integration points

- `internal/session/session.go`: `pump` already consumes PTY output, keeps a 1 MiB raw ring, and broadcasts to attached consumers. `Snapshot`, `Attach`, and `Done` provide useful observation seams. Network inference must never run under the session mutex or block this pump.
- `internal/api/hooks.go`: existing hook-driven status is outside the proposed detection path. Do not use its waiting/idle transitions to trigger or validate inbox messages.
- `internal/session/codex_status.go`: reuse the VT-emulation experience, but keep the new monitor independent of its engine-specific lifecycle reducer. Generalize terminal rendering for every engine rather than gating observation on the engine name.
- `internal/session/session.go`: custom engines report `running`; a quiet terminal is currently insufficient to infer a question or completion. Non-Codex waiting also clears on attach, so the UI status alone is not a durable attention signal.
- `internal/inbox/service.go`: durable questions, revisions, identity validation, deduplication keys, and session expiry can be reused. Introduce a trusted internal publication path with explicit origin and evidence rather than impersonating an agent through its MCP credentials.
- `internal/api/inbox.go`: answers currently inject an MCP-specific envelope instructing the agent to acknowledge feedback. That delivery contract must change for observer-created messages to support plain terminal replies.

## Options

| Approach | Strengths | Main costs and limitations | Fit |
| --- | --- | --- | --- |
| Scratchpad supervises sessions | Fast manual experiment; can use an existing agent login | User conversation and monitoring compete for context; manual interruptions, restarts, and tool permissions affect reliability; still depends on an agent remembering to orchestrate | Demonstration only |
| Dedicated supervisor agent using an SDK | Flexible reasoning, tools, tracing, and multi-step workflows | Extra runtime, credentials, context management, recovery, and possible tool actions for a bounded classification problem | Reconsider if autonomous coordination becomes a separate requirement |
| Kamacu monitor with ordinary LLM requests | Integrated lifecycle and UI; bounded requests; deterministic publication and delivery; provider adapters stay small | Requires event normalization, state tracking, evaluation, and provider configuration | Recommended foundation |
| Local model | Keeps inference local; avoids per-request API billing | Hardware, installation, warm-up latency, and accuracy vary | Optional inference backend for the same monitor |
| Terminal heuristics only | Cheap, fast, no model dependency | Cannot reliably distinguish silence, a final question, and completion across arbitrary agents | Candidate scheduling and basic degraded alerts |
| Hooks or structured engine protocols | Can supply explicit lifecycle and request identity | Introduces agent integration dependencies | Excluded by the terminal-only requirement |

A local model and an SDK are different choices: one selects where inference runs, the other selects how orchestration is implemented. Neither has to determine the other.

## Proposed flow

1. Observe the PTY stream continuously, without blocking `pump`. Maintain a headless VT emulator from session start, handling cursor movement, redraws, terminal resize, split UTF-8, and alternate screens. Keep bounded rendered scrollback and screen-change history so final text remains available after it scrolls offscreen. A raw ring may begin mid-escape sequence; stripping ANSI is insufficient.
2. Assign Kamacu-owned session generations and output sequence numbers. There are no authoritative agent turn IDs. Track visible-content changes separately from raw bytes: spinner animation, cursor movement, and repainting must not indefinitely reset the inspection timer.
3. Schedule bounded inspection after meaningful output settles. Also impose a maximum inspection interval while content changes, so an animation cannot hide a persistent question. Starting hypotheses are a 2–3 second content debounce and a 10–15 second maximum interval, to be tuned by replay evaluation. Avoid overlapping inference and do not repeatedly classify identical evidence unless a bounded verification is needed.
4. Send the current rendered screen, recent rendered history, a short prior observation, and task title to a stateless LLM call. All decision evidence must come from terminal output. Return `working`, `needs_input`, `completed`, `error`, or `unknown`, plus supporting text spans, a summary, the question, and only choices actually visible. Treat quoted questions in logs as distinct from the agent asking the user.
5. Require affirmative visible evidence before publishing. Silence alone does not distinguish thinking, a long command, a broken process, or waiting for input. A generic blank prompt is insufficient for completion. For ambiguous screens, wait for changed evidence or show a low-priority “possibly needs attention” state rather than manufacture a question. Confidence scores alone are insufficient.
6. Before committing a classification, compare the session generation, meaningful screen revision, evidence sequence, and local input generation. Cancel or discard stale results when the user interacts or the relevant screen changes. Use a stable evidence fingerprint to assign an attention episode; do not deduplicate solely by the rendered screen hash, because the same question can legitimately recur later.
7. Publish through the inbox service with observer origin, evidence excerpt, episode ID, inference version, and delivery mode. Preserve dismissed and handled episode fingerprints so unchanged output cannot repost a card. During migration, suppress duplicates from explicit tool publications without requiring those publications for detection.

The monitor reads output only for semantic detection. Kamacu may still use its own session identity, process exit, resize, and terminal-input bookkeeping for cleanup and stale-result protection; those are local transport facts, not agent lifecycle hooks. On reconnect or server restart, restore VT state/history or mark an incomplete replay uncertain until fresh evidence arrives. Do not infer a new turn merely because a browser reattached.

Terminal text is evidence, not instructions to the monitor. The classifier needs no shell, MCP, repository-write, or approval tools. Validate schemas and evidence locally. Keep timeouts, concurrency limits, a circuit breaker, and bounded request sizes. If inference is unavailable, keep terminal access and show monitoring unavailable; do not silently claim no attention is needed.

## Plain terminal replies

Publishing and answering must be designed together:

- **Ordinary conversational question:** deliver the user's plain answer as a normal prompt through the existing safe terminal message sender. Before sending, revalidate that the evidence episode still matches the current terminal. Kamacu records delivery; the agent need not acknowledge an MCP envelope. If the input mode is uncertain, open the terminal instead.
- **Native approval or form:** show the observed question and visible choices, with **Open terminal** initially. An LLM-extracted option label is not a verified keystroke mapping. Later, explicit tested terminal interaction profiles may support particular UIs without hooks. Bind each action to the current screen revision and revalidate before writing; never let the classifier autonomously approve or guess keys.
- **Completion:** keep Open task, Dismiss, and explicit session exit as Kamacu actions. Completion means terminal output reports finished work, not proof that every requirement was met.

Persist answer acceptance separately from delivery status. A crash between terminal write and recording delivery creates an ambiguous result; a PTY cannot promise exactly-once delivery. Show delivery failure or uncertainty and avoid blind replay. With no structured agent acknowledgement, successful terminal write means submitted to the terminal, not confirmed processed by the agent.

Direct terminal input immediately suspends inbox submission for the current episode to avoid racing the user. Reinspect subsequent output to determine whether the question remains. Individual keypresses, Enter, and prompt-looking output cannot universally prove that an answer was submitted or accepted across terminal apps. Mark obsolete cards superseded without inventing an answer. Viewing a terminal must not imply approval or resolution of a question.

## Provider freedom

Configure the monitoring model independently from the coding agent. A user could run one provider's terminal agent while using a different hosted or local model for classification.

Keep a narrow Go interface such as `Classify(ctx, Evidence) (Decision, Usage, error)`. Support an OpenAI-compatible endpoint with configurable base URL and model, then native provider adapters where required. Compatibility is not universal: JSON schema support, token parameters, authentication, and error formats vary. Add a setup-time capability check and validate every response locally. Plain JSON plus validation can be a fallback where schema-constrained output is unavailable.

Offer a custom command adapter if reusing installed CLI authentication is a priority: one JSON packet on stdin, one validated JSON result on stdout, timeout and output limits. This is less turnkey and provider-specific flags still need maintenance. Do not assume an existing CLI subscription supplies general API access or silently reuse credentials.

Settings should expose backend, model, endpoint, credential reference, per-workspace enablement, concurrency, and usage reporting. Make remote transmission of terminal content an explicit setup choice; send only bounded evidence and keep credentials server-side. Local inference is an optional profile, not a mandatory installation or unmeasured default model.

Ollama documents local schema-constrained outputs and partial OpenAI API compatibility. This makes it a plausible backend, not evidence that any small local model will classify Kamacu sessions well. See [structured outputs](https://docs.ollama.com/capabilities/structured-outputs) and [compatibility details](https://github.com/ollama/ollama/blob/main/docs/api/openai-compatibility.mdx).

## Runtime and SDK choice

Implement scheduling, VT rendering, attention episodes, and inbox integration in Go. This keeps observation alive with Kamacu and independent of the user's Scratchpad conversation. The monitor needs a bounded inference request, not a planning agent with tools or a persistent conversational context.

A provider SDK can simplify transport, but must not introduce a dependency on the coding agent's protocol. The Vercel AI SDK offers provider management but would introduce a JavaScript server component into the Go backend. Consider it only if broader runtime needs justify that choice; see its [provider management documentation](https://ai-sdk.dev/docs/ai-sdk-core/provider-management).

Hooks, ACP, Codex App Server, and OpenCode event APIs are outside this proposal. Terminal output monitoring must work for an arbitrary agent launched in Kamacu. Engine-specific visual profiles may improve usability later, but the generic observation path must remain functional without them.

## Delivery sequence and evaluation

1. Add generic VT rendering, bounded output history, and a background monitor behind a feature flag. Disable hook inputs to the monitor in all evaluation runs. Capture explicitly opted-in, redacted fixtures. Run in shadow mode with no inbox publications; label questions, approvals, completion, errors, and ongoing work.
2. Enable automatic cards for visibly supported questions, completions, and errors. Include evidence and Open terminal. Add configurable hosted and local inference through the same contract, plus a no-model degraded mode.
3. Add normal conversational reply delivery without MCP acknowledgements, stale-card invalidation, durable delivery status, and deduplication with explicit tool messages.
4. Add native approval/form replies only through tested terminal interaction profiles. Retain Open terminal for unsupported or ambiguous interfaces.

Evaluate visible-prompt-to-card latency, classification precision/recall by category and engine, duplicate rate, stale-card rate, delivery outcomes, tokens/cost per turn, and local CPU/RAM. Include silent long commands, TUI repaints, questions quoted in tool output, auto-approved permissions, partial output, multiple sessions, direct terminal answers, dismissal, restart, provider failure, and model timeout. Include animated waiting screens, recurring identical questions, resize, scrollback loss, alternate screens, and restart replay. Prioritize few false alerts without hiding clearly visible requests; do not rely on a model's self-reported confidence alone.

No product behavior is changed by this investigation. Repository seams and provider capabilities were inspected; no live monitoring prototype, inference benchmark, or reply integration was run. The next useful implementation is a shadow-mode monitor and labeled replay set, which will determine thresholds and model choice with evidence.
