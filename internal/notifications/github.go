package notifications

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type GitHubConfig struct {
	Hostname string `json:"hostname"`
}
type GitHub struct {
	// Run is injectable; production always executes argv directly, never a shell.
	Run func(context.Context, ...string) ([]byte, error)
	Now func() time.Time
}

func (g GitHub) Validate(raw json.RawMessage) error {
	var cfg GitHubConfig
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&cfg); err != nil {
		return errors.New("invalid GitHub configuration")
	}
	if cfg.Hostname != "" && cfg.Hostname != "github.com" {
		return errors.New("only github.com is supported by registered projects")
	}
	return nil
}
func (g GitHub) config(s Source) GitHubConfig {
	var c GitHubConfig
	_ = json.Unmarshal(s.Config, &c)
	if c.Hostname == "" {
		c.Hostname = "github.com"
	}
	return c
}

type limitedOutput struct{ bytes.Buffer }

func (b *limitedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 2<<20 {
		return 0, errors.New("GitHub response exceeds 2 MiB")
	}
	return b.Buffer.Write(p)
}

type GitHubHTTPError struct{ StatusCode int }

func (e *GitHubHTTPError) Error() string { return fmt.Sprintf("GitHub returned HTTP %d", e.StatusCode) }
func (e *GitHubHTTPError) Unwrap() error {
	if e.StatusCode == http.StatusNotFound || e.StatusCode == http.StatusGone {
		return ErrContextUnavailable
	}
	return nil
}

func (g GitHub) api(ctx context.Context, host, path string) (data []byte, headers http.Header, err error) {
	if err = ctx.Err(); err != nil {
		return nil, nil, err
	}
	cache, _ := ctx.Value(githubReadCacheKey{}).(*githubReadCache)
	key := host + "/" + strings.TrimPrefix(path, "/")
	cacheable := strings.HasPrefix(strings.TrimPrefix(path, "/"), "repos/")
	if cache != nil && cacheable {
		cache.Lock()
		v, ok := cache.reads[key]
		cache.Unlock()
		if ok {
			return bytes.Clone(v.data), v.headers.Clone(), nil
		}
	}
	err = notificationStage(ctx, "github_request", func() error {
		if run, ok := ctx.Value(notificationRunKey{}).(*notificationRun); ok {
			release, err := acquireNotificationSlot(ctx, run.github)
			if err != nil {
				return err
			}
			defer release()
		}
		var requestErr error
		data, headers, requestErr = g.apiRequest(ctx, host, path)
		return requestErr
	})
	if err == nil && cache != nil && cacheable {
		cache.Lock()
		if _, exists := cache.reads[key]; !exists && cache.bytes+len(data) <= 8<<20 && len(cache.reads) < 64 {
			cache.reads[key] = githubReadResult{data: bytes.Clone(data), headers: headers.Clone()}
			cache.bytes += len(data)
		}
		cache.Unlock()
	}
	return data, headers, err
}

