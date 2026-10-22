# Installing the Kamacu MCP server

This document is written to be executed by a coding agent, not read as prose. If
you are an agent and were pointed here, follow the steps in order, run the
verification in step 4 yourself, and report the result. Substitute real values
wherever you see `<ANGLE-BRACKET-PLACEHOLDERS>`.

## 0. The instruction this document serves

A user installs you by pasting this line to you, verbatim:

```
read https://raw.githubusercontent.com/jordeu/kamacu/master/docs/install-mcp.md and install the Kamacu MCP server for yourself
```

You are the intended executor of that line: carry out steps 1–5 below.

## 1. What you are installing

`kamacu mcp serve` is a stdio MCP server that bridges to the JSON API of the
running Kamacu app, giving you ~30 tools to drive Kamacu boards programmatically
(`list_projects`, `create_task`, `move_task`, `start_task_agent`,
`post_pr_review`, …).

**Hard prerequisite: the Kamacu app must already be running and reachable**
(someone started `kamacu serve`). It normally runs on this machine; remote
agents can use a trusted tunnel and set `KAMACU_HOOK_BASE` to their reachable
base URL. No shared filesystem is needed. The MCP server is only a bridge —
without the app it starts fine but every tool call fails (see step 4).

## 2. Preflight — run these first

### 2.1 Is the Kamacu app up?

```sh
curl -fsS http://127.0.0.1:7333/api/projects -o /dev/null -w '%{http_code}\n'
```

- `200` — the app is up. Continue.
- `connection refused` — the app is NOT running. Do not install yet: tell the
  user to start it (`kamacu serve`, or the boot service set up by
  docs/install.md) and stop here. An
  install done now will look fine but fail on every tool call.

### 2.2 Locate or install the binary

The binary is NOT installed to `PATH` by default. Resolve an absolute path:

```sh
command -v kamacu || echo NOT_ON_PATH
```

- If a path prints, use it (absolute — run `command -v` output through
  `readlink -f` if it is relative).
- If `NOT_ON_PATH`, install the Kamacu app first: fetch and follow
  https://raw.githubusercontent.com/jordeu/kamacu/master/docs/install.md —
  the install guide for the app binary. It downloads the latest release for
  this machine (Linux or macOS), verifies its checksum, and installs it to
  `~/.local/bin/kamacu` by default. Then re-run `command -v kamacu` and use
  the printed path. Building from a checkout (`make backend`; needs `git` and
  Go, not Node) remains a fallback when GitHub releases are unreachable.

From here on, `<KAMACU_BIN>` means that absolute path with `$HOME` already
expanded (e.g. `/home/you/.local/bin/kamacu`). **MCP client configs
do not expand `~` or `$HOME` — always write the fully resolved absolute path.**

### 2.3 Environment variables — usually none needed

Agents spawned *inside* Kamacu receive `KAMACU_HOOK_BASE` and
`KAMACU_HOOK_TOKEN` injected. You, installing from a plain terminal, need
**neither**:

- `KAMACU_HOOK_BASE` defaults to `http://127.0.0.1:7333`, which is where the
  app listens by default. Set it only if the user started the app with a
  non-default `--addr`.
- `KAMACU_HOOK_TOKEN` is required for HTML artifact upload control and the
  presence snapshot and inbox feedback. Built-in agents started by Kamacu receive it automatically. An
  externally configured agent needs the current instance token through its
  trusted environment configuration to use these tools. Existing project,
  task, workspace, session, and review tools do not require it. The app still
  binds to loopback by default; this does not enable public remote access.

## 3. Enable the agent integration

For Claude Code, Codex, or OpenCode, use the bundled installer:

```sh
<KAMACU_BIN> integration enable claude   # or: codex / opencode
<KAMACU_BIN> integration status
```

This registers the user-level MCP server and installs the bundled `kamacu-task`
skill. No separate download or startup prompt is needed. The same controls are
available under **Settings → Agents → Kamacu integration**. Setup affects the
user account running Kamacu on the server host; it does not configure a browser
visitor's computer. Start a new agent session afterward, then verify step 4.

The installer reuses a compatible existing registration without taking ownership
of it. Conflicting entries or edited skills are preserved and reported. For
repair, removal, update behavior, and client configuration paths, see
[Agent integration](agent-integration.md).

### Manual MCP-only registration

Use the following instructions for another MCP client or a manually maintained
configuration. These steps do not install the task skill. Pick your client.

### Before you add: check for an existing kamacu entry

```sh
claude mcp get kamacu      # or: opencode mcp list, or grep '"kamacu"' <client-config>.json
```

If a `kamacu` entry already exists, do not blindly add over it. Known client
behaviors (both verified):

- Claude Code's `mcp add` **refuses** to add when `kamacu` already exists
  in the same scope (`MCP server kamacu already exists in user config`), but an
  entry in a *different* scope can shadow it (see Troubleshooting).
