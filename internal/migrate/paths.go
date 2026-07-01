// This file is Part 2 of the startup migration (MIGRATE-02/MIGRATE-03): the
// post-store.Migrate hook wired into cmd/kamacu/main.go in Plan 21-04. Part 1
// (migrate.go) has already moved the data root and renamed the DB file; the DB
// is now open at ~/.kamacu/kamacu.db but four kinds of state a filename grep
// never surfaces still point at the OLD root:
//
//   - projects.repo_path / tasks.worktree_path absolute path VALUES stored in
//     the DB (rewritten here, managed/under-root only — D-15);
//   - the raw worktree_base setting (rewritten here);
//   - git's two-way worktree link files (repaired here — see worktree repair);
//   - kangent-* tmux_sessions rows for shells retired with the old socket
//     (deleted here so reopened tasks respawn fresh on -L kamacu — D-07/D-08).
//
// Every step SELF-GATES on observable state (RESEARCH Pattern 2): the LIKE
// OldRoot||'/%' gate stops matching once a row is rewritten, repair on a healthy
// tree is a no-op, and the DELETE empties once — so the whole hook is idempotent
// and safe to re-run on the D-04 roll-forward path with no marker file.
package migrate

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"kamacu/internal/settings"
)

// Complete is the exported Part-2 startup hook (MIGRATE-02/MIGRATE-03), called
// from cmd/kamacu/main.go right after store.Migrate (Plan 21-04) for a DoMigrate
// or RollForward decision. It runs the three self-gating steps in order and
// returns the first error: rewrite the DB's stored paths under the old root
// (D-15), repair each managed repo's git worktree link files with the rewritten
// new paths, then delete the retired kangent-* tmux rows (D-07/D-08). Because
// every step is idempotent, Complete is safe to re-run on the D-04 roll-forward
// path (a crash mid-migration finishes on the next boot with no marker file).
func Complete(ctx context.Context, db *sql.DB, cfg Config) error {
	if err := rewriteManagedPaths(db, cfg); err != nil {
		return err
	}
	if err := repairWorktrees(ctx, db, cfg); err != nil {
		return err
	}
	return deleteOldTmuxRows(ctx, db)
}

// rewriteManagedPaths prefix-swaps the DB's stored absolute paths from the old
// data root to the new one (D-15 / MIGRATE-02). ONLY managed projects and
// worktree paths UNDER the old root are touched — a user's external/unmanaged
// repo (managed=0, or living outside ~/.kangent) is provably left alone (mirrors
// projects.go:511 "the managed flag is the SINGLE source of truth; never derive
// it from the path — data-loss hazard"). The rewrite is a pure prefix swap
// (strings.TrimPrefix + filepath.Join), NEVER a global string replace.
//
// Collect-then-update is REQUIRED: the store opens with db.SetMaxOpenConns(1)
// (single-writer discipline, store.go), so the SELECT cursor MUST be closed
// before any UPDATE on the same connection (mirrors BackfillProjectIcons,
// icons.go:184). All SQL uses '?' placeholders — a path is never concatenated
// into the query text.
func rewriteManagedPaths(db *sql.DB, cfg Config) error {
	// projects: managed=1 AND repo_path under the old root (D-15).
	if err := rewritePrefixColumn(db, cfg,
		`SELECT id, repo_path FROM projects WHERE managed=1 AND repo_path LIKE ?`,
		`UPDATE projects SET repo_path = ? WHERE id = ?`); err != nil {
		return err
	}
	// tasks: worktree_path under the old root (NULL/other paths never match).
	if err := rewritePrefixColumn(db, cfg,
		`SELECT id, worktree_path FROM tasks WHERE worktree_path LIKE ?`,
		`UPDATE tasks SET worktree_path = ? WHERE id = ?`); err != nil {
		return err
	}
	// settings.worktree_base: stored RAW ("~/.kangent/worktrees/") on most
	// installs, but handle the expanded form defensively too.
	return rewriteWorktreeBaseSetting(db, cfg)
}

