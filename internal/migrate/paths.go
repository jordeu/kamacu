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
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"

	"kamacu/internal/settings"
)

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
