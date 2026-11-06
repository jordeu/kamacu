// Package notifications collects registered sources and routes user decisions
// through a bounded LLM/MCP execution loop, independently of agent sessions.
package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"kamacu/internal/store"
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
	ReviewSessionRouting *RuleFact `json:"review_session_routing,omitempty"`
	SessionRouting       *RuleFact `json:"session_routing,omitempty"`
	// MentionScanComplete excludes missing pages/resources, but not prose clipping.
	MentionScanComplete  bool                `json:"mention_scan_complete,omitempty"`
	SubjectUpdatedAt     string              `json:"subject_updated_at,omitempty"`
	Evidence             []SubjectEvidence   `json:"evidence,omitempty"`
	EvidenceComplete     bool                `json:"evidence_complete,omitempty"`
	EvidenceObservedAt   time.Time           `json:"evidence_observed_at,omitempty"`
	WorkContextTruncated bool                `json:"work_context_truncated,omitempty"`
	WorkContextLookup    string              `json:"work_context_lookup,omitempty"`
	RelatedSessions      []store.SessionWork `json:"related_sessions,omitempty"`
	PRFacts              *PRFacts            `json:"pr_facts,omitempty"`
	RelatedWork          []store.RelatedTask `json:"related_work,omitempty"`
	ExternalID           string              `json:"external_id"`
	Version              string              `json:"version"`
	Repository           string              `json:"repository"`
	URL                  string              `json:"url"`
	Title                string              `json:"title"`
	Reason               string              `json:"reason"`
	SubjectType          string              `json:"subject_type"`
	SubjectURL           string              `json:"subject_url"`
	LatestCommentURL     string              `json:"latest_comment_url"`
	Context              string              `json:"context"`
	SubjectTitle         string              `json:"subject_title,omitempty"`
	SubjectState         string              `json:"subject_state,omitempty"`
	ContextWarnings      []string            `json:"context_warnings,omitempty"`
}

// SubjectEvidence preserves source identity and author outside prose. Complete
// means the text was not clipped; it does not imply a complete thread history.
type SubjectEvidence struct {
	// ReviewState preserves GitHub's formal review result independently of prose.
	ReviewState string `json:"review_state,omitempty"`
	// ReviewBodyEmpty distinguishes citable state text from a reviewer quotation.
	ReviewBodyEmpty bool `json:"review_body_empty,omitempty"`
	// ViewerMention records a scan of the full API body for MentionViewer.
	MentionViewer string `json:"mention_viewer,omitempty"`
	ViewerMention *bool  `json:"viewer_mention,omitempty"`
	ID            string `json:"id"`
	URL           string `json:"url"`
	Actor         string `json:"actor"`
	Text          string `json:"text"`
	CreatedAt     string `json:"created_at,omitempty"`
	UpdatedAt     string `json:"updated_at"`
	Complete      bool   `json:"complete"`
	Current       bool   `json:"current"`
}
type Batch struct {
	Events     []Event
	Account    string
	Checkpoint string
	PollAfter  time.Duration
}

// Provider read operations can run concurrently for independent subjects.
type Provider interface {
	Validate(json.RawMessage) error
	CheckIdentity(context.Context, Source) error
	Collect(context.Context, Source, map[string]bool) (Batch, error)
	Enrich(context.Context, Source, Event) (Event, error)
}

// ToolResultLimit is the serialized tool-result budget used in model prompts.
const ToolResultLimit = 16000

type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}
type Tools interface {
	List(context.Context) ([]Tool, error)
	Call(context.Context, string, json.RawMessage) (string, error)
}
