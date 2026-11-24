// Package inboxagent applies explicit user rules to existing Inbox messages.
package inboxagent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"kamacu/internal/monitor"
)

var ErrChanged = errors.New("inbox agent settings changed; refresh and try again")
var ErrUnavailable = errors.New("choose an inbox model and enable at least one inbox rule")

type Rule struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Enabled bool   `json:"enabled"`
}
type Config struct {
	ProfileID      *int64 `json:"profile_id"`
	Revision       int64  `json:"revision"`
	ContextBytes   int    `json:"context_bytes"`
	OutputTokens   int    `json:"output_tokens"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Rules          []Rule `json:"rules"`
}

func (s *Service) Config(ctx context.Context) (Config, error) {
	var c Config
	var raw string
	err := s.DB.QueryRowContext(ctx, `SELECT profile_id,revision,context_bytes,output_tokens,timeout_seconds,rules FROM inbox_agent_settings WHERE id=1`).Scan(&c.ProfileID, &c.Revision, &c.ContextBytes, &c.OutputTokens, &c.TimeoutSeconds, &raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &c.Rules)
	}
	return c, err
}
func (s *Service) SaveConfig(ctx context.Context, c Config) (Config, error) {
	if c.ContextBytes < 64000 || c.ContextBytes > 8000000 || c.OutputTokens < 1024 || c.OutputTokens > 64000 || c.TimeoutSeconds < 30 || c.TimeoutSeconds > 900 {
		return c, errors.New("inbox model limits are outside supported ranges")
	}
	if len(c.Rules) > 100 {
		return c, errors.New("at most 100 inbox rules")
	}
	seen := map[string]bool{}
	for i := range c.Rules {
		r := &c.Rules[i]
		r.Text = strings.TrimSpace(r.Text)
		if r.ID == "" {
			r.ID = "I-" + uuid.NewString()
		}
		if seen[r.ID] || len(r.ID) > 100 || r.Text == "" || len(r.Text) > 8000 || !utf8.ValidString(r.Text) {
			return c, errors.New("rules require unique IDs and nonempty text up to 8000 bytes")
		}
		seen[r.ID] = true
	}
	if c.Rules == nil {
		c.Rules = []Rule{}
	}
	if c.ProfileID != nil {
		if _, err := s.profile(ctx, *c.ProfileID); err != nil {
			return c, errors.New("inbox model profile is unavailable")
		}
	}
	raw, _ := json.Marshal(c.Rules)
	r, err := s.DB.ExecContext(ctx, `UPDATE inbox_agent_settings SET profile_id=?,revision=revision+1,context_bytes=?,output_tokens=?,timeout_seconds=?,rules=? WHERE id=1 AND revision=?`, c.ProfileID, c.ContextBytes, c.OutputTokens, c.TimeoutSeconds, string(raw), c.Revision)
	if err != nil {
		return c, err
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return c, ErrChanged
	}
	return s.Config(ctx)
}
func (s *Service) profile(ctx context.Context, id int64) (monitor.Profile, error) {
	profiles, err := monitor.ListProfiles(ctx, s.DB)
	if err != nil {
		return monitor.Profile{}, err
	}
	for _, p := range profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return monitor.Profile{}, sql.ErrNoRows
}
func enabled(c Config) bool {
	for _, r := range c.Rules {
		if r.Enabled {
			return true
		}
	}
	return false
}
