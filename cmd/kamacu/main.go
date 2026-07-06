package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"kamacu/internal/api"
	"kamacu/internal/github"
	"kamacu/internal/migrate"
	"kamacu/internal/quota"
	"kamacu/internal/reaper"
	"kamacu/internal/session"
	"kamacu/internal/settings"
	"kamacu/internal/store"
	"kamacu/internal/tmux"
	"kamacu/internal/worktree"
	"kamacu/internal/ws"
	"kamacu/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7333", "listen address (localhost-only by design)")
	dbFlag := flag.String("db", "~/.kamacu/kamacu.db", "path to SQLite database file")
	claudeBin := flag.String("claude-bin", "", "path to the claude binary (default: resolve \"claude\" on PATH at spawn time)")
	var devOrigins []string
	flag.Func("dev-origin", "additional allowed Origin host:port for the Vite dev server (repeatable, e.g. localhost:5173)", func(v string) error {
		devOrigins = append(devOrigins, v)
		return nil
	})
	insecureAllowRemote := flag.Bool("insecure-allow-remote", false, "opt-in: allow binding --addr to a non-loopback address (NO AUTH — anyone who can reach the address gets a shell)")
	flag.Parse()

	// D-21: localhost-only is enforced, not aspirational. The opt-in
	// --insecure-allow-remote escape hatch bypasses the bind gate (and the
	// hostCheck / WS Origin layers below); with the flag absent, behavior is
	// byte-for-byte unchanged.
	if !*insecureAllowRemote {
		if err := ensureLoopback(*addr); err != nil {
			slog.Error("refusing to start", "error", err)
			os.Exit(1)
		}
	} else {
		// SAFE BY DEFAULT is the headline invariant; this is the one loud,
		// unmistakable line announcing the user opted out of it.
		slog.Warn("SECURITY: kamacu is listening with NO authentication via --insecure-allow-remote — anyone who can reach this address gets a shell on this host", "addr", *addr)
	}

	dbPath, err := settings.ExpandHome(*dbFlag)
	if err != nil {
		slog.Error("resolving db path", "path", *dbFlag, "error", err)
		os.Exit(1)
	}

	// Part 1 of the one-time ~/.kangent -> ~/.kamacu data-directory migration
	// (MIGRATE-01/05, D-09..D-12). Ordering is load-bearing: Prepare MUST run
	// BEFORE os.MkdirAll/store.Open below — creating ~/.kamacu early would defeat
	// the "destination absent" gate and make the atomic os.Rename fail (RESEARCH
	// Pitfall 4). It gates on the default paths (a custom --db opts out), performs
	// the preflight + atomic rename + WAL-safe DB rename + old-tmux retire, and on
	// ANY error leaves ~/.kangent byte-for-byte untouched. A migration error or a
	// both-dirs anomaly refuses to boot (D-11) — the data is safe.
	decision, migCfg, err := migrate.Prepare(context.Background(), dbPath, "~/.kamacu/kamacu.db")
	if err != nil {
		slog.Error("migrating data directory ~/.kangent -> ~/.kamacu: refusing to boot (your data is safe — nothing moved past the failure)", "error", err)
		os.Exit(1)
	}
	if decision == migrate.DoMigrate {
		// Silent success is a single line (D-10); fresh installs and already-
		// migrated boots stay quiet.
		slog.Info("migrated ~/.kangent -> ~/.kamacu")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		slog.Error("creating db directory", "dir", filepath.Dir(dbPath), "error", err)
		os.Exit(1)
	}

	db, err := store.Open(dbPath)
	if err != nil {
		slog.Error("opening database", "path", dbPath, "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := store.Migrate(db); err != nil {
		slog.Error("running migrations", "error", err)
		os.Exit(1)
	}

	// Part 2 of the data-directory migration (MIGRATE-02/03): rewrite the DB's
	// managed stored paths under the old root (D-15), repair each managed repo's
	// git worktree link files, and delete the retired kangent-* tmux rows so a
	// reopened task respawns fresh on the -L kamacu socket. It needs the schema,
	// so it MUST run after store.Migrate; only a DoMigrate/RollForward decision
	// has anything to complete (SkipCustom/FreshInstall are no-ops; RefuseBoot
	// already errored in Prepare). Every step self-gates, so a failure refuses to
	// serve (D-04) and the next boot's RollForward path re-runs the idempotent
	// steps — the data stays safe.
	if decision == migrate.DoMigrate || decision == migrate.RollForward {
		if err := migrate.Complete(context.Background(), db, migCfg); err != nil {
			slog.Error("completing data-directory migration ~/.kangent -> ~/.kamacu: refusing to serve (your data is safe — a re-run finishes it)", "error", err)
			os.Exit(1)
		}
	}

	// One-shot idempotent icon backfill (D-08): migration 00009 adds icon_letters
	// + icon_color (NOT NULL DEFAULT ''); this fills every pre-existing blank row
	// with derived letters + a palette color, reusing the SAME helpers the create
	// paths use. Ordering is load-bearing — it MUST run after Migrate (the columns
	// must exist). Cheap no-op on later boots once all rows are filled.
	if err := api.BackfillProjectIcons(db); err != nil {
		slog.Error("backfilling project icons", "error", err)
		os.Exit(1)
	}

	// One-shot idempotent workspace guard (WSDATA-02 / D-07): guarantee a default
	// Personal workspace row always exists, mirroring BackfillProjectIcons. Ordering
	// is load-bearing — it MUST run after store.Migrate (the workspaces table must
	// exist) and sits after the icon backfill to keep the startup one-shots together.
	// Cheap no-op on healthy boots; re-creates Personal only if the default is gone.
	if err := api.BackfillWorkspaces(db); err != nil {
		slog.Error("backfilling workspaces", "error", err)
		os.Exit(1)
	}

	// One-shot idempotent agent guard (M001 AGENTDATA): guarantee a default
	// Claude Code agent row always exists, mirroring BackfillWorkspaces. Ordering
	// is load-bearing -- it MUST run after store.Migrate (the agents table must
	// exist) and sits after the workspace backfill to keep the startup one-shots
	// together. Cheap no-op on healthy boots; re-creates the claude seed only if
	// the default is gone.
	if err := api.BackfillAgents(db); err != nil {
		slog.Error("backfilling agents", "error", err)
		os.Exit(1)
	}
	// M001 gate follow-up: relocate the legacy agent_extra_params global setting
	// onto the Claude agent row (one-shot, idempotent). Runs after BackfillAgents
	// (the seed row must exist). A no-op once the seed carries a value.
	if err := api.BackfillAgentExtraParams(db); err != nil {
		slog.Error("backfilling agent extra params", "error", err)
		os.Exit(1)
	}

	// Kamacu-managed tmux config (D-79 status off, D-80 mouse on), regenerated
	// at every start in the data dir next to the DB — a stable path that survives
	// reboots, so a tmux server started by a previous Kamacu run still references
	// an existing file (research Open Q2).
	tmuxConf := filepath.Join(filepath.Dir(dbPath), "kamacu-tmux.conf")
	if err := tmux.WriteConfig(tmuxConf); err != nil {
		slog.Error("writing tmux config", "path", tmuxConf, "error", err)
		os.Exit(1)
	}

	// Per-instance hook token (STAT-02 / research gap #2): generated fresh on
	// every start, held in memory only — never logged, never persisted. It is
	// embedded solely in the per-spawn settings overlay and checked by the
	// hook receiver, so webpages firing no-CORS POSTs at localhost can never
	// spoof agent status.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		slog.Error("generating hook token", "error", err)
		os.Exit(1)
	}
	hookToken := hex.EncodeToString(tokenBytes)

	// Origin allowlist (D-20): exact loopback origins with the serving port,
	// plus any --dev-origin entries (dev runs need the Vite server's origin:
	// go run ./cmd/kamacu --dev-origin localhost:5173 --dev-origin 127.0.0.1:5173).
	_, port, _ := net.SplitHostPort(*addr)
	originPatterns := append([]string{"127.0.0.1:" + port, "localhost:" + port}, devOrigins...)

	// D-23: worktrees live centrally outside every repo tree. Since Phase 6
	// placement is settings-driven per creation (worktree_base, WT-01) —
	// provisionWorktree reads the setting at use, so this root is only the
	// legacy Service field; the creation path no longer consults it.
	wtRoot, err := settings.ExpandHome("~/.kamacu/worktrees")
	if err != nil {
		slog.Error("resolving worktree root", "error", err)
		os.Exit(1)
	}
	wtSvc := worktree.NewService(wtRoot)

	mgr := session.NewManager()
	mgr.SetAgentConfig(session.AgentConfig{
		// Hooks curl this URL and ALWAYS run on the server host. hookBaseURL
		// normalizes a wildcard --addr host (0.0.0.0 / :: / empty) to
		// 127.0.0.1:<port> so local hooks never target a wildcard/remote
		// address; a specific IP (loopback or, under --insecure-allow-remote,
		// a non-loopback IP) is left as-is.
		BaseURL:   hookBaseURL(*addr),
		Token:     hookToken,
		ClaudeBin: *claudeBin,
	})
	tmuxClient := tmux.Client{Socket: tmux.DefaultSocket, ConfPath: tmuxConf}
	mgr.SetTmuxClient(tmuxClient)

	mux := http.NewServeMux()
	api.Routes(mux, db, wtSvc, mgr, tmuxClient)
	api.SessionRoutes(mux, mgr, db, tmuxClient)
	api.WorktreeRoutes(mux, db, wtSvc, mgr, tmuxClient)
	api.DiffRoutes(mux, db, wtSvc)
	api.HookRoutes(mux, mgr, hookToken)
	api.AgentRoutes(mux, mgr, db)
	quotaSvc := quota.New(quota.Config{ClaudeBin: *claudeBin})
	api.UsageRoutes(mux, quotaSvc)
	ghSvc := github.New(github.Config{})
	api.PullRequestRoutes(mux, db, ghSvc, wtSvc)
	// Worktree-cleanup panel (WTREE-01..04): the 3rd caller of CleanupWorktreeGated.
	// ghSvc is the SAME *github.Service the reaper + PR routes use (PRStateGetter) —
	// no new construction; it drives PR merged/closed eligibility + display and
	// degrades cleanly when gh is absent.
	api.WorktreeCleanupRoutes(mux, db, wtSvc, mgr, tmuxClient, ghSvc)
	mux.Handle("GET /api/sessions/{id}/ws", ws.NewHandler(mgr, originPatterns, *insecureAllowRemote))
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// SPA fallback: serve the embedded Vite build; deep links get index.html,
	// unknown /api/* paths 404 and never serve HTML.
	dist, _ := fs.Sub(web.DistFS, "dist")
	fileServer := http.FileServerFS(dist)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" {
			if f, err := dist.Open(p); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r) // hashed asset → cacheable
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFileFS(w, r, dist, "index.html") // deep links → SPA
	})

	// Startup orphan sweep (D-93): reconcile task-deletes / worktree-removes
	// that happened while Kamacu was down. Synchronous and ONCE — block until
	// done so the server starts in a clean state (NOT periodic: tmux sessions
	// only become orphaned through paths Kamacu already controls, D-99).
	sweepOrphanTmux(context.Background(), db, tmuxClient)

	// Background reaper (REAP-01 + GHCLN-01/02): the app's background goroutine.
	// It runs TWO passes per tick:
	//   1. Done-TTL: kills bash + tmux + agent sessions of tasks left in Done
	//      past the configured done_session_ttl, clocked from done_at, keeping
	//      each agent resumable (claude_session_id/transcript untouched, D-96)
	//      and never touching worktrees (D-87).
	//   2. PR reconcile (13-02): for each source='github_pr' task that still
	//      owns a worktree, read the PR's state via ghSvc (the PRStateGetter);
	//      a MERGED/CLOSED PR with a pristine, idle worktree is auto-removed and
	//      its row deleted (D-07), gated conservatively (dirty/unpushed/stash/
	//      session) and NEVER forced (force=false, stopSessions=false, D-05).
	//      ghSvc is the SAME *github.Service the PR routes use — no new construction.
	// No graceful shutdown exists — process death stops it — so context.Background()
	// is the correct process-lifetime scope. *session.Manager satisfies the
	// reaper's SessionStopper; *github.Service satisfies its PRStateGetter via PRState.
	go reaper.NewWithPR(db, mgr, wtSvc, tmuxClient, ghSvc).Run(context.Background())
	slog.Info("session reaper started (Done-TTL + PR reconcile)")

	slog.Info("kamacu listening", "url", "http://"+*addr)
	// hostCheck (DNS-rebinding defense) wraps the mux by default; the opt-in
	// --insecure-allow-remote path serves the mux directly so non-loopback
	// Host headers are accepted.
	var handler http.Handler = hostCheck(mux)
	if *insecureAllowRemote {
		handler = mux
	}
	if err := http.ListenAndServe(*addr, handler); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// sweepOrphanTmux kills any live kamacu-* tmux session on the dedicated socket
