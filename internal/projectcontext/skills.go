package projectcontext

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
)

// Ensure creates the project folder and reconciles links under the archive lock.
// Existing files, directories and links to other targets are project overrides.
func (f *Folders) Ensure(id int64) error {
	local := filepath.Join(f.Directory(id), "skills")
	if err := os.MkdirAll(local, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(f.sharedSkills, 0700); err != nil {
		return err
	}
	// A user who linked the entire skills directory controls its contents.
	info, err := os.Lstat(local)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return err
	}
	entries, err := os.ReadDir(f.sharedSkills)
	if err != nil {
		return err
	}
	active := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		source := filepath.Join(f.sharedSkills, name)
		skill, err := os.Stat(filepath.Join(source, "SKILL.md"))
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			continue
		}
		if err != nil {
			return err
		}
		if !skill.Mode().IsRegular() {
			continue
		}
		active[name] = true
		if err := os.Symlink(source, filepath.Join(local, name)); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	entries, err = os.ReadDir(local)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if active[name] || entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		link := filepath.Join(local, name)
		target, err := os.Readlink(link)
		if err != nil {
			return err
		}
		if target == filepath.Join(f.sharedSkills, name) {
			if err := os.Remove(link); err != nil {
				return err
			}
		}
	}
	return nil
}

// PrepareExisting backfills projects created before context provisioning existed.
func (f *Folders) PrepareExisting(ctx context.Context, db *sql.DB) error {
	unlock, err := f.LockArchive()
	if err != nil {
		return err
	}
	defer unlock()
	if err := f.seedDefaultSkills(); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, "SELECT id FROM projects")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if err := f.Ensure(id); err != nil {
			// Keep other projects usable. Launch retries and reports the specific error.
			slog.Warn("prepare project context", "project", id, "error", err)
		}
	}
	return rows.Err()
}

// PrepareForLaunch checks project membership while serialized with archive
// operations, so a late launch cannot recreate a deleted project's folder.
func (f *Folders) PrepareForLaunch(ctx context.Context, db *sql.DB, id int64) (string, error) {
	unlock, err := f.LockArchive()
	if err != nil {
		return "", err
	}
	defer unlock()
	if err := db.QueryRowContext(ctx, "SELECT id FROM projects WHERE id = ?", id).Scan(&id); err != nil {
		return "", err
	}
	if err := f.Ensure(id); err != nil {
		return "", err
	}
	return f.Directory(id), nil
}
