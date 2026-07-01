package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kamacu/internal/settings"
	"kamacu/internal/store"
)

// retireTmuxCalls counts invocations of the (stubbed) tmux retirement so tests
// can assert Prepare wires it WITHOUT killing a real -L kangent tmux server on
// the developer's machine.
var retireTmuxCalls int

// TestMain replaces the real tmux retirement with a counting no-op for the whole
// package test binary (agents/shells on a live host must never be killed by a
// `go test` run) and preserves it afterwards.
func TestMain(m *testing.M) {
	retireTmux = func(ctx context.Context, root string) { retireTmuxCalls++ }
	os.Exit(m.Run())
}

const testDefaultFlag = "~/.kamacu/kamacu.db"

// copyFileTest copies src to dst verbatim (raw bytes) for building crash
// fixtures whose files must not be re-opened/checkpointed first.
func copyFileTest(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

// writeHotWALFixture creates <dir>/kangent.db whose MAIN file holds a
// checkpointed schema + one "clean" project, and whose -wal holds one
// COMMITTED-but-uncheckpointed "hot" project — i.e. a database that was killed
// (Falcon SIGKILL, MEMORY.md) before its WAL could be folded. It builds the DB
// in a scratch dir and raw-copies the files so no clean Close ever checkpoints
// the hot row away, making the fixture a faithful "hot WAL on disk" snapshot.
func writeHotWALFixture(t *testing.T, dir string) {
	t.Helper()
	scratch := t.TempDir()
	scratchDB := filepath.Join(scratch, "kangent.db")

	db, err := store.Open(scratchDB)
	if err != nil {
		t.Fatalf("open scratch db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate scratch db: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO projects (name, repo_path) VALUES ('clean', '/tmp/clean')`); err != nil {
		t.Fatalf("insert clean row: %v", err)
	}
	// Fold schema + clean row into the MAIN file, then disable auto-checkpoint so
	// the next commit stays in the -wal.
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("checkpoint scratch db: %v", err)
	}
	if _, err := db.Exec(`PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatalf("disable autocheckpoint: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO projects (name, repo_path) VALUES ('hot', '/tmp/hot')`); err != nil {
		t.Fatalf("insert hot row: %v", err)
	}
	// Snapshot the raw files BEFORE any Close so the hot -wal is captured.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		src := scratchDB + suffix
		if _, err := os.Stat(src); err != nil {
			continue // -wal/-shm may be absent in some builds
		}
		copyFileTest(t, src, filepath.Join(dir, "kangent.db"+suffix))
	}
	_ = db.Close() // scratch cleanup; folds the hot row into scratch (snapshot already taken)

	// The fixture is only meaningful if the copied -wal actually carries the hot row.
	if fi, err := os.Stat(filepath.Join(dir, "kangent.db-wal")); err != nil || fi.Size() == 0 {
		t.Fatalf("fixture -wal is not hot (size 0 or missing): err=%v", err)
	}
}

// defaultResolvedDB returns the resolved default --db path so Prepare classifies
// the fixture as NON-custom (customDB=false).
func defaultResolvedDB(t *testing.T) string {
	t.Helper()
	p, err := settings.ExpandHome(testDefaultFlag)
	if err != nil {
		t.Fatalf("expand default flag: %v", err)
	}
	return p
}

