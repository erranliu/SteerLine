package ports

import (
	"encoding/json"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type NativeTaskCapabilities struct {
	DependencyReadiness string `json:"dependency_readiness"`
	DependencyBlockers  string `json:"dependency_blockers"`
	StartupBlockers     string `json:"startup_blockers"`
}

type NativeTaskDetail struct {
	taskmodel.TaskRecord
	ProjectID         string                             `json:"project_id"`
	Priority          int                                `json:"priority"`
	Contract          taskmodel.TaskContract             `json:"contract"`
	WorkflowRuns      []executionmodel.WorkflowRunRecord `json:"workflow_runs"`
	Delivery          *executionmodel.TaskDeliveryRecord `json:"delivery,omitempty"`
	Predecessors      []taskmodel.TaskDependencyStatus   `json:"predecessors"`
	Successors        []string                           `json:"successors"`
	DependencyReady   bool                               `json:"dependency_ready"`
	UnmetPredecessors []taskmodel.TaskDependencyStatus   `json:"unmet_predecessors"`
	StartupBlockers   []string                           `json:"startup_blockers"`
}

type NativeTaskPage struct {
	Protocol         string                 `json:"protocol"`
	ProjectID        string                 `json:"project_id"`
	AuthorityVersion uint64                 `json:"authority_version"`
	GraphRevision    uint64                 `json:"graph_revision"`
	DatabaseRevision uint64                 `json:"database_revision"`
	DatabaseID       string                 `json:"database_id"`
	SampledAt        time.Time              `json:"sampled_at"`
	Capabilities     NativeTaskCapabilities `json:"capabilities"`
	Tasks            []NativeTaskDetail     `json:"tasks"`
	NextCursor       string                 `json:"next_cursor,omitempty"`
	HasMore          bool                   `json:"has_more"`
}

type NativeReadyTaskSnapshot struct {
	ProjectID        string                 `json:"project_id"`
	AuthorityVersion uint64                 `json:"authority_version"`
	GraphRevision    uint64                 `json:"graph_revision"`
	DatabaseID       string                 `json:"database_id"`
	DatabaseRevision uint64                 `json:"database_revision"`
	SampledAt        time.Time              `json:"sampled_at"`
	Tasks            []taskmodel.TaskRecord `json:"tasks"`
}

// ProjectView holds product-ready read values sampled by one SQLite read
// transaction. Service observation is supplied separately by the host.
type ProjectView struct {
	SampledAt         time.Time
	Tasks             *NativeTaskPage
	Ready             *NativeReadyTaskSnapshot
	SelectedTask      *NativeTaskDetail
	History           *api.HistorySummary
	TaskError         error
	SelectedTaskError error
	HistoryError      error
}

func (p NativeTaskPage) MarshalJSON() ([]byte, error) {
	type plain NativeTaskPage
	body, err := json.Marshal(plain(p))
	if err != nil {
		return nil, err
	}
	return api.EscapeBidiJSON(body), nil
}
