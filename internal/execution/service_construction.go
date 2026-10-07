package execution

import (
	"context"
	"errors"
	"fmt"
	"os"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
	"github.com/erranliu/SteerLine/internal/execution/hostport"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// ServiceStorage is the app's explicit port bundle for one service instance.
// Each consumer receives only its own capabilities; no combined connection
// or factory enters the execution application.
type ServiceStorage struct {
	State         StateReader
	Commands      CommandStore
	Worker        storageports.WorkerMetadata
	Conversations ConversationStore
	Records       ExecutionRecordReader
	Resources     ResourcePort
	Close         func() error
}

// NewService constructs a service from one explicit, immutable implementation
// directory and its conversation source port. Lifecycle operations receive
// only execution values and already-persisted operation intents. The application
// calls PrepareServiceConfig before opening and transferring the connection.
func NewService(c Config, runtime RuntimeDependencies, settings RuntimeSettings, resolver executionbackend.LifecycleResolver, conversationSources conversationtypes.ConversationSourcePort, stores ServiceStorage, work TaskWorkPort, results TaskResultPort, host hostport.HostPort) (_ *Service, retErr error) {
	if stores.Close != nil {
		defer func() {
			if retErr != nil {
				retErr = errors.Join(retErr, stores.Close())
			}
		}()
	}
	if runtime.Directory == nil || runtime.ProcessHost == nil || runtime.Profiles == nil || resolver == nil || conversationSources == nil || stores.Records == nil || stores.Resources == nil || stores.State == nil || stores.Commands == nil || stores.Worker == nil || stores.Conversations == nil || stores.Close == nil || work == nil || results == nil || host == nil {
		return nil, errors.New("execution directory, conversation source, process host and persistence ports are required")
	}
	if err := c.ValidateAdmission(runtime.Directory); err != nil {
		return nil, err
	}
	if err := host.ValidateStateLocation(); err != nil {
		return nil, err
	}
	s := &Service{host: host, taskWork: work, taskResults: results, Config: c, baseConfig: c, runtime: runtime, runtimeSettings: settings, lifecycleResolver: resolver, conversationSources: conversationSources, domainStore: stores.State, commandStore: stores.Commands, conversationStore: stores.Conversations, closeStore: stores.Close, store: &Store{Domain: stores.Worker, Project: c.identity}, records: executionRecords{root: c.RunDir, reader: stores.Records, resources: stores.Resources}, resources: stores.Resources, workers: map[string]*Worker{}, ownedProcesses: map[string]executionbackend.StopHandle{}}
	stored, e := stores.State.ReadExecutionStateSnapshot(context.Background(), c.identity)
	if e != nil {
		return nil, e
	}
	if c.WorkflowProtocol == executionmodel.Version {
		if err := c.ValidateStoredExecutions(reconstruction.SnapshotFromStorage(stored), runtime.Directory); err != nil {
			return nil, err
		}
	}
	authority := stored.Graph.Authority
	s.State = reconstruction.Runtime{SchemaVersion: storageports.WorkerFlowRuntimeSchema, Repository: reconstruction.Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.identity}, Tasks: []taskmodel.TaskRecord{}, WorkflowRuns: []executionmodel.WorkflowRunRecord{}, Executions: []executionmodel.ExecutionRecord{}, Attempts: []executionmodel.StepAttemptRecord{}, Decisions: []executionmodel.DecisionRecord{}, Runs: []*reconstruction.RunView{}, Control: reconstruction.ControlState{DispatchMode: "auto", Revision: 1, Blockers: []reconstruction.ControlBlocker{}}}
	if authority.DispatchMode == "auto" && runtime.Profiles.DefaultProfileID() == "" {
		return nil, errors.New("automatic dispatch requires default_execution_profile")
	}
	s.State.Control = reconstruction.ControlState{DispatchMode: authority.DispatchMode, DispatchPaused: authority.DispatchPaused, Revision: authority.AuthorityVersion, Blockers: []reconstruction.ControlBlocker{}}
	if stored.Checkpoint != nil {
		old := reconstruction.RuntimeFromCheckpoint(*stored.Checkpoint, authority.DispatchMode, authority.DispatchPaused, authority.AuthorityVersion)
		if old.Repository.Identity != c.identity || old.Repository.Remote != c.Remote || old.Repository.Branch != c.Branch {
			return nil, nativeErr("authority_mismatch", "runtime repository or branch differs from the native authority; preserve state")
		}
		s.State = old
		s.runs = old.Runs
		s.State.Control.DispatchMode, s.State.Control.DispatchPaused, s.State.Control.Revision = authority.DispatchMode, authority.DispatchPaused, authority.AuthorityVersion
		if op := s.State.Control.Operation; op != nil {
			if !controlIDPattern.MatchString(op.RequestID) || !ValidControlAction(op.Action) || (op.Phase != "applying" && op.Phase != "completed") || len(op.RequestDigest) != 64 || op.Action == "start_task" && (!controlIDPattern.MatchString(op.RunID) || op.TaskID == "") || op.Phase == "completed" && op.Result == nil {
				return nil, errors.New("invalid runtime control operation; preserve the snapshot for recovery")
			}
			if op.Phase == "completed" && (op.Result.Version != ControlProtocolVersion || op.Result.RequestID != op.RequestID || op.Result.RequestDigest != op.RequestDigest || op.Result.Action != op.Action || !TerminalControlStatus(op.Result.Status)) {
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
	if err := s.applyNativeSnapshot(stored); err != nil {
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
	s.State.SchemaVersion = storageports.WorkerFlowRuntimeSchema
	capabilities := []string{"answer_decision", "open_terminal", "recheck_environment", "start_task", "start_candidate", "abandon_candidate", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service"}
	selectedProfile := runtime.Profiles.DefaultProfileID()
	if c.selectedProfileID != "" {
		selectedProfile = c.selectedProfileID
	}
	_, _, err := runtime.Profiles.ResolveExecutionProfile(selectedProfile, c.fixedProfileDigest)
	if err != nil {
		return nil, fmt.Errorf("resolve initial execution profile: %w", err)
	}
	s.State.Service = reconstruction.ServiceState{InstanceID: newID(), PID: os.Getpid(), Status: "running", Claimant: settings.Claimant, Mode: "maintenance", Ready: false, ControlProtocol: ControlProtocolVersion, Capabilities: capabilities, ConfigDigest: c.acceptedConfigVersion, HeartbeatIntervalMS: settings.Heartbeat.Milliseconds()}
	identity, err := host.CurrentProcessIdentity()
	if err != nil {
		return nil, fmt.Errorf("identify service process: %w", err)
	}
	s.State.Service.ProcessIdentity = identity
	return s, nil
}

func (s *Service) Close() error {
	if s == nil || s.closeStore == nil {
		return nil
	}
	return s.closeStore()
}
