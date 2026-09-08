# Installing the Kamacu MCP server

This document is written to be executed by a coding agent, not read as prose. If
you are an agent and were pointed here, follow the steps in order, run the
verification in step 4 yourself, and report the result. Substitute real values
wherever you see `<ANGLE-BRACKET-PLACEHOLDERS>`.

## 1. What you are installing

`kamacu mcp serve` is a stdio MCP server that bridges to the JSON API of the
running Kamacu app, giving you ~30 tools to drive Kamacu boards programmatically
(`list_projects`, `create_task`, `move_task`, `start_task_agent`,
`post_pr_review`, …).

**Hard prerequisite: the Kamacu app must already be running on this machine**
(someone started `kamacu serve`). The MCP server is only a bridge — without the
app it starts fine but every tool call fails (see step 4).

## 2. Preflight — run these first

### 2.1 Is the Kamacu app up?

```sh
curl -fsS http://127.0.0.1:7333/api/projects -o /dev/null -w '%{http_code}\n'
```

- `200` — the app is up. Continue.
- `connection refused` — the app is NOT running. Do not install yet: tell the
  user to start it (`kamacu serve` from a Kamacu checkout) and stop here. An
  install done now will look fine but fail on every tool call.

### 2.2 Locate or build the binary

The binary is NOT installed to `PATH` by default. Resolve an absolute path:

```sh
command -v kamacu || echo NOT_ON_PATH
```

- If a path prints, use it (absolute — run `command -v` output through
  `readlink -f` if it is relative).
- If `NOT_ON_PATH`, clone and build. Requires `git` and Go. Does NOT require
  Node (the MCP bridge needs no frontend):

  ```sh
  git clone https://github.com/jordeu/kamacu.git "$HOME/.kamacu/mcp/kamacu"
  make -C "$HOME/.kamacu/mcp/kamacu" backend   # → bin/kamacu
  ```

  The binary is now at `$HOME/.kamacu/mcp/kamacu/bin/kamacu`.

From here on, `<KAMACU_BIN>` means that absolute path with `$HOME` already
expanded (e.g. `/home/you/.kamacu/mcp/kamacu/bin/kamacu`). **MCP client configs
do not expand `~` or `$HOME` — always write the fully resolved absolute path.**

### 2.3 Environment variables — usually none needed

Agents spawned *inside* Kamacu receive `KAMACU_HOOK_BASE` and
`KAMACU_HOOK_TOKEN` injected. You, installing from a plain terminal, need
**neither**:

- `KAMACU_HOOK_BASE` defaults to `http://127.0.0.1:7333`, which is where the
  app listens by default. Set it only if the user started the app with a
  non-default `--addr`.
- `KAMACU_HOOK_TOKEN` is not required: the API ignores it on `/api/*`; the real
  boundary is that the app binds to loopback only.

## 3. Register the server with your client

Pick exactly one section — yours.

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

Verify with `opencode mcp list` (expect `✓ kamacu connected`). There is no
remove subcommand — to uninstall, delete the `kamacu` key from the JSON file.

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

Add `"env": {"KAMACU_HOOK_BASE": "http://127.0.0.1:7333"}` only if the app
runs on a non-default port.

## 4. Verify the install — run this yourself

A client's "connected" badge only proves the stdio handshake; it does NOT
prove the bridge reaches the app (the handshake succeeds even with the app
down). Call a real tool over stdio:

```sh
KAMACU_BIN=<KAMACU_BIN> sh -c '
(echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-06-18\",\"capabilities\":{},\"clientInfo\":{\"name\":\"install-check\",\"version\":\"0\"}}}"
 sleep 0.5
 echo "{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}"
 sleep 0.3
 echo "{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"list_projects\",\"arguments\":{}}}"
 sleep 1) | "$KAMACU_BIN" mcp serve 2>/dev/null | tail -n 1'
```

- **Success**: the last line is a JSON-RPC `result` whose `content[0].text` is
  a JSON array of projects (possibly `[]`).
- **Failure — app down**: the last line is
  `"error":{"code":0,"message":"kamacu bridge: Get \"http://127.0.0.1:7333/api/projects\": dial tcp 127.0.0.1:7333: connect: connection refused"}`.
  The app is not running — see troubleshooting.

Then confirm your client sees it (`claude mcp list` / `opencode mcp list` /
your client's equivalent), and report both results to the user.

## 5. Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| `tools/call` returns `kamacu bridge: ... connect: connection refused` | Kamacu app not running (handshake still succeeds — do not trust the badge) | Start the app: `kamacu serve` in a Kamacu checkout; re-run step 4 |
| Same refused error but with a different port in the URL | App started with non-default `--addr` | Add `"env": {"KAMACU_HOOK_BASE": "http://127.0.0.1:<PORT>"}` to the server config |
| Client fails to spawn the server; config shows a path with `~` or a dead path | Config holds an unexpanded or stale binary path | Re-register with the fully resolved absolute path (step 2.2) |
| `claude mcp list` warns `kamacu is defined in multiple scopes with different endpoints` | An old install (e.g. a previous checkout) left an entry in another scope | Keep one: `claude mcp remove kamacu -s <scope>` for the stale ones |
| Server entry missing from the client after "successful" add | Added in `local` scope from a different directory | Re-add with `--scope user`, or re-run from the right project directory |
