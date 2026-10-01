package steerline

import (
	"context"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	"strings"
)

func publicationTargetInstruction(c Config) string {
	return fmt.Sprintf("\nFixed publication responsibility: %s. Provider: %s %s/%s. Git remote: %s (%s). Target branch: %s. Use these original targets throughout this execution; report a mismatch before publishing.\n", c.PublicationPolicy.Publisher, c.PR.Kind, c.PR.BaseURL, c.PR.Repository, c.Remote, c.publicationRemoteURL, c.Branch)
}

func (c Config) publicationSettings() contracts.PublicationSettings {
	refs := []string{"SSH_AUTH_SOCK", "GIT_CONFIG_GLOBAL", "GIT_SSH_COMMAND"}
	if c.PR.Kind == "command" {
		refs = append(refs, "GH_TOKEN", "GITHUB_TOKEN", "GH_CONFIG_DIR")
	}
	if c.PR.TokenEnv != "" {
		refs = append(refs, c.PR.TokenEnv)
	}
	return contracts.PublicationSettings{Provider: contracts.PRProviderConfig(c.PR), Create: publication.CreateDescription(c.PublicationPolicy, c.PR), Git: c.Git, Remote: c.Remote, RemoteURL: c.publicationRemoteURL, BaseBranch: c.Branch, AuthenticationRefs: refs}
}

func (c *Config) bindPublicationTarget(ctx context.Context) error {
	url, err := (Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}).Output(ctx, c.Git, "remote", "get-url", c.Remote)
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
	push, err := (Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}).Output(ctx, c.Git, "remote", "get-url", "--push", "--all", c.Remote)
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
	return publication.NewClient(c.PR, Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}, duration(c.CommandTimeout))
}

func verifyPublicationTargetForRun(ctx context.Context, c Config, run *Run) error {
	if run != nil && run.WorktreePath != nil && c.hostName() != "orca_supervised" {
		c.Repository = *run.WorktreePath
	}
	return c.bindPublicationTarget(ctx)
}

func fixedWorkflowPublicationConfig(c Config, w WorkflowRunRecord) (Config, error) {
	if w.ExecutionProfile == nil || !verifyFixedExecutionProfile(*w.ExecutionProfile) || w.ConfigDigest != w.ExecutionProfile.AcceptedConfigVersion || w.Repository == "" || w.Branch != w.ExecutionProfile.Publication.BaseBranch || w.ExecutionProfile.Publication.RemoteURL == "" {
		return Config{}, errors.New("publication requires the original v4 execution snapshot; preserve original state")
	}
	c = c.ApplyFixedExecutionProfile(*w.ExecutionProfile)
	c.Repository = w.Repository
	return copyWorkerFlowConfig(c), nil
}
