package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func OpenClient(ctx context.Context, path string) (*productclient.Client, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return openProjectClient(ctx, productapi.ProjectRef{ConfigPath: abs})
}

func openProjectClient(ctx context.Context, ref productapi.ProjectRef) (*productclient.Client, error) {
	return productclient.Open(ctx, ref, ClientResources)
}

// openServiceProjectClient binds the configured service's own task-discard
// command port into its local HTTP product client. Other clients retain the
// ordinary repository-locking TaskCommands writer.
func openServiceProjectClient(ctx context.Context, config Config, service *taskrunner.Actor) (*productclient.Client, error) {
	return openServiceProjectClientWithCommands(ctx, config, service, nil, nil)
}
func openServiceProjectClientWithCommands(ctx context.Context, config Config, service *taskrunner.Actor, commands func(context.Context, bool) (productclient.TaskCommands, error), store *sqlite.SQLiteStore) (*productclient.Client, error) {
	absolute, err := filepath.Abs(config.ConfigPath)
	if err != nil {
		return nil, err
	}
	ref := productapi.ProjectRef{ConfigPath: absolute}
	return productclient.Open(ctx, ref, func(ctx context.Context, ref productapi.ProjectRef) (productclient.Resources, error) {
		bound := config
		bound.ConfigPath = absolute
		if err := bound.ResolveIdentity(ctx); err != nil {
			return productclient.Resources{}, err
		}
		resources, err := clientResources(ctx, bound, ref)
		if err != nil {
			return productclient.Resources{}, err
		}
		resources.Discard = &clientDiscardCommands{service: service}
		if store != nil {
			resources.OpenReader = func(ctx context.Context) (productclient.Reader, error) {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				return &clientReader{Service: query.NewService(store, bound.Identity()), ConversationService: query.NewConversationService(store, bound.Identity()), store: store, project: bound.Identity(), config: bound, executableTypes: append([]string(nil), bound.ExecutableTypes...), freshness: projectReadFreshness(bound), borrowed: true}, nil
			}
		}
		if commands != nil {
			resources.OpenCommands = commands
		}
		return resources, nil
	})
}

// ClientResources is the application construction seam. It binds one immutable
// config/directory to pure ports; all product orchestration lives in the adapter.
func ClientResources(ctx context.Context, ref productapi.ProjectRef) (productclient.Resources, error) {
	c, err := LoadQueryConfig(ref.ConfigPath)
	if err != nil {
		return productclient.Resources{}, err
	}
	return clientResources(ctx, c, ref)
}