// TestNaiveDBRenameLosesHotWAL is the GUARD test: it proves that renaming only
// kangent.db -> kamacu.db (orphaning the hot -wal) LOSES the hot row — i.e. the
// checkpoint-first path in completeDBRename is load-bearing, not decorative.
func TestNaiveDBRenameLosesHotWAL(t *testing.T) {
	dir := t.TempDir()
	writeHotWALFixture(t, dir)

	// NAIVE: main file only — exactly what completeDBRename must NOT do.
	if err := os.Rename(filepath.Join(dir, "kangent.db"), filepath.Join(dir, "kamacu.db")); err != nil {
		t.Fatalf("naive rename: %v", err)
	}

	db, err := store.Open(filepath.Join(dir, "kamacu.db"))
	if err != nil {
		t.Fatalf("open kamacu.db: %v", err)
	}
	defer db.Close()
	var hot int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects WHERE name='hot'`).Scan(&hot); err != nil {
		t.Fatalf("count hot: %v", err)
	}
	if hot != 0 {
		t.Fatalf("guard: naive rename unexpectedly preserved the hot row (got %d) — "+
			"the checkpoint-first path cannot be proven load-bearing in this environment", hot)
	}
}

// TestPrepareDoMigratePreservesHotWAL exercises the full DoMigrate path and
// asserts: decision, src gone, dst present, DB file renamed, tmux retired, and —
// critically — the hot-WAL row survives (checkpoint-first, Pitfall 1).
func TestPrepareDoMigratePreservesHotWAL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldRoot := filepath.Join(home, ".kangent")
	newRoot := filepath.Join(home, ".kamacu")
	writeHotWALFixture(t, oldRoot)

	before := retireTmuxCalls
	dec, cfg, err := Prepare(context.Background(), defaultResolvedDB(t), testDefaultFlag)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if dec != DoMigrate {
		t.Fatalf("decision = %v, want DoMigrate", dec)
	}
	if cfg.NewRoot != newRoot || cfg.OldRoot != oldRoot || cfg.CustomDB {
		t.Errorf("cfg = %+v, want {OldRoot:%s NewRoot:%s CustomDB:false}", cfg, oldRoot, newRoot)
	}
	if _, err := os.Stat(oldRoot); !os.IsNotExist(err) {
		t.Errorf("old root should be gone after migrate: err=%v", err)
	}
	if fi, err := os.Stat(newRoot); err != nil || !fi.IsDir() {
		t.Errorf("new root should exist as a dir: err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(newRoot, "kamacu.db")); err != nil {
		t.Errorf("kamacu.db should exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(newRoot, "kangent.db")); !os.IsNotExist(err) {
		t.Errorf("kangent.db should be gone: err=%v", err)
	}
	if retireTmuxCalls != before+1 {
		t.Errorf("retireTmux calls = %d, want %d (Prepare must retire the old tmux server)", retireTmuxCalls, before+1)
	}

	db, err := store.Open(filepath.Join(newRoot, "kamacu.db"))
	if err != nil {
		t.Fatalf("open migrated db: %v", err)
	}
	defer db.Close()
	var total, hot int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&total); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects WHERE name='hot'`).Scan(&hot); err != nil {
		t.Fatalf("count hot: %v", err)
	}
	if total != 2 {
		t.Errorf("projects after migrate = %d, want 2 (clean + hot)", total)
	}
	if hot != 1 {
		t.Errorf("hot-WAL row lost during migrate: got %d, want 1", hot)
	}
}

