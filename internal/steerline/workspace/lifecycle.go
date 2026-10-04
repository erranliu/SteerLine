package workspace

import (
	"context"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// InspectLifecycleWorkspace verifies a host-local workspace against its original
// committed binding. Both local execution adapters use the same Git owner.
func InspectLifecycleWorkspace(ctx context.Context, setup executionbackend.LifecycleSetup, snapshot executionbackend.LifecycleSnapshot) (executionbackend.WorkspaceInspection, error) {
	ref := snapshot.Ref
	if ref.Incomplete || ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || !commitPattern.MatchString(ref.BaseCommit) || setup.ProcessHost == nil || snapshot.Session.ProjectID == "" {
		return executionbackend.WorkspaceInspection{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "workspace ref is incomplete"}
	}
	timeout := setup.CommandTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	git := Git{Command: executionbackend.LaunchCommand{Executable: setup.GitExecutable, Args: append([]string(nil), setup.GitArgs...)}, Timeout: timeout, Start: setup.ProcessHost.StartManaged}
	return Inspect(ctx, git, Repository{Path: setup.RepositoryPath, CommonDir: snapshot.Session.ProjectID}, &executionbackend.WorkspaceIntent{Path: ref.WorkspacePath, Branch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit})
}
