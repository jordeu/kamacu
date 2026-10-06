This terminal replay was captured from codex-cli 0.160.1 against an isolated
local mock Responses provider on 2026-10-06. The task requests `echo status-probe`
with escalation, waits for human approval, then returns a fixed final message.
No external inference or user conversation is involved.

The JSON records terminal dimensions and byte offsets for the running,
unanswered approval, and completed screens. The ANSI file preserves cursor
movement, erasure, OSC titles, Unicode and synchronized-update sequences.
Tests replay it in deliberately fragmented chunks through the status observer.
