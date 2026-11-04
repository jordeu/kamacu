package diagnostics

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const TranscriptMaxAge = 7 * 24 * time.Hour
const TranscriptMaxBytes int64 = 256 << 20

// Group identifies the workflow that owns related inference calls. Its context
// must live for the whole workflow, including gaps between inference requests.
type Group struct {
	Kind        string    `json:"kind"`
	ID          string    `json:"id"`
	StartedAt   time.Time `json:"started_at"`
	SessionID   string    `json:"session_id,omitempty"`
	TaskID      int64     `json:"task_id,omitempty"`
	TaskTitle   string    `json:"task_title,omitempty"`
	ProjectID   int64     `json:"project_id,omitempty"`
	ProjectName string    `json:"project_name,omitempty"`
	done        <-chan struct{}
}
type groupKey struct{}

func WithGroup(ctx context.Context, group Group) context.Context {
	if group.StartedAt.IsZero() {
		group.StartedAt = time.Now().UTC()
	}
	group.done = ctx.Done()
	return context.WithValue(ctx, groupKey{}, group)
}

// Recorder writes private, human-readable transcripts. Retention deletes only
// complete workflow folders, never individual interactions within them.
type Recorder struct {
	mu          sync.Mutex
	root        string
	active      map[string]<-chan struct{}
	lastCleanup time.Time
	maxAge      time.Duration
	maxBytes    int64
}

func NewRecorder(dir string) (*Recorder, error) {
	r := &Recorder{root: filepath.Join(dir, "llm"), active: map[string]<-chan struct{}{}, maxAge: TranscriptMaxAge, maxBytes: TranscriptMaxBytes}
	if err := os.MkdirAll(r.root, 0700); err != nil {
		return nil, err
	}
	if err := r.cleanup(time.Now().UTC(), ""); err != nil {
		return nil, err
	}
	return r, nil
}

