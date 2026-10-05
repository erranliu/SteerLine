package steerlineapp

import (
	"context"
	"errors"
	"fmt"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
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

// ClientResources is the application construction seam. It binds one immutable
// config/directory to pure ports; all product orchestration lives in the adapter.
func ClientResources(ctx context.Context, ref productapi.ProjectRef) (productclient.Resources, error) {
	c, err := LoadQueryConfig(ref.ConfigPath)
	if err != nil {
		return productclient.Resources{}, err
	}
	return clientResources(ctx, c, ref)
}

func clientResources(ctx context.Context, c execution.Config, ref productapi.ProjectRef) (productclient.Resources, error) {
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
	ref.ApplicationHome, ref.ApplicationBin = HomeDir(), BinDir()
	ref.Presentation.ServiceInstruction = clientServiceInstruction(ref)
	if ref.Key == "" {
		ref.Key = ref.ConfigPath + "\x00" + ref.RunDir
	}
	freshness := 30 * time.Second
	if poll, err := time.ParseDuration(c.TaskPoll); err == nil && poll*3 > freshness {
		freshness = poll * 3
	}
	return productclient.Resources{
		Project: ref, DefaultRequestedBy: steerline.DefaultRequestedBy(), DefaultAnswerer: steerline.AnswererName(),
		ReadFailure: func(ctx context.Context, selected string, readErr error) productapi.ProjectSnapshot {
			legacy := steerline.ConnectService(ctx, c)
			obs := productObservation(legacy.Snapshot)
			connection := productConnection(legacy, obs)
			return productapi.ProjectSnapshot{SelectedTaskID: selected, Observation: obs, Connection: connection, TaskError: readErr.Error(), NativeTaskError: readErr.Error(), HistoryError: readErr.Error()}
		},
		OpenReader: func(ctx context.Context) (productclient.Reader, error) {
			cfg := c
			s, err := steerline.OpenNativeTaskReader(ctx, &cfg)
			if err != nil {
				return nil, err
			}
			return &clientReader{Service: query.NewService(s, c.Identity()), ConversationService: query.NewConversationService(s, c.Identity()), store: s, project: c.Identity(), config: c, executableTypes: append([]string(nil), c.ExecutableTypes...), freshness: freshness}, nil
		},
		OpenCommands: func(ctx context.Context, mutation bool) (productclient.TaskCommands, error) {
			cfg := c
			s, err := steerline.OpenNativeTaskProject(ctx, &cfg)
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
				s, err = steerline.OpenNativeTaskCommandStore(ctx, &cfg)
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
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			reader, err := steerline.OpenNativeTaskReader(ctx, &c)
			if err != nil {
				return nil, err
			}
			defer reader.Close()
			view, err := query.NewService(reader, c.Identity()).ReadProjectView(ctx, "", 500)
			if err != nil {
				return nil, err
			}
			if view.Ready == nil {
				return nil, errors.New("canonical task candidate snapshot is unavailable")
			}
			return query.ReadyCandidates(*view.Ready, view.SampledAt, productapi.ServiceConnection{}, c.ExecutableTypes), nil
		},
		ResolveStartTask: func(ctx context.Context, id, profileID string) (productapi.ControlPayload, error) {
			if profileID == "" {
				return productapi.ControlPayload{}, errors.New("start_task requires -execution-profile")
			}
			_, p, err := c.ResolveExecutionProfile(profileID, "")
			if err != nil {
				return productapi.ControlPayload{}, err
			}
			return productapi.ControlPayload{TaskID: id, ExecutionProfileID: p.ID, ExecutionProfileDigest: p.Digest()}, nil
		},
		InspectRecovery: func(ctx context.Context, id string) (productapi.RecoveryInspection, error) {
			result, err := execution.InspectRecovery(ctx, c, id)
			return recoveryInspection(result), err
		},
	}, nil
}

type clientReader struct {
	query.Service
	query.ConversationService
	store           persistence.Connection
	project         string
	config          execution.Config
	executableTypes []string
	freshness       time.Duration
}

func (r *clientReader) Close() error { return r.store.Close() }

func (r *clientReader) ReadProject(ctx context.Context, selected string, _ productapi.ProjectSnapshot) productapi.ProjectSnapshot {
	view, err := r.ReadProjectView(ctx, selected, 500)
	if err != nil {
		legacyConnection := steerline.ConnectService(ctx, r.config)
		observation := productObservation(legacyConnection.Snapshot)
		connection := productConnection(legacyConnection, observation)
		page := query.ProjectPage(view, selected, r.freshness, observation, connection, nil, r.executableTypes)
		page.TaskError, page.NativeTaskError, page.HistoryError = err.Error(), err.Error(), err.Error()
		return page
	}
	observed := steerline.ObserveRuntimeSnapshot(r.config, view.Runtime, view.RuntimeError, view.SampledAt)
	legacyConnection := steerline.ConnectServiceFromSnapshot(ctx, r.config, observed)
	observation := productObservation(legacyConnection.Snapshot)
	connection := productConnection(legacyConnection, observation)
	decisionReasons := map[string]string{}
	if view.Runtime != nil {
		for _, run := range view.Runtime.Runs {
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
func (r *clientReader) QueryTaskGraph(ctx context.Context, filter productapi.GraphFilter) (productapi.GraphPage, error) {
	page, err := r.Service.QueryTaskGraph(ctx, filter)
	return page, productReadError(err)
}
func (r *clientReader) Conversation(ctx context.Context, filter productapi.ConversationFilter, cursor string, limit int) (productapi.ConversationPage, error) {
	page, err := r.ConversationService.ReadConversation(ctx, contracts.ConversationFilter(filter), cursor, limit)
	if err != nil {
		return productapi.ConversationPage{}, err
	}
	return query.ConversationPage(page)
}
func (r *clientReader) Body(ctx context.Context, ref productapi.ConversationBodyRef, offset int64, limit int) (productapi.ConversationBodyPage, error) {
	page, err := r.ConversationService.ReadMessageBody(ctx, contracts.ConversationBodyRef{ExecutionID: ref.ExecutionID, AttemptID: ref.AttemptID, ItemKey: ref.ItemKey, Revision: ref.Revision, SHA256: ref.SHA256, Bytes: ref.Bytes, Source: ref.Source, Completeness: contracts.BodyCompleteness(ref.Completeness)}, offset, limit)
	if err != nil {
		return productapi.ConversationBodyPage{}, err
	}
	return query.ConversationBodyPage(page), nil
}
func (r *clientReader) Raw(ctx context.Context, ref productapi.ConversationRawRef, offset int64, limit int) (productapi.ConversationRawPage, error) {
	page, err := r.ConversationService.ReadRaw(ctx, contracts.ConversationRawRef{ExecutionID: ref.ExecutionID, AttemptID: ref.AttemptID, ID: ref.ID, SHA256: ref.SHA256, Bytes: ref.Bytes}, offset, limit)
	if err != nil {
		return productapi.ConversationRawPage{}, err
	}
	return query.ConversationRawPage(page), nil
}
func (r *clientReader) Capabilities(ctx context.Context, f productapi.CapabilityFilter) ([]productapi.ConversationCapabilityReport, error) {
	records, err := r.ConversationService.ReadCapabilities(ctx, executionbackend.BackendKind(f.Backend), executionbackend.ExecutionProtocol(f.Protocol), f.TargetID, 100)
	if err != nil {
		return nil, err
	}
	out := make([]productapi.ConversationCapabilityReport, 0, len(records))
	for _, record := range records {
		if f.ExecutionID != "" && record.ExecutionID != f.ExecutionID || f.AttemptID != "" && record.AttemptID != f.AttemptID || f.Source != "" && record.Source != f.Source || f.SourceIdentity != "" && record.SourceIdentity != f.SourceIdentity {
			continue
		}
		out = append(out, query.CapabilityReport(record))
	}
	return out, nil
}
func (r *clientReader) Command(ctx context.Context, id string) (productapi.CommandReceipt, error) {
	record, err := r.store.Command(ctx, r.project, id)
	if errors.Is(err, storageports.ErrNotFound) {
		return productapi.CommandReceipt{}, errors.Join(productapi.ErrRecordNotFound, err)
	}
	return query.CommandReceipt(record), err
}
func (r *clientReader) Activity(ctx context.Context, runID, executionID, attemptID string, cursor productapi.ActivityCursor) (productapi.LogBatch, error) {
	return r.Service.ReadActivity(ctx, runID, executionID, attemptID, cursor)
}

func (r *clientReader) DatabaseVersion(ctx context.Context) (string, uint64, error) {
	return r.store.DatabaseVersion(ctx)
}

// These compile-time assertions keep the injected port tied to real consumers.
var _ productclient.Reader = (*clientReader)(nil)

type clientControl struct{ config execution.Config }

func (r clientControl) Submit(ctx context.Context, in productapi.ControlInput) (productapi.ControlRequest, error) {
	if err := ctx.Err(); err != nil {
		return productapi.ControlRequest{}, err
	}
	var payload any
	if in.Payload != (productapi.ControlPayload{}) {
		payload = in.Payload
	}
	return steerline.SubmitControlTargeted(r.config, string(in.Action), payload, in.RequestedBy, in.RequestID, in.InstanceID, in.ExpectedRevision)
}

func (r clientControl) Receipt(ctx context.Context, id string) (*productapi.ControlReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := r.Request(ctx, id); err != nil {
		return nil, err
	}
	return steerline.ReadVerifiedControlReceipt(r.config, id)
}

func (r clientControl) Request(ctx context.Context, id string) (*productapi.ControlRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	req, err := steerline.ReadControlRequest(r.config, id)
	if err == nil && req.RepositoryIdentity != r.config.Identity() {
		return nil, errors.New("control request project identity does not match")
	}
	return req, err
}

func (r clientControl) Stop(ctx context.Context, by string, timeout time.Duration) (productapi.ServiceStopResult, error) {
	return steerline.StopService(ctx, r.config, by, timeout)
}

type clientDecision struct{ config execution.Config }

func (r clientDecision) Eligibility(ctx context.Context, run *productapi.CandidateSummary) error {
	return execution.DecisionReplyEligibility(ctx, r.config, runtimeDecisionFacts(run))
}

func (r clientDecision) Submit(ctx context.Context, in productapi.DecisionInput) (productapi.CommandReceipt, bool, error) {
	a := in.Answer
	record, submitted, err := steerline.SubmitAnswerDecision(ctx, r.config, a.RunID, a.DecisionID, a.OptionID, a.Note, in.RequestedBy, a.ExpectedDecisionVersion, a.ExpectedRunRevision, a.ServiceInstanceID, in.CommandID, a.Answers)
	return query.CommandReceipt(record), submitted, err
}

func (r clientDecision) Answer(ctx context.Context, in productapi.AnswerInput) (productapi.CommandReceipt, bool, error) {
	record, submitted, err := steerline.SubmitDecisionAnswer(ctx, r.config, steerline.AnswerOptions{RunID: in.RunID, DecisionID: in.DecisionID, Option: in.Option, Note: in.Note, By: in.RequestedBy, ExpectedVersion: in.ExpectedVersion, CommandID: in.CommandID})
	return query.CommandReceipt(record), submitted, err
}

func productObservation(value steerline.ObservationSnapshot) productapi.ObservationSnapshot {
	return productapi.ObservationSnapshot{Page: query.ServicePage(value.Runtime), State: value.State, SampledAt: value.SampledAt, HeartbeatAge: value.HeartbeatAge, FreshnessLimit: value.FreshnessLimit, ClockSkew: value.ClockSkew, Error: value.Error}
}

func productConnection(value steerline.ServiceConnection, observation productapi.ObservationSnapshot) productapi.ServiceConnection {
	valueObservation := productObservation(value.Snapshot)
	if value.Snapshot.Runtime == nil && observation.Page != nil {
		valueObservation = observation
	}
	return productapi.ServiceConnection{Snapshot: valueObservation, Process: value.Process, State: value.State, IdentityMatched: value.IdentityMatched, ConfigMatched: value.ConfigMatched, Protocol: value.Protocol, Capabilities: append([]string(nil), value.Capabilities...), CanControl: value.CanControl, CanStop: value.CanStop, CanStart: value.CanStart, StartupID: value.StartupID, Diagnostic: value.Diagnostic}
}

func decisionAvailability(ctx context.Context, c execution.Config, run *contracts.Run, connection steerline.ServiceConnection) string {
	if run == nil || run.Decision == nil {
		return "当前没有待回答的问题"
	}
	if run.Decision.RequestPath != "" || run.Decision.AnswerPath != "" {
		return "问题绑定了已移除的 sidecar-answer 格式；保留状态并转入人工恢复，不导入答案文件"
	}
	if err := execution.DecisionReplyEligibility(ctx, c, run); err != nil {
		return err.Error()
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

func runtimeDecisionFacts(run *productapi.CandidateSummary) *contracts.Run {
	if run == nil {
		return nil
	}
	v := &contracts.Run{TaskID: run.TaskID, RunID: run.RunID, WorkflowVersion: run.WorkflowVersion, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, TaskStatus: run.TaskStatus, RecoveryRequired: run.RecoveryRequired, RecoveryState: run.RecoveryState}
	if run.Abandonment != nil {
		v.Abandonment = &executionmodel.CandidateAbandonment{}
	}
	if run.EnvironmentBlocker != nil {
		v.EnvironmentBlocker = &executionmodel.WorkerHostBlocker{ID: run.EnvironmentBlocker.ID, TurnID: run.EnvironmentBlocker.TurnID, Version: run.EnvironmentBlocker.Version}
	}
	if run.Decision != nil {
		d := run.Decision.Request
		request := executionmodel.DecisionRequest{Question: d.Question, Context: d.Context, Recommended: d.Recommended, AllowFreeText: d.AllowFreeText}
		for _, o := range d.Options {
			request.Options = append(request.Options, executionbackend.DecisionOption{ID: o.ID, Label: o.Label, Detail: o.Detail})
		}
		if d.HasNativeQuestions {
			request.QuestionRoute = &executionbackend.ImplementationDataEnvelope{}
		}
		v.Decision = &executionmodel.DecisionState{ID: run.Decision.ID, AttemptID: run.Decision.AttemptID, Version: run.Decision.Version, Status: run.Decision.Status, Request: request}
	}
	return v
}

func recoveryInspection(r execution.RecoveryInspection) productapi.RecoveryInspection {
	v := productapi.RecoveryInspection{Version: r.Version, SampledAt: r.SampledAt, RunID: r.RunID, RunRevision: r.RunRevision, ServiceInstanceID: r.ServiceInstanceID, ServiceStatus: r.ServiceStatus, DispatchMode: r.DispatchMode, DispatchPaused: r.DispatchPaused, SlotOccupied: r.SlotOccupied, TaskID: r.TaskID, DecisionID: r.DecisionID, DecisionStatus: r.DecisionStatus, ExecutionStatus: r.ExecutionStatus, DeliveryStatus: r.DeliveryStatus, Backend: r.Backend, Protocol: r.Protocol, Target: productapi.TargetSummary{ID: r.Target.ID}, ProcessCheck: r.ProcessCheck, WorkspaceError: r.WorkspaceError, ClaimError: r.ClaimError, PRError: r.PRError, ConcurrentActionReason: r.ConcurrentActionReason}
	if r.Workspace != nil {
		w := r.Workspace
		v.Workspace = &productapi.WorkspaceSummary{Worktree: w.Worktree, Base: w.Base, Head: w.Head, Dirty: w.Dirty, CheckedAt: w.CheckedAt}
	}
	if r.NativeClaim != nil {
		v.NativeClaim = ptr(query.TaskSummary(*r.NativeClaim))
	}
	if r.PR != nil {
		v.PR = &productapi.RecoveryPRSummary{URL: r.PR.URL, State: r.PR.State, Repository: r.PR.Repository, BaseBranch: r.PR.BaseBranch}
	}
	for _, b := range r.Blockers {
		v.Blockers = append(v.Blockers, productapi.ControlBlocker{RequestID: b.RequestID, Action: b.Action, TaskID: b.TaskID, RunID: b.RunID, AttemptID: b.AttemptID, DecisionID: b.DecisionID, Reason: b.Reason})
	}
	return v
}

func ptr[T any](v T) *T { return &v }

// projectPresentation exposes only accepted non-secret display fields and fixed
// profile digests. Configuration compilation stays with the original owner.
func projectPresentation(c execution.Config) productapi.ProjectPresentation {
	result := productapi.ProjectPresentation{NativeWorkflow: c.WorkflowProtocol == executionmodel.Version, ConfigDigest: c.LoadedConfigDigest(), TaskAuthority: c.TaskAuthority, WorkflowProtocol: c.WorkflowProtocol, ShutdownGrace: c.ShutdownGrace, DefaultProfileID: c.DefaultProfileID()}
	for _, p := range c.AvailableExecutionProfiles() {
		reason := p.StaticUnavailableReason()
		if reason == "" {
			if _, _, err := c.ResolveExecutionProfile(p.ID, p.Digest()); err != nil {
				reason = err.Error()
			}
		}
		fixed := executionmodel.FixedExecutionProfile{ID: p.ID, Name: p.Name, Host: p.Host, Agent: p.Agent, Digest: p.Digest(), AcceptedConfigVersion: p.AcceptedConfigVersion, PolicySource: p.PolicySource, PublicationPolicy: p.PublicationPolicy, Publication: p.Publication, ExecutionPolicy: p.ExecutionPolicy, ImplementationDisplay: append([]string(nil), p.ImplementationDisplay...), ExecutionRequirements: p.ExecutionRequirements}
		summary := query.Profile(fixed)
		summary.UnavailableReason = reason
		result.Profiles = append(result.Profiles, summary)
	}
	return result
}

func clientServiceInstruction(ref productapi.ProjectRef) string {
	if runtime.GOOS != "windows" {
		return "steerline start-service -config " + ref.ConfigPath
	}
	launcher := filepath.Join(ref.ApplicationHome, "service-window.bat")
	binary := filepath.Join(ref.ApplicationBin, "steerline.exe")
	if _, err := os.Stat(binary); err != nil {
		return fmt.Sprintf("服务窗口不可用：缺少 %s；运行 steerline install", binary)
	}
	if _, err := os.Stat(launcher); err != nil {
		return fmt.Sprintf("服务窗口不可用：缺少 %s；运行 steerline launchers", launcher)
	}
	return launcher
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
	return err
}
