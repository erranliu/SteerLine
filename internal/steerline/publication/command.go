package publication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"os"
	"strings"
)

// Redact the referenced credential even when a command echoes it in an error.
func (c *Client) command(ctx context.Context, cmd contracts.Command, args ...string) ([]byte, error) {
	out, err := c.Runner.Output(ctx, cmd, args...)
	if err != nil {
		return nil, c.redact(err)
	}
	return out, nil
}

func (c *Client) Find(ctx context.Context, request Request, head string) (string, error) {
	if err := c.validateHostRequest(request); err != nil {
		return "", err
	}
	repo := c.Config.Repository
	owner, _, ok := strings.Cut(repo, "/")
	if !ok || owner == "" {
		return "", errors.New("local GitHub publication repository is invalid")
	}
	gh := contracts.Command{Executable: c.Config.Query.Executable}

	endpoint := fmt.Sprintf("repos/%s/pulls?state=all&head=%s:%s&per_page=100", repo, owner, request.Workspace.Branch)
	out, err := c.command(ctx, gh, "api", endpoint)
	if err != nil {
		return "", fmt.Errorf("query exact branch PR before publication: %w", err)
	}
	var prs []struct {
		URL   string `json:"html_url"`
		State string `json:"state"`
		Head  struct {
			Ref  string `json:"ref"`
			SHA  string `json:"sha"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if err = json.Unmarshal(out, &prs); err != nil {
		return "", fmt.Errorf("decode exact branch PR query: %w", err)
	}
	var existing string
	for _, pr := range prs {
		if pr.Head.Ref != request.Workspace.Branch || pr.Head.Repo.FullName != repo || pr.Base.Ref != request.BaseBranch {
			continue
		}
		if pr.Head.SHA != head || pr.State != "open" || existing != "" {
			return "", errors.New("fixed branch already has an incompatible or ambiguous PR")
		}
		existing = pr.URL
	}
	if existing != "" {
		if _, err := c.Config.PRNumber(existing); err != nil {
			return "", err
		}
		return existing, nil
	}
	return "", nil
}

// Create preserves the existing host-only GitHub draft PR capability.
// Gitea and arbitrary command providers do not gain host publication support.
func (c *Client) Create(ctx context.Context, request Request, head string) (string, error) {
	if err := c.validateHostRequest(request); err != nil {
		return "", err
	}
	existing, err := c.Find(ctx, request, head)
	if err != nil || existing != "" {
		return existing, err
	}
	created, err := c.command(ctx, contracts.Command{Executable: c.Config.Query.Executable}, "pr", "create", "--repo", c.Config.Repository, "--base", request.BaseBranch,
		"--head", request.Workspace.Branch, "--draft", "--title", "SteerLine task "+request.TaskID, "--body", "SteerLine worker-flow run "+request.RunID)
	if err != nil {
		return "", fmt.Errorf("create draft PR from fixed branch: %w", err)
	}
	url := strings.TrimSpace(string(created))
	if _, err = c.Config.PRNumber(url); err != nil {
		return "", fmt.Errorf("created PR URL is invalid: %w", err)
	}
	return url, nil
}
func (c *Client) validateHostRequest(request Request) error {
	if err := request.Validate(); err != nil {
		return executionapi.UnsupportedAdapterOperation("PublishPR", err.Error())
	}
	if !request.Provider.Matches(c.Config) {
		return errors.New("PR provider differs from the fixed publication request")
	}
	return nil
}

func (c *Client) redact(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, name := range []string{c.Config.TokenEnv, "GH_TOKEN", "GITHUB_TOKEN", "GITEA_TOKEN"} {
		if name != "" {
			if token := os.Getenv(name); token != "" {
				message = strings.ReplaceAll(message, token, "[REDACTED]")
			}
		}
	}
	if message == err.Error() {
		return err
	}
	return errors.New(message)
}
