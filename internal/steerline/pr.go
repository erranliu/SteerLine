package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type PRInfo struct {
	URL          string     `json:"url"`
	Repository   string     `json:"repository"`
	BaseBranch   string     `json:"base_branch"`
	HeadRevision string     `json:"head_revision,omitempty"`
	State        string     `json:"state"`
	Merged       bool       `json:"merged"`
	MergedAt     *time.Time `json:"merged_at"`
	MergeCommit  *string    `json:"merge_commit"`
}
type PRClient struct {
	Config PRConfig
	Runner Runner
}

var ErrPRIdentity = errors.New("PR identity mismatch")

func (c PRConfig) ValidateRemote(remote string) error {
	if c.Kind != "gitea" && c.Kind != "command" {
		return errors.New("pr.kind must be gitea or command")
	}
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid PR base_url; credentials must use token_env")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(c.Repository) || strings.Contains(c.Repository, "..") {
		return errors.New("PR repository must be owner/name")
	}
	if strings.Contains(remote, "://") {
		r, e := url.Parse(remote)
		if e != nil {
			return e
		}
		hostMatches := strings.EqualFold(r.Host, u.Host)
		if r.Scheme == "ssh" {
			hostMatches = strings.EqualFold(r.Hostname(), u.Hostname())
		}
		expectedPath := strings.Trim(strings.TrimSuffix(u.Path, "/")+"/"+c.Repository, "/")
		if !hostMatches || strings.TrimSuffix(strings.TrimPrefix(r.Path, "/"), ".git") != expectedPath {
			return errors.New("configured PR host/repository does not match Git remote")
		}
	} else {
		parts := strings.SplitN(remote, ":", 2)
		if len(parts) != 2 {
			return errors.New("unsupported Git remote URL")
		}
		host := parts[0]
		if i := strings.LastIndex(host, "@"); i >= 0 {
			host = host[i+1:]
		}
		if !strings.EqualFold(host, u.Hostname()) || strings.TrimSuffix(parts[1], ".git") != c.Repository {
			return errors.New("configured PR host/repository does not match SSH remote")
		}
	}
	return nil
}
func (c PRConfig) PRNumber(raw string) (int, error) {
	u, e := url.Parse(raw)
	if e != nil {
		return 0, e
	}
	base, _ := url.Parse(c.BaseURL)
	if base == nil || !strings.EqualFold(u.Host, base.Host) || u.Scheme != base.Scheme || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return 0, errors.New("PR URL host/scheme or format mismatch")
	}
	prefix := strings.TrimSuffix(base.Path, "/") + "/" + c.Repository + "/pulls/"
	if c.Kind == "command" && strings.HasPrefix(u.Path, strings.TrimSuffix(base.Path, "/")+"/"+c.Repository+"/pull/") {
		prefix = strings.TrimSuffix(base.Path, "/") + "/" + c.Repository + "/pull/"
	}
	if !strings.HasPrefix(u.Path, prefix) {
		return 0, errors.New("PR URL repository/path mismatch")
	}
	tail := strings.TrimPrefix(u.Path, prefix)
	n, e := strconv.Atoi(tail)
	if e != nil || n <= 0 || strconv.Itoa(n) != tail {
		return 0, errors.New("invalid PR number")
	}
	return n, nil
}
func (c *PRClient) get(ctx context.Context, path string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.Runner.Timeout)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(c.Config.BaseURL, "/")+"/api/v1/"+path, nil)
	if e != nil {
		return e
	}
	if c.Config.TokenEnv != "" {
		token := os.Getenv(c.Config.TokenEnv)
		if token == "" {
			return fmt.Errorf("missing authentication environment variable %s", c.Config.TokenEnv)
		}
		req.Header.Set("Authorization", "token "+token)
	}
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("PR API redirects are not accepted") }}
	resp, e := client.Do(req)
	if e != nil {
		return fmt.Errorf("PR API request failed: %w", e)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PR API returned HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if e != nil {
		return e
	}
	return json.Unmarshal(b, out)
}
func (c *PRClient) Check(ctx context.Context) error {
	var repo struct {
		FullName string `json:"full_name"`
	}
	if e := c.get(ctx, "repos/"+c.Config.Repository, &repo); e != nil {
		return e
	}
	if repo.FullName != c.Config.Repository {
		return errors.New("Gitea repository identity mismatch")
	}
	return nil
}
func (c *PRClient) Query(ctx context.Context, raw string) (PRInfo, error) {
	n, e := c.Config.PRNumber(raw)
	if e != nil {
		return PRInfo{}, e
	}
	var info PRInfo
	if c.Config.Kind == "command" {
		args := make([]string, len(c.Config.Query.Args))
		for i, a := range c.Config.Query.Args {
			args[i] = strings.NewReplacer("{pr_url}", raw, "{number}", strconv.Itoa(n), "{repository}", c.Config.Repository).Replace(a)
		}
		out, e := c.Runner.Output(ctx, Command{Executable: c.Config.Query.Executable, Args: args})
		if e != nil {
			return info, e
		}
		if e = decode(out, &info); e != nil {
			return info, e
		}
	} else {
		var pr struct {
			URL         string     `json:"html_url"`
			Number      int        `json:"number"`
			State       string     `json:"state"`
			Merged      *bool      `json:"merged"`
			MergedAt    *time.Time `json:"merged_at"`
			MergeCommit *string    `json:"merge_commit_sha"`
			Base        struct {
				Ref  string `json:"ref"`
				Repo struct {
					FullName string `json:"full_name"`
				} `json:"repo"`
			} `json:"base"`
			Head struct {
				SHA string `json:"sha"`
			} `json:"head"`
		}
		if e = c.get(ctx, fmt.Sprintf("repos/%s/pulls/%d", c.Config.Repository, n), &pr); e != nil {
			return info, e
		}
		if pr.Merged == nil || pr.Number != n {
			return info, errors.New("PR response lacks explicit merge state or matching number")
		}
		if *pr.Merged && pr.State != "closed" {
			return info, errors.New("inconsistent Gitea merged/open state")
		}
		info = PRInfo{URL: pr.URL, Repository: pr.Base.Repo.FullName, BaseBranch: pr.Base.Ref, HeadRevision: pr.Head.SHA, State: pr.State, Merged: *pr.Merged, MergedAt: pr.MergedAt, MergeCommit: pr.MergeCommit}
		if info.Merged {
			info.State = "merged"
		} else if info.State == "closed" {
			info.State = "closed_unmerged"
		}
	}
	if info.URL != raw || info.Repository != c.Config.Repository || info.BaseBranch == "" {
		return info, ErrPRIdentity
	}
	if _, e = c.Config.PRNumber(info.URL); e != nil {
		return info, e
	}
	switch info.State {
	case "open", "closed_unmerged":
		if info.Merged {
			return info, errors.New("inconsistent PR merge state")
		}
	case "merged":
		if !info.Merged || info.MergedAt == nil || info.MergedAt.IsZero() || info.MergeCommit == nil || !commitHash.MatchString(*info.MergeCommit) {
			return info, errors.New("incomplete merge evidence")
		}
	case "unknown":
		return info, errors.New("PR provider returned unknown")
	default:
		return info, errors.New("unrecognized PR state")
	}
	return info, nil
}