func (g GitHub) apiRequest(ctx context.Context, host, path string) ([]byte, http.Header, error) {
	args := []string{"api", "--hostname", host, "--method", "GET", "--include", "-H", "Accept: application/vnd.github+json", path}
	var raw []byte
	var err error
	if g.Run != nil {
		raw, err = g.Run(ctx, args...)
	} else {
		cmd := exec.CommandContext(ctx, "gh", args...)
		var out limitedOutput
		cmd.Stdout = &out
		err = cmd.Run()
		raw = out.Bytes()
	}
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if len(raw) > 2<<20 {
		return nil, nil, errors.New("GitHub response too large")
	}
	// gh exits nonzero on HTTP errors but --include still emits the response.
	// Preserve the status so missing optional context can be distinguished from
	// authorization, rate-limit and transport failures without exposing stderr.
	resp, parseErr := http.ReadResponse(bufio.NewReader(bytes.NewReader(raw)), nil)
	if parseErr != nil {
		if err != nil {
			return nil, nil, errors.New("GitHub request failed; check gh installation, login, notification access and rate limits")
		}
		return nil, nil, errors.New("invalid gh API response")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.Header, &GitHubHTTPError{StatusCode: resp.StatusCode}
	}
	if err != nil {
		return nil, nil, errors.New("GitHub request failed; check gh installation, login, notification access and rate limits")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return data, resp.Header, err
}

// CheckIdentity revalidates credentials independently of the polling schedule.
func (g GitHub) CheckIdentity(ctx context.Context, s Source) error {
	if s.Account == "" {
		return errors.New("notification source has no bound GitHub account")
	}
	_, err := g.account(ctx, s)
	return err
}
func (g GitHub) account(ctx context.Context, s Source) (string, error) {
	raw, _, err := g.api(ctx, g.config(s).Hostname, "user")
	if err != nil {
		return "", err
	}
	var user struct {
		Login string `json:"login"`
	}
	if json.Unmarshal(raw, &user) != nil || user.Login == "" {
		return "", errors.New("cannot identify GitHub account")
	}
	if s.Account != "" && !strings.EqualFold(s.Account, user.Login) {
		return "", errors.New("active gh account changed; register a new source for this account")
	}
	return user.Login, nil
}
func (g GitHub) Collect(ctx context.Context, s Source, repos map[string]bool) (Batch, error) {
	cfg := g.config(s)
	now := time.Now()
	if g.Now != nil {
		now = g.Now()
	}
	batch := Batch{Checkpoint: now.UTC().Format(time.RFC3339), PollAfter: time.Minute}
	account, err := g.account(ctx, s)
	if err != nil {
		return batch, err
	}
	batch.Account = account
	since, err := time.Parse(time.RFC3339, s.Checkpoint)
	if err != nil {
		return batch, err
	}
	query := url.Values{"all": {"true"}, "since": {since.Add(-2 * time.Second).UTC().Format(time.RFC3339)}, "before": {batch.Checkpoint}, "per_page": {"100"}}
	path := "notifications?" + query.Encode()
	// A bounded scan leaves the checkpoint unchanged on overflow; never lose pages.
	for page := 0; path != ""; page++ {
		if page >= 100 {
			return batch, errors.New("notification scan exceeded 100 pages; checkpoint retained")
		}
		raw, h, err := g.api(ctx, cfg.Hostname, path)
		if err != nil {
			return batch, err
		}
		if seconds, _ := strconv.Atoi(h.Get("X-Poll-Interval")); seconds > 0 {
			batch.PollAfter = max(batch.PollAfter, time.Duration(seconds)*time.Second)
		}
		var items []struct {
			ID         string `json:"id"`
			Updated    string `json:"updated_at"`
			Reason     string `json:"reason"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
			Subject struct {
				Title  string `json:"title"`
				URL    string `json:"url"`
				Latest string `json:"latest_comment_url"`
				Type   string `json:"type"`
			} `json:"subject"`
		}
		if err = json.Unmarshal(raw, &items); err != nil {
			return batch, err
		}
		for _, item := range items {
			updated, e := time.Parse(time.RFC3339, item.Updated)
			if e != nil {
				return batch, e
			}
			// The overlap protects page/time boundaries, but never imports pre-enable activity.
			if updated.Before(time.UnixMilli(s.CreatedAt)) || !repos[strings.ToLower(item.Repository.FullName)] {
				continue
			}
			subjectPath, e := githubPath(cfg.Hostname, item.Subject.URL)
			if e != nil {
				continue
			}
			webPath := strings.Replace(subjectPath, "repos/", "", 1)
			webPath = strings.Replace(webPath, "/pulls/", "/pull/", 1)
			batch.Events = append(batch.Events, Event{ExternalID: item.ID, Version: item.Updated, Repository: item.Repository.FullName, URL: "https://" + cfg.Hostname + "/" + webPath, Title: item.Subject.Title, Reason: item.Reason, SubjectType: item.Subject.Type, SubjectURL: item.Subject.URL, LatestCommentURL: item.Subject.Latest})
		}
		path = ""
		for _, part := range strings.Split(h.Get("Link"), ",") {
			if strings.Contains(part, `rel="next"`) {
				a, b, ok := strings.Cut(strings.TrimSpace(part), ">")
				_ = b
				if ok {
					path, err = githubPath(cfg.Hostname, strings.TrimPrefix(a, "<"))
					if err != nil {
						return batch, err
					}
				}
			}
		}
	}
	return batch, nil
}
func githubPath(host, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" {
		return "", errors.New("invalid GitHub resource URL")
	}
	apiHost := host
	prefix := "/api/v3/"
	if host == "github.com" {
		apiHost = "api.github.com"
		prefix = "/"
	}
	if !strings.EqualFold(u.Host, apiHost) || !strings.HasPrefix(u.Path, prefix) {
		return "", errors.New("GitHub resource URL belongs to another host")
	}
	return strings.TrimPrefix(u.RequestURI(), prefix), nil
}
func (g GitHub) Enrich(ctx context.Context, s Source, event Event) (Event, error) {
	event.PRFacts = nil
	event.Context = ""
	event.SubjectState = ""
	event.SubjectTitle = ""
	event.SubjectUpdatedAt = ""
	event.ContextWarnings = nil
	event.Evidence = nil
	event.EvidenceComplete = true
	event.MentionScanComplete = true
	event.EvidenceObservedAt = time.Now()
	if g.Now != nil {
		event.EvidenceObservedAt = g.Now()
	}
	host := g.config(s).Hostname
	path, err := githubPath(host, event.SubjectURL)
	if err != nil {
		return event, err
	}
	paths := []string{path}
	if event.LatestCommentURL != "" {
		p, e := githubPath(host, event.LatestCommentURL)
		if e != nil {
			return event, e
		}
		if p != path {
			paths = append(paths, p)
		}
	}
	issuePath := strings.Replace(path, "/pulls/", "/issues/", 1)
	if event.SubjectType == "PullRequest" || event.SubjectType == "Issue" {
		paths = append(paths, issuePath+"/comments?per_page=100")
	}
	if event.SubjectType == "PullRequest" {
		paths = append(paths, path+"/comments?per_page=100", path+"/reviews?per_page=100")
	}
	var prose strings.Builder
	seen := map[string]bool{}
	evidenceBytes := 0
	for _, first := range paths {
		next := first
		for page := 0; next != ""; page++ {
			if page >= 10 || evidenceBytes >= 96000 {
				event.EvidenceComplete = false
				event.MentionScanComplete = false
				event.ContextWarnings = append(event.ContextWarnings, "Thread evidence exceeded its bounded retrieval budget; absence of a mention is unknown.")
				break
			}
			currentPath := next
			raw, h, e := g.api(ctx, host, currentPath)
			if e != nil {
				if !errors.Is(e, ErrContextUnavailable) {
					return event, e
				}
				event.EvidenceComplete = false
				event.MentionScanComplete = false
				event.ContextWarnings = append(event.ContextWarnings, fmt.Sprintf("Could not load %s: %s. Missing context is not evidence that the notification is resolved.", currentPath, e))
				if currentPath == path {
					return event, nil
				}
				break
			}
			var data any
			if e = json.Unmarshal(raw, &data); e != nil {
				return event, e
			}
			if currentPath == path {
				if subject, ok := data.(map[string]any); ok {
					event.SubjectTitle, _ = subject["title"].(string)
					event.SubjectState, _ = subject["state"].(string)
					event.SubjectUpdatedAt, _ = subject["updated_at"].(string)
					if merged, _ := subject["merged"].(bool); merged {
						event.SubjectState = "merged"
					}
				}
			}
			items := []any{data}
			if list, ok := data.([]any); ok {
				items = list
			}
			for _, item := range items {
				v, ok := item.(map[string]any)
				if !ok {
					continue
				}
				body, _ := v["body"].(string)
				link, _ := v["html_url"].(string)
				if link == "" {
					link = "https://" + host + "/" + strings.TrimPrefix(currentPath, "repos/")
				}
				if seen[link] {
					continue
				}
				seen[link] = true
				actor := ""
				if u, ok := v["user"].(map[string]any); ok {
					actor, _ = u["login"].(string)
				}
				updated, _ := v["updated_at"].(string)
				created, _ := v["created_at"].(string)
				if updated == "" {
					updated, _ = v["submitted_at"].(string)
				}
				reviewState := ""
				if strings.Contains(link, "#pullrequestreview-") || (event.SubjectType == "PullRequest" && strings.HasPrefix(currentPath, path+"/reviews")) {
					state, _ := v["state"].(string)
					switch state {
					case "APPROVED", "CHANGES_REQUESTED", "COMMENTED", "DISMISSED", "PENDING":
						reviewState = state
					}
				}
				// Reviews may have no body. Keep their verified result citable so
				// classification does not require invented prose or an empty quote.
				reviewBodyEmpty := strings.TrimSpace(body) == "" && reviewState != ""
				if reviewBodyEmpty {
					body = "GitHub review state: " + reviewState + "."
				}
				// Scan the complete API body before clipping model evidence.
				mentioned := textMentionsViewer(body, s.Account)
				complete := true
				if len(body) > 12000 {
					body = strings.ToValidUTF8(body[:12000], "")
					complete = false
					event.EvidenceComplete = false
				}
				if evidenceBytes+len(body) > 96000 {
					event.EvidenceComplete = false
					event.MentionScanComplete = false
					break
				}
				evidenceBytes += len(body)
				event.Evidence = append(event.Evidence, SubjectEvidence{ReviewState: reviewState, ReviewBodyEmpty: reviewBodyEmpty, MentionViewer: s.Account, ViewerMention: &mentioned, ID: link, URL: link, Actor: actor, Text: body, CreatedAt: created, UpdatedAt: updated, Complete: complete})
			}
			clipped, _ := json.Marshal(compactGitHub(data))
			if prose.Len()+len(clipped) < 48000 {
				prose.WriteString(currentPath + "\n")
				prose.Write(clipped)
				prose.WriteByte('\n')
			}
			next = ""
			for _, part := range strings.Split(h.Get("Link"), ",") {
				if strings.Contains(part, `rel="next"`) {
					target, _, ok := strings.Cut(strings.TrimSpace(part), ">")
					if ok {
						next, e = githubPath(host, strings.TrimPrefix(target, "<"))
						if e != nil {
							return event, e
						}
					}
				}
			}
		}
	}
	event.Context = prose.String()
	for i := range event.Evidence {
		event.Evidence[i].Current = scopeEvidence([]RawNotification{{Event: event}}, event, event.Evidence[i]) == evidenceCurrent
	}
	return event, nil
}

// SubjectVersion is a lightweight publication-time check for both issues and
// PRs. PR approval history is revalidated independently through PRContext.
func (g GitHub) SubjectVersion(ctx context.Context, s Source, event Event) (string, error) {
	path, err := githubPath(g.config(s).Hostname, event.SubjectURL)
	if err != nil {
		return "", err
	}
	raw, _, err := g.api(ctx, g.config(s).Hostname, path)
	if err != nil {
		return "", err
	}
	var subject struct {
		UpdatedAt string `json:"updated_at"`
	}
	if err = json.Unmarshal(raw, &subject); err != nil {
		return "", err
	}
	return subject.UpdatedAt, nil
}

func compactGitHub(v any) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, 0, len(x))
		for _, a := range x {
			out = append(out, compactGitHub(a))
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for _, k := range []string{"id", "title", "body", "state", "html_url", "created_at", "updated_at", "submitted_at", "path", "line", "diff_hunk", "in_reply_to_id", "commit_id"} {
			if a, ok := x[k]; ok {
				if text, ok := a.(string); ok && len([]rune(text)) > 6000 {
					a = string([]rune(text)[:6000]) + "\n[truncated]"
				}
				out[k] = a
			}
		}
		if user, ok := x["user"].(map[string]any); ok {
			out["author"] = user["login"]
		}
		return out
	default:
		return v
	}
}
