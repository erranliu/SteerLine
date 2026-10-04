package publication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func (c *Client) get(ctx context.Context, path string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
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

func (c *Client) Check(ctx context.Context) error {
	if c.Config.Kind != "gitea" {
		return executionbackend.UnsupportedAdapterOperation("CheckPRProvider", "repository preflight is only supported for Gitea")
	}
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
