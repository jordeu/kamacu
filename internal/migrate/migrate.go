// Package migrate implements the one-time, gated, idempotent, failure-safe
// on-disk migration of the data directory ~/.kangent -> ~/.kamacu run at startup
// after the Phase 20 Kamacu rebrand (MIGRATE-01..05).
//
// Three load-bearing constraints from 21-RESEARCH.md shape the design:
//   - the os.Rename of the data root is the single commit point (D-03); every
//     check that can refuse the migration runs BEFORE it (D-02), so a preflight
//     failure leaves ~/.kangent byte-for-byte untouched (MIGRATE-05);
//   - a bare kangent.db -> kamacu.db rename with a hot WAL destroys every
//     committed-but-uncheckpointed row, so the WAL is folded with
//     PRAGMA wal_checkpoint(TRUNCATE) before the single-file rename (Pitfall 1);
//   - a cross-device (EXDEV) layout or the both-dirs-present anomaly must refuse
//     to boot rather than guess which directory is authoritative (D-11).
//
// This file is Part 1 (the pre-DB startup one-shot). Nothing imports it yet; it
// is wired into cmd/kamacu/main.go in Plan 21-04.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"kamacu/internal/settings"
	"kamacu/internal/store"
	"kamacu/internal/tmux"
)

// Fixed migration endpoints — the default-path literals the gate keys on (D-13).
// A custom --db (resolvedDBPath != ExpandHome(defaultDBFlag)) opts out entirely.
const (
	oldDataDir    = "~/.kangent"          // source data root
	newDataDir    = "~/.kamacu"           // destination data root
	oldDBName     = "kangent.db"          // pre-rename DB file basename
	newDBName     = "kamacu.db"           // post-rename DB file basename
	defaultDBFlag = "~/.kamacu/kamacu.db" // the flipped --db default (D-12); main.go (21-04) passes this

	// oldTmuxSocket is the LITERAL retired tmux socket (-L) name (D-05/D-08). It
	// is intentionally NOT tmux.DefaultSocket: Plan 21-04 flips that constant to
	// "kamacu", but this migration must always retire the OLD "kangent" server.
	oldTmuxSocket = "kangent"
)

// Decision is the outcome of the pure startup Gate: exactly one of five states
// derived from three observable booleans (RESEARCH Pattern 1, D-13/D-04/D-11).
type Decision int

const (
	// SkipCustom: the user overrode --db to a non-default location; a bespoke
	// layout is never touched (D-13 opt-out).
	SkipCustom Decision = iota
	// DoMigrate: default paths, ~/.kangent present and ~/.kamacu absent — run the
	// full migration (dir rename + WAL-safe DB rename + tmux retire).
	DoMigrate
	// RollForward: ~/.kangent gone and ~/.kamacu present — already migrated, or a
	// crash left remaining idempotent steps to finish (D-04).
	RollForward
	// FreshInstall: neither dir present — a new install; store.Open creates
	// ~/.kamacu fresh and there is nothing to migrate.
	FreshInstall
	// RefuseBoot: BOTH dirs present on default paths — an ambiguous anomaly; do
	// not guess which is authoritative, refuse to boot (D-11).
	RefuseBoot
)

// String returns a stable human label for each Decision (used in slog lines).
func (d Decision) String() string {
	switch d {
	case SkipCustom:
		return "SkipCustom"
	case DoMigrate:
		return "DoMigrate"
	case RollForward:
		return "RollForward"
	case FreshInstall:
		return "FreshInstall"
	case RefuseBoot:
		return "RefuseBoot"
	default:
		return "Decision(unknown)"
	}
}

// Config carries the resolved (~-expanded, absolute) migration endpoints.
type Config struct {
	OldRoot  string // expanded ~/.kangent
	NewRoot  string // expanded ~/.kamacu
	CustomDB bool   // true when --db was overridden (SkipCustom)
}

// Gate is the pure 5-branch decision function (RESEARCH Pattern 1). It performs
// NO I/O: callers stat the dirs and detect a custom --db, then Gate maps the
// three booleans to exactly one Decision. Purity makes every branch — including
// the RefuseBoot anomaly — trivially table-testable.
func Gate(customDB, srcExists, dstExists bool) Decision {
	switch {
	case customDB:
		return SkipCustom
	case srcExists && !dstExists:
		return DoMigrate
	case !srcExists && dstExists:
		return RollForward
	case !srcExists && !dstExists:
		return FreshInstall
	default: // srcExists && dstExists
		return RefuseBoot
	}
}