// rewritePrefixColumn runs one collect-then-update prefix swap. selectSQL takes a
// single bound LIKE pattern (cfg.OldRoot+"/%") and returns (id, path) rows;
// updateSQL takes (newPath, id). The new path is computed in Go as
// filepath.Join(cfg.NewRoot, strings.TrimPrefix(old, cfg.OldRoot)) — a prefix
// swap, not a replace, so a repo whose LEAF happens to repeat the old-root name
// is never double-rewritten.
func rewritePrefixColumn(db *sql.DB, cfg Config, selectSQL, updateSQL string) error {
	rows, err := db.Query(selectSQL, cfg.OldRoot+"/%")
	if err != nil {
		return err
	}
	type rewrite struct {
		id      int64
		newPath string
	}
	var rewrites []rewrite
	for rows.Next() {
		var id int64
		var oldPath string
		if err := rows.Scan(&id, &oldPath); err != nil {
			rows.Close()
			return err
		}
		newPath := filepath.Join(cfg.NewRoot, strings.TrimPrefix(oldPath, cfg.OldRoot))
		rewrites = append(rewrites, rewrite{id: id, newPath: newPath})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close() // close the cursor BEFORE any UPDATE (SetMaxOpenConns(1))

	for _, rw := range rewrites {
		if _, err := db.Exec(updateSQL, rw.newPath, rw.id); err != nil {
			return err
		}
	}
	return nil
}

// rewriteWorktreeBaseSetting rewrites the worktree_base settings row's prefix
// from the old root to the new one. It handles BOTH the raw "~/.kangent" form
// (how the default is stored, settings.go) and the expanded cfg.OldRoot form,
// preserving the exact suffix (incl. the trailing slash) via string prefix swap.
// A missing row (the live install has none) or a value under neither old root is
// a clean no-op — self-gating for the roll-forward path.
func rewriteWorktreeBaseSetting(db *sql.DB, cfg Config) error {
	var value string
	err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, settings.KeyWorktreeBase).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	var newValue string
	switch {
	case strings.HasPrefix(value, oldDataDir): // raw "~/.kangent/..." form
		newValue = newDataDir + strings.TrimPrefix(value, oldDataDir)
	case strings.HasPrefix(value, cfg.OldRoot): // expanded absolute form
		newValue = cfg.NewRoot + strings.TrimPrefix(value, cfg.OldRoot)
	default:
		return nil // under neither old root — leave untouched
	}
	if newValue == value {
		return nil
	}
	_, err = db.Exec(`UPDATE settings SET value = ? WHERE key = ?`, newValue, settings.KeyWorktreeBase)
	return err
}

