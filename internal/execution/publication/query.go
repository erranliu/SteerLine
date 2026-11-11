package publication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/model"
)

type QueryClient struct {
	Command interface {
		Output(context.Context, backend.Command, ...string) ([]byte, error)
	}
	Timeout time.Duration
}

// Query reads one bound PR. Diagnostics never include provider output or credentials.
func (c QueryClient) Query(ctx context.Context, p model.PRProviderConfig, url string) (model.PRInfo, error) {
	var info model.PRInfo
	number, err := p.PRNumber(url)
	if err != nil || p.ValidateRemote(strings.TrimRight(p.BaseURL, "/")+"/"+p.Repository+".git") != nil ||
		ValidateNonSecret(Config(p), backend.Command{}) != nil || c.Timeout <= 0 {
		return info, errors.New("invalid frozen PR query configuration")
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	switch p.Kind {
	case "command":
		if c.Command == nil || p.Query.Executable == "" {
			return info, errors.New("frozen PR query command is unavailable")
		}
		expand := strings.NewReplacer("{pr_url}", url, "{number}", strconv.Itoa(number), "{repository}", p.Repository)
		args := make([]string, len(p.Query.Args))
		for i, arg := range p.Query.Args {
			args[i] = expand.Replace(arg)
		}
		out, err := c.Command.Output(ctx, backend.Command{Executable: p.Query.Executable, Args: args})
		if err != nil {
			return info, errors.New("PR query command failed")
		}
		if err := backend.DecodeStrictJSON(out, &info, "trailing PR query data"); err != nil {
			return info, errors.New("invalid PR query response")
		}
	case "gitea":
		info, err = queryGitea(ctx, p, number)
		if err != nil {
			return info, err
		}
	}
	if info.URL != url || info.Repository != p.Repository || info.BaseBranch == "" {
		return model.PRInfo{}, errors.New("platform PR differs from the frozen target")
	}
	return info, nil
}

func queryGitea(ctx context.Context, p model.PRProviderConfig, number int) (model.PRInfo, error) {
	var info model.PRInfo
	url := fmt.Sprintf("%s/api/v1/repos/%s/pulls/%d", strings.TrimSuffix(p.BaseURL, "/"), p.Repository, number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return info, errors.New("invalid Gitea PR query target")
	}
	if p.TokenEnv != "" {
		token := os.Getenv(p.TokenEnv)
		if token == "" {
			return info, errors.New("Gitea PR query authentication is unavailable")
		}
		req.Header.Set("Authorization", "token "+token)
	}
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return info, errors.New("Gitea PR query failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("Gitea PR query returned HTTP %d", resp.StatusCode)
	}
	type ref struct {
		Branch   string `json:"ref"`
		Revision string `json:"sha"`
		Repo     struct {
			Name string `json:"full_name"`
		} `json:"repo"`
	}
	var pr struct {
		Body        string     `json:"body"`
		URL         string     `json:"html_url"`
		Number      int        `json:"number"`
		State       string     `json:"state"`
		Merged      *bool      `json:"merged"`
		MergedAt    *time.Time `json:"merged_at"`
		MergeCommit *string    `json:"merge_commit_sha"`
		Base, Head  ref
	}
	const max = 4 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil || len(raw) > max || json.Unmarshal(raw, &pr) != nil || pr.Number != number || pr.Merged == nil || (*pr.Merged && pr.State != "closed") {
		return info, errors.New("invalid Gitea PR query response")
	}
	state := pr.State
	if *pr.Merged {
		state = "merged"
	} else if state == "closed" {
		state = "closed_unmerged"
	}
	return model.PRInfo{Body: pr.Body, URL: pr.URL, Repository: pr.Base.Repo.Name, BaseBranch: pr.Base.Branch, HeadRepository: pr.Head.Repo.Name,
		HeadBranch: pr.Head.Branch, HeadRevision: pr.Head.Revision, State: state, Merged: *pr.Merged, MergedAt: pr.MergedAt, MergeCommit: pr.MergeCommit}, nil
}