func clientResources(ctx context.Context, c Config, ref productapi.ProjectRef) (productclient.Resources, error) {
	if err := c.ResolveIdentity(ctx); err != nil {
		return productclient.Resources{}, err
	}
	abs, err := filepath.Abs(ref.ConfigPath)
	if err != nil {
		return productclient.Resources{}, err
	}
	ref.ConfigPath = abs
	ref.Identity = c.Identity()
	ref.RunDir = c.RunDir
	ref.Presentation = projectPresentation(c)
	runtimeInfo := CurrentRuntimeIdentity(ref.ConfigPath)
	ref.ApplicationHome, ref.ApplicationBin = runtimeInfo.Home, BinDir()
	ref.ExecutablePath, ref.SourceCommit, ref.SourceDirty = runtimeInfo.Executable, runtimeInfo.SourceCommit, runtimeInfo.SourceDirty
	ref.Presentation.ServiceInstruction = clientServiceInstruction(ref)
	if ref.Key == "" {
		ref.Key = ref.ConfigPath + "\x00" + ref.RunDir
	}
	freshness := projectReadFreshness(c)
	return productclient.Resources{
		Project: ref, DefaultRequestedBy: steerline.DefaultRequestedBy(), DefaultAnswerer: AnswererName(),
		ReadFailure: func(ctx context.Context, selected string, readErr error) productapi.ProjectSnapshot {
			observationErr := readErr
			if errors.Is(readErr, os.ErrNotExist) {
				observationErr = nil
			}
			nativeObservation := observeHostRuntime(ctx, c, nil, observationErr, time.Now().UTC())
			legacy := connectHostServiceFromSnapshot(ctx, c, nativeObservation)
			obs := productObservation(nativeObservation)
			connection := productConnection(legacy, obs)
			return productapi.ProjectSnapshot{SelectedTaskID: selected, Observation: obs, Connection: connection, TaskError: readErr.Error(), NativeTaskError: readErr.Error(), HistoryError: readErr.Error()}
		},
		OpenReader: func(ctx context.Context) (productclient.Reader, error) {
			return openClientReader(ctx, c, freshness)
		},
		OpenCommands: func(ctx context.Context, mutation bool) (productclient.TaskCommands, error) {
			cfg := c
			s, err := OpenNativeTaskProject(ctx, &cfg)
			if err != nil {
				return nil, err
			}
			var lock *steerline.RepoLock
			if mutation {
				if err = s.Close(); err != nil {
					return nil, err
				}
				lock, err = steerline.LockRepository(c.Identity())
				if err != nil {
					return nil, err
				}
				s, err = OpenNativeTaskCommandStore(ctx, &cfg)
				if err != nil {
					lock.Close()
					return nil, err
				}
			}
			commands, err := newClientCommands(s, c, lock)
			if err != nil {
				_ = s.Close()
				if lock != nil {
					lock.Close()
				}
				return nil, err
			}
			return commands, nil
		},
		Control: clientControl{config: c}, Decision: clientDecision{config: c},
		TaskSnapshot: func(ctx context.Context) (*productapi.TaskCandidates, error) {
			reader, err := openClientReader(ctx, c, freshness)
			if err != nil {
				return nil, err
			}
			defer reader.Close()
			project := reader.ReadProject(ctx, "", productapi.ProjectSnapshot{})
			if project.Tasks == nil || project.TaskError != "" {
				if project.TaskError != "" {
					return nil, errors.New(project.TaskError)
				}
				return nil, errors.New("canonical task candidate snapshot is unavailable")
			}
			return project.Tasks, nil
		},
		ResolveStartTask: func(ctx context.Context, id, profileID string) (productapi.ControlPayload, error) {
			if profileID == "" {
				return productapi.ControlPayload{}, errors.New("start_task requires -execution-profile")
			}
			p, err := c.RunnerExecution.Profile(profileID, "")
			if err != nil {
				return productapi.ControlPayload{}, err
			}
			reader, err := OpenNativeTaskReader(ctx, &c)
			if err != nil {
				return productapi.ControlPayload{}, err
			}
			defer reader.Close()
			graph, err := reader.TaskGraphSnapshot(ctx, c.Identity())
			if err != nil {
				return productapi.ControlPayload{}, err
			}
			task, ok := graph.ByID[id]
			if !ok {
				return productapi.ControlPayload{}, taskError("task_not_found", "task was not found")
			}
			return productapi.ControlPayload{TaskID: id, ExpectedTaskVersion: task.Version, ExpectedGraphRevision: graph.Authority.GraphRevision, ExecutionProfileID: p.ID, ExecutionProfileDigest: p.Digest()}, nil
		},
		InspectRecovery: func(ctx context.Context, id string) (productapi.RecoveryInspection, error) {
			return inspectRunnerRecovery(ctx, c, id)
		},
	}, nil
}

type clientReader struct {
	borrowed bool

	query.Service
	query.ConversationService
	store interface {
		storageports.ApplicationProjectSnapshotReader
		Close() error
		Command(context.Context, string, string) (storageports.CommandRecord, error)
		DatabaseVersion(context.Context) (string, uint64, error)
	}
	project         string
	config          Config
	executableTypes []string
	freshness       time.Duration
}

func openClientReader(ctx context.Context, config Config, freshness time.Duration) (*clientReader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c := config
	store, err := OpenNativeTaskReader(ctx, &c)
	if err != nil {
		return nil, err
	}
	return &clientReader{Service: query.NewService(store, c.Identity()), ConversationService: query.NewConversationService(store, c.Identity()),
		store: store, project: c.Identity(), config: c, executableTypes: append([]string(nil), c.ExecutableTypes...), freshness: freshness}, nil
}

func (r *clientReader) Close() error {
	if r.borrowed {
		return nil
	}
	return r.store.Close()
}

