package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kamacu/internal/store"
)

func fixture(t *testing.T) *Service {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`INSERT INTO projects(id,name,repo_path) VALUES(1,'test','/test')`, `INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'test',0),(2,1,'other',1)`} {
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	s, err := New(db, filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func publish(t *testing.T, s *Service, id, filename string, data []byte) Artifact {
	t.Helper()
	ctx := context.Background()
	u, err := s.Begin(ctx, 1, id, "Preview", filename, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Receive(ctx, u.ID, u.Token, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	a, err := s.Finalize(ctx, 1, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func archive(t *testing.T, headers []*tar.Header, contents []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for i, h := range headers {
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(contents[i]))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(contents[i])); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
func TestPublicationReplacementAndCollection(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	a := publish(t, s, "", "demo.html", []byte("<h1>old</h1>"))
	if _, err := s.Rename(ctx, 1, a.ID, "My title"); err != nil {
		t.Fatal(err)
	}
	b := publish(t, s, a.ID, "demo.html", []byte("<h1>new</h1>"))
	if a.ID != b.ID || a.Revision == b.Revision || b.Title != "My title" {
		t.Fatalf("replacement: %+v", b)
	}
	for _, revision := range []string{a.Revision, b.Revision} {
		f, err := s.Open(ctx, a.ID, revision, "index.html")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	restored, err := New(s.DB, s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err = restored.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := restored.List(ctx, 1)
	if err != nil || len(list) != 1 || list[0].Revision != b.Revision {
		t.Fatalf("restart: %v %v", list, err)
	}
	if _, err = s.DB.Exec("DELETE FROM projects WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Open(ctx, a.ID, a.Revision, "index.html"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted content accessible: %v", err)
	}
	if err = s.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(s.Root, "revisions"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("orphan revisions: %v %v", entries, err)
	}
}
func TestUploadAuthorizationRetryAndConflict(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	data := []byte("<h1>preview</h1>")
	u, err := s.Begin(ctx, 1, "", "Preview", "a.html", int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Receive(ctx, u.ID, "wrong", bytes.NewReader(data)); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	if _, err = s.Finalize(ctx, 1, u.ID); err == nil {
		t.Fatal("finalized missing upload")
	}
	if err = s.Receive(ctx, u.ID, u.Token, strings.NewReader("short")); err == nil {
		t.Fatal("accepted short body")
	}
	if err = s.Receive(ctx, u.ID, u.Token, bytes.NewReader(append(data, '!'))); err == nil {
		t.Fatal("accepted oversized body")
	}
	if err = s.Receive(ctx, u.ID, u.Token, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Finalize(ctx, 2, u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong task: %v", err)
	}
	a, err := s.Finalize(ctx, 1, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.Finalize(ctx, 1, u.ID)
	if err != nil || retry.ID != a.ID {
		t.Fatal("finalize not idempotent", err)
	}
	pending, err := s.Begin(ctx, 1, a.ID, "", "a.html", int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	publish(t, s, a.ID, "a.html", []byte("new"))
	if err = s.Receive(ctx, pending.ID, pending.Token, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Finalize(ctx, 1, pending.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflict: %v", err)
	}
	expired, err := s.Begin(ctx, 1, "", "Expired", "a.html", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE artifact_uploads SET expires_at=? WHERE id=?", time.Now().Unix()-1, expired.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Receive(ctx, expired.ID, expired.Token, strings.NewReader("x")); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
}
func TestArchivesAndFailedReplacement(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	a := publish(t, s, "", "a.html", []byte("working"))
	cases := []struct {
		name     string
		headers  []*tar.Header
		contents []string
	}{
		{"traversal", []*tar.Header{{Name: "../index.html", Typeflag: tar.TypeReg}}, []string{"x"}},
		{"absolute", []*tar.Header{{Name: "/index.html", Typeflag: tar.TypeReg}}, []string{"x"}},
		{"symlink", []*tar.Header{{Name: "index.html", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}}, []string{""}},
		{"hardlink", []*tar.Header{{Name: "index.html", Typeflag: tar.TypeLink, Linkname: "/etc/passwd"}}, []string{""}},
		{"duplicate", []*tar.Header{{Name: "index.html", Typeflag: tar.TypeReg}, {Name: "index.html", Typeflag: tar.TypeReg}}, []string{"x", "y"}},
		{"collision", []*tar.Header{{Name: "index.html", Typeflag: tar.TypeDir}, {Name: "index.html", Typeflag: tar.TypeReg}}, []string{"", "x"}},
		{"missing entrypoint", []*tar.Header{{Name: "nested/index.html", Typeflag: tar.TypeReg}}, []string{"x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := archive(t, tc.headers, tc.contents)
			u, err := s.Begin(ctx, 1, a.ID, "", "a.tar.gz", int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			if err = s.Receive(ctx, u.ID, u.Token, bytes.NewReader(data)); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Finalize(ctx, 1, u.ID); err == nil {
				t.Fatal("accepted invalid archive")
			}
			current, err := s.get(ctx, 1, a.ID)
			if err != nil || current.Revision != a.Revision {
				t.Fatal("failed upload changed published revision", err)
			}
		})
	}
	data := archive(t, []*tar.Header{{Name: "index.html", Typeflag: tar.TypeReg}, {Name: "assets/style.css", Typeflag: tar.TypeReg}}, []string{"<link href='assets/style.css'>", "body{color:red}"})
	b := publish(t, s, a.ID, "a.tar.gz", data)
	f, err := s.Open(ctx, b.ID, b.Revision, "assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	content, _ := io.ReadAll(f)
	if string(content) != "body{color:red}" {
		t.Fatal(string(content))
	}
	// Gzip checksum corruption must not publish even after tar reaches its EOF.
	data[len(data)-1] ^= 0xff
	u, err := s.Begin(ctx, 1, a.ID, "", "a.tar.gz", int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Receive(ctx, u.ID, u.Token, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Finalize(ctx, 1, u.ID); err == nil {
		t.Fatal("accepted bad gzip checksum")
	}
}
func TestArtifactLimits(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	for _, size := range []int64{0, -1, MaxUpload + 1} {
		if _, err := s.Begin(ctx, 1, "", "Preview", "a.html", size); err == nil {
			t.Fatalf("accepted size %d", size)
		}
	}
	for i := 0; i < MaxArtifacts; i++ {
		publish(t, s, "", "a.html", []byte(fmt.Sprint(i)))
	}
	if _, err := s.Begin(ctx, 1, "", "Extra", "a.html", 1); err == nil {
		t.Fatal("accepted 21st artifact")
	}
	// A compressed bomb fits the upload bound but exceeds extracted limits.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "index.html", Typeflag: tar.TypeReg, Size: MaxExtracted + 1}); err != nil {
		t.Fatal(err)
	}
	chunk := make([]byte, 1024*1024)
	remaining := MaxExtracted + 1
	for remaining > 0 {
		n := int64(len(chunk))
		if remaining < n {
			n = remaining
		}
		tw.Write(chunk[:n])
		remaining -= n
	}
	tw.Close()
	gz.Close()
	source := filepath.Join(t.TempDir(), "bomb")
	os.WriteFile(source, buf.Bytes(), 0600)
	if err := extract(source, t.TempDir(), "a.tar.gz"); err == nil {
		t.Fatal("accepted expansion bomb")
	}
}

func TestSlowUploadDoesNotBlockPreviewsOrCollection(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	a := publish(t, s, "", "a.html", []byte("working"))
	u, err := s.Begin(ctx, 1, a.ID, "", "a.html", 1)
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	receiving := make(chan error, 1)
	go func() { receiving <- s.Receive(ctx, u.ID, u.Token, reader) }()
	// Wait until the receiver has reserved this upload before checking collection.
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		started := s.receiving[u.ID]
		s.mu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("upload did not start")
		}
		time.Sleep(time.Millisecond)
	}
	checked := make(chan error, 1)
	go func() {
		f, err := s.Open(ctx, a.ID, a.Revision, "index.html")
		if err == nil {
			f.Close()
			err = s.Sweep(ctx)
		}
		checked <- err
	}()
	select {
	case err := <-checked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("slow upload blocked preview or collection")
	}
	if err = s.Receive(ctx, u.ID, u.Token, strings.NewReader("x")); err == nil {
		t.Fatal("accepted concurrent writer")
	}
	if _, err = writer.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	if err = <-receiving; err != nil {
		t.Fatal(err)
	}
	if _, err = s.Finalize(ctx, 1, u.ID); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentFinalizationRespectsTaskLimit(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	for i := 0; i < MaxArtifacts-1; i++ {
		publish(t, s, "", "a.html", []byte("x"))
	}
	uploads := make([]Upload, 2)
	for i := range uploads {
		var err error
		uploads[i], err = s.Begin(ctx, 1, "", "Preview", "a.html", 1)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Receive(ctx, uploads[i].ID, uploads[i].Token, strings.NewReader("x")); err != nil {
			t.Fatal(err)
		}
	}
	results := make(chan error, 2)
	for _, u := range uploads {
		go func() { _, err := s.Finalize(ctx, 1, u.ID); results <- err }()
	}
	successes := 0
	for range uploads {
		if <-results == nil {
			successes++
		}
	}
	list, err := s.List(ctx, 1)
	if err != nil || successes != 1 || len(list) != MaxArtifacts {
		t.Fatalf("limit race: successes=%d count=%d err=%v", successes, len(list), err)
	}
}

func TestArchiveDotPrefixAndFileCount(t *testing.T) {
	s := fixture(t)
	data := archive(t, []*tar.Header{{Name: "./", Typeflag: tar.TypeDir}, {Name: "./index.html", Typeflag: tar.TypeReg}}, []string{"", "working"})
	publish(t, s, "", "a.tar.gz", data)
	headers := []*tar.Header{{Name: "index.html", Typeflag: tar.TypeReg}}
	contents := []string{"working"}
	for i := 0; i < MaxFiles; i++ {
		headers = append(headers, &tar.Header{Name: fmt.Sprintf("files/%d", i), Typeflag: tar.TypeReg})
		contents = append(contents, "x")
	}
	source := filepath.Join(t.TempDir(), "archive")
	if err := os.WriteFile(source, archive(t, headers, contents), 0600); err != nil {
		t.Fatal(err)
	}
	if err := extract(source, t.TempDir(), "a.tar.gz"); err == nil {
		t.Fatal("accepted 1001 files")
	}
}