- `opencode mcp add` **silently replaces** an existing `kamacu` entry with no
  warning.

If an entry exists, tell the user what it points at before changing anything,
and skip re-adding if it already works (verify with step 4).

### Claude Code

```sh
claude mcp add kamacu --scope user -- <KAMACU_BIN> mcp serve
```

Scope choice:

- `--scope user` — available in all your projects. Recommended for a personal
  machine (this is a localhost-only server).
- `--scope local` (the default) — this project only, private to you.
- `--scope project` — writes a committable `.mcp.json` in the repo. Usually
  wrong here: it embeds your absolute binary path, which other machines won't
  have.

Verify and clean up with:

```sh
claude mcp list                 # expect: kamacu: <KAMACU_BIN> mcp serve - ✔ Connected
claude mcp remove kamacu -s user
```

### OpenCode

```sh
opencode mcp add kamacu -- <KAMACU_BIN> mcp serve
```

This writes a global entry to `~/.config/opencode/opencode.json` (all
projects). For a single-project install, put the same entry in that project's
`opencode.json` instead:

```json
{
  "mcp": {
    "kamacu": {
      "type": "local",
      "command": ["<KAMACU_BIN>", "mcp", "serve"]
    }
  }
}
```

**Merge, never overwrite.** When you edit a JSON config file by hand: read the
existing file first (create it only if missing), add just the `kamacu` key
under `mcp`, and preserve every other key and entry. The config likely holds
other MCP servers the user depends on — replacing the file with the block above
would delete them. If the existing file does not parse as JSON, stop and report
the contents to the user instead of repairing or replacing it.