func (r *clientReader) ReadProject(ctx context.Context, selected string, _ productapi.ProjectSnapshot) productapi.ProjectSnapshot {
	snapshot, err := r.store.ReadApplicationProjectSnapshot(ctx, r.project, selected, 500)
	if err != nil {
		nativeObservation := observeHostRuntime(ctx, r.config, nil, err, time.Now().UTC())
		legacyConnection := connectHostServiceFromSnapshot(ctx, r.config, nativeObservation)
		observation := productObservation(nativeObservation)
		connection := productConnection(legacyConnection, observation)
		page := query.ProjectPage(storageports.ProjectView{}, selected, r.freshness, observation, connection, nil, r.executableTypes)
		page.TaskError, page.NativeTaskError, page.HistoryError = err.Error(), err.Error(), err.Error()
		return page
	}
	view := snapshot.ProjectView
	nativeObservation := applicationProjectObservation(ctx, r.config, snapshot)
	legacyConnection := connectHostServiceFromSnapshot(ctx, r.config, nativeObservation)
	observation := productObservation(nativeObservation)
	connection := productConnection(legacyConnection, observation)
	decisionReasons := map[string]string{}
	if legacyConnection.Snapshot.Runtime != nil {
		for _, run := range legacyConnection.Snapshot.Runtime.Runs {
			if run == nil || run.Decision == nil {
				continue
			}
			reason := decisionAvailability(ctx, r.config, run, legacyConnection)
			if reason != "" {
				decisionReasons[run.RunID] = reason
			}
		}
	}
	return query.ProjectPage(view, selected, r.freshness, observation, connection, decisionReasons, r.executableTypes)
}

func applicationProjectObservation(ctx context.Context, config Config, snapshot storageports.ApplicationProjectSnapshot) steerline.ObservationSnapshot {
	sampledAt := snapshot.ProjectView.SampledAt
	if sampledAt.IsZero() {
		sampledAt = time.Now().UTC()
	}
	if snapshot.RunnerError != nil {
		return observeHostRuntime(ctx, config, nil, snapshot.RunnerError, sampledAt)
	}
	if snapshot.RunnerState.Checkpoint == nil {
		return observeHostRuntime(ctx, config, nil, nil, sampledAt)
	}
	runtimeState := taskrunner.Project(snapshot.RunnerState, *snapshot.RunnerState.Checkpoint)
	return observeHostRuntime(ctx, config, &runtimeState, nil, sampledAt)
}

func (r *clientReader) ShowTask(ctx context.Context, id string) (productapi.TaskPage, error) {
	page, err := r.Service.ShowNativeTask(ctx, id)
	if err != nil {
		return productapi.TaskPage{}, productReadError(err)
	}
	return query.TaskPage(page), nil
}
func (r *clientReader) ListTasks(ctx context.Context, cursor string, limit int) (productapi.TaskPage, error) {
	page, err := r.Service.ListNativeTasks(ctx, cursor, limit)
	if err != nil {
		return productapi.TaskPage{}, productReadError(err)
	}
	return query.TaskPage(page), nil
}
func (r *clientReader) TaskGraph(ctx context.Context, cursor string, limit int) (productapi.GraphPage, error) {
	page, err := r.Service.TaskGraph(ctx, cursor, limit)
	return page, productReadError(err)
}
func (r *clientReader) Conversation(ctx context.Context, filter productapi.ConversationFilter, cursor string, limit int) (productapi.ConversationPage, error) {
	return r.ConversationService.ReadConversation(ctx, filter, cursor, limit)
}
func (r *clientReader) Body(ctx context.Context, ref productapi.ConversationBodyRef, offset int64, limit int) (productapi.ConversationBodyPage, error) {
	return r.ConversationService.ReadMessageBody(ctx, ref, offset, limit)
}

func (r *clientReader) RecentActivity(ctx context.Context, run, execution, attempt string) (productapi.LogBatch, error) {
	return r.Service.ReadRecentActivity(ctx, run, execution, attempt)
}

func (r *clientReader) Activity(ctx context.Context, run, execution, attempt string, cursor productapi.ActivityCursor) (productapi.LogBatch, error) {
	return r.Service.ReadActivity(ctx, run, execution, attempt, cursor)
}

