// Package notifications collects registered sources and routes user decisions
// through a bounded LLM/MCP execution loop, independently of agent sessions.
package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrContextUnavailable identifies a missing subject/comment, not an auth,
// rate-limit, transport or identity failure. Providers may return partial evidence
// with ContextWarnings instead, preserving any subject state they verified.
var ErrContextUnavailable = errors.New("notification context is unavailable")

type Source struct {
	PendingEvents   int             `json:"pending_events"`
	ProcessingError string          `json:"processing_error"`
	ID              string          `json:"id"`
	Provider        string          `json:"provider"`
	Name            string          `json:"name"`
	Config          json.RawMessage `json:"config"`
	Enabled         bool            `json:"enabled"`
	Account         string          `json:"account"`
	Checkpoint      string          `json:"checkpoint"`
	NextPoll        int64           `json:"next_poll"`
	LastSuccess     *int64          `json:"last_success"`
	LastError       string          `json:"last_error"`
	CreatedAt       int64           `json:"created_at"`
}
type Event struct {
	ExternalID       string   `json:"external_id"`
	Version          string   `json:"version"`
	Repository       string   `json:"repository"`
	URL              string   `json:"url"`
	Title            string   `json:"title"`
	Reason           string   `json:"reason"`
	SubjectType      string   `json:"subject_type"`
	SubjectURL       string   `json:"subject_url"`
	LatestCommentURL string   `json:"latest_comment_url"`
	Context          string   `json:"context"`
	SubjectState     string   `json:"subject_state,omitempty"`
	ContextWarnings  []string `json:"context_warnings,omitempty"`
}
type Batch struct {
	Events     []Event
	Account    string
	Checkpoint string
	PollAfter  time.Duration
}
type Provider interface {
	Validate(json.RawMessage) error
	CheckIdentity(context.Context, Source) error
	Collect(context.Context, Source, map[string]bool) (Batch, error)
	Enrich(context.Context, Source, Event) (Event, error)
}
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}
type Tools interface {
	List(context.Context) ([]Tool, error)
	Call(context.Context, string, json.RawMessage) (string, error)
}
