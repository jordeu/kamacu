# Agent feedback inbox

The global Inbox collects completion summaries and questions from task agents across all workspaces and the global Scratchpad. Each
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
Scratchpad questions share the same delivery, expiry, answer and resume protocol.
Built-in Claude, Codex, and OpenCode engines receive connection credentials;
custom engines receive task/session identity but retain their existing credential
configuration requirements.

Agents first display the question and numbered options in the terminal, then send the same question to the inbox and end their turn. Kamacu delivers the answer directly into the originating terminal.

The inbox starts with collapsed message rows; expand any row to review context and answer. A global right sidebar shows an inbox icon and pending count when collapsed. Expand it to read compact messages, choose an option, write a reply, dismiss, or mark completed work as seen without leaving the current page. Its expanded state persists across navigation and reloads. Open full message expands that message on the full-width Inbox page. On narrow screens the expanded panel overlays content.

The MCP server advertises workflow instructions and these tools:

| Tool | Purpose |
| --- | --- |
| `report_completion` | Required when work is finished, even without a question. Supply `request_key`, `abstract`, `summary`, and Markdown `context` describing changes, verification, and limitations. |
| `ask_feedback` | Create a question with `request_key`, `abstract`, `summary`, `context`, `question`, and optional `options: [{id, label}]`. Reuse the key when retrying the same request. |
| `resolve_feedback` | Report a terminal answer using `question_id`, `revision`, and either `answer` or `option_id`. Call before acting on the answer. |
| `acknowledge_feedback` | Confirm receipt of an archived result using `question_id` and `revision`. Acknowledge inbox answers before acting. |
| `list_feedback` | Recover questions for the current task, including archived questions and previous sessions. Ten per page; advance `offset` by 10 when `has_more` is true. |
| `reopen_feedback` | Explicitly reopen an unanswered question using its ID and revision. A dismissed question additionally requires `user_requested: true`, only after the user asks to reopen it. |

A typical interaction:

1. Call `ask_feedback`, providing enough context to decide without opening the task.
   Present the same question as ordinary terminal text. Do not open a blocking
   native selection dialog.
2. End the turn once the question is posted. Do not poll or hold a tool call open.
3. An inbox answer arrives as a terminal prompt with the question ID and revision.
   Call `acknowledge_feedback` before acting and use its returned answer. For an
   answer typed directly in the terminal, call `resolve_feedback` first.
4. Clarifications leave the question open. Dismissed or expired means there is no
   answer, never implicit approval.

The first answer recorded by Kamacu wins. `already_resolved` returns the current
question and winning answer; `stale_revision` means the question was reopened
since the submitted form/result was obtained. Review the current state rather
than replaying stale input. Reads do not consume answers, so saved answers can be recovered after a delivery failure. One session can have only one pending question; `existing_pending`
returns it instead of creating a duplicate. Reusing a creation key returns the
original record, even if it is archived.

## Completion summaries

Agents must call `report_completion` before ending a completed unit of work, even when they have no question. Messages show what changed, verification and remaining limitations. The inbox provides three paths:

- **Send next prompt:** free text becomes the agent's next instruction through the terminal injection and acknowledgment protocol.
- **Seen:** the agent acknowledges and ends its turn, leaving the session open without further work or another completion message.
- **Seen and exit session:** Kamacu archives the message and stops its originating session. For eligible tasks, the existing dialog offers moving the task to Done.

Completion messages use the same identity, retry keys, revisions and recovery rules as questions. Only an explicit exit option stops a session; free text containing the button label is still a next prompt. The result's `reason` is `seen` or `exit` for those actions, and empty for a text prompt. Dismissal and expiry never imply more work or permission to exit.

Existing installations receive the updated bundled skill through integration sync; restart agents to load the updated workflow and MCP tool list.

## Session lifetime and recovery

- Pending questions stay open while their originating agent session is alive.
  They no longer require polling or expire after five minutes of silence.
- Session exit archives pending questions as **Unanswered**. A reconciliation
  sweep also handles missing sessions after restart.
- Saved answers survive failed terminal delivery. The browser reports delivery
  failures; open the terminal and ask the agent to recover using `list_feedback`.
  Delivery is attempted once per accepted browser answer; repeating the HTTP
  request does not inject a duplicate prompt.
- A live successor on the same task can acknowledge recovered answers. Reopening
  an unanswered question transfers ownership and increments its revision.
- Reopening a dismissed question requires the user's explicit instruction.

Question changes and lifecycle history are persisted in SQLite. Answering and
archiving happen together; acknowledgment is tracked separately so the UI can
show **Awaiting agent** after the user has answered. Deleting a task deletes its
questions and history along with it.

## Limits and transport

Questions allow up to 10 options. Maximum UTF-8 sizes: request key 200 bytes,
summary and question 4,000 bytes each, context 32,000 bytes, option ID 100 bytes,
option label 500 bytes, answer 16,000 bytes. The HTTP body limit is 96 KiB.

Answers use the existing bracketed-paste terminal transport followed by a separate
Enter key. The payload is JSON-encoded so control characters in answers cannot
break out of the paste. **Seen** also delivers a prompt to acknowledge and stop;
**Seen and exit session** stops the session directly without injection.

The legacy HTTP wait action remains available for older clients, but new MCP
sessions no longer advertise `wait_for_feedback`. Restart agents after integration
sync to load the new skill and tool list.

## Verification

Automated tests cover routing, duplicate suppression, delivery failures, durable
answer recovery, and pending questions surviving without polls. Live CLI timing
still needs checking with Claude, Codex, and OpenCode: an answer can arrive while
an agent is finishing its previous turn, or while its prompt contains a draft.

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

New questions and completion reports require a self-contained plain-text `abstract` of at most 180 Unicode characters. The sidebar wraps this abstract and option labels; detailed summaries and context remain available in the full inbox. Existing messages receive a shortened summary during migration.
