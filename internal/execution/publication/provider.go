package publication

import (
	"context"
	"errors"
	"fmt"
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
