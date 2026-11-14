package notifications

import (
	"context"
	"database/sql"
	"errors"
	"kamacu/internal/monitor"
	"time"
)

type Config struct {
	ProfileID      *int64 `json:"profile_id"`
	Revision       int64  `json:"revision"`
	ContextBytes   int    `json:"context_bytes"`
	OutputTokens   int    `json:"output_tokens"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxReads       int    `json:"max_reads"`
}

func (s *Service) Config(ctx context.Context) (Config, error) {
	var c Config
	err := s.DB.QueryRowContext(ctx, `SELECT profile_id,revision,context_bytes,output_tokens,timeout_seconds,max_reads FROM notification_settings WHERE id=1`).Scan(&c.ProfileID, &c.Revision, &c.ContextBytes, &c.OutputTokens, &c.TimeoutSeconds, &c.MaxReads)
	return c, err
}
func (s *Service) SaveConfig(ctx context.Context, c Config) (Config, error) {
	if c.ContextBytes < 64000 || c.ContextBytes > 8000000 || c.OutputTokens < 1024 || c.OutputTokens > 64000 || c.TimeoutSeconds < 30 || c.TimeoutSeconds > 900 || c.MaxReads < 0 || c.MaxReads > 40 {
		return c, errors.New("notification model limits are outside supported ranges")
	}
	r, err := s.DB.ExecContext(ctx, `UPDATE notification_settings SET profile_id=?,context_bytes=?,output_tokens=?,timeout_seconds=?,max_reads=?,revision=revision+1 WHERE id=1 AND revision=?`, c.ProfileID, c.ContextBytes, c.OutputTokens, c.TimeoutSeconds, c.MaxReads, c.Revision)
	if err != nil {
		return c, err
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return c, ErrRulesChanged
	}
	return s.Config(ctx)
}
func (s *Service) notificationProfile(ctx context.Context) (monitor.Profile, error) {
	if frozen, ok := ctx.Value(notificationProfileKey{}).(monitor.Profile); ok {
		return frozen, nil
	}
	c, err := s.Config(ctx)
	if err != nil {
		return monitor.Profile{}, err
	}
	if c.ProfileID == nil {
		return monitor.Profile{}, errors.New("choose a notification processing model in Settings")
	}
	profiles, err := monitor.ListProfiles(ctx, s.DB)
	if err != nil {
		return monitor.Profile{}, err
	}
	for _, p := range profiles {
		if p.ID == *c.ProfileID {
			return p, nil
		}
	}
	return monitor.Profile{}, sql.ErrNoRows
}

type notificationProfileKey struct{}

func notificationCallContext(ctx context.Context, c Config) (context.Context, context.CancelFunc) {
	if c.TimeoutSeconds <= 0 {
		// Internal callers may supply only context/read limits. Their parent
		// context still controls cancellation; persisted settings are positive.
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, time.Duration(c.TimeoutSeconds)*time.Second)
}

func (s *Service) modelContext(ctx context.Context) (context.Context, context.CancelFunc, Config, error) {
	return s.notificationContext(ctx, false)
}

// Decision workflows include source collection, bounded tool/model rounds and
// one freshness pass. Each model call gets its own configured timeout; this
// outer limit bounds the entire workflow without spending later calls' time
// during earlier work. Single-call operations retain their original limit.
func (s *Service) decisionContext(ctx context.Context) (context.Context, context.CancelFunc, Config, error) {
	return s.notificationContext(ctx, true)
}

func (s *Service) notificationContext(ctx context.Context, decision bool) (context.Context, context.CancelFunc, Config, error) {
	c, err := s.Config(ctx)
	if err != nil {
		return nil, nil, c, err
	}
	p, err := s.notificationProfile(ctx)
	if err != nil {
		return nil, nil, c, err
	}
	ctx = context.WithValue(ctx, notificationProfileKey{}, p)
	ctx = monitor.WithGenerationOptions(ctx, monitor.GenerationOptions{OutputTokens: c.OutputTokens, Timeout: time.Duration(c.TimeoutSeconds) * time.Second})
	limit := time.Duration(c.TimeoutSeconds) * time.Second
	if decision {
		// MaxReads+4 model turns, MaxReads lookups, and setup/freshness.
		limit *= time.Duration(2*c.MaxReads + 6)
	}
	out, cancel := context.WithTimeout(ctx, limit)
	return out, cancel, c, nil
}

func (s *Service) TestModel(ctx context.Context) error {
	run, cancel, _, err := s.modelContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	profile, err := s.notificationProfile(run)
	if err != nil {
		return err
	}
	if s.Model == nil {
		return errors.New("notification model is unavailable")
	}
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"outcome", "reason", "rule_ids"}, "properties": map[string]any{"outcome": map[string]string{"type": "string"}, "reason": map[string]string{"type": "string"}, "rule_ids": map[string]any{"type": "array", "items": map[string]string{"type": "string"}}}}
	_, err = s.Model.GenerateValidatedJSON(run, profile, `Notification model connection test. Rule R001: ignore already-resolved merge requests. The PR is merged and the old comment asks 'Can this be merged?'. Return JSON {outcome:"ignore",reason:"...",rule_ids:["R001"]}. Do not ask the user to acknowledge the merge.`, schema, func(raw []byte) error {
		var out struct {
			Outcome string   `json:"outcome"`
			Reason  string   `json:"reason"`
			RuleIDs []string `json:"rule_ids"`
		}
		if err := strictRuleJSON(raw, &out); err != nil {
			return err
		}
		if out.Outcome != "ignore" || out.Reason == "" || len(out.RuleIDs) != 1 || out.RuleIDs[0] != "R001" {
			return errors.New("model did not follow the notification response contract")
		}
		return nil
	})
	return err
}
