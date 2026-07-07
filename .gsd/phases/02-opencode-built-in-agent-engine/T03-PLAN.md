---
estimated_steps: 10
estimated_files: 4
skills_used: []
---

# T03: Ship + idempotently install the env-gated opencode status plugin (internal/opencode leaf)

WHY: opencode loads status plugins from static files in ~/.config/opencode/plugin/ (verified empirically: the dir is SINGULAR `plugin/`, NOT `plugins/` as D013's prose said — the existing reference plugin slayzone-notify.js lives there). Unlike claude (hook via --settings argv), opencode can't receive a per-instance hook command via argv — the plugin must be a FILE on disk. So Kamacu ships an env-gated plugin that no-ops for non-Kamacu opencode runs and curls the hook receiver with claude-compatible event names (D013/D014). This on-disk counterpart is what makes "activity-based status" actually work.

DO:
1. New leaf package internal/opencode/ (D009). Two files:
   a. internal/opencode/kamacu-status.js — the plugin source (a real .js file so it uses template literals naturally). Read the reference plugin at /home/jordi/.config/opencode/plugin/slayzone-notify.js for the EXACT opencode plugin API shape (`export const Plugin = async ({ client }) => ({ event: async ({event})=>{...}, 'permission.ask': async (permission, output)=>{...} })`, client.session.list() for child detection, event.properties.sessionID / info.parentID). The kamacu plugin MUST: (i) singleton guard + env gate `if (globalThis.__kamacuOpencodeStatusV1) return {}; ...; const sid = process?.env?.KAMACU_SESSION_ID; if (!sid) return {};` (no-op outside Kamacu PTYs); (ii) read `base=process.env.KAMACU_HOOK_BASE||'http://127.0.0.1:7333'` and `token=process.env.KAMACU_HOOK_TOKEN||''`, build `url=base+'/api/hooks/sessions/'+sid`; (iii) `notify(eventName)` = global `fetch(url,{method:'POST',headers:{'X-Kangent-Token':token,'Content-Type':'application/json'},body:JSON.stringify({hook_event_name:eventName})})` wrapped in try/catch (hook failures NEVER bubble into the opencode TUI), with a 3s AbortController timeout mirroring claude's `-m 3`; (iv) child-session suppression — copy the reference plugin's isChildSession/rootSessionID/stopSent/childSessionCache logic VERBATIM (a subagent child session's idle must NOT falsely emit Stop and idle the root); (v) event mapping emitting ONLY claude-compatible names hooks.go handles: session.created(non-child)→'SessionStart'; session.status type=='idle' OR session.idle OR session.error→'Stop'; session.status type=='busy'/session.busy→emit NOTHING (working is driven by PTY output via noteAgentOutputLocked); 'permission.ask' with output.status==='ask'→'Notification' (NOT 'PermissionRequest'). File header: a `// kamacu-managed` marker line + purpose + env-gate note.
   b. internal/opencode/plugin.go — `//go:embed kamacu-status.js` + `var pluginSource []byte`; `func PluginSource() []byte`; `func InstallPlugin(opencodeConfigDir string) error` (if dir doesn't exist → return nil, SKIP, never create ~/.config/opencode for non-opencode users; else MkdirAll <dir>/plugin 0o755 and write <dir>/plugin/kamacu-status.js IF AND ONLY IF it does not already exist — write-if-absent, D006 conservative, never clobber a user edit; 0o644; return real I/O errors); `func ConfigDir() (string, error)` = os.UserConfigDir() joined with "opencode".
2. Wire in cmd/kamacu/main.go: after the BackfillOpenCodeAgent call (T01), resolve the dir and install non-fatally: `if dir, err := opencode.ConfigDir(); err == nil { if err := opencode.InstallPlugin(dir); err != nil { slog.Warn("installing opencode status plugin", "error", err) } }`. A plugin write failure must NEVER stop the server (D005). Add the import.
3. New file internal/opencode/plugin_test.go: TestPluginSourceContent (assert PluginSource() contains `KAMACU_SESSION_ID`, `SessionStart`, `Stop`, `Notification`, the `kamacu-managed` marker, and does NOT contain `'PermissionRequest'`); TestInstallPluginWritesWhenAbsent (temp dir WITH an existing opencode/ subdir → plugin/kamacu-status.js created == PluginSource()); TestInstallPluginSkipsWhenConfigDirMissing (non-existent dir → nil + nothing created); TestInstallPluginIdempotentNoClobber (pre-write a different sentinel file → InstallPlugin leaves it unchanged).

CONSTRAINTS: zero new third-party Go deps (embed + the opencode runtime's fetch, not a Go dep); do NOT depend on curl on PATH; never touch opencode.json or other opencode files.

DONE WHEN: PluginSource() is content-correct; InstallPlugin writes into an existing opencode config dir, skips silently when there's no opencode, never clobbers an existing file; main.go wires it non-fatally; `go build ./...` compiles. The live opencode→plugin→hook loop itself is deferred to UAT (needs opencode installed) — this task proves the contract + install mechanics.

Skills: go, javascript, opencode-plugin-api.

## Inputs

- `cmd/kamacu/main.go`

## Expected Output

- `internal/opencode/kamacu-status.js`
- `internal/opencode/plugin.go`
- `internal/opencode/plugin_test.go`

## Verification

go test ./internal/opencode/ -count=1 && go build ./...
