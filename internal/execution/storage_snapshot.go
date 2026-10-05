package execution

import (
	"context"

	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func readCommandSnapshot(ctx context.Context, reader storageports.TaskCandidateReader, project, taskID string) (storageports.TaskCandidateSnapshot, error) {
	return reader.ReadTaskCandidateSnapshot(ctx, project, taskID)
}

func readServiceSnapshot(ctx context.Context, reader storageports.ExecutionStateReader, project string) (reconstruction.Snapshot, error) {
	state, err := reader.ReadExecutionStateSnapshot(ctx, project)
	if err != nil {
		return reconstruction.Snapshot{}, err
	}
	return reconstruction.SnapshotFromStorage(state), nil
}
