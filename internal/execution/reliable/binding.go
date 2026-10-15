package reliable

import (
	"errors"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Workspace struct {
	InspectionScope, Repository, Worktree, Branch, Base, Head string
	Dirty                                                     bool
	VerifiedAt                                                time.Time
}

// BindingSnapshot contains only execution identity facts, never business state.
type BindingSnapshot struct {
	ProjectIdentity, ExecutionID, CurrentAttemptID, Generation, ConfigurationDigest string
	Target                                                                          executionbackend.ExecutionTarget
	Workspace                                                                       Workspace
	Process                                                                         *executionbackend.ProcessIdentity
	SessionID, SessionGeneration, PendingCommandID, PendingTurnID, CurrentTurnID    string
	ImplementationData                                                              []executionbackend.ImplementationDataEnvelope
	Environment                                                                     *EnvironmentBinding
	PolicyResources                                                                 *PolicyBinding
}
type EnvironmentBinding struct {
	OriginalTerminal       *executionbackend.ProcessIdentity
	Runtime, LaunchCommand string
	WriteRoots             []string
}
type PolicyBinding struct{ WriteRoots []string }

func MergeReceipt(value BindingSnapshot, receipt executionbackend.LifecycleReceipt) (BindingSnapshot, error) {
	candidate := value
	candidate.ImplementationData = append([]executionbackend.ImplementationDataEnvelope(nil), value.ImplementationData...)
	if value.Environment != nil {
		env := *value.Environment
		env.WriteRoots = append([]string(nil), env.WriteRoots...)
		candidate.Environment = &env
	}
	if err := mergeReceipt(&candidate, receipt); err != nil {
		return value, err
	}
	return candidate, nil
}
func putEnvelope(record *BindingSnapshot, envelope executionbackend.ImplementationDataEnvelope) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	next := make([]executionbackend.ImplementationDataEnvelope, 0, len(record.ImplementationData)+1)
	for _, existing := range record.ImplementationData {
		if existing.ImplementationKey != envelope.ImplementationKey {
			next = append(next, existing.Clone())
		}
	}
	record.ImplementationData = append(next, envelope.Clone())
	return nil
}
func canonical(s string) string {
	s = filepath.Clean(s)
	if runtime.GOOS == "windows" {
		return strings.ToLower(s)
	}
	return s
}
func mergeReceipt(execution *BindingSnapshot, receipt executionbackend.LifecycleReceipt) error {
	if receipt.Target != nil {
		current, next := execution.Target, *receipt.Target
		if next.ID != current.ID || next.ConnectionID != current.ConnectionID || next.ExternalRuntime != current.ExternalRuntime {
			return ErrStale
		}
		for _, field := range [][2]string{{current.WorktreeID, next.WorktreeID},
			{current.ExecutionHost, next.ExecutionHost}} {
			if field[0] != "" && field[0] != field[1] {
				return ErrStale
			}
		}
		if current.WorktreeID != "" && (current.Branch != next.Branch || current.BaseRevision != next.BaseRevision) {
			return ErrStale
		}
		execution.Target = next
	}
	if receipt.Workspace != nil {
		if execution.Workspace.Base != "" && execution.Workspace.Base != receipt.Workspace.Base {
			return ErrStale
		}
		workspace := receipt.Workspace
		if !execution.Workspace.VerifiedAt.IsZero() && execution.Workspace.Worktree != "" && (canonical(execution.Workspace.Worktree) != canonical(workspace.Worktree) || execution.Workspace.Branch != workspace.Branch) {
			return ErrStale
		}
		if workspace.Worktree == "" || workspace.Branch == "" || workspace.Base == "" || workspace.Head == "" {
			return errors.New("lifecycle workspace receipt is incomplete")
		}
		execution.Workspace = Workspace{InspectionScope: workspace.InspectionScope, Repository: workspace.Repository, Worktree: workspace.Worktree, Branch: workspace.Branch,
			Base: workspace.Base, Head: workspace.Head, Dirty: workspace.Dirty, VerifiedAt: workspace.CheckedAt}
	}
	if receipt.State.WorktreeID != "" {
		if execution.Target.WorktreeID != "" && execution.Target.WorktreeID != receipt.State.WorktreeID {
			return ErrStale
		}
		execution.Target.WorktreeID = receipt.State.WorktreeID
	}
	if receipt.OriginalHost != nil && execution.Environment != nil {
		original := *receipt.OriginalHost
		if execution.Environment.OriginalTerminal != nil && !SameProcess(*execution.Environment.OriginalTerminal, original) {
			return ErrStale
		}
		execution.Environment.OriginalTerminal = &original
		execution.Environment.Runtime = original.RuntimeID
	}
	if receipt.Process != nil {
		if execution.Process != nil && *execution.Process != *receipt.Process && !SameProcess(*execution.Process, *receipt.Process) {
			return ErrStale
		}
		process := *receipt.Process
		execution.Process = &process
		if execution.Environment != nil {
			if execution.Environment.OriginalTerminal != nil && !SameProcess(*execution.Environment.OriginalTerminal, process) {
				return ErrStale
			}
			execution.Environment.OriginalTerminal = &process
			execution.Environment.Runtime = process.RuntimeID
		}
	}
	if receipt.State.SessionID != "" {
		if execution.SessionID != "" && execution.SessionID != receipt.State.SessionID {
			return ErrStale
		}
		execution.SessionID = receipt.State.SessionID
	}
	if receipt.State.SessionGeneration != "" {
		if execution.SessionGeneration != "" && execution.SessionGeneration != receipt.State.SessionGeneration {
			return ErrStale
		}
		execution.SessionGeneration = receipt.State.SessionGeneration
	}
	if receipt.State.TurnID != "" {
		if (execution.PendingCommandID != "" || execution.CurrentTurnID == "") && execution.PendingTurnID != "" && execution.PendingTurnID != receipt.State.TurnID {
			return ErrStale
		}
		if execution.PendingCommandID != "" || execution.CurrentTurnID == "" {
			execution.PendingTurnID = receipt.State.TurnID
		}
	}
	for _, envelope := range receipt.Implementation {
		if err := envelope.ValidateResourceIdentity(execution.ProjectIdentity, execution.ExecutionID, execution.CurrentAttemptID, execution.Generation, execution.ConfigurationDigest); err != nil {
			return err
		}
		if err := putEnvelope(execution, envelope); err != nil {
			return err
		}
	}
	if execution.Environment != nil && execution.Environment.OriginalTerminal != nil && execution.PolicyResources != nil && receipt.State.LaunchCommand != "" {
		execution.Environment.LaunchCommand = receipt.State.LaunchCommand
		execution.Environment.WriteRoots = append([]string(nil), execution.PolicyResources.WriteRoots...)
	}
	return nil
}

func SameProcess(a, b executionbackend.ProcessIdentity) bool {
	return a.Kind == b.Kind && a.PID == b.PID && a.Created == b.Created && a.Executable == b.Executable &&
		a.Terminal == b.Terminal && a.RuntimeID == b.RuntimeID && a.Incarnation == b.Incarnation &&
		a.ExecutionHost == b.ExecutionHost && a.WorktreeID == b.WorktreeID
}