func (r *clientReader) Command(ctx context.Context, id string) (productapi.CommandReceipt, error) {
	record, err := r.store.Command(ctx, r.project, id)
	if errors.Is(err, storageports.ErrNotFound) {
		hc, configErr := hostRuntimeConfig(ctx, r.config)
		if configErr != nil {
			return productapi.CommandReceipt{}, configErr
		}
		if request, readErr := steerline.ReadControlRequest(hc, id); readErr == nil {
			payload, decodeErr := taskrunner.DecodeControlPayload(*request)
			if decodeErr == nil && payload.CallerAnswer != nil {
				return runnerAnswerReceipt(ctx, r.config, *request, nil)
			}
		}
		return productapi.CommandReceipt{}, errors.Join(productapi.ErrRecordNotFound, err)
	}
	if err == nil && record.Kind == "runner_control" {
		var request productapi.ControlRequest
		if json.Unmarshal(record.Payload, &request) != nil {
			return productapi.CommandReceipt{}, errors.New("original caller control is invalid")
		}
		payload, decodeErr := taskrunner.DecodeControlPayload(request)
		if decodeErr == nil && payload.CallerAnswer != nil {
			return runnerAnswerReceipt(ctx, r.config, request, &record)
		}
	}
	return query.CommandReceipt(record), err
}

func (r *clientReader) DatabaseVersion(ctx context.Context) (string, uint64, error) {
	return r.store.DatabaseVersion(ctx)
}

// These compile-time assertions keep the injected port tied to real consumers.
var _ productclient.Reader = (*clientReader)(nil)

type clientControl struct{ config Config }

func (r clientControl) Submit(ctx context.Context, in productapi.ControlInput) (productapi.ControlRequest, error) {
	if err := ctx.Err(); err != nil {
		return productapi.ControlRequest{}, err
	}
	var payload any
	if in.Payload != (productapi.ControlPayload{}) {
		payload = in.Payload
	}
	hostConfig, err := hostRuntimeConfig(ctx, r.config)
	if err != nil {
		return productapi.ControlRequest{}, err
	}
	return steerline.SubmitControlTargeted(hostConfig, string(in.Action), payload, in.RequestedBy, in.RequestID, in.InstanceID, in.ExpectedRevision)
}

func (r clientControl) SubmitInbox(ctx context.Context, in productapi.ControlInput) (productapi.ControlRequest, error) {
	if err := ctx.Err(); err != nil {
		return productapi.ControlRequest{}, err
	}
	hostConfig, err := hostRuntimeConfig(ctx, r.config)
	if err != nil {
		return productapi.ControlRequest{}, err
	}
	return steerline.SubmitControlInbox(hostConfig, in)
}

func (r clientControl) Receipt(ctx context.Context, id string) (*productapi.ControlReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := r.Request(ctx, id); err != nil {
		return nil, err
	}
	hostConfig, err := hostRuntimeConfig(ctx, r.config)
	if err != nil {
		return nil, err
	}
	return steerline.ReadVerifiedControlReceipt(hostConfig, id)
}

func (r clientControl) Request(ctx context.Context, id string) (*productapi.ControlRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hostConfig, err := hostRuntimeConfig(ctx, r.config)
	if err != nil {
		return nil, err
	}
	req, err := steerline.ReadControlRequest(hostConfig, id)
	if err == nil && req.RepositoryIdentity != hostConfig.Identity() {
		return nil, errors.New("control request project identity does not match")
	}
	return req, err
}

func (r clientControl) Stop(ctx context.Context, by string, timeout time.Duration) (productapi.ServiceStopResult, error) {
	hostConfig, err := hostRuntimeConfig(ctx, r.config)
	if err != nil {
		return productapi.ServiceStopResult{}, err
	}
	return steerline.StopService(ctx, hostConfig, by, timeout)
}

func productObservation(value steerline.ObservationSnapshot) productapi.ObservationSnapshot {
	return productapi.ObservationSnapshot{Page: productServicePage(value.Runtime), State: value.State, SampledAt: value.SampledAt, HeartbeatAge: value.HeartbeatAge, FreshnessLimit: value.FreshnessLimit, ClockSkew: value.ClockSkew, Error: value.Error}
}