// TestPrepareRollForwardCompletesDBRename simulates a crash AFTER the dir rename
// but BEFORE the DB-file rename (~/.kamacu present with kangent.db still inside):
// Prepare must finish the DB rename idempotently and a second call is a no-op.
func TestPrepareRollForwardCompletesDBRename(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	newRoot := filepath.Join(home, ".kamacu")
	writeHotWALFixture(t, newRoot) // dst present, kangent.db still there (no ~/.kangent)

	dec, _, err := Prepare(context.Background(), defaultResolvedDB(t), testDefaultFlag)
	if err != nil {
		t.Fatalf("Prepare (roll-forward): %v", err)
	}
	if dec != RollForward {
		t.Fatalf("decision = %v, want RollForward", dec)
	}
	if _, err := os.Stat(filepath.Join(newRoot, "kamacu.db")); err != nil {
		t.Errorf("kamacu.db should exist after roll-forward: %v", err)
	}
	if _, err := os.Stat(filepath.Join(newRoot, "kangent.db")); !os.IsNotExist(err) {
		t.Errorf("kangent.db should be gone after roll-forward: err=%v", err)
	}

	db, err := store.Open(filepath.Join(newRoot, "kamacu.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	var hot int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects WHERE name='hot'`).Scan(&hot); err != nil {
		db.Close()
		t.Fatalf("count hot: %v", err)
	}
	db.Close()
	if hot != 1 {
		t.Errorf("hot row lost during roll-forward: got %d, want 1", hot)
	}

	// Second call: clean no-op (still RollForward, no error).
	dec2, _, err2 := Prepare(context.Background(), defaultResolvedDB(t), testDefaultFlag)
	if err2 != nil {
		t.Fatalf("second Prepare: %v", err2)
	}
	if dec2 != RollForward {
		t.Errorf("second decision = %v, want RollForward (idempotent no-op)", dec2)
	}
}

// TestPrepareRefusesCrossDeviceLeavingSourceIntact injects a cross-device
// (EXDEV) layout via the sameFilesystemFn seam and asserts Prepare refuses with
// an error and mutates NOTHING — the source stays byte-for-byte intact
// (D-02/MIGRATE-05) and the destination is never created.
func TestPrepareRefusesCrossDeviceLeavingSourceIntact(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldRoot := filepath.Join(home, ".kangent")
	writeHotWALFixture(t, oldRoot)

	orig := sameFilesystemFn
	sameFilesystemFn = func(src, target string) (bool, error) { return false, nil }
	defer func() { sameFilesystemFn = orig }()

	dec, _, err := Prepare(context.Background(), defaultResolvedDB(t), testDefaultFlag)
	if err == nil {
		t.Fatal("Prepare: expected error on cross-device layout, got nil")
	}
	if dec != DoMigrate {
		t.Errorf("decision = %v, want DoMigrate (gate classifies before preflight)", dec)
	}
	// MIGRATE-05: source + its DB left untouched; destination never created.
	if _, err := os.Stat(oldRoot); err != nil {
		t.Errorf("source must remain intact after a refused migration: %v", err)
	}
	if _, err := os.Stat(filepath.Join(oldRoot, "kangent.db")); err != nil {
		t.Errorf("source DB must remain intact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".kamacu")); !os.IsNotExist(err) {
		t.Errorf("destination must not be created on refusal: err=%v", err)
	}
}

// TestPrepareRefuseBootWhenBothDirsExist asserts the both-dirs-present anomaly
// returns RefuseBoot with an error naming BOTH dirs and mutates nothing (D-11).
func TestPrepareRefuseBootWhenBothDirsExist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldRoot := filepath.Join(home, ".kangent")
	newRoot := filepath.Join(home, ".kamacu")
	if err := os.MkdirAll(oldRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldRoot, "marker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	dec, _, err := Prepare(context.Background(), defaultResolvedDB(t), testDefaultFlag)
	if dec != RefuseBoot {
		t.Errorf("decision = %v, want RefuseBoot", dec)
	}
	if err == nil {
		t.Fatal("expected refuse-to-boot error, got nil")
	}
	if !strings.Contains(err.Error(), oldRoot) || !strings.Contains(err.Error(), newRoot) {
		t.Errorf("refuse-to-boot error should name both dirs, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(oldRoot, "marker")); err != nil {
		t.Errorf("no mutation expected on RefuseBoot, marker missing: %v", err)
	}
}

// TestPrepareSkipCustomDBDoesNotMigrate asserts a custom --db opts out entirely
// (D-13): decision SkipCustom, CustomDB=true, and nothing is moved or created.
func TestPrepareSkipCustomDBDoesNotMigrate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldRoot := filepath.Join(home, ".kangent")
	if err := os.MkdirAll(oldRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldRoot, "marker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	customDB := filepath.Join(t.TempDir(), "custom.db")
	dec, cfg, err := Prepare(context.Background(), customDB, testDefaultFlag)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if dec != SkipCustom {
		t.Errorf("decision = %v, want SkipCustom", dec)
	}
	if !cfg.CustomDB {
		t.Errorf("cfg.CustomDB = false, want true")
	}
	if _, err := os.Stat(filepath.Join(oldRoot, "marker")); err != nil {
		t.Errorf("no mutation expected on SkipCustom, marker missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".kamacu")); !os.IsNotExist(err) {
		t.Errorf("destination must not be created on SkipCustom: err=%v", err)
	}
}

// TestPrepareFreshInstallNoop asserts a fresh install (neither dir) is a no-op.
func TestPrepareFreshInstallNoop(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dec, _, err := Prepare(context.Background(), defaultResolvedDB(t), testDefaultFlag)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if dec != FreshInstall {
		t.Errorf("decision = %v, want FreshInstall", dec)
	}
	if _, err := os.Stat(filepath.Join(home, ".kamacu")); !os.IsNotExist(err) {
		t.Errorf("Prepare must not create ~/.kamacu on a fresh install: err=%v", err)
	}
}

// TestGate locks the five-branch decision table (RESEARCH Pattern 1, D-13/D-04/
// D-11). These expectations are plan-locked (Task 1 <behavior>): if a case fails,
// the bug is in Gate (migrate.go), never these wants.
func TestGate(t *testing.T) {
	tests := []struct {
		name      string
		customDB  bool
		srcExists bool
		dstExists bool
		want      Decision
	}{
		{"custom db opts out", true, true, false, SkipCustom},
		{"custom db opts out even with both dirs present", true, true, true, SkipCustom},
		{"src only migrates", false, true, false, DoMigrate},
		{"dst only rolls forward", false, false, true, RollForward},
		{"neither dir is a fresh install", false, false, false, FreshInstall},
		{"both dirs present refuses boot (anomaly)", false, true, true, RefuseBoot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Gate(tt.customDB, tt.srcExists, tt.dstExists); got != tt.want {
				t.Errorf("Gate(customDB=%v, src=%v, dst=%v) = %v, want %v",
					tt.customDB, tt.srcExists, tt.dstExists, got, tt.want)
			}
		})
	}
}

// TestDecisionString locks the stable human label for each Decision variant
// (used in slog lines, Task 1 <behavior>).
func TestDecisionString(t *testing.T) {
	tests := []struct {
		d    Decision
		want string
	}{
		{SkipCustom, "SkipCustom"},
		{DoMigrate, "DoMigrate"},
		{RollForward, "RollForward"},
		{FreshInstall, "FreshInstall"},
		{RefuseBoot, "RefuseBoot"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("Decision(%d).String() = %q, want %q", int(tt.d), got, tt.want)
		}
	}
}
