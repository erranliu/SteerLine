package execution

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
)

// ValidateAdmission validates the selected immutable execution profile. Task
// authority, project file identity and the profile catalog are admitted by app.
func (c Config) ValidateAdmission(directory *ExecutionDirectory) error {
	if c.WorkflowProtocol != executionmodel.Version {
		return nativeErr("unsupported_schema", "only worker-flow-v1 may start a service; preserve older execution state")
	}
	return validateWorkflowConfig(c, directory)
}

func (c Config) ValidateTaskCommandConfiguration(directory *ExecutionDirectory) error {
	return validateWorkflowConfig(c, directory)
}

func (c Config) Registration(directory *ExecutionDirectory) (ExecutionRegistration, error) {
	return registrationFor(c, directory)
}

func (c Config) WorkspaceGit(processHost executionbackend.ManagedProcessHost) workspace.Git {
	return workspaceGit(c, processHost)
}

func (c Config) TaskRepositoryTarget(ctx context.Context, processHost executionbackend.ManagedProcessHost) (string, error) {
	return configuredRepositoryTarget(ctx, c, processHost)
}

func (c Config) ValidateRepositoryTarget(ctx context.Context, authority executionmodel.ProjectTaskAuthority, processHost executionbackend.ManagedProcessHost) error {
	return validateNativeConfiguredTarget(ctx, c, authority, processHost)
}
