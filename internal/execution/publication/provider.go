package publication

import (
	"bytes"
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

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// CommandRunner is supplied by the host. No service or business state crosses this boundary.
type CommandRunner interface {
	Output(context.Context, executionbackend.Command, ...string) ([]byte, error)
}

type Provider interface {
	Query(context.Context, string) (executionmodel.PRInfo, error)
	Close(context.Context, string) (executionmodel.PRInfo, error)
}

type Client struct {
	Config  Config
	Runner  CommandRunner
	Timeout time.Duration
}

func NewClient(config Config, runner CommandRunner, timeout time.Duration) *Client {
	return &Client{Config: config, Runner: runner, Timeout: timeout}
}

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func providerError(code, message string) error { return &Error{Code: code, Message: message} }

func (c *Client) validateKind(operation string) error {
	switch c.Config.Kind {
	case "command", "gitea":
		return nil
	default:
		return executionbackend.UnsupportedAdapterOperation(operation, "configured PR provider kind is unsupported")
	}
}

func (c *Client) Query(ctx context.Context, raw string) (executionmodel.PRInfo, error) {
	if err := c.validateKind("QueryPR"); err != nil {
		return executionmodel.PRInfo{}, err
	}
	n, e := c.Config.PRNumber(raw)
	if e != nil {
		return executionmodel.PRInfo{}, e
	}
	var info executionmodel.PRInfo
	if c.Config.Kind == "command" {
		args := make([]string, len(c.Config.Query.Args))
		for i, a := range c.Config.Query.Args {
			args[i] = strings.NewReplacer("{pr_url}", raw, "{number}", strconv.Itoa(n), "{repository}", c.Config.Repository).Replace(a)
		}
		out, e := c.command(ctx, executionbackend.Command{Executable: c.Config.Query.Executable, Args: args})
		if e != nil {
			return info, e
		}
		if e = decodeStrictJSON(out, &info); e != nil {
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
				Ref  string `json:"ref"`
				SHA  string `json:"sha"`
				Repo struct {
					FullName string `json:"full_name"`
				} `json:"repo"`
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
		info = executionmodel.PRInfo{URL: pr.URL, Repository: pr.Base.Repo.FullName, BaseBranch: pr.Base.Ref, HeadRepository: pr.Head.Repo.FullName, HeadBranch: pr.Head.Ref, HeadRevision: pr.Head.SHA, State: pr.State, Merged: *pr.Merged, MergedAt: pr.MergedAt, MergeCommit: pr.MergeCommit}
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
func (c *Client) Close(ctx context.Context, raw string) (executionmodel.PRInfo, error) {
	if err := c.validateKind("ClosePR"); err != nil {
		return executionmodel.PRInfo{}, err
	}
	number, err := c.Config.PRNumber(raw)
	if err != nil {
		return executionmodel.PRInfo{}, err
	}
	current, err := c.Query(ctx, raw)
	if err != nil {
		return executionmodel.PRInfo{}, err
	}
	if current.Merged || current.State == "merged" {
		return executionmodel.PRInfo{}, providerError("already_merged", "merged PR cannot be closed as discarded cleanup")
	}
	if current.State == "closed_unmerged" {
		return current, nil
	}
	if current.State != "open" {
		return executionmodel.PRInfo{}, providerError("merge_state_unknown", "PR must be verified open before cleanup closure")
	}
	var closed executionmodel.PRInfo
	if c.Config.Kind == "command" {
		if c.Config.Close.Executable == "" {
			return executionmodel.PRInfo{}, executionbackend.UnsupportedAdapterOperation("ClosePR", "command PR provider has no fixed close command")
		}
		args := make([]string, len(c.Config.Close.Args))
		for i, arg := range c.Config.Close.Args {
			args[i] = strings.NewReplacer("{pr_url}", raw, "{number}", strconv.Itoa(number), "{repository}", c.Config.Repository, "{state}", "closed").Replace(arg)
		}
		out, runErr := c.command(ctx, executionbackend.Command{Executable: c.Config.Close.Executable, Args: args})
		if runErr != nil {
			return executionmodel.PRInfo{}, runErr
		}
		if err = decodeStrictJSON(out, &closed); err != nil {
			return executionmodel.PRInfo{}, err
		}
	} else {
		endpoint := strings.TrimSuffix(c.Config.BaseURL, "/") + "/api/v1/repos/" + c.Config.Repository + "/pulls/" + strconv.Itoa(number)
		body, _ := json.Marshal(map[string]string{"state": "closed"})
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
		if requestErr != nil {
			return executionmodel.PRInfo{}, requestErr
		}
		request.Header.Set("Content-Type", "application/json")
		if c.Config.TokenEnv != "" {
			token := os.Getenv(c.Config.TokenEnv)
			if token == "" {
				return executionmodel.PRInfo{}, fmt.Errorf("missing authentication environment variable %s", c.Config.TokenEnv)
			}
			request.Header.Set("Authorization", "token "+token)
		}
		client := http.Client{Timeout: c.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("PR API redirects are not accepted") }}
		response, requestErr := client.Do(request)
		if requestErr != nil {
			return executionmodel.PRInfo{}, fmt.Errorf("PR close request outcome is unknown: %w", requestErr)
		}
		defer response.Body.Close()
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		if readErr != nil {
			return executionmodel.PRInfo{}, readErr
		}
		if response.StatusCode != http.StatusOK {
			return executionmodel.PRInfo{}, fmt.Errorf("PR close API returned HTTP %d", response.StatusCode)
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
				Ref  string `json:"ref"`
				SHA  string `json:"sha"`
				Repo struct {
					FullName string `json:"full_name"`
				} `json:"repo"`
			} `json:"head"`
		}
		if readErr = json.Unmarshal(data, &payload); readErr != nil {
			return executionmodel.PRInfo{}, readErr
		}
		state := payload.State
		if payload.Merged {
			state = "merged"
		} else if state == "closed" {
			state = "closed_unmerged"
		}
		closed = executionmodel.PRInfo{URL: payload.URL, Repository: payload.Base.Repo.FullName, BaseBranch: payload.Base.Ref, HeadRepository: payload.Head.Repo.FullName, HeadBranch: payload.Head.Ref, HeadRevision: payload.Head.SHA, State: state, Merged: payload.Merged, MergedAt: payload.MergedAt, MergeCommit: payload.MergeCommit}
	}
	if closed.URL != raw || closed.Repository != c.Config.Repository || closed.BaseBranch != current.BaseBranch || closed.HeadRevision != current.HeadRevision || closed.Merged || closed.State != "closed_unmerged" {
		return executionmodel.PRInfo{}, providerError("merge_state_unknown", "PR close response does not confirm closure of the fixed unmerged PR")
	}
	return closed, nil
}
