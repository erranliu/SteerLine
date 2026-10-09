package codexworker

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
)

// Workspace preparation uses the same background effect/receipt path as
// session preparation. An uncertain original effect is inspected, never replayed.
func (l ExecutionLifecycle) prepareWorkspace(ctx context.Context, request executionbackend.LifecycleRequest, reconcile bool) (executionbackend.LifecycleReceipt, error) {
	receipt := executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown}
	intent := request.Input.Workspace
	if intent == nil || *intent != request.Snapshot.WorkspacePlan || intent.Path == "" || l.setup.ProcessHost == nil {
		receipt.Disposition = executionbackend.OperationRejected
		return receipt, errors.New("workspace operation differs from its original fixed intent")
	}
	timeout := l.setup.CommandTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	git := workspace.Git{Command: executionbackend.LaunchCommand{Executable: l.setup.GitExecutable, Args: append([]string(nil), l.setup.GitArgs...)}, Timeout: timeout, Start: l.setup.ProcessHost.StartManaged}
	repository := workspace.Repository{Path: l.setup.RepositoryPath, CommonDir: request.Snapshot.Session.ProjectID}
	if reconcile {
		inspection, err := workspace.Inspect(ctx, git, repository, intent)
		if err != nil {
			return receipt, err
		}
		receipt.Disposition, receipt.Workspace = executionbackend.OperationConfirmed, &inspection
		return receipt, nil
	}
	prepared, err := workspace.Prepare(ctx, git, repository, filepath.Dir(intent.Path), intent)
	receipt.Disposition, receipt.Workspace = prepared.Status, prepared.Inspection
	return receipt, err
}