// sameFilesystemFn is indirected so tests can simulate a cross-device (EXDEV)
// layout without needing two real filesystems.
var sameFilesystemFn = sameFilesystem

// retireTmux kills the old -L kangent tmux server (D-05/D-08), retiring every
// detached shell the previous kangent run left behind in one idempotent call
// (RESEARCH Pattern 4 — agents run as bare PTYs, never under tmux). The LITERAL
// oldTmuxSocket is used deliberately, NEVER tmux.DefaultSocket. KillServer treats
// "no server" (exit 1) as success, so the error is ignored. The now-stale
// kangent-tmux.conf is best-effort removed (it is regenerated at boot, so this is
// cosmetic). It is a package var so unit tests can stub it and never kill a real
// tmux server on the host.
var retireTmux = func(ctx context.Context, root string) {
	old := tmux.Client{Socket: oldTmuxSocket, ConfPath: os.DevNull}
	_ = old.KillServer(ctx)
	_ = os.Remove(filepath.Join(root, "kangent-tmux.conf"))
}

// Prepare runs Part 1 of the startup migration BEFORE store.Open. It resolves
// the endpoints, gates on the default-path invariant (D-13), and for a
// DoMigrate/RollForward decision performs the preflight, the atomic os.Rename
// commit point, the WAL-safe DB file rename, and the tmux retirement. It returns
// the Decision, the resolved Config, and any error. On ANY error at or before
// the rename NOTHING is mutated (MIGRATE-05); the caller turns a non-nil error
// into refuse-to-boot (slog.Error + os.Exit(1), D-11).
//
// resolvedDBPath is the fully-expanded --db value; defaultDBFlag is the raw
// default flag value (~/.kamacu/kamacu.db). A mismatch means the user chose a
// custom layout, so the whole migration is skipped (SkipCustom, D-13).
func Prepare(ctx context.Context, resolvedDBPath, defaultDBFlag string) (Decision, Config, error) {
	oldRoot, err := settings.ExpandHome(oldDataDir)
	if err != nil {
		return RefuseBoot, Config{}, fmt.Errorf("migrate: resolve %s: %w", oldDataDir, err)
	}
	newRoot, err := settings.ExpandHome(newDataDir)
	if err != nil {
		return RefuseBoot, Config{}, fmt.Errorf("migrate: resolve %s: %w", newDataDir, err)
	}
	defDB, err := settings.ExpandHome(defaultDBFlag)
	if err != nil {
		return RefuseBoot, Config{}, fmt.Errorf("migrate: resolve default db flag %s: %w", defaultDBFlag, err)
	}

	cfg := Config{
		OldRoot:  oldRoot,
		NewRoot:  newRoot,
		CustomDB: resolvedDBPath != defDB,
	}
	decision := Gate(cfg.CustomDB, dirExists(oldRoot), dirExists(newRoot))

	switch decision {
	case SkipCustom, FreshInstall:
		return decision, cfg, nil // no mutation

	case RefuseBoot:
		return decision, cfg, fmt.Errorf(
			"migrate: both %s and %s exist on default paths — refusing to boot; "+
				"your data is safe, resolve manually by keeping exactly one data directory",
			oldRoot, newRoot)

	case DoMigrate:
		// Preflight FIRST — any failure leaves the source byte-for-byte untouched
		// (D-02/MIGRATE-05); nothing above has mutated anything yet.
		if err := preflight(oldRoot, newRoot); err != nil {
			return decision, cfg, fmt.Errorf("migrate: preflight failed, source left untouched: %w", err)
		}
		// COMMIT POINT (D-03): atomic same-filesystem move of the whole tree.
		if err := os.Rename(oldRoot, newRoot); err != nil {
			if errors.Is(err, syscall.EXDEV) {
				return decision, cfg, fmt.Errorf(
					"migrate: %s and %s are on different filesystems — refusing to boot; "+
						"cross-device move is out of scope, your data is safe at %s",
					oldRoot, newRoot, oldRoot)
			}
			return decision, cfg, fmt.Errorf("migrate: rename %s -> %s: %w", oldRoot, newRoot, err)
		}
		if err := completeDBRename(newRoot); err != nil {
			return decision, cfg, err
		}
		retireTmux(ctx, newRoot)
		return decision, cfg, nil

	case RollForward:
		// The dir already moved; finish the remaining idempotent steps (D-04).
		if err := completeDBRename(newRoot); err != nil {
			return decision, cfg, err
		}
		retireTmux(ctx, newRoot)
		return decision, cfg, nil

	default:
		return decision, cfg, fmt.Errorf("migrate: unhandled decision %s", decision)
	}
}