// Close is retained for symmetry with the server logger; records hold no open files.
func (r *Recorder) Close() error { return nil }
func (r *Recorder) Record(ctx context.Context, fields map[string]any) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.record(ctx, fields); err != nil {
		slog.Error("writing LLM transcript", "error", err)
	}
}
func component(value string) string {
	if value != "" && len(value) <= 100 && strings.Trim(value, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") == "" {
		return value
	}
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("id-%x", sum[:12])
}
func (r *Recorder) record(ctx context.Context, fields map[string]any) error {
	now := time.Now().UTC()
	g, ok := ctx.Value(groupKey{}).(Group)
	if !ok {
		g = Group{Kind: "call", ID: uuid.NewString(), StartedAt: now}
	}
	purpose, _ := fields["purpose"].(string)
	if purpose != "monitoring" {
		purpose = "notifications"
	}
	var rel string
	if purpose == "monitoring" && g.SessionID != "" {
		rel = filepath.Join("monitoring", "session-"+component(g.SessionID), now.Format("2006-01-02"))
	} else {
		rel = filepath.Join(purpose, g.StartedAt.UTC().Format("2006-01-02"), g.StartedAt.UTC().Format("150405.000000000")+"_"+component(g.Kind)+"-"+component(g.ID))
	}
	dir := filepath.Join(r.root, rel)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if purpose == "notifications" && g.done != nil {
		r.active[dir] = g.done
	}
	manifest := groupManifest{Group: g, UpdatedAt: now, Purpose: purpose}
	if err := writeJSON(filepath.Join(dir, "group.json"), manifest); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	// Sequence numbers survive process restarts and are serialized by the recorder.
	sequence := 1
	for _, entry := range entries {
		var n int
		if entry.IsDir() {
			fmt.Sscanf(entry.Name(), "%d_", &n)
			if n >= sequence {
				sequence = n + 1
			}
		}
	}
	id := uuid.NewString()
	if provider, err := json.Marshal(fields["provider"]); err == nil {
		var v struct {
			ID string `json:"interaction_id"`
		}
		if json.Unmarshal(provider, &v) == nil && v.ID != "" {
			id = v.ID
		}
	}
	name := fmt.Sprintf("%03d_%s", sequence, component(id))
	pending, err := os.MkdirTemp(dir, ".pending-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(pending)
	metadata := make(map[string]any, len(fields))
	for k, v := range fields {
		if k != "prompt" && k != "response" && k != "provider" && k != "schema" {
			metadata[k] = v
		}
	}
	metadata["group"] = g
	metadata["interaction_id"] = id
	metadata["recorded_at"] = now
	prompt, _ := fields["prompt"].(string)
	response, _ := fields["response"].(string)
	files := map[string][]byte{"prompt.md": []byte(prompt), "response.md": readableResponse(response)}
	for file, value := range map[string]any{"metadata.json": metadata, "provider.json": fields["provider"], "schema.json": fields["schema"]} {
		data, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			return e
		}
		files[file] = append(data, '\n')
	}
	for file, data := range files {
		if err = os.WriteFile(filepath.Join(pending, file), data, 0600); err != nil {
			return err
		}
	}
	if err = os.Rename(pending, filepath.Join(dir, name)); err != nil {
		return err
	}
	summary := filepath.Join(dir, "summary.md")
	if _, err = os.Stat(summary); os.IsNotExist(err) {
		header := fmt.Sprintf("# %s: %s %s\n\nStarted: %s (UTC)\n", purpose, g.Kind, g.ID, g.StartedAt.UTC().Format(time.RFC3339))
		if g.SessionID != "" {
			header += fmt.Sprintf("\nSession: %s\n", g.SessionID)
		}
		if g.TaskID > 0 {
			header += fmt.Sprintf("\nTask: %d — %s\n", g.TaskID, g.TaskTitle)
		}
		if g.ProjectID > 0 {
			header += fmt.Sprintf("\nProject: %d — %s\n", g.ProjectID, g.ProjectName)
		}
		header += "\n| Call | Model | Duration (ms) | Result |\n| --- | --- | --- | --- |\n"
		if err = os.WriteFile(summary, []byte(header), 0600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	result := "OK"
	if detail, _ := fields["error"].(string); detail != "" {
		result = "Error: " + detail
	}
	row := fmt.Sprintf("| [%03d prompt](%s/prompt.md) · [response](%s/response.md) · [metadata](%s/metadata.json) | %s | %v | %s |\n", sequence, name, name, name, tableText(fmt.Sprint(fields["model"])), fields["duration_ms"], tableText(result))
	f, err := os.OpenFile(summary, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(row)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if purpose == "monitoring" && g.SessionID != "" {
		if err = writeSessionSummary(filepath.Dir(dir)); err != nil {
			return err
		}
	}
	if now.Sub(r.lastCleanup) >= time.Minute {
		return r.cleanup(now, dir)
	}
	return nil
}
func writeSessionSummary(dir string) error {
	days, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var links strings.Builder
	var latest Group
	for _, day := range days {
		if !day.IsDir() || strings.HasPrefix(day.Name(), ".") {
			continue
		}
		manifestPath := filepath.Join(dir, day.Name(), "group.json")
		manifest, err := readGroupManifest(manifestPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			slog.Warn("skipping invalid LLM transcript group", "path", manifestPath, "error", err)
			continue
		}
		latest = manifest.Group
		fmt.Fprintf(&links, "- [%s](%s/summary.md)\n", day.Name(), day.Name())
	}
	summary := fmt.Sprintf("# Monitoring session %s\n\n", latest.SessionID)
	if latest.TaskID > 0 {
		summary += fmt.Sprintf("Task: %d — %s\n\n", latest.TaskID, latest.TaskTitle)
	} else {
		summary += "Scope: Scratchpad\n\n"
	}
	if latest.ProjectID > 0 {
		summary += fmt.Sprintf("Project: %d — %s\n\n", latest.ProjectID, latest.ProjectName)
	}
	summary += "Interactions are grouped by UTC day.\n\n" + links.String()
	return os.WriteFile(filepath.Join(dir, "summary.md"), []byte(summary), 0600)
}

func tableText(s string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(s)
}
func readableResponse(raw string) []byte {
	var out bytes.Buffer
	if json.Indent(&out, []byte(raw), "", "  ") == nil {
		return []byte("```json\n" + out.String() + "\n```\n")
	}
	return []byte(raw)
}
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".metadata-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

type groupManifest struct {
	Group     Group     `json:"group"`
	UpdatedAt time.Time `json:"updated_at"`
	Purpose   string    `json:"purpose"`
}

// Invalid saved diagnostics are preserved for inspection and excluded from
// retention; they must not prevent fresh captures or server startup.
func readGroupManifest(path string) (manifest groupManifest, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	if (manifest.Purpose != "monitoring" && manifest.Purpose != "notifications") || manifest.Group.ID == "" || manifest.UpdatedAt.IsZero() {
		return manifest, fmt.Errorf("incomplete transcript group manifest")
	}
	return manifest, nil
}

type storedGroup struct {
	path     string
	manifest groupManifest
	size     int64
}

func (r *Recorder) cleanup(now time.Time, protect string) error {
	groups := []storedGroup{}
	var total int64
	err := filepath.WalkDir(r.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		manifestPath := filepath.Join(path, "group.json")
		m, e := readGroupManifest(manifestPath)
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			slog.Warn("skipping invalid LLM transcript group", "path", manifestPath, "error", e)
			return filepath.SkipDir
		}
		var size int64
		e = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if !entry.IsDir() {
				info, e := entry.Info()
				if e != nil {
					return e
				}
				size += info.Size()
			}
			return nil
		})
		if e != nil {
			return e
		}
		groups = append(groups, storedGroup{path, m, size})
		total += size
		return filepath.SkipDir
	})
	if err != nil {
		return err
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].manifest.UpdatedAt.Before(groups[j].manifest.UpdatedAt) })
	for _, g := range groups {
		active := g.path == protect
		if done := r.active[g.path]; done != nil {
			select {
			case <-done:
				delete(r.active, g.path)
			default:
				active = true
			}
		}
		// A monitoring day is the retention unit; never cut today's session in half.
		if g.manifest.Purpose == "monitoring" && filepath.Base(g.path) >= now.Format("2006-01-02") {
			active = true
		}
		if active || (now.Sub(g.manifest.UpdatedAt) <= r.maxAge && total <= r.maxBytes) {
			continue
		}
		if err = os.RemoveAll(g.path); err != nil {
			return err
		}
		total -= g.size
		delete(r.active, g.path)
		parent := filepath.Dir(g.path)
		// Drop empty date/session containers, including a session's summary.
		entries, e := os.ReadDir(parent)
		if e != nil {
			return e
		}
		if len(entries) == 0 || (g.manifest.Purpose == "monitoring" && len(entries) == 1 && entries[0].Name() == "summary.md") {
			if err = os.RemoveAll(parent); err != nil {
				return err
			}
		} else if g.manifest.Purpose == "monitoring" {
			if err = writeSessionSummary(parent); err != nil {
				return err
			}
		}
	}
	r.lastCleanup = now
	return nil
}
