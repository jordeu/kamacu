package api

import (
	"time"
)

type activityTask struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	DoneAt       string `json:"doneAt"`
	InProgressAt string `json:"-"`
	InReviewAt   string `json:"-"`
	ProjectID    int64  `json:"projectId"`
	ProjectName  string `json:"projectName"`
}

type timeStat struct {
	N      int   `json:"n"`
	Min    int64 `json:"min"`
	Max    int64 `json:"max"`
	Median int64 `json:"median"`
}

type statsBlock struct {
	TaskCount       int      `json:"taskCount"`
	ReviewCount     int      `json:"reviewCount"`
	Cycle           timeStat `json:"cycle"`
	DwellInProgress timeStat `json:"dwellInProgress"`
	DwellInReview   timeStat `json:"dwellInReview"`
}

func parseActivityTime(s string) (time.Time, bool) {
	panic("not implemented")
}

func parseScope(raw string) (kind string, id int64, err error) {
	panic("not implemented")
}

func parseWindow(raw string) (time.Duration, error) {
	panic("not implemented")
}

func computeTimeStat(durations []time.Duration) timeStat {
	panic("not implemented")
}

func buildDurationSlices(tasks []activityTask) (cycle, dwellInProgress, dwellInReview []time.Duration) {
	panic("not implemented")
}

func rollupReviewState(states []string) string {
	panic("not implemented")
}
