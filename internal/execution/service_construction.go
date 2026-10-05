package execution

import (
	"context"
	"errors"
	"fmt"
	"os"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
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
	Close         func() error
}

// NewService constructs a service from one explicit, immutable implementation
// directory and its conversation source port. Lifecycle operations receive
// only execution values and already-persisted operation intents. The application
// calls PrepareServiceConfig before opening and transferring the connection.
func NewService(c Config, directory *ExecutionDirectory, resolver executionbackend.LifecycleResolver, conversationSources contracts.ConversationSourcePort, processHost executionbackend.ManagedProcessHost, stores ServiceStorage, work TaskWorkPort, results TaskResultPort, host HostPort) (_ *Service, retErr error) {
	if c.ExecutionRecords == nil {
		c.ExecutionRecords = stores.Records
	}
	if directory == nil || resolver == nil || conversationSources == nil || processHost == nil || c.ExecutionRecords == nil || stores.State == nil || stores.Commands == nil || stores.Worker == nil || stores.Conversations == nil || stores.Close == nil || work == nil || results == nil || host == nil || c.Resources == nil {
		return nil, errors.New("execution directory, conversation source, process host and persistence ports are required")
	}
	c.ExecutionDirectory = directory
	c.ProcessHost = processHost
	var admissionErr error
	admissionErr = c.ValidateAdmission()
	if admissionErr != nil {
		return nil, admissionErr
	}
	if err := host.ValidateStateLocation(c); err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			_ = stores.Close()
		}
	}()
	s := &Service{host: host, taskWork: work, taskResults: results, Config: c, baseConfig: c, lifecycleResolver: resolver, conversationSources: conversationSources, domainStore: stores.State, commandStore: stores.Commands, conversationStore: stores.Conversations, closeStore: stores.Close, store: &Store{Domain: stores.Worker, Project: c.identity}, workers: map[string]*Worker{}, ownedProcesses: map[string]executionbackend.StopHandle{}}
	checkpoint, e := stores.State.LoadCheckpoint(context.Background(), c.identity, false)
	if errors.Is(e, storageports.ErrNotFound) {
		checkpoint, e = nil, nil
	}
	if e != nil {
		return nil, e
	}
	s.State = contracts.Runtime{SchemaVersion: storageports.WorkerFlowRuntimeSchema, Repository: contracts.Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.identity}, Tasks: []taskmodel.TaskRecord{}, WorkflowRuns: []executionmodel.WorkflowRunRecord{}, Executions: []executionmodel.ExecutionRecord{}, Attempts: []executionmodel.StepAttemptRecord{}, Decisions: []executionmodel.DecisionRecord{}, Runs: []*contracts.Run{}, Control: contracts.ControlState{DispatchMode: "auto", Revision: 1, Blockers: []contracts.ControlBlocker{}}}
	authority, authErr := stores.State.NativeAuthority(context.Background(), c.identity)
	if authErr != nil {
		return nil, authErr
	}
	if authority.DispatchMode == "auto" && c.DefaultProfileID() == "" {
		return nil, errors.New("automatic dispatch requires default_execution_profile")
	}
	s.State.Control = contracts.ControlState{DispatchMode: authority.DispatchMode, DispatchPaused: authority.DispatchPaused, Revision: authority.AuthorityVersion, Blockers: []contracts.ControlBlocker{}}
	if checkpoint != nil {
		old := reconstruction.RuntimeFromCheckpoint(*checkpoint, authority.DispatchMode, authority.DispatchPaused, authority.AuthorityVersion)
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
	s.State.SchemaVersion = storageports.WorkerFlowRuntimeSchema
	capabilities := []string{"answer_decision", "open_terminal", "recheck_environment", "start_task", "start_candidate", "abandon_candidate", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service"}
	s.State.Service = contracts.ServiceState{ExecutionPolicy: c.ExecutionPolicy, PolicySource: c.policySource, InstanceID: newID(), PID: os.Getpid(), Status: "running", Claimant: c.Claimant, Mode: "maintenance", Ready: false, ControlProtocol: ControlProtocolVersion, Capabilities: capabilities, ConfigDigest: c.configDigest, HeartbeatIntervalMS: duration(c.Heartbeat).Milliseconds()}
	identity, err := host.CurrentProcessIdentity()
	if err != nil {
		return nil, fmt.Errorf("identify service process: %w", err)
	}
	s.State.Service.ProcessIdentity = identity
	if err := stores.Conversations.ValidateConversationCheckpointFormat(context.Background(), c.identity, conversationSources.JournalSources()); err != nil {
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
