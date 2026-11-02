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
func (g GitHub) api(ctx context.Context, host, path string) ([]byte, http.Header, error) {
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
	if err != nil {
		return nil, nil, errors.New("GitHub request failed; check gh installation, login, notification access and rate limits")
	}
	if len(raw) > 2<<20 {
		return nil, nil, errors.New("GitHub response too large")
	}
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(raw)), nil)
	if err != nil {
		return nil, nil, errors.New("invalid gh API response")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, resp.Header, fmt.Errorf("GitHub returned HTTP %d", resp.StatusCode)
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
	host := g.config(s).Hostname
	path, err := githubPath(host, event.SubjectURL)
	if err != nil {
		return event, err
	}
	// Subject and latest comment provide the concrete reason for attention. PR
	// inline comments are a separate API and must also be included.
	paths := []string{path}
	if event.LatestCommentURL != "" {
		p, e := githubPath(host, event.LatestCommentURL)
		if e != nil {
			return event, e
		}
		paths = append(paths, p)
	}
	if event.SubjectType == "PullRequest" {
		paths = append(paths, path+"/comments?sort=updated&direction=desc&per_page=30", path+"/reviews?per_page=30")
	}
	var context strings.Builder
	for _, p := range paths {
		raw, headers, e := g.api(ctx, host, p)
		// Reviews are oldest-first. Fetch the last page when available so new
		// review bodies are included even for long-lived PRs.
		if e == nil && strings.Contains(p, "/reviews?") {
			for _, part := range strings.Split(headers.Get("Link"), ",") {
				if strings.Contains(part, `rel="last"`) {
					target, _, ok := strings.Cut(strings.TrimSpace(part), ">")
					if ok {
						last, parseErr := githubPath(host, strings.TrimPrefix(target, "<"))
						if parseErr != nil {
							return event, parseErr
						}
						raw, _, e = g.api(ctx, host, last)
					}
				}
			}
		}
		if e != nil {
			return event, e
		}
		// Keep useful fields; omit large API link graphs and unrelated user metadata.
		var data any
		if e = json.Unmarshal(raw, &data); e != nil {
			return event, e
		}
		clipped := compactGitHub(data)
		body, _ := json.Marshal(clipped)
		if context.Len()+len(body) > 48000 {
			context.WriteString("[Additional context omitted because of size limits. Fetch full current feedback in the target agent session.]\n")
			break
		}
		context.WriteString(p + " (bounded excerpt; fetch full feedback before fixing)\n")
		context.Write(body)
		context.WriteByte('\n')
	}
	event.Context = context.String()
	return event, nil
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
