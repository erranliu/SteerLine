package steerline

import (
	"context"
	"errors"
	"fmt"
	"os"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// NewService constructs a service from one explicit, immutable implementation
// directory and its conversation source port. Lifecycle operations receive
// only execution values and already-persisted operation intents. The application
// calls PrepareServiceConfig before opening and transferring the connection.
func NewService(c Config, directory *ExecutionDirectory, resolver executionbackend.LifecycleResolver, conversationSources contracts.ConversationSourcePort, processHost executionbackend.ManagedProcessHost, db persistence.Connection, work TaskWorkPort, results TaskResultPort) (_ *Service, retErr error) {
	if directory == nil || resolver == nil || conversationSources == nil || processHost == nil || c.Persistence == nil || db == nil || work == nil || results == nil {
		return nil, errors.New("execution directory, conversation source, process host and persistence ports are required")
	}
	c.ExecutionDirectory = directory
	c.ProcessHost = processHost
	var admissionErr error
	c, admissionErr = admitServiceConfig(c)
	if admissionErr != nil {
		return nil, admissionErr
	}
	defer func() {
		if retErr != nil {
			_ = db.Close()
		}
	}()
	s := &Service{taskWork: work, taskResults: results, Config: c, baseConfig: c, lifecycleResolver: resolver, conversationSources: conversationSources, domainStore: db, commandStore: db, conversationStore: db, closeStore: db.Close, store: &Store{Domain: db, Project: c.identity}, workers: map[string]*Worker{}, ownedProcesses: map[string]executionbackend.StopHandle{}}
	old, e := db.LoadRuntime(c.identity, false)
	if errors.Is(e, storageports.ErrNotFound) {
		old, e = nil, nil
	}
	if e != nil {
		return nil, e
	}
	s.State = Runtime{SchemaVersion: WorkerFlowRuntimeSchema, Repository: Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.identity}, Tasks: []taskmodel.TaskRecord{}, WorkflowRuns: []executionmodel.WorkflowRunRecord{}, Executions: []executionmodel.ExecutionRecord{}, Attempts: []executionmodel.StepAttemptRecord{}, Decisions: []executionmodel.DecisionRecord{}, Runs: []*Run{}, Control: ControlState{DispatchMode: "auto", Revision: 1, Blockers: []ControlBlocker{}}}
	authority, authErr := db.NativeAuthority(context.Background(), c.identity)
	if authErr != nil {
		return nil, authErr
	}
	if authority.DispatchMode == "auto" && c.DefaultProfileID() == "" {
		return nil, errors.New("automatic dispatch requires default_execution_profile")
	}
	s.State.Control = ControlState{DispatchMode: authority.DispatchMode, DispatchPaused: authority.DispatchPaused, Revision: authority.AuthorityVersion, Blockers: []ControlBlocker{}}
	if old != nil {
		if old.Repository.Identity != c.identity || old.Repository.Remote != c.Remote || old.Repository.Branch != c.Branch {
			return nil, nativeErr("authority_mismatch", "runtime repository or branch differs from the native authority; preserve state")
		}
		s.State = *old
		s.runs = old.Runs
		s.State.Control.DispatchMode, s.State.Control.DispatchPaused, s.State.Control.Revision = authority.DispatchMode, authority.DispatchPaused, authority.AuthorityVersion
		if op := s.State.Control.Operation; op != nil {
			if !controlIDPattern.MatchString(op.RequestID) || !validControlAction(op.Action) || (op.Phase != "applying" && op.Phase != "completed") || len(op.RequestDigest) != 64 || op.Action == "start_task" && (!controlIDPattern.MatchString(op.RunID) || op.TaskID == "") || op.Phase == "completed" && op.Result == nil {
				return nil, errors.New("invalid runtime control operation; preserve the snapshot for recovery")
			}
			if op.Phase == "completed" && (op.Result.Version != ControlProtocolVersion || op.Result.RequestID != op.RequestID || op.Result.RequestDigest != op.RequestDigest || op.Result.Action != op.Action || !terminalControlStatus(op.Result.Status)) {
				return nil, errors.New("completed control receipt does not match its operation")
			}
		}
		if s.State.Control.Operation == nil && s.State.CurrentTask != nil && s.find(*s.State.CurrentTask) == nil {
			// Automatic dispatch persists the reservation before fetch. With no run
			// record there has been no claim; clear only this pre-claim reservation.
			s.State.CurrentTask = nil
			s.State.SlotOccupied = false
		}
	}
	if err := s.refreshNativeSnapshot(context.Background()); err != nil {
		return nil, err
	}
	for _, run := range s.runs {
		if run.WorkflowVersion != executionmodel.Version {
			return nil, nativeErr("unsupported_schema", "unsupported workflow version")
		}
		if _, err := s.executionConfigForRun(run); err != nil {
			return nil, err
		}
	}
	s.State.SchemaVersion = WorkerFlowRuntimeSchema
	capabilities := []string{"answer_decision", "open_terminal", "recheck_environment", "start_task", "start_candidate", "abandon_candidate", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service"}
	s.State.Service = ServiceState{ExecutionPolicy: c.ExecutionPolicy, PolicySource: c.policySource, InstanceID: newID(), PID: os.Getpid(), Status: "running", Claimant: c.Claimant, Mode: "maintenance", Ready: false, ControlProtocol: ControlProtocolVersion, Capabilities: capabilities, ConfigDigest: c.configDigest, HeartbeatIntervalMS: duration(c.Heartbeat).Milliseconds()}
	identity, err := currentProcessIdentity()
	if err != nil {
		return nil, fmt.Errorf("identify service process: %w", err)
	}
	s.State.Service.ProcessIdentity = identity
	if err := db.ValidateConversationCheckpointFormat(context.Background(), c.identity, conversationSources.JournalSources()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Close() error {
	if s == nil || s.closeStore == nil {
		return nil
	}
	return s.closeStore()
}
