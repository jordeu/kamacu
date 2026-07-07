// kamacu-managed — DO NOT EDIT BY HAND.
//
// Kamacu opencode status plugin (M002, D013/D014).
//
// This file is regenerated at every Kamacu start by internal/opencode.InstallPlugin
// into ${XDG_CONFIG_HOME:-~/.config}/opencode/plugin/kamacu-status.js (note: opencode
// loads plugins from the SINGULAR `plugin/` dir). Edits here are overwritten on the
// next boot; it is managed by Kamacu the same way ~/.kamacu/kamacu-tmux.conf is.
//
// Purpose: forward opencode session-lifecycle events to Kamacu's UNCHANGED
// Claude-compatible hook receiver (internal/api/hooks.go, POST
// /api/hooks/sessions/{id}) so an opencode-engine task reports
// working/waiting/idle exactly like a claude-engine task. opencode cannot
// receive a per-instance hook command via argv (unlike claude's --settings
// overlay), so this on-disk plugin is the counterpart that makes
// activity-based status work.
//
// Env gate (D014): the plugin is a complete no-op unless Kamacu spawned the
// opencode process — Kamacu's manager.Spawn injects KAMACU_SESSION_ID,
// KAMACU_HOOK_TOKEN, KAMACU_HOOK_BASE only for engine=="opencode". A plain
// `opencode` run (no Kamacu) loads the file but returns {} immediately, so it
// never touches the network and is invisible to non-Kamacu users.
//
// Event mapping (claude-compatible hook_event_name values):
//   opencode event              -> hook_event_name -> Kamacu effect
//   session.created (non-child) -> SessionStart    -> MarkHooksAlive (BEL fallback off)
//   permission.ask status=ask   -> Notification    -> SetWaiting
//   session.status idle         -> Stop            -> SetIdle (turn end)
//   session.idle / session.error-> Stop            -> SetIdle (legacy/forward-compat)
//   session.status busy         -> (no hook)       -> "working" is the activity heuristic
//
// "busy" emits NO hook on purpose: Kamacu's agentStatusLocked marks a session
// "working" from PTY output activity (D-47). The hooks only carry the three
// transitions the receiver switches on, matching claude's overlay byte-for-byte
// in contract.
//
// Subagent (child session) events are suppressed — opencode tools spawn many
// child sessions whose idle would otherwise flip the root task to idle early.

export const KamacuStatusPlugin = async ({ $, client }) => {
  // Singleton guard: opencode may re-import the plugin; load it once.
  if (globalThis.__kamacuOpencodePluginV1) return {}
  globalThis.__kamacuOpencodePluginV1 = true

  // D014 env gate. Missing any of the three => not a Kamacu spawn => no-op.
  const sessionID = process?.env?.KAMACU_SESSION_ID
  const token = process?.env?.KAMACU_HOOK_TOKEN
  const baseURL = process?.env?.KAMACU_HOOK_BASE
  if (!sessionID || !token || !baseURL) return {}

  const url = baseURL.replace(/\/+$/, '') + '/api/hooks/sessions/' + sessionID
  const header = 'X-Kangent-Token: ' + token

  // Root (non-child) session we attribute events to. The first non-child
  // session.created pins it; child/subagent events are ignored after that.
  let rootSessionID = null
  // Dedup: emit at most one Stop per busy->idle transition. Re-armed on the
  // next busy so each turn still produces exactly one Stop. Without this,
  // repeated idle events spam the receiver and a second turn's Stop would be
  // dropped (stopSent still true from turn 1).
  let stopSent = false
  const childSessionCache = new Map() // sessionID -> isChild bool

  // POST a claude-compatible hook event. Hook failures MUST NEVER bubble into
  // the opencode TUI (mirrors claude's async overlay + the reference plugin).
  const notify = async (hookEventName) => {
    const payload = JSON.stringify({ hook_event_name: hookEventName })
    try {
      // Reuses the exact proven wire path as claude's overlay (curl -s -m 3).
      // Each ${} is a separate argv element (Bun $), so it is injection-safe.
      await $`curl -s -m 3 -H ${header} --data-binary ${payload} ${url}`
    } catch {
      // curl missing / network down / receiver gone — swallow, never crash.
    }
  }

  const isChildSession = async (id) => {
    if (!id) return true
    if (!client?.session?.list) return true
    if (childSessionCache.has(id)) return childSessionCache.get(id)
    try {
      const sessions = await client.session.list()
      const s = sessions.data?.find((x) => x.id === id)
      const isChild = !!s?.parentID
      childSessionCache.set(id, isChild)
      return isChild
    } catch {
      // On error assume CHILD — safer than emitting a false-positive Stop.
      return true
    }
  }

  const handleBusy = (id) => {
    if (!rootSessionID) rootSessionID = id
    if (id !== rootSessionID) return
    stopSent = false // re-arm: the next idle emits a fresh Stop
  }

  const handleStop = async (id) => {
    if (rootSessionID && id !== rootSessionID) return
    if (!stopSent) {
      stopSent = true
      await notify('Stop')
    }
  }

  return {
    event: async ({ event }) => {
      const id =
        event.properties?.sessionID ?? event.properties?.info?.id ?? null

      if (event.type === 'session.created') {
        const isChild = Boolean(event.properties?.info?.parentID)
        if (id) childSessionCache.set(id, isChild)
        if (!isChild) {
          rootSessionID = id
          stopSent = false
          await notify('SessionStart')
        }
        return
      }

      if (event.type === 'session.deleted') {
        if (id) childSessionCache.delete(id)
        return
      }

      if (await isChildSession(id)) return

      // Modern shape: session.status with nested status.type.
      if (event.type === 'session.status') {
        const status = event.properties?.status
        if (status?.type === 'busy') handleBusy(id)
        else if (status?.type === 'idle') await handleStop(id)
      }
      // Legacy / forward-compat top-level event shapes.
      if (event.type === 'session.busy') handleBusy(id)
      if (event.type === 'session.idle') await handleStop(id)
      if (event.type === 'session.error') await handleStop(id)
    },
    'permission.ask': async (_permission, output) => {
      if (output?.status === 'ask') await notify('Notification')
    },
  }
}
