package ports

import (
	"time"

	"github.com/erranliu/SteerLine/internal/runnerstate"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type NativeTaskCapabilities struct {
	DependencyReadiness string `json:"dependency_readiness"`
	DependencyBlockers  string `json:"dependency_blockers"`
	StartupBlockers     string `json:"startup_blockers"`
}

type NativeTaskDetail struct {
	taskmodel.TaskRecord
	ProjectID         string                           `json:"project_id"`
	Priority          int                              `json:"priority"`
	Contract          taskmodel.TaskContract           `json:"contract"`
	Candidates        []runnerstate.Candidate          `json:"candidates"`
	Delivery          *taskmodel.TaskDeliveryRecord    `json:"delivery,omitempty"`
	Predecessors      []taskmodel.TaskDependencyStatus `json:"predecessors"`
	Successors        []string                         `json:"successors"`
	DependencyReady   bool                             `json:"dependency_ready"`
	UnmetPredecessors []taskmodel.TaskDependencyStatus `json:"unmet_predecessors"`
	StartupBlockers   []string                         `json:"startup_blockers"`
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

// ProjectView holds task and caller-owned candidate facts sampled by one read
// transaction. Product projections and service observation are supplied later.
type ProjectView struct {
	SampledAt         time.Time
	Tasks             *NativeTaskPage
	Ready             *NativeReadyTaskSnapshot
	SelectedTask      *NativeTaskDetail
	Candidates        []runnerstate.Candidate
	TaskError         error
	SelectedTaskError error
	HistoryError      error
}
