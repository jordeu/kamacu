# Agent integration

Session status and inbox notifications come from terminal monitoring. Configure
its model in **Settings → Terminal monitoring**. See [the inbox guide](inbox.md).

The optional MCP integration provides HTML previews and other Kamacu tools.
Agents ask questions and show completion reports normally in the terminal.

## Enable

Open **Settings → Agents → Kamacu integration** and choose **Enable integration**
for Claude Code, Codex, OpenCode, or Pi. This creates a user-level `kamacu mcp serve`
registration and adds a project context instruction to the agent’s global guidance. The client must be installed on the host and available on Kamacu's
PATH. Integration is opt-in.

The equivalent commands are included in the binary:

```sh
kamacu integration enable claude
kamacu integration enable codex
kamacu integration enable opencode
kamacu integration enable pi
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
| Pi | `~/.pi/agent/mcp.json` |

`CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `XDG_CONFIG_HOME`, and `PI_CODING_AGENT_DIR` are respected for the
corresponding client configurations. The installer preserves unrelated settings
and comments. OpenCode configuration with both JSON and JSONC files requires
consolidation before automated setup. Project-specific configuration can shadow
these user-level MCP entries; check it if a newly started client still connects
to a different Kamacu server.

Task identity and connection credentials remain in the runtime environment.
Kamacu forwards the variables to Codex and uses environment references in Pi's
MCP registration.

## Pi sessions and models

Install Pi 1.6.1 or later on the Kamacu host, sign in to the provider through
Pi, and select Pi in the project, task, or Scratchpad settings. To offer model
choices on tasks, open **Settings → Agents**, edit Pi, then discover or add
provider-qualified IDs such as `provider/model-id`. Save the models you want
in the task picker. Use **Agent default** to leave the choice to Pi. For models
registered by extensions, supply a discovery command that lists those models.

The selected task model applies the next time the agent starts or resumes.
Kamacu tracks the conversation it launches. Use Kamacu's Reset to start a
new tracked conversation. Pi's own `/new` and `/resume` can still switch
conversations within the terminal. Kamacu keeps saved Pi transcripts in a
private directory alongside its database and offers Resume after Pi has
written the conversation there.

## Project instructions and skills

Kamacu creates a context folder with a `skills/` directory for each project on
creation and fills in missing folders at server startup. Find the project ID in
its Kamacu URL. This example shows project 12 on the default database.

```text
~/.kamacu/projects/12/context/
├── instructions.md
└── skills/
    └── review-migration/
        ├── SKILL.md
        └── references/checklist.md
```

Write standing project guidance in `instructions.md`. Give each skill a
`SKILL.md` with a name and a description of when to use it:

```markdown
---
name: review-migration
description: Use when reviewing SQLite schema changes.
---
Read references/checklist.md and verify the upgrade from the previous release.
```

On server startup, Kamacu installs two bundled skills as ordinary files under
`~/.kamacu/skills/`, even before the first project exists.

- `find-project` identifies projects using Kamacu’s project list.
- `new-task` checks the project board for duplicates and creates a task when requested.

Both skills ship in the Kamacu binary and work without a separate checkout or
download. On upgrade, startup replaces the old directory links to
`kamacu-director/.claude/skills/{find-project,new-task}` with the bundled
files. The director checkout keeps its original files. Subsequent starts fill
in missing files and preserve local edits. To restore a bundled version, remove its shared `SKILL.md` and restart
the server. Place additional skill directories in the same library to make
them available across projects.

Kamacu links each library entry containing a regular `SKILL.md` into the
project’s `context/skills/`. Editing the original updates every linked project
immediately. Startup and agent Start or Resume pick up newly shared skills and
remove links to retired entries. A file, directory or custom link with the
same name in a project takes precedence. To customize a shared skill for just
one project, replace its project link with a local skill directory. Custom
database instances share the same library on the host.

On Start and Resume, an enabled agent receives the absolute directory in
`KAMACU_PROJECT_CONTEXT`. Its global instruction tells it to read the current
guidance, discover the skills, and read a matching skill when needed. Changes
are visible the next time the agent reads a file. Project instructions and local
skills are optional. Skills are read through the agent’s ordinary file tools.
An agent that requests permission for directories outside its worktree may
prompt for read access to the context folder and shared skill sources.
Scratchpad, shell tabs and custom agents do not receive the variable.

The integration installs its block in Claude’s user `CLAUDE.md`, Codex’s
`AGENTS.md` (or active `AGENTS.override.md`), OpenCode’s global `AGENTS.md`,
or the active global context file in Pi’s agent directory. For a fresh Pi
profile, it creates `AGENTS.md`. Pi’s `--no-context-files` flag skips the
managed block along with other context files.
If OpenCode uses Claude’s global file as a fallback, the integrations share
the block. Repair preserves edits to the block and surrounding guidance.
Disable removes unchanged managed instructions when the last owner releases them.

Deleting the project archives its context at
`~/.kamacu/archive/projects/<id>/<archive-id>/context/`. Archived shared skills remain
links to their source. Copy files back manually to restore them. If deletion is interrupted and you have already
recreated the live folder, recovery keeps your current files there and places
the previous copy in the archive. Removing a task or its worktree leaves project context
in place. Custom databases use `~/.kamacu/instances/<hash>/` as the base
for both directories. The hash is the lowercase SHA-256 of the absolute
symlink-resolved database path, without a trailing newline. If you relocate
a custom database, move its context and archive directories to its new base.

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
