package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// NotificationPolicy contains explicit rules; arbitrary prose is never silently
// compiled into a rule. Direct mentions bypass suppression, including drafts.
// Free-text notification instructions still apply in semantic classification.
type NotificationPolicy struct {
	ExcludedRepositories []string `json:"excluded_repositories"`
	IgnoreDrafts         bool     `json:"ignore_drafts"`
	IgnoreApproved       bool     `json:"ignore_approved"`
	IgnoreOwnActivity    bool     `json:"ignore_own_activity"`
	BatchReviewTask      string   `json:"batch_review_task"`
}

func ParseNotificationPolicy(raw string) (NotificationPolicy, error) {
	var p NotificationPolicy
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if len(raw) > 8000 || strings.TrimSpace(raw) == "null" {
		return p, errors.New("Invalid notification preferences.")
	}
	if err := d.Decode(&p); err != nil {
		return p, errors.New("Invalid notification preferences.")
	}
	if d.Decode(new(any)) != io.EOF {
		return p, errors.New("Invalid notification preferences.")
	}
	if len(p.ExcludedRepositories) > 100 || len(p.BatchReviewTask) > 200 {
		return p, errors.New("Notification preferences exceed limits.")
	}
	for _, repo := range p.ExcludedRepositories {
		parts := strings.Split(repo, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(repo, " \t\n\r?#:") {
			return p, errors.New("Use repository names in owner/repository format.")
		}
	}
	return p, nil
}
