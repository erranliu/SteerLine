package publication

import (
	"context"

	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"

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

// Create preserves the existing host-only GitHub draft PR capability.
// Gitea and arbitrary command providers do not gain host publication support.

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
