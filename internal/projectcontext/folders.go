// Package projectcontext locates hand-edited agent guidance and archives it when a project is deleted.
package projectcontext

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

type Folders struct {
	root         string
	sharedSkills string
}

// New resolves aliases of the same database to the same instance directory.
// Both the Kamacu directory and database already exist when the server starts.
func New(home, database string) (*Folders, error) {
	home, err := filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(home)
	if err != nil {
		return nil, err
	}
	path, err := filepath.Abs(database)
	if err != nil {
		return nil, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	shared := filepath.Join(root, "skills")
	def := filepath.Join(root, "kamacu.db")
	if real, err := filepath.EvalSymlinks(def); err == nil {
		def = real
	}
	if path != def {
		root = filepath.Join(root, "instances", fmt.Sprintf("%x", sha256.Sum256([]byte(path))))
	}
	return &Folders{root: root, sharedSkills: shared}, nil
}

func (f *Folders) Directory(projectID int64) string {
	return filepath.Join(f.root, "projects", strconv.FormatInt(projectID, 10), "context")
}

// LockArchive serializes recovery and project deletion across servers using the
// same database. Worktree cleanup needs the database itself between these steps.
func (f *Folders) LockArchive() (func(), error) {
	return lockDirectory(f.root)
}

func lockDirectory(path string) (func(), error) {
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, err
	}
	dir, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(dir.Fd()), unix.LOCK_EX); err != nil {
		dir.Close()
		return nil, err
	}
	return func() { dir.Close() }, nil
}

// StageArchive runs under LockArchive before any project teardown.
// The pending directory is also the journal: recovery can tell whether the
// matching project survived and restore the files after a rollback or crash.
func (f *Folders) StageArchive(id int64, repo, created string) (string, error) {
	source := f.Directory(id)
	if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	parent := filepath.Join(f.root, "archive", "projects", strconv.FormatInt(id, 10))
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	pending := filepath.Join(parent, uuid.NewString()+".pending-"+identity(repo, created))
	if err := os.Mkdir(pending, 0700); err != nil {
		return "", err
	}
	if err := os.Rename(source, filepath.Join(pending, "context")); err != nil {
		return "", errors.Join(err, os.Remove(pending))
	}
	return pending, nil
}

func identity(repo, created string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(repo+"\x00"+created)))
}

// Finalize publishes a completed archive.
func (f *Folders) Finalize(pending string) error {
	name, _, _ := strings.Cut(filepath.Base(pending), ".pending-")
	return os.Rename(pending, filepath.Join(filepath.Dir(pending), name))
}

type projectReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Reconcile settles a staged archive against the committed project identity.
// Both ordinary delete and restart recovery hold LockArchive here.
func (f *Folders) Reconcile(ctx context.Context, db projectReader, pending string) error {
	name, key, ok := strings.Cut(filepath.Base(pending), ".pending-")
	if !ok {
		return nil
	}
	if _, err := uuid.Parse(name); err != nil {
		return nil
	}
	id, err := strconv.ParseInt(filepath.Base(filepath.Dir(pending)), 10, 64)
	if err != nil {
		return err
	}
	var repo, created string
	err = db.QueryRowContext(ctx, "SELECT repo_path, created_at FROM projects WHERE id=?", id).Scan(&repo, &created)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	sameProject := err == nil && identity(repo, created) == key
	staged := filepath.Join(pending, "context")
	if _, err := os.Lstat(staged); errors.Is(err, os.ErrNotExist) {
		return os.Remove(pending)
	} else if err != nil {
		return err
	}
	if !sameProject {
		return f.Finalize(pending)
	}
	source := f.Directory(id)
	if _, err := os.Lstat(source); err == nil {
		// The user recreated the live folder. Preserve both copies and allow startup.
		slog.Warn("project context recreated during deletion; keeping previous files in archive", "active", source, "pending", pending)
		return f.Finalize(pending)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
		return err
	}
	if err := os.Rename(staged, source); err != nil {
		return err
	}
	return os.Remove(pending)
}

func (f *Folders) Recover(ctx context.Context, db *sql.DB) error {
	unlock, err := f.LockArchive()
	if err != nil {
		return err
	}
	defer unlock()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Use a consistent view of project identity while settling interrupted archives.
	entries, err := os.ReadDir(filepath.Join(f.root, "archive", "projects"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		_, err := strconv.ParseInt(entry.Name(), 10, 64)
		if err != nil || !entry.IsDir() {
			continue
		}
		parent := filepath.Join(f.root, "archive", "projects", entry.Name())
		archives, err := os.ReadDir(parent)
		if err != nil {
			return err
		}
		for _, archive := range archives {
			if !archive.IsDir() {
				continue
			}
			if err := f.Reconcile(ctx, tx, filepath.Join(parent, archive.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
