---
name: kamacu-task
description: Use before asking the user a question, requesting feedback or clarification, seeking approval, or offering decision choices. Also use when publishing an HTML preview. Checks the session environment for a Kamacu task or Scratchpad and routes feedback through its inbox when present; otherwise leaves normal interaction unchanged.
---

# Kamacu task feedback

## Check task context

Read only `KAMACU_TASK_ID` and `KAMACU_SESSION_ID` from your environment. Do not dump the environment or print credentials. If either is missing, or the task ID is not a nonnegative integer, use your normal interaction workflow and stop applying this skill. `KAMACU_TASK_ID=0` identifies the global Scratchpad; positive IDs identify regular tasks. Never invent, change, or copy identity from another session.

For a task or Scratchpad session, call Kamacu's `list_feedback` MCP tool to validate the session and recover existing questions. The MCP server supplies and validates identity; do not pass task or session IDs as tool arguments. If the server is unavailable or rejects the session, explain that the inbox is unavailable. Do not claim to have posted a question or silently proceed on an assumption. Normal chat remains available.

## Ask with enough context to decide

Before asking for a decision, call `ask_feedback` with:

- A stable `request_key`; reuse it when retrying the same question.
- A short `summary` of the task and its objective.
- Markdown `context` explaining the findings, relevant alternatives, tradeoffs, and your recommendation. Give enough information to decide without opening the task. Do not include credentials.
- The specific `question` and, when useful, simple `options` with stable IDs and clear labels. Free-text replies are also supported.

Present the same question as ordinary terminal text. Avoid native blocking question dialogs: the user must be able to answer in either the inbox or the terminal. Only one question can be pending per session; `existing_pending` means use the returned question.

## Wait and reconcile both answer channels

Call `wait_for_feedback` repeatedly, with up to 20 seconds per call. Process terminal messages between polls. Do not continue other work while waiting.

- **Inbox answer:** call `acknowledge_feedback` with the question ID and current revision before acting.
- **Terminal answer:** call `resolve_feedback` with the question ID, current revision, and either `answer` or `option_id` before acting. Only report an answer actually supplied by the user.
- **Clarification:** explain in chat and keep polling; clarification is not a decision.
- **Conflicting answers:** the first answer recorded by Kamacu wins. Use the returned answer when another channel already resolved the question. `stale_revision` requires reviewing the current state before submitting anything again.
- **Tool error or interrupted call:** recover the current state before retrying or acting. Never claim an answer was recorded or a message archived without confirmation.

Polling renews a five-minute heartbeat. When polling stops for five minutes, or the owning session stops, Kamacu archives the question as unanswered. Cancellation of a single poll does not withdraw it. Dismissed and expired results are not answers or approvals; stop polling them.

## Resume or reopen

Use `list_feedback` on resume to recover this task's questions. It does not renew heartbeats. Continue polling a pending question owned by this session, or acknowledge an answered result before using it.

If an unanswered question is still needed, explicitly call `reopen_feedback`; a new live session on the same task can take ownership. Use the returned revision. Reopen a dismissed question only after the user's explicit instruction, with `user_requested: true`. Answered questions cannot reopen. If the question or decision context has changed, create a new question once the previous one is no longer pending.

## HTML artifacts

To publish a preview, use the artifact MCP tools with `task_id` equal to `KAMACU_TASK_ID`, including `0` for Scratchpad. Begin an upload, HTTP PUT the raw HTML file or bundled archive to the returned URL using its headers, then finalize it. Scratchpad previews appear as tabs in Scratchpad; regular task previews appear in their task.
