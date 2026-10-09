package execution

import (
	"context"
	"encoding/json"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
)

func publicationTargetInstruction(c Config) string {
	facts, _ := json.Marshal(struct {
		Publisher   string `json:"publisher"`
		Provider    string `json:"provider"`
		ProviderURL string `json:"provider_url"`
		Repository  string `json:"repository"`
		Remote      string `json:"remote"`
		RemoteURL   string `json:"remote_url"`
		BaseBranch  string `json:"base_branch"`
	}{string(c.PublicationPolicy.Publisher), c.PR.Kind, c.PR.BaseURL, c.PR.Repository, c.Remote, c.publicationRemoteURL, c.Branch})
	return "\nFixed publication target:\n" + string(facts) + "\n"
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

func (c *Config) BindPublicationTarget(ctx context.Context, processHost executionbackend.ManagedProcessHost) error {
	url, err := (Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout), ProcessHost: processHost}).Output(ctx, c.Git, "remote", "get-url", c.Remote)
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
	push, err := (Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout), ProcessHost: processHost}).Output(ctx, c.Git, "remote", "get-url", "--push", "--all", c.Remote)
	if err != nil || strings.TrimSpace(string(push)) != target {
		return errors.New("publication requires one identical fetch/push remote URL")
	}
	if c.publicationRemoteURL != "" && target != c.publicationRemoteURL {
		return errors.New("publication remote differs from the fixed execution; original state is preserved")
	}
	c.publicationRemoteURL = target
	return nil
}

func publicationClient(c Config, processHost executionbackend.ManagedProcessHost) *publication.Client {
	return publication.NewClient(publication.Config(c.PR), Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout), ProcessHost: processHost}, duration(c.CommandTimeout))
}

func verifyPublicationTargetForRun(ctx context.Context, c Config, directory *ExecutionDirectory, processHost executionbackend.ManagedProcessHost, run *reconstructiontypes.RunView) error {
	entry, err := registrationFor(c, directory)
	if err != nil {
		return err
	}
	if run != nil && run.WorktreePath != nil && entry.LocalWorkspace {
		c.Repository = *run.WorktreePath
	}
	return c.BindPublicationTarget(ctx, processHost)
}

func fixedWorkflowPublicationConfig(c Config, w executionmodel.WorkflowRunRecord, directory *ExecutionDirectory) (Config, error) {
	if w.ExecutionProfile == nil || !VerifyFixedExecutionProfile(*w.ExecutionProfile, directory) || w.ConfigDigest != w.ExecutionProfile.AcceptedConfigVersion || w.Repository == "" || w.Branch != w.ExecutionProfile.Publication.BaseBranch || w.ExecutionProfile.Publication.RemoteURL == "" {
		return Config{}, errors.New("publication requires the original v6 execution snapshot; preserve original state")
	}
	c = c.ApplyFixedExecutionProfile(*w.ExecutionProfile, directory)
	c.Repository = w.Repository
	return copyWorkerFlowConfig(c), nil
}