func productServicePage(runtimeState *reconstruction.Runtime) *productapi.ServicePage {
	if runtimeState == nil {
		return nil
	}
	view := &query.ServiceView{Revision: runtimeState.Revision, UpdatedAt: runtimeState.UpdatedAt, HeartbeatAt: runtimeState.HeartbeatAt,
		CurrentTask: copyClientValue(runtimeState.CurrentTask), SlotOccupied: runtimeState.SlotOccupied,
		Repository: productapi.RepositorySummary{Path: runtimeState.Repository.Path, Remote: runtimeState.Repository.Remote, Branch: runtimeState.Repository.Branch, Identity: runtimeState.Repository.Identity},
		Service:    productapi.ServiceSummary{InstanceID: runtimeState.Service.InstanceID, Status: runtimeState.Service.Status, ConfigDigest: runtimeState.Service.ConfigDigest, Claimant: runtimeState.Service.Claimant, Mode: runtimeState.Service.Mode, PID: runtimeState.Service.PID, Error: copyClientValue(runtimeState.Service.Error), StartupID: copyClientValue(runtimeState.Service.StartupID), Ready: runtimeState.Service.Ready, ControlProtocol: runtimeState.Service.ControlProtocol, Capabilities: publicProjectCapabilities(runtimeState.Service.Capabilities)},
		Control:    productapi.ControlSummary{DispatchMode: runtimeState.Control.DispatchMode, DispatchPaused: runtimeState.Control.DispatchPaused, Revision: runtimeState.Control.Revision},
		Deliveries: append([]executionmodel.TaskDeliveryRecord(nil), runtimeState.Deliveries...),
	}
	if operation := runtimeState.Control.Operation; operation != nil {
		view.Control.Operation = &productapi.PendingControl{RequestID: operation.RequestID, Action: operation.Action, TaskID: operation.TaskID, Phase: operation.Phase, RunID: operation.RunID, AttemptID: operation.AttemptID, DecisionID: operation.DecisionID}
	}
	for _, blocker := range runtimeState.Control.Blockers {
		view.Control.Blockers = append(view.Control.Blockers, productapi.ControlBlocker{RequestID: blocker.RequestID, Action: blocker.Action, TaskID: blocker.TaskID, RunID: blocker.RunID, AttemptID: blocker.AttemptID, DecisionID: blocker.DecisionID, Reason: blocker.Reason})
	}
	for _, run := range runtimeState.Runs {
		if run == nil {
			view.Runs = append(view.Runs, nil)
			continue
		}
		view.Runs = append(view.Runs, &query.CandidateView{FlowRef: run.FlowRef, WorkflowStage: run.WorkflowStage, WorkflowStatus: run.WorkflowStatus, StageDiagnostic: run.StageDiagnostic, StageResults: executionmodel.CloneStageResults(run.StageResults), ReportToolPolicy: run.ReportToolPolicy, SessionID: run.SessionID, SessionGeneration: run.SessionGeneration, CurrentTurnID: run.CurrentTurnID, TurnNumber: run.TurnNumber, PRHistory: append([]string(nil), run.PRHistory...), CandidateNumber: run.CandidateNumber, CandidateCreatedAt: run.CandidateCreatedAt, CandidateStopReason: run.CandidateStopReason, CandidateStoppedBy: run.CandidateStoppedBy, CandidateEndedAt: copyClientValue(run.CandidateEndedAt), CandidateTiming: run.CandidateTiming, ResourceStatus: run.ResourceStatus, RecoveryRequired: run.RecoveryRequired, RecoveryState: run.RecoveryState, RecoveryVersion: run.RecoveryVersion, TaskID: run.TaskID, DomainTaskID: run.DomainTaskID, WorkflowVersion: run.WorkflowVersion, ExecutionID: run.ExecutionID, Title: run.Title, RunID: run.RunID, AttemptID: run.AttemptID, ControlRevision: run.ControlRevision, Claimant: run.Claimant, Host: run.Host, Backend: run.Backend, Protocol: run.Protocol, Agent: run.Agent, Target: run.Target, TaskStatus: run.TaskStatus, ExecutionStatus: run.ExecutionStatus, StartedAt: copyClientValue(run.StartedAt), EndedAt: copyClientValue(run.EndedAt), WorktreePath: copyClientValue(run.WorktreePath), WorktreeBranch: copyClientValue(run.WorktreeBranch), BaseCommit: copyClientValue(run.BaseCommit), LastEvent: copyClientValue(run.LastEvent), LogPath: copyClientValue(run.LogPath), ResultPath: copyClientValue(run.ResultPath), Summary: copyClientValue(run.Summary), Error: copyClientValue(run.Error), Process: run.Process, EnvironmentBlocker: run.EnvironmentBlocker, ExecutionProfile: run.ExecutionProfile, Decision: run.Decision, PR: run.PR})
	}
	return query.ServicePage(view)
}

