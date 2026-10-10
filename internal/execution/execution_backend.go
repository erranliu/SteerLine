package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func executionStopperFor(c Config, directory *ExecutionDirectory) (executionbackend.ExecutionStopper, error) {
	entry, err := registrationFor(c, directory)
	if err != nil {
		return nil, err
	}
	stopper, ok := entry.Backend(c).(executionbackend.ExecutionStopper)
	if !ok {
		return nil, unsupported("StopExecution")
	}
	return stopper, nil
}

func unsupported(name string) error {
	return &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: name + " is not supported by this backend"}
}

func evidenceDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func digestValue(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type executionRecords struct {
	root      string
	reader    ExecutionRecordReader
	resources ResourcePort
}

func (s *Service) readRunWorkspace(ctx context.Context, run *reconstructiontypes.RunView) (executionbackend.WorkspaceInspection, error) {
	if run == nil || run.WorkflowVersion != executionmodel.Version {
		return executionbackend.WorkspaceInspection{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "workspace inspection requires its fixed worker-flow-v2 identity"}
	}
	w, err := s.inspectWorkflowWorkspace(ctx, run)
	return executionbackend.WorkspaceInspection{Repository: w.Repository, Worktree: w.Worktree, Branch: w.Branch, Base: w.Base, Head: w.Head, Dirty: w.Dirty, CheckedAt: w.VerifiedAt}, err
}

func (s *Service) inspectRunProcess(run *reconstructiontypes.RunView) string {
	if run == nil || run.Process == nil {
		return "execution identity unavailable"
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return "fixed execution identity unavailable: " + err.Error()
	}
	if fixed.execution.Process == nil || *fixed.execution.Process != *run.Process {
		return "execution binding conflicts with runtime identity"
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return "execution binding invalid: " + err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), positiveDuration(fixed.config.CommandTimeout, 30*time.Second))
	defer cancel()
	evidence, err := lifecycle.VerifyManualRecovery(ctx, snapshot)
	if err != nil {
		return "execution identity unverifiable: " + err.Error()
	}
	if evidence.ProcessAbsent {
		return "process absent"
	}
	if evidence.ManagedProcess && evidence.SessionAvailable {
		return "matching process still exists"
	}
	return "execution identity unverifiable"
}
