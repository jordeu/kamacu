// Package diagnostics provides bounded server logs and opt-in LLM transcripts.
package diagnostics

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const MaxLogBytes = 10 << 20
const LogBackups = 5

// RotatingWriter bounds disk usage and serializes concurrent writes.
type RotatingWriter struct {
	mu      sync.Mutex
	path    string
	max     int64
	backups int
	file    *os.File
	size    int64
}

func NewRotatingWriter(path string, maxBytes int64, backups int) (*RotatingWriter, error) {
	if maxBytes <= 0 || backups < 1 {
		return nil, fmt.Errorf("invalid rotation limits")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	w := &RotatingWriter{path: path, max: maxBytes, backups: backups}
	return w, w.open()
}
func (w *RotatingWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file, w.size = f, st.Size()
	return nil
}
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if w.size > 0 && w.size+int64(len(p)) > w.max {
		if err := w.file.Close(); err != nil {
			return 0, err
		}
		w.file = nil
		for i := w.backups; i >= 1; i-- {
			src := w.path
			if i > 1 {
				src = fmt.Sprintf("%s.%d", w.path, i-1)
			}
			if err := os.Rename(src, fmt.Sprintf("%s.%d", w.path, i)); err != nil && !os.IsNotExist(err) {
				return 0, err
			}
		}
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// Configure mirrors text logs to stderr and a rotating private file.
func Configure(dir string, debug bool) (func(), error) {
	w, err := NewRotatingWriter(filepath.Join(dir, "kamacu.log"), MaxLogBytes, LogBackups)
	if err != nil {
		return nil, err
	}
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, w), &slog.HandlerOptions{Level: level})))
	return func() { slog.SetDefault(previous); w.Close() }, nil
}

// Recorder is nil outside debug mode. Each JSONL record contains a complete
// interaction, without provider credentials or HTTP authorization headers.
type Recorder struct{ writer *RotatingWriter }

func NewRecorder(dir string) (*Recorder, error) {
	w, err := NewRotatingWriter(filepath.Join(dir, "llm", "interactions.jsonl"), MaxLogBytes, LogBackups)
	if err != nil {
		return nil, err
	}
	return &Recorder{w}, nil
}
func (r *Recorder) Close() error {
	if r == nil {
		return nil
	}
	return r.writer.Close()
}
func (r *Recorder) Record(fields map[string]any) {
	if r == nil {
		return
	}
	fields["recorded_at"] = time.Now().UTC()
	data, err := json.Marshal(fields)
	if err == nil {
		_, err = r.writer.Write(append(data, '\n'))
	}
	if err != nil {
		slog.Error("writing LLM transcript", "error", err)
	}
}
