package projectcontext

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kamacu/internal/store"
)

func TestInterruptedArchivesAndInstancePaths(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "kamacu.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE projects (id INTEGER PRIMARY KEY, repo_path TEXT, created_at TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO projects VALUES(12, '/repo', 'original')"); err != nil {
		t.Fatal(err)
	}
	f, err := New(home, path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Directory(12) != filepath.Join(home, "projects", "12", "context") {
		t.Fatal(f.Directory(12))
	}
	file := filepath.Join(f.Directory(12), "instructions.md")
	if err := os.MkdirAll(f.Directory(12), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.StageArchive(12, "/repo", "original"); err != nil {
		t.Fatal(err)
	}
	if err := f.Recover(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "preserve" {
		t.Fatal("restore", err)
	}
	if _, err := f.StageArchive(12, "/repo", "original"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE projects SET created_at='recreated' WHERE id=12"); err != nil {
		t.Fatal(err)
	}
	if err := f.Recover(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("context inherited by reused id", err)
	}
	archives, err := filepath.Glob(filepath.Join(home, "archive", "projects", "12", "*", "context", "instructions.md"))
	if err != nil || len(archives) != 1 {
		t.Fatal(archives, err)
	}
	if got, err := os.ReadFile(archives[0]); err != nil || string(got) != "preserve" {
		t.Fatal("archive", err)
	}
	// Recovery keeps a user's newly recreated folder and archives the interrupted copy.
	if err := os.MkdirAll(f.Directory(12), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("before interruption"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.StageArchive(12, "/repo", "recreated"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(f.Directory(12), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("user replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.Recover(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "user replacement" {
		t.Fatal("overwrote replacement", err)
	}
	archives, err = filepath.Glob(filepath.Join(home, "archive", "projects", "12", "*", "context", "instructions.md"))
	if err != nil || len(archives) != 2 {
		t.Fatal(archives, err)
	}
	custom := filepath.Join(home, "custom.db")
	if err := os.WriteFile(custom, nil, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(home, "alias")
	if err := os.Symlink(custom, alias); err != nil {
		t.Fatal(err)
	}
	other, err := New(home, custom)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := New(home, alias)
	if err != nil || linked.Directory(12) != other.Directory(12) || other.Directory(12) == f.Directory(12) {
		t.Fatal(other, linked, err)
	}
}
