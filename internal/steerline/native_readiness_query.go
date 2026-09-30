package steerline

import (
	"context"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/task"
)

func nativeStartupBlockers(record TaskRecord, graph taskGraphSnapshot) []string {
	_, delivered := graph.Deliveries[record.TaskID]
	return task.StartupBlockers(record.Status, delivered, graph.Authority.DispatchPaused, graph.Authority.DispatchMode, graph.ExecutionSlotBusy)
}

func nativeDeliveryInconsistent(record TaskRecord, graph taskGraphSnapshot) bool {
	_, delivered := graph.Deliveries[record.TaskID]
	return task.DeliveryInconsistent(record.Status, delivered)
}

func taskDependencyState(record TaskRecord, graph taskGraphSnapshot) (bool, []TaskDependencyStatus) {
	return task.DependencyStateForTask(record, graph)
}

type NativeReadyTaskSnapshot struct {
	ProjectID        string       `json:"project_id"`
	AuthorityVersion uint64       `json:"authority_version"`
	GraphRevision    uint64       `json:"graph_revision"`
	DatabaseID       string       `json:"database_id"`
	DatabaseRevision uint64       `json:"database_revision"`
	SampledAt        time.Time    `json:"sampled_at"`
	Tasks            []TaskRecord `json:"tasks"`
}

// ReadyNativeTaskSnapshot returns the center's dispatch-order candidates and
// the database version sampled with them. The result is never a claim permit.
func (q QueryService) ReadyNativeTaskSnapshot(ctx context.Context) (NativeReadyTaskSnapshot, error) {
	graph, err := taskGraphFromStore(ctx, q.Store, q.Project)
	if err != nil {
		return NativeReadyTaskSnapshot{}, err
	}
	return nativeReadyTaskSnapshot(graph), nil
}

func nativeReadyTaskSnapshot(graph taskGraphSnapshot) NativeReadyTaskSnapshot {
	return NativeReadyTaskSnapshot{ProjectID: graph.Authority.ProjectIdentity, AuthorityVersion: graph.Authority.AuthorityVersion, GraphRevision: graph.Authority.GraphRevision, DatabaseID: graph.DatabaseID, DatabaseRevision: graph.DatabaseRevision, SampledAt: graph.SampledAt, Tasks: task.ReadyTasks(graph)}
}

// ReadyNativeTasks preserves the candidate API used by dispatch and control.
func (q QueryService) ReadyNativeTasks(ctx context.Context) ([]TaskRecord, error) {
	snapshot, err := q.ReadyNativeTaskSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return snapshot.Tasks, nil
}
