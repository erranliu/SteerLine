package reconstruction

import (
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

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
	if v.SlotOccupied && v.CurrentTask != nil {
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