// Close requests closure of the exact fixed PR and returns the provider's
// observed state. It never closes a merged PR.
func (c *PRClient) Close(ctx context.Context, raw string) (PRInfo, error) {
	number, err := c.Config.PRNumber(raw)
	if err != nil {
		return PRInfo{}, err
	}
	current, err := c.Query(ctx, raw)
	if err != nil {
		return PRInfo{}, err
	}
	if current.Merged || current.State == "merged" {
		return PRInfo{}, nativeErr("already_merged", "merged PR cannot be closed as discarded cleanup")
	}
	if current.State == "closed_unmerged" {
		return current, nil
	}
	if current.State != "open" {
		return PRInfo{}, nativeErr("merge_state_unknown", "PR must be verified open before cleanup closure")
	}
	var closed PRInfo
	if c.Config.Kind == "command" {
		if c.Config.Close.Executable == "" {
			return PRInfo{}, UnsupportedAdapterOperation("ClosePR", "command PR provider has no fixed close command")
		}
		args := make([]string, len(c.Config.Close.Args))
		for i, arg := range c.Config.Close.Args {
			args[i] = strings.NewReplacer("{pr_url}", raw, "{number}", strconv.Itoa(number), "{repository}", c.Config.Repository, "{state}", "closed").Replace(arg)
		}
		out, runErr := c.Runner.Output(ctx, Command{Executable: c.Config.Close.Executable, Args: args})
		if runErr != nil {
			return PRInfo{}, runErr
		}
		if err = decode(out, &closed); err != nil {
			return PRInfo{}, err
		}
	} else {
		endpoint := strings.TrimSuffix(c.Config.BaseURL, "/") + "/api/v1/repos/" + c.Config.Repository + "/pulls/" + strconv.Itoa(number)
		body, _ := json.Marshal(map[string]string{"state": "closed"})
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
		if requestErr != nil {
			return PRInfo{}, requestErr
		}
		request.Header.Set("Content-Type", "application/json")
		if c.Config.TokenEnv != "" {
			token := os.Getenv(c.Config.TokenEnv)
			if token == "" {
				return PRInfo{}, fmt.Errorf("missing authentication environment variable %s", c.Config.TokenEnv)
			}
			request.Header.Set("Authorization", "token "+token)
		}
		client := http.Client{Timeout: c.Runner.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("PR API redirects are not accepted") }}
		response, requestErr := client.Do(request)
		if requestErr != nil {
			return PRInfo{}, fmt.Errorf("PR close request outcome is unknown: %w", requestErr)
		}
		defer response.Body.Close()
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		if readErr != nil {
			return PRInfo{}, readErr
		}
		if response.StatusCode != http.StatusOK {
			return PRInfo{}, fmt.Errorf("PR close API returned HTTP %d", response.StatusCode)
		}
		var payload struct {
			URL         string     `json:"html_url"`
			Number      int        `json:"number"`
			State       string     `json:"state"`
			Merged      bool       `json:"merged"`
			MergedAt    *time.Time `json:"merged_at"`
			MergeCommit *string    `json:"merge_commit_sha"`
			Base        struct {
				Ref  string `json:"ref"`
				Repo struct {
					FullName string `json:"full_name"`
				} `json:"repo"`
			} `json:"base"`
			Head struct {
				SHA string `json:"sha"`
			} `json:"head"`
		}
		if readErr = json.Unmarshal(data, &payload); readErr != nil {
			return PRInfo{}, readErr
		}
		state := payload.State
		if payload.Merged {
			state = "merged"
		} else if state == "closed" {
			state = "closed_unmerged"
		}
		closed = PRInfo{URL: payload.URL, Repository: payload.Base.Repo.FullName, BaseBranch: payload.Base.Ref, HeadRevision: payload.Head.SHA, State: state, Merged: payload.Merged, MergedAt: payload.MergedAt, MergeCommit: payload.MergeCommit}
	}
	if closed.URL != raw || closed.Repository != c.Config.Repository || closed.BaseBranch != current.BaseBranch || closed.HeadRevision != current.HeadRevision || closed.Merged || closed.State != "closed_unmerged" {
		return PRInfo{}, nativeErr("merge_state_unknown", fmt.Sprintf("PR close response does not confirm closure of the fixed unmerged PR: got=%+v expected_url=%s expected_repository=%s expected_branch=%s expected_head=%s", closed, raw, c.Config.Repository, current.BaseBranch, current.HeadRevision))
	}
	return closed, nil
}
