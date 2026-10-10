package reconstruction

import (
	"encoding/json"
	"errors"
	"sort"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// Snapshot is execution's private reconstruction input. Persistence exposes
// task graph and execution records through separate consumer ports.
type Snapshot struct {
	ProjectIdentity string
	Tasks           []taskmodel.TaskRecord
	WorkflowRuns    []executionmodel.WorkflowRunRecord
	Executions      []executionmodel.ExecutionRecord
	Attempts        []executionmodel.StepAttemptRecord
	Decisions       []executionmodel.DecisionRecord
	Operations      []executionmodel.OperationRecord
	Deliveries      []executionmodel.TaskDeliveryRecord
}

func SnapshotFromStorage(input storageports.ExecutionStateSnapshot) Snapshot {
	deliveries := make([]executionmodel.TaskDeliveryRecord, 0, len(input.Graph.Deliveries))
	keys := make([]string, 0, len(input.Graph.Deliveries))
	for id := range input.Graph.Deliveries {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		deliveries = append(deliveries, input.Graph.Deliveries[id])
	}
	r := input.Records
	return Snapshot{ProjectIdentity: r.ProjectIdentity, Tasks: input.Graph.Tasks, WorkflowRuns: r.WorkflowRuns, Executions: r.Executions, Attempts: r.Attempts, Decisions: r.Decisions, Operations: r.Operations, Deliveries: deliveries}
}

// Metadata combines service-owned checkpoint fields with the independently read task authority.
// It contains no configuration, database handle, actor or previously projected Run.
type Metadata struct {
	Checkpoint        storageports.ServiceCheckpoint
	DispatchMode      string
	DispatchPaused    bool
	AuthorityRevision uint64
}

func MetadataOf(v Runtime) Metadata {
	checkpoint := Checkpoint(v)
	checkpoint.SchemaVersion = v.SchemaVersion
	return Metadata{Checkpoint: checkpoint, DispatchMode: v.Control.DispatchMode, DispatchPaused: v.Control.DispatchPaused, AuthorityRevision: v.Control.Revision}
}

func Checkpoint(v Runtime) storageports.ServiceCheckpoint {
	var reserved *string
	if unclaimedReservation(v) {
		reserved = v.CurrentTask
	}
	checkpoint := storageports.ServiceCheckpoint{SchemaVersion: storageports.WorkerFlowRuntimeSchema, Revision: v.Revision,
		UpdatedAt: v.UpdatedAt, HeartbeatAt: v.HeartbeatAt,
		Service:    storageports.CheckpointServiceState{Mode: v.Service.Mode, InstanceID: v.Service.InstanceID, PID: v.Service.PID, Status: v.Service.Status, Claimant: v.Service.Claimant, Error: copyString(v.Service.Error), ProcessIdentity: copyValue(v.Service.ProcessIdentity), StartupID: copyString(v.Service.StartupID), ConfigDigest: v.Service.ConfigDigest, Ready: v.Service.Ready, ControlProtocol: v.Service.ControlProtocol, Capabilities: append([]string(nil), v.Service.Capabilities...), HeartbeatIntervalMS: v.Service.HeartbeatIntervalMS},
		Repository: storageports.CheckpointRepository{Path: v.Repository.Path, Remote: v.Repository.Remote, Branch: v.Repository.Branch, Identity: v.Repository.Identity}, ReservedTask: reserved}
	if operation := v.Control.Operation; operation != nil {
		checkpoint.Operation = &storageports.CheckpointControlOperation{RequestID: operation.RequestID, Action: operation.Action, TaskID: operation.TaskID, Payload: append(operation.Payload[:0:0], operation.Payload...), Phase: operation.Phase, RunID: operation.RunID, AttemptID: operation.AttemptID, DecisionID: operation.DecisionID, RequestDigest: operation.RequestDigest, Result: copyValue(operation.Result)}
	}
	for _, blocker := range v.Control.Blockers {
		checkpoint.Blockers = append(checkpoint.Blockers, storageports.CheckpointControlBlocker{RequestID: blocker.RequestID, Action: blocker.Action, TaskID: blocker.TaskID, RunID: blocker.RunID, AttemptID: blocker.AttemptID, DecisionID: blocker.DecisionID, Reason: blocker.Reason})
	}
	return checkpoint
}

func RuntimeFromCheckpoint(checkpoint storageports.ServiceCheckpoint, dispatchMode string, dispatchPaused bool, authorityRevision uint64) Runtime {
	state := checkpoint.Service
	service := ServiceState{Mode: state.Mode, InstanceID: state.InstanceID, PID: state.PID, Status: state.Status, Claimant: state.Claimant, Error: copyString(state.Error), ProcessIdentity: copyValue(state.ProcessIdentity), StartupID: copyString(state.StartupID), ConfigDigest: state.ConfigDigest, Ready: state.Ready, ControlProtocol: state.ControlProtocol, Capabilities: append([]string(nil), state.Capabilities...), HeartbeatIntervalMS: state.HeartbeatIntervalMS}
	repository := Repository{Path: checkpoint.Repository.Path, Remote: checkpoint.Repository.Remote, Branch: checkpoint.Repository.Branch, Identity: checkpoint.Repository.Identity}
	control := ControlState{DispatchMode: dispatchMode, DispatchPaused: dispatchPaused, Revision: authorityRevision, Blockers: []ControlBlocker{}}
	if checkpoint.Operation != nil {
		op := checkpoint.Operation
		control.Operation = &ControlOperation{RequestID: op.RequestID, Action: op.Action, TaskID: op.TaskID, Payload: append([]byte(nil), op.Payload...), Phase: op.Phase, RunID: op.RunID, AttemptID: op.AttemptID, DecisionID: op.DecisionID, RequestDigest: op.RequestDigest, Result: copyValue(op.Result)}
	}
	for _, blocker := range checkpoint.Blockers {
		control.Blockers = append(control.Blockers, ControlBlocker{RequestID: blocker.RequestID, Action: blocker.Action, TaskID: blocker.TaskID, RunID: blocker.RunID, AttemptID: blocker.AttemptID, DecisionID: blocker.DecisionID, Reason: blocker.Reason})
	}
	return Runtime{SchemaVersion: checkpoint.SchemaVersion, Revision: checkpoint.Revision, UpdatedAt: checkpoint.UpdatedAt, HeartbeatAt: checkpoint.HeartbeatAt, Service: service, Repository: repository, CurrentTask: copyString(checkpoint.ReservedTask), SlotOccupied: checkpoint.ReservedTask != nil, Control: control}
}

func copyString(value *string) *string { return copyValue(value) }

func copyValue[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// CloneDomain isolates nested records and retains the control revision stored outside JSON payloads.
func CloneSnapshot(snapshot Snapshot) (Snapshot, error) {
	var owned Snapshot
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
func Build(snapshot Snapshot, metadata Metadata) (Runtime, error) {
	owned, err := CloneSnapshot(snapshot)
	if err != nil {
		return Runtime{}, err
	}
	data, err := json.Marshal(metadata.Checkpoint)
	if err != nil {
		return Runtime{}, err
	}
	var checkpoint storageports.ServiceCheckpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return Runtime{}, err
	}
	view := RuntimeFromCheckpoint(checkpoint, metadata.DispatchMode, metadata.DispatchPaused, metadata.AuthorityRevision)
	view.Tasks, view.WorkflowRuns, view.Executions = owned.Tasks, owned.WorkflowRuns, owned.Executions
	view.Attempts, view.Decisions, view.Operations, view.Deliveries = owned.Attempts, owned.Decisions, owned.Operations, owned.Deliveries
	view.Runs, err = Runs(owned)
	if err != nil {
		return Runtime{}, err
	}
	view.SlotOccupied, view.CurrentTask = Slot(view)
	return view, nil
}

// BuildFromStorage reconstructs service state from execution-owned reads. It
// never goes through product query pages or uses a prior in-memory Runtime.
func BuildFromStorage(snapshot storageports.ExecutionStateSnapshot, checkpoint storageports.ServiceCheckpoint) (Runtime, error) {
	if snapshot.Records.ProjectIdentity == "" || snapshot.Graph.Authority.ProjectIdentity != snapshot.Records.ProjectIdentity || checkpoint.Repository.Identity != snapshot.Records.ProjectIdentity {
		return Runtime{}, errors.New("execution snapshot, task authority and checkpoint identities differ")
	}
	return Build(SnapshotFromStorage(snapshot), Metadata{Checkpoint: checkpoint, DispatchMode: snapshot.Graph.Authority.DispatchMode, DispatchPaused: snapshot.Graph.Authority.DispatchPaused, AuthorityRevision: snapshot.Graph.Authority.AuthorityVersion})
}

// Runs computes fresh projections without copying an actor's entire working snapshot.
// The center alone decides whether to apply these values to existing private Run pointers.
func Runs(snapshot Snapshot) ([]*RunView, error) {
	runs := make([]*RunView, 0, len(snapshot.WorkflowRuns))
	for i := range snapshot.WorkflowRuns {
		workflow := &snapshot.WorkflowRuns[i]
		var task taskmodel.TaskRecord
		found := false
		for _, record := range snapshot.Tasks {
			if record.TaskID == workflow.TaskID {
				task, found = record, true
				break
			}
		}
		if !found {
			return nil, errors.New("workflow has no task record")
		}
		if task.Status == "discarded" && workflow.Status == "abandoned" {
			continue
		}
		execution := LatestExecution(snapshot.Executions, workflow.WorkflowRunID)
		if workflow.SelectedResult != "" {
			execution = nil
			for i := range snapshot.Executions {
				e := &snapshot.Executions[i]
				if e.WorkflowRunID == workflow.WorkflowRunID && e.ExecutionID == workflow.SelectedResult {
					execution = e
					break
				}
			}
		}
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

func Slot(v Runtime) (bool, *string) {
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

func unclaimedReservation(v Runtime) bool {
	return v.SlotOccupied && v.CurrentTask != nil && LatestWorkflow(v.WorkflowRuns, *v.CurrentTask) == nil
}
