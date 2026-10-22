// Package artifact stores uploaded previews independently of task worktrees.
package artifact

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const MaxUpload int64 = 20_000_000
const MaxExtracted int64 = 100_000_000
const MaxFiles = 1000
const MaxArtifacts = 20

var ErrNotFound = errors.New("artifact or task not found")
var ErrConflict = errors.New("artifact changed; begin a new upload")
var ErrUnauthorized = errors.New("invalid or expired upload capability")

type Artifact struct {
	ID        string `json:"id"`
	TaskID    int64  `json:"task_id"`
	Title     string `json:"title"`
	Revision  string `json:"revision"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Upload struct {
	ID        string `json:"upload_id"`
	Token     string `json:"-"`
	ExpiresAt int64  `json:"expires_at"`
}

// mu serializes publishing and collection, including the filesystem/DB gap.
// All SQL cursors are closed before subsequent queries (the store has one connection).
type Service struct {
	DB        *sql.DB
	Root      string
	mu        sync.Mutex
	receiving map[string]bool
}

func New(db *sql.DB, root string) (*Service, error) {
	for _, dir := range []string{"uploads", "revisions"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			return nil, err
		}
	}
	return &Service{DB: db, Root: root, receiving: make(map[string]bool)}, nil
}

func ValidTitle(title string) bool {
	return strings.TrimSpace(title) != "" && utf8.RuneCountInString(title) <= 100
}
func digest(token string) string { h := sha256.Sum256([]byte(token)); return hex.EncodeToString(h[:]) }
func (s *Service) task(ctx context.Context, id int64) error {
	var n int
	err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM tasks WHERE id=? AND project_id IS NOT NULL", id).Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) List(ctx context.Context, task int64) ([]Artifact, error) {
	if err := s.task(ctx, task); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT id,task_id,title,revision,created_at,updated_at FROM artifacts WHERE task_id=? ORDER BY created_at,id", task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Artifact{}
	for rows.Next() {
		var a Artifact
		if err := rows.Scan(&a.ID, &a.TaskID, &a.Title, &a.Revision, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (s *Service) Begin(ctx context.Context, task int64, artifactID, title, filename string, size int64) (Upload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.task(ctx, task); err != nil {
		return Upload{}, err
	}
	if size <= 0 || size > MaxUpload {
		return Upload{}, fmt.Errorf("upload must contain 1 to %d bytes", MaxUpload)
	}
	if !strings.HasSuffix(filename, ".html") && !strings.HasSuffix(filename, ".tar.gz") {
		return Upload{}, errors.New("filename must end in .html or .tar.gz")
	}
	expected := ""
	if artifactID != "" {
		if err := s.DB.QueryRowContext(ctx, "SELECT revision FROM artifacts WHERE id=? AND task_id=?", artifactID, task).Scan(&expected); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return Upload{}, ErrNotFound
			}
			return Upload{}, err
		}
		title = ""
	} else {
		title = strings.TrimSpace(title)
		if !ValidTitle(title) {
			return Upload{}, errors.New("title must contain 1 to 100 characters")
		}
		var count int
		if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM artifacts WHERE task_id=?", task).Scan(&count); err != nil {
			return Upload{}, err
		}
		if count >= MaxArtifacts {
			return Upload{}, errors.New("task already has 20 artifacts")
		}
		artifactID = uuid.NewString()
	}
	// Bound abandoned pending uploads as well as published artifacts.
	var pending int
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM artifact_uploads WHERE task_id=? AND expires_at>? AND published_revision=''", task, time.Now().Unix()).Scan(&pending); err != nil {
		return Upload{}, err
	}
	if pending >= MaxArtifacts {
		return Upload{}, errors.New("too many pending uploads; wait for expiry")
	}
	u := Upload{ID: uuid.NewString(), Token: uuid.NewString() + uuid.NewString(), ExpiresAt: time.Now().Add(15 * time.Minute).Unix()}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO artifact_uploads(id,task_id,artifact_id,title,filename,size_bytes,expected_revision,token_hash,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, u.ID, task, artifactID, title, filename, size, expected, digest(u.Token), u.ExpiresAt)
	return u, err
}

// Receive streams without holding the publication lock, so a slow remote
// uploader cannot block existing previews or unrelated artifact operations.
func (s *Service) Receive(ctx context.Context, id, token string, reader io.Reader) error {
	s.mu.Lock()
	var hash string
	var expires, size int64
	var uploaded int
	err := s.DB.QueryRowContext(ctx, "SELECT token_hash,expires_at,size_bytes,uploaded FROM artifact_uploads WHERE id=?", id).Scan(&hash, &expires, &size, &uploaded)
	if err != nil {
		s.mu.Unlock()
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnauthorized
		}
		return err
	}
	if time.Now().Unix() >= expires || subtle.ConstantTimeCompare([]byte(hash), []byte(digest(token))) != 1 {
		s.mu.Unlock()
		return ErrUnauthorized
	}
	if uploaded != 0 || s.receiving[id] {
		s.mu.Unlock()
		return errors.New("upload already complete or in progress")
	}
	s.receiving[id] = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.receiving, id); s.mu.Unlock() }()
	dest := filepath.Join(s.Root, "uploads", id)
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(f, io.LimitReader(reader, size+1))
	closeErr := f.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	if copyErr != nil || closeErr != nil || n != size || time.Now().Unix() >= expires {
		os.Remove(dest)
		return errors.New("upload interrupted, expired, or size does not match size_bytes")
	}
	result, err := s.DB.ExecContext(ctx, "UPDATE artifact_uploads SET uploaded=1 WHERE id=?", id)
	if err != nil {
		os.Remove(dest)
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count == 0 {
		os.Remove(dest)
		return ErrNotFound
	}
	return nil
}

func (s *Service) Finalize(ctx context.Context, task int64, id string) (Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var artifactID, title, filename, expected, published string
	var expires int64
	var uploaded int
	err := s.DB.QueryRowContext(ctx, `SELECT artifact_id,title,filename,expected_revision,published_revision,expires_at,uploaded FROM artifact_uploads WHERE id=? AND task_id=?`, id, task).Scan(&artifactID, &title, &filename, &expected, &published, &expires, &uploaded)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Artifact{}, ErrNotFound
		}
		return Artifact{}, err
	}
	if published != "" {
		a, err := s.get(ctx, task, artifactID)
		a.Revision = published
		return a, err
	}
	if time.Now().Unix() >= expires {
		return Artifact{}, ErrUnauthorized
	}
	if uploaded == 0 {
		return Artifact{}, errors.New("upload bytes before finalizing")
	}
	revision := uuid.NewString()
	dir := filepath.Join(s.Root, "revisions", revision)
	if err = os.Mkdir(dir, 0700); err != nil {
		return Artifact{}, err
	}
	committed := false
	defer func() {
		if !committed {
			os.RemoveAll(dir)
		}
	}()
	if err = extract(filepath.Join(s.Root, "uploads", id), dir, filename); err != nil {
		return Artifact{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Artifact{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if expected == "" {
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM artifacts WHERE task_id=?", task).Scan(&count); err != nil {
			return Artifact{}, err
		}
		if count >= MaxArtifacts {
			return Artifact{}, errors.New("task already has 20 artifacts")
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO artifacts(id,task_id,title,revision,created_at,updated_at) VALUES(?,?,?,?,?,?)", artifactID, task, title, revision, now, now)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, "UPDATE artifacts SET revision=?,updated_at=? WHERE id=? AND task_id=? AND revision=?", revision, now, artifactID, task, expected)
		if err == nil {
			n, _ := result.RowsAffected()
			if n == 0 {
				return Artifact{}, ErrConflict
			}
		}
	}
	if err != nil {
		return Artifact{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO artifact_revisions(id,artifact_id) VALUES(?,?)", revision, artifactID); err != nil {
		return Artifact{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE artifact_uploads SET published_revision=? WHERE id=?", revision, id); err != nil {
		return Artifact{}, err
	}
	if err = tx.Commit(); err != nil {
		return Artifact{}, err
	}
	committed = true
	os.Remove(filepath.Join(s.Root, "uploads", id))
	return s.get(ctx, task, artifactID)
}

func (s *Service) get(ctx context.Context, task int64, id string) (Artifact, error) {
	var a Artifact
	err := s.DB.QueryRowContext(ctx, "SELECT id,task_id,title,revision,created_at,updated_at FROM artifacts WHERE id=? AND task_id=?", id, task).Scan(&a.ID, &a.TaskID, &a.Title, &a.Revision, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return a, err
}
func (s *Service) Rename(ctx context.Context, task int64, id, title string) (Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	title = strings.TrimSpace(title)
	if !ValidTitle(title) {
		return Artifact{}, errors.New("title must contain 1 to 100 characters")
	}
	result, err := s.DB.ExecContext(ctx, "UPDATE artifacts SET title=? WHERE task_id=? AND id=?", title, task, id)
	if err != nil {
		return Artifact{}, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return Artifact{}, ErrNotFound
	}
	return s.get(ctx, task, id)
}
func (s *Service) Delete(ctx context.Context, task int64, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.DB.ExecContext(ctx, "DELETE FROM artifacts WHERE task_id=? AND id=?", task, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	_, err = s.DB.ExecContext(ctx, "DELETE FROM artifact_uploads WHERE task_id=? AND artifact_id=?", task, id)
	return err
}

// Open returns only regular uploaded files from a still-owned revision.
func (s *Service) Open(ctx context.Context, id, revision, name string) (*os.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validPath(name) {
		return nil, ErrNotFound
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM artifact_revisions WHERE id=? AND artifact_id=?", revision, id).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	f, err := os.Open(filepath.Join(s.Root, "revisions", revision, filepath.FromSlash(name)))
	if err != nil {
		return nil, ErrNotFound
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, ErrNotFound
	}
	return f, nil
}

// Sweep is safe against concurrent finalization and retries filesystem failures.
func (s *Service) Sweep(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.DB.ExecContext(ctx, "DELETE FROM artifact_uploads WHERE expires_at<=? OR (expected_revision<>'' AND NOT EXISTS(SELECT 1 FROM artifacts WHERE artifacts.id=artifact_uploads.artifact_id))", time.Now().Unix()); err != nil {
		return err
	}
	for _, spec := range []struct{ dir, query string }{{"uploads", "SELECT id FROM artifact_uploads WHERE uploaded=1 AND published_revision=''"}, {"revisions", "SELECT id FROM artifact_revisions"}} {
		rows, err := s.DB.QueryContext(ctx, spec.query)
		if err != nil {
			return err
		}
		keep := map[string]bool{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			keep[id] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(filepath.Join(s.Root, spec.dir))
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !keep[e.Name()] && !(spec.dir == "uploads" && s.receiving[e.Name()]) {
				if err = os.RemoveAll(filepath.Join(s.Root, spec.dir, e.Name())); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validPath(name string) bool {
	return name != "" && len(name) <= 1024 && !strings.ContainsAny(name, "\\:\x00") && !strings.HasPrefix(name, "/") && path.Clean(name) == name && name != "." && name != ".." && !strings.HasPrefix(name, "../")
}

func extract(source, dir, filename string) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	if strings.HasSuffix(filename, ".html") {
		out, err := os.OpenFile(filepath.Join(dir, "index.html"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, f)
		closeErr := out.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		return errors.New("invalid gzip archive")
	}
	defer gz.Close()
	// Bound tar headers, padding, and metadata too, not only extracted payloads.
	limited := &io.LimitedReader{R: gz, N: MaxExtracted + 4_000_000 + 1}
	tr := tar.NewReader(limited)
	seen := map[string]bool{}
	var total int64
	entries := 0
	files := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("invalid tar archive")
		}
		entries++
		if entries > 4000 {
			return errors.New("archive exceeds 4000 entries")
		}
		name := strings.TrimSuffix(h.Name, "/")
		for strings.HasPrefix(name, "./") {
			name = strings.TrimPrefix(name, "./")
		}
		if name == "." && h.Typeflag == tar.TypeDir {
			continue
		}
		if !validPath(name) || seen[name] {
			return errors.New("unsafe or duplicate archive path")
		}
		seen[name] = true
		dest := filepath.Join(dir, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(dest, 0700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			files++
			if files > MaxFiles {
				return errors.New("archive exceeds 1000 files")
			}
			if h.Size < 0 || h.Size > MaxExtracted-total {
				return errors.New("archive exceeds 100 MB extracted")
			}
			total += h.Size
			if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
				return err
			}
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return errors.New("archive contains conflicting paths")
			}
			_, copyErr := io.CopyN(out, tr, h.Size)
			closeErr := out.Close()
			if copyErr != nil || closeErr != nil {
				return errors.New("truncated archive file")
			}
		default:
			return errors.New("archive may contain only regular files and directories")
		}
	}
	// Force gzip checksum validation, also bounding trailing decompressed data.
	if _, err = io.Copy(io.Discard, limited); err != nil {
		return errors.New("invalid gzip checksum")
	}
	if limited.N <= 0 {
		return errors.New("archive decompression limit exceeded")
	}
	info, err := os.Stat(filepath.Join(dir, "index.html"))
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("archive requires index.html at its root")
	}
	return nil
}
