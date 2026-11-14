package notifications

import (
	"context"
	"strings"
	"time"
)

// Progress is live, process-local telemetry. Durable batch status remains the
// authority for completion and failures; progress never writes to the database.
type Progress struct {
	Stage          string `json:"stage"`
	Completed      int    `json:"completed"`
	Total          int    `json:"total"`
	StageStartedAt int64  `json:"stage_started_at"`
	GitHubRequests int64  `json:"github_requests"`
	ModelPasses    int64  `json:"model_passes"`
	Lookups        int    `json:"lookups"`
}

func (s *Service) setProcessingStage(ctx context.Context, stage string, total int) {
	run, ok := ctx.Value(notificationRunKey{}).(*notificationRun)
	if !ok || strings.HasPrefix(run.batchID, "preview:") {
		return
	}
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	if s.progressRun != run {
		s.progress = Progress{}
		s.progressRun = run
	}
	s.progress.Stage = stage
	s.progress.Completed = 0
	s.progress.Total = total
	s.progress.StageStartedAt = time.Now().UnixMilli()
}

func (s *Service) advanceProcessing(ctx context.Context) {
	run, _ := ctx.Value(notificationRunKey{}).(*notificationRun)
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	if run != nil && s.progressRun == run && s.progress.Completed < s.progress.Total {
		s.progress.Completed++
	}
}

func (s *Service) processingLookup(ctx context.Context) {
	run, _ := ctx.Value(notificationRunKey{}).(*notificationRun)
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	if run != nil && s.progressRun == run {
		s.progress.Lookups++
	}
}

func (s *Service) processingProgress(p Processing) *Progress {
	if p.Status != "running" {
		return nil
	}
	s.progressMu.RLock()
	defer s.progressMu.RUnlock()
	if s.progressRun == nil || s.progressRun.batchID != p.ID {
		return nil
	}
	progress := s.progress
	progress.GitHubRequests = s.progressRun.requests.Load()
	progress.ModelPasses = s.progressRun.rounds.Load()
	return &progress
}
