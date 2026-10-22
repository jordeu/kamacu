# Agent feedback inbox

The global Inbox collects questions from task agents across all workspaces and the global Scratchpad. Each
question includes its task summary, Markdown decision context, optional answer
buttons, a free-text answer field, and an **Open task** link. The link selects the
Agent tab and focuses the originating terminal if that session is still available.
Answered, dismissed, and unanswered questions appear in Archived.

## Agent protocol

Enable the bundled task skill and MCP registration in **Settings → Agents →
Kamacu integration**, or run `kamacu integration enable <claude|codex|opencode>`.
See [integration setup](agent-integration.md). Start the
agent from Kamacu so its MCP subprocess inherits `KAMACU_TASK_ID`,
`KAMACU_SESSION_ID`, `KAMACU_HOOK_BASE`, and `KAMACU_HOOK_TOKEN`. Task and session
identity are taken from that environment, not model-supplied tool arguments.
Existing agents must be restarted to receive the new task identity. Scratchpad
uses `KAMACU_TASK_ID=0`; the server verifies that its session is a live Scratchpad
agent. Its messages show **Scratchpad** and an **Open Scratchpad** link to the
originating terminal. Agents launched outside Kamacu cannot create inbox questions.
Scratchpad questions share the same polling, expiry, answer and resume protocol.
Built-in Claude, Codex, and OpenCode engines receive connection credentials;
custom engines receive task/session identity but retain their existing credential
configuration requirements.

The MCP server advertises workflow instructions and these tools:

| Tool | Purpose |
| --- | --- |
| `ask_feedback` | Create a question with `request_key`, `summary`, `context`, `question`, and optional `options: [{id, label}]`. Reuse the key when retrying the same request. |
| `wait_for_feedback` | Poll by `question_id`, optionally holding the request for `wait_seconds` (0–20; default 20). Renews the heartbeat once on arrival. |
| `resolve_feedback` | Report a terminal answer using `question_id`, `revision`, and either `answer` or `option_id`. Call before acting on the answer. |
| `acknowledge_feedback` | Confirm receipt of an archived result using `question_id` and `revision`. Acknowledge inbox answers before acting. |
| `list_feedback` | Recover questions for the current task, including archived questions and previous sessions. Ten per page; advance `offset` by 10 when `has_more` is true. Does not renew heartbeats. |
| `reopen_feedback` | Explicitly reopen an unanswered question using its ID and revision. A dismissed question additionally requires `user_requested: true`, only after the user asks to reopen it. |

A typical interaction:

1. Call `ask_feedback`, providing enough context to decide without opening the task.
   Present the same question as ordinary terminal text. Do not open a blocking
   native selection dialog.
2. Keep calling `wait_for_feedback`, processing terminal messages between calls.
   Do not do unrelated work while waiting. A request for clarification is not an
   answer: explain in the terminal, then continue polling.
3. If the inbox answers, acknowledge the returned result and proceed. If the user
   answers in the terminal, call `resolve_feedback` first and use the returned
   state/answer. Never assume a failed or timed-out submission was accepted.
4. Stop polling when the result is terminal. Dismissed or expired means there is
   no answer, never implicit approval.

The first answer recorded by Kamacu wins. `already_resolved` returns the current
question and winning answer; `stale_revision` means the question was reopened
since the submitted form/result was obtained. Review the current state rather
than replaying stale input. Reads do not consume answers, so interrupted polls
can be retried. One session can have only one pending question; `existing_pending`
returns it instead of creating a duplicate. Reusing a creation key returns the
original record, even if it is archived.

## Heartbeat and recovery

- A question expires five minutes after creation or its most recent poll.
- Expiration archives it as **Unanswered**. Polling an expired question returns
  `expired` and does not reopen it.
- A detected session exit archives pending questions immediately. A one-second
  reconciliation sweep also handles stopped/missing sessions and lease expiry.
- A live successor session on the same task may acknowledge a recovered answer.
  The original owner and answer remain unchanged; the acknowledgment records the
  receiving session in the event history.
- Restart reconciliation archives pending questions whose owning session no longer
  exists. Answered questions and acknowledgment state remain durable.
- An explicit reopen starts a fresh heartbeat and increments the revision. A new
  live session on the same task may take ownership; updates from the previous
  session are rejected. Answered questions cannot be reopened.
- Cancelling a poll leaves the question intact. Only an explicit user instruction
  permits reopening a dismissed question; this instruction is enforced by the
  agent workflow, not cryptographically verified by Kamacu.

Question changes and lifecycle history are persisted in SQLite. Answering and
archiving happen together; acknowledgment is tracked separately so the UI can
show **Awaiting agent** after the user has answered. Deleting a task deletes its
questions and history along with it.

## Limits and transport

Questions allow up to 10 options. Maximum UTF-8 sizes: request key 200 bytes,
summary and question 4,000 bytes each, context 32,000 bytes, option ID 100 bytes,
option label 500 bytes, answer 16,000 bytes. The HTTP body limit is 96 KiB.

Feedback long polls have a dedicated 25-second bridge timeout, rather than the
ordinary bridge's 10-second timeout. Client tool timeouts must exceed the chosen
wait duration. Terminal input may be queued until the poll finishes; terminal-side
archiving depends on the agent reporting the answer. MCP elicitation support is
not required.

## Verification

An isolated live MCP probe on 2026-10-22 asked each installed client to poll a tool
that waited eight seconds, submitted a terminal answer during that wait, and
verified a subsequent MCP call reported the same answer:

| Client | Result |
| --- | --- |
| Claude Code 2.1.308 | Passed |
| Codex CLI 0.163.0 | Passed |
| OpenCode 1.18.35 | Passed |

This checks the communication pattern, not a guarantee that every model will
follow the workflow in every conversation. Unit/integration tests cover identity,
concurrent answers, expiry, reopening, ownership transfer, dismissal, recovery,
acknowledgment, and cancellation.

Browser acceptance uses a disposable database and the built frontend:

```sh
npm ci --prefix web
npm run build --prefix web
KAMACU_BROWSER_TEST=1 \
KAMACU_PLAYWRIGHT_PACKAGE=/absolute/path/to/playwright \
go test ./internal/api -run '^TestInboxBrowser$' -count=1
```

Optionally set `KAMACU_BROWSER_EXECUTABLE` to an installed Chromium executable and
`KAMACU_INBOX_SCREENSHOTS` to a directory for desktop/mobile captures.

Codex filters the environment of stdio MCP subprocesses. At launch, Kamacu inspects enabled local Kamacu MCP registrations and adds the four `KAMACU_*` variable names to their existing `env_vars` allowlist using process-local configuration overrides. Credentials remain in the environment; user configuration is not rewritten. Register the server as `kamacu` if you use a wrapper command. Discovery failures stop launch with a diagnostic instead of starting with broken inbox identity.
