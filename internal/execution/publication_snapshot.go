package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/execution/publication"
)

func publicationTargetInstruction(c Config) string {
	return fmt.Sprintf("\nFixed publication responsibility: %s. Provider: %s %s/%s. Git remote: %s (%s). Target branch: %s. Use these original targets throughout this execution; report a mismatch before publishing.\n", c.PublicationPolicy.Publisher, c.PR.Kind, c.PR.BaseURL, c.PR.Repository, c.Remote, c.publicationRemoteURL, c.Branch)
}

func (c Config) publicationSettings() executionmodel.PublicationSettings {
	refs := []string{"SSH_AUTH_SOCK", "GIT_CONFIG_GLOBAL", "GIT_SSH_COMMAND"}
	if c.PR.Kind == "command" {
		refs = append(refs, "GH_TOKEN", "GITHUB_TOKEN", "GH_CONFIG_DIR")
	}
	if c.PR.TokenEnv != "" {
		refs = append(refs, c.PR.TokenEnv)
	}
	return executionmodel.PublicationSettings{Provider: executionmodel.PRProviderConfig(c.PR), Git: c.Git, Remote: c.Remote, RemoteURL: c.publicationRemoteURL, BaseBranch: c.Branch, AuthenticationRefs: refs}
}

func (c *Config) BindPublicationTarget(ctx context.Context) error {
	url, err := (Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout), ProcessHost: c.ProcessHost}).Output(ctx, c.Git, "remote", "get-url", c.Remote)
	if err != nil {
		return errors.New("publication remote cannot be read; original state is preserved")
	}
	target := strings.TrimSpace(string(url))
	if target == "" {
		return errors.New("publication remote is empty")
	}
	if err := publication.ValidateURL(target); err != nil {
		return err
	}
	push, err := (Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout), ProcessHost: c.ProcessHost}).Output(ctx, c.Git, "remote", "get-url", "--push", "--all", c.Remote)
	if err != nil || strings.TrimSpace(string(push)) != target {
		return errors.New("publication requires one identical fetch/push remote URL")
	}
	if c.publicationRemoteURL != "" && target != c.publicationRemoteURL {
		return errors.New("publication remote differs from the fixed execution; original state is preserved")
	}
	c.publicationRemoteURL = target
	return nil
}

func publicationClient(c Config) *publication.Client {
	return publication.NewClient(c.PR, Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout), ProcessHost: c.ProcessHost}, duration(c.CommandTimeout))
}

func verifyPublicationTargetForRun(ctx context.Context, c Config, run *contracts.Run) error {
	entry, err := registrationFor(c)
	if err != nil {
		return err
	}
	if run != nil && run.WorktreePath != nil && entry.LocalWorkspace {
		c.Repository = *run.WorktreePath
	}
	return c.BindPublicationTarget(ctx)
}

func fixedWorkflowPublicationConfig(c Config, w executionmodel.WorkflowRunRecord) (Config, error) {
	if w.ExecutionProfile == nil || !VerifyFixedExecutionProfile(*w.ExecutionProfile, c.ExecutionDirectory) || w.ConfigDigest != w.ExecutionProfile.AcceptedConfigVersion || w.Repository == "" || w.Branch != w.ExecutionProfile.Publication.BaseBranch || w.ExecutionProfile.Publication.RemoteURL == "" {
		return Config{}, errors.New("publication requires the original v6 execution snapshot; preserve original state")
	}
	c = c.ApplyFixedExecutionProfile(*w.ExecutionProfile)
	c.Repository = w.Repository
	return copyWorkerFlowConfig(c), nil
}