// whose name has no matching tmux_sessions row OR whose task no longer exists
// (D-93). It reconciles deletes/removes that happened while Kamacu was down —
// the kill-before-remove paths in worktrees.go/tasks.go cover the online case,
// and this covers the offline case. Best-effort throughout: a missing/broken
// tmux binary is a no-op (nothing to sweep), and a kill failure is warn-only.
// The branch is never touched (D-34) and worktrees are never removed here
// (D-87) — this kills orphaned shells only.
func sweepOrphanTmux(parent context.Context, db *sql.DB, tmuxClient tmux.Client) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	names, err := tmuxClient.ListSessions(ctx)
	if err != nil {
		// tmux missing/broken — nothing to sweep (degrade, don't break).
		slog.Warn("orphan sweep: listing tmux sessions", "error", err)
		return
	}
	if len(names) == 0 {
		return // no server running -> no sessions
	}

	// Known = a tmux_sessions row whose task STILL exists. The JOIN drops rows
	// whose task was deleted while down, so those sessions get swept too.
	known := make(map[string]bool)
	rows, err := db.QueryContext(ctx,
		`SELECT ts.name FROM tmux_sessions ts JOIN tasks t ON t.id = ts.task_id`)
	if err != nil {
		slog.Warn("orphan sweep: loading known tmux sessions", "error", err)
		return // can't tell orphan from live -> never kill blindly (Pitfall 6)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			slog.Warn("orphan sweep: scanning known name", "error", err)
			continue
		}
		known[name] = true
	}
	if err := rows.Err(); err != nil {
		slog.Warn("orphan sweep: iterating known names", "error", err)
		return
	}
	rows.Close()

	for _, name := range names {
		// Never touch a session Kamacu did not create (belt-and-braces — the
		// dedicated socket should only ever hold kamacu-* sessions).
		if !strings.HasPrefix(name, "kamacu-") {
			continue
		}
		if known[name] {
			continue
		}
		if err := tmuxClient.KillSession(ctx, name); err != nil {
			slog.Warn("orphan sweep: killing orphan tmux session", "name", name, "error", err)
			continue
		}
		slog.Info("swept orphan tmux session", "name", name)
	}
}