// deleteOldTmuxRows removes every retired kangent-* tmux_sessions row (D-07/D-08)
// so a reopened task respawns a fresh shell on the new -L kamacu socket with
// Bash-1 numbering instead of reattaching to a dead kangent-* name. The old
// socket's server itself is killed in Part 1 (retireTmux); this clears the DB
// side. Mirrors the reaper's DELETE FROM tmux_sessions idiom (reaper.go:335).
// Self-gates: once the rows are gone, a re-run affects nothing.
func deleteOldTmuxRows(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE FROM tmux_sessions WHERE name LIKE 'kangent-%'`)
	return err
}

// repairWorktrees fixes git's two-way worktree link files for every managed repo
// after the root moved (MIGRATE-02). RESEARCH Pitfall 2 is load-bearing: a bare
// `git worktree repair` is a NO-OP when both the main repo and its linked
// worktrees move together (our exact case), so the NEW worktree path is passed
// explicitly; and a stale (deleted) path makes git exit 1, so each path is
// os.Stat-filtered first (projects.go:576 idiom) — git never sees a missing path.
//
// Repair is PER PATH, never one batch invocation (Gap 1, 21-VERIFICATION.md): a
// DB-referenced worktree dir can EXIST on disk yet be UNREGISTERED in the repo's
// .git/worktrees/ (the os.Stat filter misses this — it only drops non-existent
// paths). `git worktree repair <that path>` then exits 1 ("does not reference a
// repository"); in one batch call that single straggler would fail the whole
// invocation and abort the migration (the real sched repo's 4 stale dirs → the
// app refused to boot on every RollForward). Per-path, a nonzero exit is logged
// via a terminal-visible slog.Warn (D-11) and SKIPPED, so Complete still proceeds
// to deleteOldTmuxRows (MIGRATE-03) — a stale/unopenable dir is Phase-23 cleanup's
// job, and the skip is non-destructive.
//
// Runs AFTER rewriteManagedPaths, so the DB already holds new-root paths. All git
// invocations use an arg-array exec (worktree.go invariant / ASVS V5), never a
// shell — task-derived paths never reach a shell interpreter. Repairing a healthy
// tree is a clean no-op, so this is idempotent for the D-04 roll-forward path.
//
// Cursor discipline (SetMaxOpenConns(1)): the managed repos are collected and the
// SELECT cursor closed BEFORE the per-repo task queries run, so at most one DB
// cursor is ever open at a time.
func repairWorktrees(ctx context.Context, db *sql.DB, cfg Config) error {
	rows, err := db.Query(
		`SELECT id, repo_path FROM projects WHERE managed=1 AND repo_path LIKE ?`,
		cfg.NewRoot+"/%")
	if err != nil {
		return err
	}
	type repo struct {
		id       int64
		repoPath string
	}
	var repos []repo
	for rows.Next() {
		var r repo
		if err := rows.Scan(&r.id, &r.repoPath); err != nil {
			rows.Close()
			return err
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close() // close BEFORE the per-repo task queries (single-writer)

	for _, r := range repos {
		paths, err := existingWorktreePaths(db, r.id)
		if err != nil {
			return err
		}
		// Repair PER PATH, not one batch invocation: an individual worktree dir
		// may exist on disk + in the DB yet be UNREGISTERED in .git/worktrees/
		// (the real sched repo's 4 stale dirs — Gap 1). `git worktree repair
		// <that path>` exits 1 ("does not reference a repository"). A stale
		// straggler must never abort the whole migration, so on a nonzero exit we
		// log a terminal-visible slog.Warn (D-11) and skip it, letting Complete
		// proceed to deleteOldTmuxRows (MIGRATE-03) so the app boots. A valid
		// moved worktree still repairs (RESEARCH D) and a healthy tree is a no-op
		// (RESEARCH E), so per-path repair keeps D-04 roll-forward idempotent.
		for _, p := range paths {
			cmd := exec.CommandContext(ctx, "git", "-C", r.repoPath, "worktree", "repair", p)
			var errb bytes.Buffer
			cmd.Stderr = &errb
			if err := cmd.Run(); err != nil {
				msg := strings.TrimSpace(errb.String())
				if msg == "" {
					msg = err.Error()
				}
				slog.Warn("migrate: skipping unrepairable worktree (stale/unregistered)",
					"repo", r.repoPath, "worktree", p, "error", msg)
				continue
			}
		}
	}
	return nil
}

// existingWorktreePaths returns the project's non-empty tasks.worktree_path values
// that still exist on disk (os.Stat filter). Skipping vanished trees keeps git
// from exiting 1 on a stale path (RESEARCH Pitfall 2/G). The cursor is fully
// drained and closed before returning so the caller may issue the git exec (and
// the next project's query) on the single connection.
func existingWorktreePaths(db *sql.DB, projectID int64) ([]string, error) {
	rows, err := db.Query(
		`SELECT worktree_path FROM tasks
		 WHERE project_id = ? AND worktree_path IS NOT NULL AND worktree_path != ''`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		if _, statErr := os.Stat(p); statErr == nil {
			paths = append(paths, p)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return paths, nil
}
