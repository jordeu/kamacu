# Agent integration

Kamacu bundles a `kamacu-task` skill with the application executable. The skill
teaches task agents to use the feedback inbox, include enough context to decide,
poll while waiting, and reconcile terminal answers. Agents discover it through
their native skill system; Kamacu does not inject a startup prompt.

## Enable

Open **Settings → Agents → Kamacu integration** and choose **Enable integration**
for Claude Code, Codex, or OpenCode. This installs both the skill and a user-level
`kamacu mcp serve` registration. The client must be installed on the host and
available on Kamacu's PATH. Existing users get the same opt-in controls after
upgrading; nothing is silently enabled.

The equivalent offline installer is included in the binary:

```sh
kamacu integration enable claude
kamacu integration enable codex
kamacu integration enable opencode
kamacu integration status
```

Enable only the clients you use. Commands return JSON status. The server must be
running for MCP tools to work, but installation itself needs no network connection
or running Kamacu server. Start a new agent session after setup or repair.

Setup changes files for the account running Kamacu, on the server host. It cannot
install a skill on the computer viewing a remote Kamacu page. Custom engines
need their own MCP registration and a client-compatible skill installation.

## Files and scope

| Client | Task skill | MCP registration |
| --- | --- | --- |
| Claude Code | `~/.claude/skills/kamacu-task/SKILL.md` | User entry in `~/.claude.json` |
| Codex | `~/.agents/skills/kamacu-task/SKILL.md` | Codex's `mcp add` command edits its user configuration |
| OpenCode | Same `.agents/skills` file as Codex | `~/.config/opencode/opencode.json` or existing `opencode.jsonc` |

`CLAUDE_CONFIG_DIR`, `CODEX_HOME`, and `XDG_CONFIG_HOME` are respected for the
corresponding client configurations. Codex's skill remains in `.agents/skills`.
The installer preserves unrelated settings and comments. OpenCode configuration
with both JSON and JSONC files requires consolidation before automated setup.
Project-specific configuration can shadow these user-level MCP entries; check
it if a newly started client still connects to a different Kamacu server.

Task identity and connection credentials remain inherited environment variables;
the installer never copies them into configuration. Kamacu's Codex launcher
forwards their names into the MCP subprocess environment.

The skill checks only `KAMACU_TASK_ID` and `KAMACU_SESSION_ID`, then validates the
live session using `list_feedback`. Outside a task session it leaves the agent's
normal interaction behavior alone. Discovery is model-selected, not an enforcement
mechanism: an environment variable does not automatically invoke a skill.

## Updates, repair, and removal

The skill source is embedded from
[`internal/integration/kamacu-task/SKILL.md`](../internal/integration/kamacu-task/SKILL.md).
It ships with each Kamacu version and requires no external marketplace download.

For opted-in clients, server startup checks the integration and updates unchanged
managed skill files and managed MCP command paths. The settings page reports
missing files, user edits, and configuration conflicts. Use **Repair** or:

```sh
kamacu integration repair claude
```

Repair restores missing files and refreshes unchanged managed copies. It never
forces an overwrite of user edits. To replace an edited skill, preserve it
elsewhere and remove the original file before repairing. For an edited MCP
entry, preserve the desired settings and remove or correct that entry through
the client's configuration tools before repairing.

Use **Remove** or:

```sh
kamacu integration remove claude
```

Removal disables future managed updates and removes only unchanged resources
installed by Kamacu. User-edited files and compatible registrations that predated
setup are retained and remain the user's responsibility. The Codex/OpenCode
skill stays while either integration is enabled. Other clients may also discover
compatible skill directories, so removing one client's integration does not
promise that a shared skill disappears from every client's catalogue.

Ownership hashes and opt-in state live in `~/.kamacu/integrations.json`; no
credentials or copies of client configuration are stored there. Partial installs
can be repaired or removed. File writes are atomic and integration operations
share a lock across the server and CLI. Updates must not be performed concurrently
with manual edits to the same client configuration.

## Verification

The settings page verifies installed files and the global registration, not an
end-to-end client connection or project-level overrides. Use the live tool check
in [MCP setup](install-mcp.md#4-verify-the-install--run-this-yourself) to verify
connectivity. For the task inbox, start a task agent and ask it for a decision;
confirm that it loads the skill and posts a question before polling.

Tests cover opt-in, upgrades, idempotency, missing-file repair, shared ownership,
user edits, configuration conflicts, JSONC comments, removal, and browser origin
protection. An optional real Codex configuration test uses a disposable home:

```sh
KAMACU_CODEX_TEST=1 go test ./internal/integration -run TestRealCodexRegistration
```

Manual discovery probes on 2026-10-22 used the bundled skill with Claude Code
2.1.308, Codex 0.163.0, and OpenCode 1.18.35. Each loaded or selected the skill
and called `list_feedback` followed by `ask_feedback` when asked an ordinary
choice question, without being told to load a skill. A Codex probe without the
Kamacu task environment asked in normal chat and made no feedback calls. These
are small smoke tests with a local MCP fixture, not a guarantee of activation
in every conversation or an end-to-end test of the running app.