// ensureLoopback rejects any listen address that is not loopback (D-21).
// Empty host (":7333" = all interfaces) is refused, and non-"localhost"
// hostnames are refused outright — never resolved — so /etc/hosts games
// cannot widen the bind.
func ensureLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid --addr %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("--addr %q is not a loopback address; kamacu serves a shell and must stay local", addr)
	}
	return nil
}

// hookBaseURL builds the agent-status hook base URL from the listen addr.
// Claude Code hooks curl this URL and ALWAYS run on the server host, so a
// wildcard bind must resolve to loopback: when the host portion is a wildcard
// — empty (":7333"), "0.0.0.0", or "::"/"[::]" — it substitutes "127.0.0.1"
// while preserving the port (the server also listens on loopback when bound to
// a wildcard). A SPECIFIC host (a loopback IP, a hostname, or — under
// --insecure-allow-remote — a reachable non-loopback IP) is left unchanged. On
// a SplitHostPort parse error it falls back to "http://" + addr (the prior
// behavior).
func hookBaseURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return "http://127.0.0.1:" + port
	default:
		return "http://" + addr
	}
}

// hostCheck is middleware wrapping the entire mux, rejecting non-loopback
// Host headers (DNS-rebinding defense). Hostname-only and PORT-AGNOSTIC by
// design: rebinding attacks present an attacker hostname, never a loopback
// name, and port-agnosticism keeps the Vite dev proxy (which forwards
// Host: 127.0.0.1:5173) working.
func hostCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h // strips the port, including the [::1]:7333 bracket form
		}
		switch host {
		case "localhost", "127.0.0.1", "::1", "[::1]":
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden host"}`))
		}
	})
}
