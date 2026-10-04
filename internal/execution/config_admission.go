package execution

import (
	"context"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

func (c *Config) ValidateAdmission() error {
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return err
	}
	if c.WorkflowProtocol != executionmodel.Version {
		return nativeErr("unsupported_schema", "only worker-flow-v1 may start a service; preserve older execution state")
	}
	if err := validateWorkflowConfig(c, nil); err != nil {
		return err
	}
	if err := validateExecutionProfiles(*c); err != nil {
		return err
	}
	return c.ResolveIdentity(context.Background())
}
func (c *Config) ValidateTaskCommandConfiguration() error     { return validateWorkflowConfig(c, nil) }
func (c Config) Registration() (ExecutionRegistration, error) { return registrationFor(c) }
func (c Config) WorkspaceGit() workspace.Git                  { return workspaceGit(c) }
func (c Config) TaskRepositoryTarget(ctx context.Context) (string, error) {
	return configuredRepositoryTarget(ctx, c)
}
func (c Config) ValidateRepositoryTarget(ctx context.Context, authority executionmodel.ProjectTaskAuthority) error {
	return validateNativeConfiguredTarget(ctx, c, authority)
}
func (c Config) LoadedPolicySource() string     { return c.policySource }
func (c Config) FixedPublicationRemote() string { return c.publicationRemoteURL }
func (c Config) FixedProfileDigest() string     { return c.fixedProfileDigest }
