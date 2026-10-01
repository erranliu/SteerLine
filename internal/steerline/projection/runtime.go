package projection

import (
	"encoding/json"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

// Metadata combines service-owned checkpoint fields with the independently read task authority.
// It contains no configuration, database handle, actor or previously projected Run.
type Metadata struct {
	Checkpoint        contracts.ServiceCheckpoint
	DispatchMode      string
	DispatchPaused    bool
	AuthorityRevision uint64
}

func MetadataOf(v contracts.Runtime) Metadata {
	checkpoint := Checkpoint(v)
	checkpoint.SchemaVersion = v.SchemaVersion
	return Metadata{Checkpoint: checkpoint, DispatchMode: v.Control.DispatchMode, DispatchPaused: v.Control.DispatchPaused, AuthorityRevision: v.Control.Revision}
}

func Checkpoint(v contracts.Runtime) contracts.ServiceCheckpoint {
	var reserved *string
	if unclaimedReservation(v) {
		reserved = v.CurrentTask
	}
	return contracts.ServiceCheckpoint{SchemaVersion: contracts.WorkerFlowRuntimeSchema, Revision: v.Revision,
		UpdatedAt: v.UpdatedAt, HeartbeatAt: v.HeartbeatAt, Service: v.Service,
		Repository: v.Repository, Operation: v.Control.Operation, Blockers: v.Control.Blockers, ReservedTask: reserved}
}

// CloneDomain isolates nested records and retains the control revision stored outside JSON payloads.
func CloneDomain(snapshot contracts.DomainSnapshot) (contracts.DomainSnapshot, error) {
	var owned contracts.DomainSnapshot
	data, err := json.Marshal(snapshot)
	if err == nil {
		err = json.Unmarshal(data, &owned)
	}
	if err == nil {
		for i := range snapshot.WorkflowRuns {
			owned.WorkflowRuns[i].ControlRevision = snapshot.WorkflowRuns[i].ControlRevision
		}
	}
	return owned, err
}

// Build creates a fresh, independently owned Runtime from one already-collected snapshot.
// Existing client views cannot fill missing records or supply execution identity.
func Build(snapshot contracts.DomainSnapshot, metadata Metadata) (contracts.Runtime, error) {
	owned, err := CloneDomain(snapshot)
	if err != nil {
		return contracts.Runtime{}, err
	}
	data, err := json.Marshal(metadata.Checkpoint)
	if err != nil {
		return contracts.Runtime{}, err
	}
	var checkpoint contracts.ServiceCheckpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return contracts.Runtime{}, err
	}
	view := contracts.RuntimeFromCheckpoint(checkpoint)
	view.Control.DispatchMode, view.Control.DispatchPaused, view.Control.Revision = metadata.DispatchMode, metadata.DispatchPaused, metadata.AuthorityRevision
	view.Tasks, view.WorkflowRuns, view.Executions = owned.Tasks, owned.WorkflowRuns, owned.Executions
	view.Attempts, view.Decisions, view.Operations, view.Deliveries = owned.Attempts, owned.Decisions, owned.Operations, owned.Deliveries
	view.Runs, err = Runs(owned)
	if err != nil {
		return contracts.Runtime{}, err
	}
	view.SlotOccupied, view.CurrentTask = Slot(view)
	return view, nil
}

// Runs computes fresh projections without copying an actor's entire working snapshot.
// The center alone decides whether to apply these values to existing private Run pointers.
func Runs(snapshot contracts.DomainSnapshot) ([]*contracts.Run, error) {
	runs := make([]*contracts.Run, 0, len(snapshot.WorkflowRuns))
	for _, task := range snapshot.Tasks {
		workflow := LatestWorkflow(snapshot.WorkflowRuns, task.TaskID)
		if workflow == nil {
			continue
		}
		if task.Status == "discarded" && workflow.Phase == "abandoned" {
			continue
		}
		execution := LatestExecution(snapshot.Executions, workflow.WorkflowRunID)
		if execution == nil {
			return nil, errors.New("workflow has no execution record")
		}
		view, err := Run(task, *workflow, *execution, snapshot.Attempts, snapshot.Decisions, snapshot.Deliveries)
		if err != nil {
			return nil, err
		}
		runs = append(runs, &view)
	}
	return runs, nil
}

func Slot(v contracts.Runtime) (bool, *string) {
	occupied := v.Control.Operation != nil || len(v.Control.Blockers) != 0
	var tasks []string
	for _, run := range v.Runs {
		if run.ExecutionStatus == "unknown" || run.ExecutionStatus == "pending" || run.ExecutionStatus == "running" || run.FinishTaskStatus == "waiting_decision" {
			occupied = true
			tasks = append(tasks, run.TaskID)
		}
	}
	current := v.CurrentTask
	if unclaimedReservation(v) {
		occupied = true
	} else {
		current = nil
		if len(tasks) == 1 {
			current = ptr(tasks[0])
		}
	}
	return occupied, current
}
func unclaimedReservation(v contracts.Runtime) bool {
	return v.SlotOccupied && v.CurrentTask != nil && LatestWorkflow(v.WorkflowRuns, *v.CurrentTask) == nil
}
