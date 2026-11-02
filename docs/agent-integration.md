# Agent integration

Session status and inbox notifications come from terminal monitoring. Configure
its model in **Settings → Terminal monitoring**. See [the inbox guide](inbox.md).

The optional MCP integration provides HTML previews and other Kamacu tools.
Agents ask questions and show completion reports normally in the terminal.

## Enable

Open **Settings → Agents → Kamacu integration** and choose **Enable integration**
for Claude Code, Codex, or OpenCode. This creates a user-level `kamacu mcp serve`
registration. The client must be installed on the host and available on Kamacu's
PATH. Integration is opt-in.

The equivalent commands are included in the binary:

```sh
kamacu integration enable claude
kamacu integration enable codex
kamacu integration enable opencode
kamacu integration status
```

Enable only the clients you use. Commands return JSON status. The server must be
running for MCP tools to work, but registration itself needs no network connection
or running Kamacu server. Start a new agent session after setup or repair.

Setup changes configuration for the account running Kamacu, on the server host.
It does not configure the computer viewing a remote Kamacu page. Custom engines
can use a manual MCP registration; see [MCP setup](install-mcp.md).

## Files and scope

| Client | MCP registration |
| --- | --- |
| Claude Code | User entry in `~/.claude.json` |
| Codex | Codex's `mcp add` command edits its user configuration |
| OpenCode | `~/.config/opencode/opencode.json` or existing `opencode.jsonc` |

`CLAUDE_CONFIG_DIR`, `CODEX_HOME`, and `XDG_CONFIG_HOME` are respected for the
corresponding client configurations. The installer preserves unrelated settings
and comments. OpenCode configuration with both JSON and JSONC files requires
consolidation before automated setup. Project-specific configuration can shadow
these user-level MCP entries; check it if a newly started client still connects
to a different Kamacu server.

Task identity and connection credentials remain inherited environment variables;
the installer never copies them into configuration. Kamacu's Codex launcher
forwards their names into the MCP subprocess environment.

## Updates, repair, and removal

For opted-in clients, server startup checks the MCP integration and updates
unchanged managed command paths. The settings page reports missing registrations,
user edits, and configuration conflicts. Use **Repair** or:

```sh
kamacu integration repair claude
```

Repair restores missing MCP registrations and refreshes unchanged managed
entries. It never forces an overwrite of user edits. For an edited MCP entry,
preserve the desired settings and remove or correct that entry through the
client's configuration tools before repairing.

Use **Remove** or:

```sh
kamacu integration remove claude
```

Removal disables future managed updates and removes only unchanged MCP
registrations installed by Kamacu. User-edited entries and compatible
registrations that predated setup are retained.

Ownership hashes and opt-in state live in `~/.kamacu/integrations.json`; no
credentials or copies of client configuration are stored there. Partial setups
can be repaired or removed. File writes are atomic and integration operations
share a lock across the server and CLI. Updates must not be performed concurrently
with manual edits to the same client configuration.

## Verification

The settings page verifies the global registration, not an end-to-end client
connection or project-level overrides. Use the live tool check in
[MCP setup](install-mcp.md#4-verify-the-install--run-this-yourself) to verify
connectivity.

Tests cover opt-in, upgrades, idempotency, missing-registration repair, independent
client registrations, user edits, configuration conflicts, JSONC comments, removal,
and browser origin protection. An optional real Codex configuration test uses a
disposable home:

```sh
KAMACU_CODEX_TEST=1 go test ./internal/integration -run TestRealCodexRegistration
```