Verify with `opencode mcp list` (expect `✓ kamacu connected`). There is no
remove subcommand — to uninstall, delete the `kamacu` key from the JSON file
(again: merge out the key, don't rewrite the file).

### Any other stdio MCP client

Use your client's local-stdio server config with this block:

```json
{
  "mcpServers": {
    "kamacu": {
      "command": "<KAMACU_BIN>",
      "args": ["mcp", "serve"]
    }
  }
}
```

**Merge, never overwrite** — same rule as above: add only the `kamacu` key to
the existing config file, keep every other server entry, and if the file won't
parse as JSON, stop and report instead of replacing it. Add
`"env": {"KAMACU_HOOK_BASE": "http://127.0.0.1:7333"}` inside the `kamacu`
object only if the app runs on a non-default port.

## 4. Verify the install — run this yourself

A client's "connected" badge only proves the stdio handshake; it does NOT
prove the bridge reaches the app (the handshake succeeds even with the app
down). Call a real tool over stdio:

```sh
KAMACU_BIN=<KAMACU_BIN> sh -c '
( echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-06-18\",\"capabilities\":{},\"clientInfo\":{\"name\":\"install-check\",\"version\":\"0\"}}}"
  echo "{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}"
  echo "{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"list_projects\",\"arguments\":{}}}"
  # hold stdin open (bounded ~15s) so the server can finish replying
  for i in $(seq 1 150); do sleep 0.1; done
) | "$KAMACU_BIN" mcp serve 2>/dev/null | grep -m1 "\"id\":2"'
```

The reader returns as soon as the tool response arrives, not after a fixed
delay. **Success**: the printed line is a JSON-RPC `result` whose
`content[0].text` is a JSON array of projects (possibly `[]`).
**Failure — app down**: the printed line is
`"error":{"code":0,"message":"kamacu bridge: Get \"http://127.0.0.1:7333/api/projects\": dial tcp 127.0.0.1:7333: connect: connection refused"}` — start the app (see
troubleshooting). **Nothing printed at all** means the server process itself
did not answer — re-check the binary path from step 2.2.

Then confirm your client sees it (`claude mcp list` / `opencode mcp list` /
your client's equivalent), and report both results to the user.

## 5. Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| `tools/call` returns `kamacu bridge: ... connect: connection refused` | Kamacu app not running (handshake still succeeds — do not trust the badge) | Start the app: `kamacu serve`, or the boot service from docs/install.md; re-run step 4 |
| Same refused error but with a different port in the URL | App started with non-default `--addr` | Add `"env": {"KAMACU_HOOK_BASE": "http://127.0.0.1:<PORT>"}` to the server config |
| Client fails to spawn the server; config shows a path with `~` or a dead path | Config holds an unexpanded or stale binary path | Re-register with the fully resolved absolute path (step 2.2) |
| `claude mcp list` warns `kamacu is defined in multiple scopes with different endpoints` | An old install (e.g. a previous checkout) left an entry in another scope | Keep one: `claude mcp remove kamacu -s <scope>` for the stale ones |
| An `opencode mcp add kamacu` run replaced a working entry without warning | `opencode mcp add` silently overwrites an existing `kamacu` entry | Check first (step 3 pre-check); restore the previous entry from the config if you clobbered it |
| Server entry missing from the client after "successful" add | Added in `local` scope from a different directory | Re-add with `--scope user`, or re-run from the right project directory |

## Feedback inbox

Task agents also receive `KAMACU_TASK_ID` and `KAMACU_SESSION_ID`. Keep these
inherited from the launching process; do not hard-code them in MCP configuration.
If your client filters subprocess environment variables, allow all four
`KAMACU_*` names listed above. Restart existing task agents after upgrading so
they receive the task identity. See [the inbox protocol](inbox.md) for question
creation, polling, reporting terminal answers, and recovery.

## HTML artifacts

Artifacts are interactive previews attached to project tasks. Each appears as a
named tab after Diff. Users can double-click a tab title to rename it or use its
remove control. Agents can create, list, and replace content; replacements keep
the artifact ID and any user-edited title.

Package one of:

- A `.html` file containing its CSS, JavaScript, and images inline.
- A `.tar.gz` containing `index.html` at the root plus local supporting files.
  Use relative URLs for those files. For example:

  ```sh
  tar -czf preview.tar.gz -C preview index.html assets
  ```

The limit is 20,000,000 uploaded bytes, 100,000,000 extracted bytes, 1,000 files,
and 20 published artifacts per task. Symlinks, hardlinks, unsafe paths, duplicate
paths, special files, and archives without a root `index.html` are rejected.
Directories and metadata have separate bounds to limit decompression overhead.

Use this protocol from either a local or remote agent:

1. Call `begin_artifact_upload` with `task_id`, `title`, `filename`, and the exact
   `size_bytes`. For replacement, call `begin_artifact_replacement` with
   `task_id`, `artifact_id`, `filename`, and `size_bytes`.
2. The result supplies `upload_id`, an absolute `upload_url`, `method: "PUT"`,
   required `headers`, and Unix `expires_at` (15 minutes). Upload raw file bytes
   with that method and those headers. With curl, use `--data-binary @file`, not
   multipart form upload. The local filename is never sent as a server path.
3. After HTTP 204, call `finalize_artifact_upload` with `task_id` and `upload_id`.
   Validation and publishing happen here; the new tab appears only on success.
   Retry successful finalization safely before the upload session expires. Conflicting replacements require
   beginning a new upload against the latest revision.
4. Use `list_artifacts(task_id)` to retrieve published IDs, titles, and revisions.

Keep the upload capability private. It permits writing only that pending
upload; it does not authorize finalization or other API operations. Interrupted
transfers can retry before expiry. Completed transfers are immutable. Failed
validation leaves the previous published artifact intact.

Previews run JavaScript in an opaque-origin sandbox. Browser storage, access to
Kamacu's DOM/session, forms, popups, workers, and nested frames are disabled.
CSP restricts scripts, styles, images, fonts, media, and connections to the
artifact's uploaded revision (plus embedded image/media data). CDN resources
and external API/WebSocket calls are blocked; bundle dependencies locally.
Do not depend on service workers, browser storage, or evaluation of strings as
JavaScript.

Connection Allowlists are disabled by default for browser compatibility. To
enable them, set the environment variable when starting Kamacu:

```sh
KAMACU_ARTIFACT_CONNECTION_ALLOWLISTS=1 kamacu serve
```

With this option enabled, interactive previews require native **Connection
Allowlists** support in the viewer's browser. A trusted renderer checks native
WebRTC blocking before parsing uploaded HTML; unsupported browsers show an
error and never execute uploaded code. The native policy additionally blocks
external navigation, redirects, and WebRTC/STUN. With the option disabled,
previews still use the CSP sandbox and resource restrictions described above,
but CSP alone cannot contain WebRTC or all external navigation.
Non-HTML assets remain usable as subresources but download on direct navigation.
Restart Kamacu after changing the variable.

See the [Connection Allowlists design](https://wicg.github.io/connection-allowlists/)
and [Chrome's implementation guidance](https://developer.chrome.com/blog/connection-allowlists-origin-trial).

Preview availability is checked before mounting the iframe. HTTP failures,
connection failures, unsupported browsers, and missing renderer readiness are
reported in the task UI. Retry rechecks availability and remounts the preview,
even if its revision has not changed.

New tabs do not steal focus. Visited previews stay mounted across tab switches.
When content changes, the viewer shows “Updated” with a Reload action; accepting
it resets interaction state. Reloading the task also resets state. Files survive
worktree cleanup and restart, and are removed when their artifact, task, or
project is deleted. Storage lives in `<database path>.artifacts`; orphaned files
are collected at startup and once per minute. Previous immutable revisions remain available until deletion
so an open old preview can still load its assets.

Scratchpad agents use `KAMACU_TASK_ID=0`. Pass `task_id: 0` to the artifact tools
to publish, list, or replace Scratchpad previews. They appear as tabs in Scratchpad
with the same sandbox, limits, rename, and removal controls as task artifacts.