func copyClientValue[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func productConnection(value steerline.ServiceConnection, observation productapi.ObservationSnapshot) productapi.ServiceConnection {
	valueObservation := productObservation(value.Snapshot)
	if value.Snapshot.Runtime == nil && observation.Page != nil {
		valueObservation = observation
	}
	return productapi.ServiceConnection{Snapshot: valueObservation, Process: value.Process, State: value.State, IdentityMatched: value.IdentityMatched, ConfigMatched: value.ConfigMatched, Protocol: value.Protocol, Capabilities: publicProjectCapabilities(value.Capabilities), CanControl: value.CanControl, CanStop: false, CanStart: value.CanStart, StartupID: value.StartupID, Diagnostic: value.Diagnostic}
}

func decisionAvailability(ctx context.Context, c Config, run *reconstruction.RunView, connection steerline.ServiceConnection) string {
	if run == nil || run.Decision == nil {
		return "当前没有待回答的问题"
	}
	if run.Decision.RequestPath != "" || run.Decision.AnswerPath != "" {
		return "问题绑定了已移除的 sidecar-answer 格式；保留状态并转入人工恢复，不导入答案文件"
	}
	if run.Decision.Status != "pending" {
		return "Runner 当前未允许回答或恢复；刷新原任务状态"
	}
	if !connection.CanControl || connection.Snapshot.Runtime == nil {
		return "服务离线或未就绪；答案不会写入，恢复服务后再通过同一命令提交"
	}
	for _, capability := range connection.Snapshot.Runtime.Service.Capabilities {
		if capability == "answer_decision" {
			return "提交后核对原会话接收与工作轮次开始"
		}
	}
	return "服务未声明 answer_decision 能力"
}

// projectPresentation exposes only accepted non-secret display fields and fixed
// profile digests. Configuration compilation stays with the original owner.
func projectPresentation(c Config) productapi.ProjectPresentation {
	result := productapi.ProjectPresentation{NativeWorkflow: c.WorkflowProtocol == executionmodel.Version, ConfigDigest: c.LoadedConfigDigest(), TaskAuthority: c.TaskAuthority, WorkflowProtocol: c.WorkflowProtocol, ShutdownGrace: c.ShutdownGrace, DefaultProfileID: c.DefaultProfileID()}
	for _, p := range c.RunnerExecution.Profiles {
		host := p.Execution.Target.Environment
		if host == "" {
			host = p.Execution.Target.Kind
		}
		result.Profiles = append(result.Profiles, productapi.ExecutionProfileSummary{ID: p.ID, Name: p.Name, Host: host, Agent: "codex", Digest: p.Digest(), AcceptedConfigVersion: c.LoadedConfigDigest(), ImplementationDisplay: []string{p.Execution.Adapter, host, p.Workflow.ID}, Details: []string{"SteerRunner " + c.RunnerExecution.Runner.Version, "CLI: " + runnerclient.Protocol, "工作流: " + p.Workflow.ID}})
	}
	return result
}

func clientServiceInstruction(ref productapi.ProjectRef) string {
	return filepath.Join(ref.ApplicationHome, "start.bat")
}

func productReadError(err error) error {
	if err == nil {
		return nil
	}
	var domain *taskmodel.Error
	if errors.As(err, &domain) {
		switch domain.Code {
		case "task_not_found":
			return errors.Join(&productapi.Error{Kind: productapi.UnknownRecord, Message: err.Error()}, err)
		case "authority_mismatch":
			return errors.Join(&productapi.Error{Kind: productapi.IdentityConflict, Message: err.Error()}, err)
		}
	}
	var hostFailure *steerline.HostError
	if errors.As(err, &hostFailure) && hostFailure.Code == "authority_mismatch" {
		return errors.Join(&productapi.Error{Kind: productapi.IdentityConflict, Message: err.Error()}, err)
	}
	return err
}

func publicProjectCapabilities(input []string) []string {
	result := make([]string, 0, len(input))
	for _, capability := range input {
		if capability != "stop_service" {
			result = append(result, capability)
		}
	}
	return result
}

func projectReadFreshness(c Config) time.Duration {
	freshness := 30 * time.Second
	if poll, err := time.ParseDuration(c.TaskPoll); err == nil && poll*3 > freshness {
		freshness = poll * 3
	}
	return freshness
}
