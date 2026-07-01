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

// Fixed migration endpoints — the default-path literals the gate keys on (D-13).
// A custom --db (resolvedDBPath != ExpandHome(defaultDBFlag)) opts out entirely.
const (
	oldDataDir    = "~/.kangent"          // source data root
	newDataDir    = "~/.kamacu"           // destination data root
	oldDBName     = "kangent.db"          // pre-rename DB file basename
	newDBName     = "kamacu.db"           // post-rename DB file basename
	defaultDBFlag = "~/.kamacu/kamacu.db" // the flipped --db default (D-12); main.go (21-04) passes this
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
func (d Decision) String() string { return "" } // RED stub — implemented in GREEN

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
func Gate(customDB, srcExists, dstExists bool) Decision { return FreshInstall } // RED stub — implemented in GREEN
