package execution

import (
	"context"

	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type transactionalSnapshotReader interface {
	Query(context.Context, string) (storageports.ExecutionSnapshot, error)
	TaskGraphSnapshot(context.Context, string) (storageports.TaskGraphSnapshot, error)
}

func readCommandSnapshot(ctx context.Context, store CommandStore, project string) (reconstruction.Snapshot, error) {
	if reader, ok := store.(storageports.ExecutionStateReader); ok {
		state, err := reader.ReadExecutionStateSnapshot(ctx, project)
		if err != nil {
			return reconstruction.Snapshot{}, err
		}
		return reconstruction.SnapshotFromStorage(state), nil
	}
	records, err := store.Query(ctx, project)
	if err != nil {
		return reconstruction.Snapshot{}, err
	}
	graph, err := store.TaskGraphSnapshot(ctx, project)
	if err != nil {
		return reconstruction.Snapshot{}, err
	}
	return reconstruction.SnapshotFromStorage(storageports.ExecutionStateSnapshot{Graph: graph, Records: records}), nil
}

func readServiceSnapshot(ctx context.Context, reader storageports.ExecutionStateReader, project string) (reconstruction.Snapshot, error) {
	state, err := reader.ReadExecutionStateSnapshot(ctx, project)
	if err != nil {
		return reconstruction.Snapshot{}, err
	}
	return reconstruction.SnapshotFromStorage(state), nil
}

func readTransactionalSnapshot(ctx context.Context, reader transactionalSnapshotReader, project string) (reconstruction.Snapshot, error) {
	records, err := reader.Query(ctx, project)
	if err != nil {
		return reconstruction.Snapshot{}, err
	}
	graph, err := reader.TaskGraphSnapshot(ctx, project)
	if err != nil {
		return reconstruction.Snapshot{}, err
	}
	return reconstruction.SnapshotFromStorage(storageports.ExecutionStateSnapshot{Graph: graph, Records: records}), nil
}