// preflight verifies the invariants that MUST hold before the os.Rename commit
// point (D-02): the source exists, the destination is absent, and both live on
// the SAME filesystem (a cross-device rename moves nothing — Pitfall 5). It
// mutates nothing, so any failure leaves ~/.kangent untouched (MIGRATE-05).
func preflight(oldRoot, newRoot string) error {
	if !dirExists(oldRoot) {
		return fmt.Errorf("source %s does not exist", oldRoot)
	}
	if _, err := os.Stat(newRoot); err == nil {
		return fmt.Errorf("destination %s already exists", newRoot)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat destination %s: %w", newRoot, err)
	}
	same, err := sameFilesystemFn(oldRoot, newRoot)
	if err != nil {
		return fmt.Errorf("same-filesystem check: %w", err)
	}
	if !same {
		return fmt.Errorf("%s and %s are on different filesystems (cross-device move unsupported)", oldRoot, newRoot)
	}
	return nil
}

// completeDBRename performs the WAL-safe rename of kangent.db -> kamacu.db under
// root (D-12). It self-gates (RESEARCH Pattern 2): a clean no-op once kamacu.db
// exists, so it is safe to re-run on a RollForward boot. Before the single-file
// rename it folds the hot WAL with PRAGMA wal_checkpoint(TRUNCATE) (Pitfall 1) —
// a bare rename of a DB with a hot WAL strands every uncheckpointed transaction.
func completeDBRename(root string) error {
	oldDB := filepath.Join(root, oldDBName)
	newDB := filepath.Join(root, newDBName)

	if !fileExists(oldDB) {
		return nil // already renamed (or never existed) — nothing to do
	}
	if fileExists(newDB) {
		return fmt.Errorf("migrate: both %s and %s exist — refusing to overwrite", oldDB, newDB)
	}

	// Fold the WAL into the main file so the single-file rename strands no data.
	db, err := store.Open(oldDB)
	if err != nil {
		return fmt.Errorf("migrate: open %s for checkpoint: %w", oldDB, err)
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		db.Close()
		return fmt.Errorf("migrate: wal_checkpoint(TRUNCATE) on %s: %w", oldDB, err)
	}
	if err := db.Close(); err != nil { // a clean close removes -wal/-shm
		return fmt.Errorf("migrate: close %s after checkpoint: %w", oldDB, err)
	}

	if err := os.Rename(oldDB, newDB); err != nil {
		return fmt.Errorf("migrate: rename %s -> %s: %w", oldDB, newDB, err)
	}
	// Defensive: a clean close already removed these; drop any straggler.
	_ = os.Remove(oldDB + "-wal")
	_ = os.Remove(oldDB + "-shm")
	return nil
}

// sameFilesystem reports whether src and the PARENT of target share a device.
// target's parent is stat-ed (not target) because target does not exist yet at
// preflight time. Comparing syscall.Stat_t.Dev is the portable pre-rename
// cross-device detector (RESEARCH Code Examples, Pitfall 5).
func sameFilesystem(src, target string) (bool, error) {
	si, err := os.Stat(src)
	if err != nil {
		return false, err
	}
	ti, err := os.Stat(filepath.Dir(target))
	if err != nil {
		return false, err
	}
	ss, ok1 := si.Sys().(*syscall.Stat_t)
	ts, ok2 := ti.Sys().(*syscall.Stat_t)
	if !ok1 || !ok2 {
		return false, errors.New("migrate: same-filesystem check unsupported on this platform")
	}
	return ss.Dev == ts.Dev, nil
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// fileExists reports whether path exists and is a regular (non-directory) file.
func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}
