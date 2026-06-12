// Package quota proxies Anthropic's undocumented OAuth usage endpoint as a
// strictly best-effort, demand-driven quota indicator. Kangent is a read-only
// passenger on ~/.claude/.credentials.json: the file is re-read on every poll,
// never written, and the token never outlives a single request, never appears
// in logs, and never crosses the Kangent API (the browser receives digested
// numbers only).
package quota

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"time"
)

// Window is one normalized usage window. The JSON tags ARE the /api/usage
// response contract — the frontend renders these fields verbatim.
type Window struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Utilization float64 `json:"utilization"`
	ResetsAt    *string `json:"resetsAt"` // ISO 8601 passthrough, CAN be null
}

// Result is the full /api/usage payload: a state enum plus last-good windows.
type Result struct {
	State     string     `json:"state"` // "ok" | "no_credentials" | "auth_expired" | "error"
	Stale     bool       `json:"stale"`
	FetchedAt *time.Time `json:"fetchedAt"` // last SUCCESSFUL fetch; null until first success
	Windows   []Window   `json:"windows"`   // null when no data to show
}

// credsFile decodes ONLY the two fields Kangent needs. The file also carries
// unrelated mcpOAuth third-party secrets — never decode, hold, or log the rest.
type credsFile struct {
	ClaudeAiOauth struct {
		AccessToken string `json:"accessToken"`
		ExpiresAt   int64  `json:"expiresAt"` // epoch MILLISECONDS (verified: 1781238586291)
	} `json:"claudeAiOauth"`
}

// errNoCredentials means there is nothing to poll with: file absent,
// unreadable, malformed, claudeAiOauth key absent, or empty token. This is a
// first-class state (API-key-only users), not an error to surface.
var errNoCredentials = errors.New("no oauth credentials")

// readCredentials extracts the access token and its expiry from the claude
// credentials file at path. Called fresh on every poll — claude rewrites the
// file constantly and rotates the token (~8h), so caching it would go stale.
func readCredentials(path string) (token string, expiresAt time.Time, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, errNoCredentials
	}
	var c credsFile
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", time.Time{}, errNoCredentials
	}
	if c.ClaudeAiOauth.AccessToken == "" {
		return "", time.Time{}, errNoCredentials
	}
	return c.ClaudeAiOauth.AccessToken, time.UnixMilli(c.ClaudeAiOauth.ExpiresAt), nil
}

// usageWindow is the lenient per-window upstream shape. Pointer fields:
// resets_at CAN be null and whole windows CAN be null (both observed live).
type usageWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    *string  `json:"resets_at"`
}

// windowOrder is the fixed preference order (D-75 — rows must not jump
// between polls); windowLabels carries the full short names (D-74).
var windowOrder = [...]string{"five_hour", "seven_day", "seven_day_opus", "seven_day_sonnet"}

var windowLabels = map[string]string{
	"five_hour":        "5h",
	"seven_day":        "7d",
	"seven_day_opus":   "Opus",
	"seven_day_sonnet": "Sonnet",
}

// normalizeWindows decodes the upstream usage body into ordered windows:
// known keys first in preference order, then remaining keys alphabetically
// with raw-key labels (D-74 passthrough). Whole-null windows, keys whose
// shape lacks utilization, and the literal "extra_usage" block (deferred
// QUOTA-FUT-01 — its shape may grow a utilization field someday) are skipped.
func normalizeWindows(body []byte) ([]Window, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	delete(m, "extra_usage")

	var out []Window
	emit := func(key string) {
		raw, ok := m[key]
		if !ok {
			return
		}
		delete(m, key)
		var uw usageWindow
		if err := json.Unmarshal(raw, &uw); err != nil || uw.Utilization == nil {
			return // null window, noise key, or non-window shape
		}
		label, ok := windowLabels[key]
		if !ok {
			label = key // raw-key passthrough for future windows
		}
		out = append(out, Window{
			Key:         key,
			Label:       label,
			Utilization: *uw.Utilization,
			ResetsAt:    uw.ResetsAt,
		})
	}

	for _, k := range windowOrder {
		emit(k)
	}
	rest := make([]string, 0, len(m))
	for k := range m {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	for _, k := range rest {
		emit(k)
	}
	return out, nil
}
