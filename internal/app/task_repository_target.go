package app

import (
	"context"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"net/url"
	"strings"
	"time"
)

func (c Config) TaskRepositoryTarget(ctx context.Context) (string, error) {
	r := commandRunner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second), ProcessHost: c.ProcessHost}
	if c.Remote == "" || c.Branch == "" {
		return "", taskError("authority_mismatch", "native project requires a fixed remote and branch")
	}
	remoteURL, err := r.Output(ctx, c.Git, "config", "--get", "remote."+c.Remote+".url")
	if err != nil {
		return "", taskError("authority_mismatch", "configured Git remote cannot be resolved")
	}
	target := strings.TrimSpace(string(remoteURL))
	if target == "" {
		return "", taskError("authority_mismatch", "configured Git remote resolved to an empty repository")
	}
	if err := publication.ValidateURL(target); err != nil {
		return "", err
	}
	if strings.Contains(target, "://") {
		parsed, parseErr := url.Parse(target)
		if parseErr != nil {
			return "", taskError("authority_mismatch", "configured Git remote URL is invalid")
		}
		if (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.User != nil {
			return "", taskError("authority_mismatch", "Git remote URL must not embed credentials")
		}
	}
	if c.PR.Repository != "" {
		if err := c.PR.ValidateRemote(target); err != nil {
			return "", taskError("authority_mismatch", "PR repository does not match the configured Git remote")
		}
	}
	return target, nil
}
