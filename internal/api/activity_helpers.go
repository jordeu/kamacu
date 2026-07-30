package api

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
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
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseScope(raw string) (kind string, id int64, err error) {
	if raw == "" || raw == "global" {
		return "global", 0, nil
	}
	parts := strings.SplitN(raw, ":", 2)
	prefix := parts[0]
	if prefix != "workspace" && prefix != "project" {
		return "", 0, fmt.Errorf("activity: invalid scope %q", raw)
	}
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("activity: scope %q missing id", raw)
	}
	id, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("activity: scope %q has non-numeric id", raw)
	}
	if id < 0 {
		return "", 0, fmt.Errorf("activity: scope %q has negative id", raw)
	}
	return prefix, id, nil
}

func parseWindow(raw string) (time.Duration, error) {
	switch raw {
	case "", "week":
		return 7 * 24 * time.Hour, nil
	case "month":
		return 30 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("activity: invalid window %q (want week|month)", raw)
	}
}

func computeTimeStat(durations []time.Duration) timeStat {
	if len(durations) == 0 {
		return timeStat{}
	}
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	n := len(sorted)
	var med time.Duration
	if n%2 == 1 {
		med = sorted[n/2]
	} else {
		med = (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return timeStat{
		N:      n,
		Min:    int64(sorted[0].Seconds()),
		Max:    int64(sorted[n-1].Seconds()),
		Median: int64(med.Seconds()),
	}
}

func buildDurationSlices(tasks []activityTask) (cycle, dwellInProgress, dwellInReview []time.Duration) {
	for _, tk := range tasks {
		done, okDone := parseActivityTime(tk.DoneAt)
		if !okDone {
			continue
		}
		inProg, okProg := parseActivityTime(tk.InProgressAt)
		inRev, okRev := parseActivityTime(tk.InReviewAt)
		if okProg {
			cycle = append(cycle, done.Sub(inProg))
		}
		if okRev {
			dwellInReview = append(dwellInReview, done.Sub(inRev))
		}
		if okRev && okProg {
			dwellInProgress = append(dwellInProgress, inRev.Sub(inProg))
		} else if okProg {
			dwellInProgress = append(dwellInProgress, done.Sub(inProg))
		}
	}
	return cycle, dwellInProgress, dwellInReview
}

func rollupReviewState(states []string) string {
	if len(states) == 0 {
		return "ok"
	}
	hasOK := false
	hasOther := false
	for _, s := range states {
		if s == "ok" {
			hasOK = true
		} else {
			hasOther = true
		}
	}
	if !hasOther {
		return "ok"
	}
	if hasOK {
		return "partial"
	}
	for _, prio := range []string{"error", "auth_required", "no_gh"} {
		for _, s := range states {
			if s == prio {
				return prio
			}
		}
	}
	return "error"
}
