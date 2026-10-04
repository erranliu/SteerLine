package execution

import (
	"context"
	"net/url"
	"strings"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
)

func configuredRepositoryTarget(ctx context.Context, c Config) (string, error) {
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout), ProcessHost: c.ProcessHost}
	if c.Remote == "" || c.Branch == "" {
		return "", nativeErr("authority_mismatch", "native project requires a fixed remote and branch")
	}
	remoteURL, err := r.Output(ctx, c.Git, "remote", "get-url", c.Remote)
	if err != nil {
		return "", nativeErr("authority_mismatch", "configured Git remote cannot be resolved")
	}
	target := strings.TrimSpace(string(remoteURL))
	if target == "" {
		return "", nativeErr("authority_mismatch", "configured Git remote resolved to an empty repository")
	}
	if err := publication.ValidateURL(target); err != nil {
		return "", err
	}
	if c.publicationRemoteURL != "" && target != c.publicationRemoteURL {
		return "", nativeErr("authority_mismatch", "Git remote differs from the original publication target")
	}
	if strings.Contains(target, "://") {
		parsed, parseErr := url.Parse(target)
		if parseErr != nil {
			return "", nativeErr("authority_mismatch", "configured Git remote URL is invalid")
		}
		if (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.User != nil {
			return "", nativeErr("authority_mismatch", "Git remote URL must not embed credentials")
		}
	}
	if c.PR.Repository != "" {
		if err := c.PR.ValidateRemote(target); err != nil {
			return "", nativeErr("authority_mismatch", "PR repository does not match the configured Git remote")
		}
	}
	return target, nil
}

func validateNativeConfiguredTarget(ctx context.Context, c Config, authority executionmodel.ProjectTaskAuthority) error {
	if c.Remote != authority.Remote || c.Branch != authority.Branch {
		return nativeErr("authority_mismatch", "configured remote or branch differs from the registered native project")
	}
	target, err := configuredRepositoryTarget(ctx, c)
	if err != nil {
		return err
	}
	if !sameRepositoryTarget(target, authority.Repository) {
		return nativeErr("authority_mismatch", "configured Git repository differs from the registered native project")
	}
	return nil
}

func sameRepositoryTarget(a, b string) bool {
	// A remote URL is not a Windows filesystem path: its repository component
	// may be case sensitive and must never be folded with canonical().
	return a == b
}
