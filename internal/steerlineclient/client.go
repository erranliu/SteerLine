// Package steerlineclient owns the single product adapter. Concrete resource
// construction is injected by the application; it never imports SQL or app wiring.
package steerlineclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/query"
	"github.com/erranliu/SteerLine/internal/steerline/task"
	"github.com/erranliu/SteerLine/internal/steerline/taskview"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

var _ api.Client = (*Client)(nil)

type Client struct {
	mu        sync.RWMutex
	resources api.Resources
	closed    bool
	closeErr  error
}

func Open(ctx context.Context, ref api.ProjectRef, factory api.Factory) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if factory == nil {
		return nil, &productapi.Error{Kind: productapi.Unavailable, Cause: errors.New("product client resource factory is required")}
	}
	r, err := factory(ctx, ref)
	if err != nil {
		return nil, classify(err)
	}
	if r.Project.Identity == "" || r.Project.ConfigPath == "" || r.Project.RunDir == "" ||
		(ref.Identity != "" && ref.Identity != r.Project.Identity) || (ref.RunDir != "" && ref.RunDir != r.Project.RunDir) ||
		(ref.ConfigPath != "" && ref.ConfigPath != r.Project.ConfigPath) {
		if r.Close != nil {
			_ = r.Close()
		}
		return nil, &productapi.Error{Kind: productapi.IdentityConflict, Cause: errors.New("product client project identity or configuration does not match")}
	}
	r.Project = api.CloneProjectRef(r.Project)
	r.ExecutableTypes = append([]string(nil), r.ExecutableTypes...)
	return &Client{resources: r}, nil
}

func (c *Client) Project() api.ProjectRef { return api.CloneProjectRef(c.resources.Project) }

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		if c.resources.Close != nil {
			c.closeErr = c.resources.Close()
		}
	}
	return c.closeErr
}

func (c *Client) begin(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return &productapi.Error{Kind: productapi.Closed, Cause: errors.New("product client is closed")}
	}
	return nil
}

func classify(err error) error {
	if err == nil {
		return nil
	}
	var existing *productapi.Error
	if errors.As(err, &existing) {
		return err
	}
	kind := productapi.ReadFailed
	var te *taskmodel.Error
	switch {
	case errors.Is(err, productapi.ErrRecordNotFound):
		kind = productapi.UnknownRecord
	case os.IsNotExist(err):
		kind = productapi.MissingDatabase
	case errors.As(err, &te) && te.Code == "task_not_found":
		kind = productapi.UnknownRecord
	case errors.As(err, &te) && te.Code == "authority_mismatch":
		kind = productapi.IdentityConflict
	case strings.Contains(err.Error(), "schema") || strings.Contains(err.Error(), "unsupported") || strings.Contains(err.Error(), "older configurations"):
		kind = productapi.IncompatibleFormat
	case strings.Contains(err.Error(), "identity") || strings.Contains(err.Error(), "does not match"):
		kind = productapi.IdentityConflict
	}
	return &productapi.Error{Kind: kind, Cause: err}
}

func read[T any](ctx context.Context, c *Client, fn func(api.Reader) (T, error)) (result T, err error) {
	if err = c.begin(ctx); err != nil {
		return result, err
	}
	defer c.mu.RUnlock()
	if c.resources.OpenReader == nil {
		return result, errors.New("product reader is unavailable")
	}
	r, err := c.resources.OpenReader(ctx)
	if err != nil {
		return result, classify(err)
	}
	defer func() { err = classify(errors.Join(err, r.Close())) }()
	return fn(r)
}

func command[T any](ctx context.Context, c *Client, mutation bool, fn func(api.TaskCommands) (T, error)) (result T, err error) {
	if err = c.begin(ctx); err != nil {
		return result, err
	}
	defer c.mu.RUnlock()
	if c.resources.OpenCommands == nil {
		return result, errors.New("product commands are unavailable")
	}
	r, err := c.resources.OpenCommands(ctx, mutation)
	if err != nil {
		return result, classify(err)
	}
	defer func() { err = errors.Join(err, r.Close()) }()
	return fn(r)
}

func (c *Client) ShowTask(ctx context.Context, id string) (contracts.NativeTaskPage, error) {
	return read(ctx, c, func(r api.Reader) (contracts.NativeTaskPage, error) { return r.ShowNativeTask(ctx, id) })
}

func (c *Client) ListTasks(ctx context.Context, cursor string, limit int) (contracts.NativeTaskPage, error) {
	return read(ctx, c, func(r api.Reader) (contracts.NativeTaskPage, error) { return r.ListNativeTasks(ctx, cursor, limit) })
}

func (c *Client) TaskGraph(ctx context.Context, cursor string, limit int) (taskview.Page, error) {
	return read(ctx, c, func(r api.Reader) (taskview.Page, error) { return r.TaskGraph(ctx, cursor, limit) })
}

func (c *Client) QueryTaskGraph(ctx context.Context, filter taskview.Filter) (taskview.Page, error) {
	if filter.ProjectID != "" && filter.ProjectID != c.Project().Identity {
		return taskview.Page{}, &productapi.Error{Kind: productapi.IdentityConflict, Cause: errors.New("task graph belongs to another project")}
	}
	filter.ProjectID = c.Project().Identity
	return read(ctx, c, func(r api.Reader) (taskview.Page, error) { return r.QueryTaskGraph(ctx, filter) })
}

func (c *Client) ValidateTaskBatch(ctx context.Context, payload []byte) (taskmodel.TaskBatchValidation, error) {
	return command(ctx, c, false, func(r api.TaskCommands) (taskmodel.TaskBatchValidation, error) { return r.Validate(ctx, payload) })
}

func (c *Client) CreateTaskBatch(ctx context.Context, in productapi.CreateInput) (taskmodel.TaskBatchReceipt, error) {
	return command(ctx, c, true, func(r api.TaskCommands) (taskmodel.TaskBatchReceipt, error) { return r.Create(ctx, in) })
}

func (c *Client) DiscardTask(ctx context.Context, in productapi.DiscardInput) (taskmodel.TaskDiscardReceipt, error) {
	return command(ctx, c, true, func(r api.TaskCommands) (taskmodel.TaskDiscardReceipt, error) { return r.Discard(ctx, in) })
}

func (c *Client) AddPredecessors(ctx context.Context, in task.TaskPredecessorAddInput) (task.TaskPredecessorAddReceipt, error) {
	return command(ctx, c, true, func(r api.TaskCommands) (task.TaskPredecessorAddReceipt, error) { return r.AddPredecessors(ctx, in) })
}

func (c *Client) Conversation(ctx context.Context, filter contracts.ConversationFilter, cursor string) (contracts.ConversationPage, error) {
	return read(ctx, c, func(r api.Reader) (contracts.ConversationPage, error) {
		return r.ReadConversation(ctx, filter, cursor, query.MaxEventPageSize)
	})
}

func (c *Client) Body(ctx context.Context, body contracts.ConversationBodyRef, offset int64, limit int) (contracts.ConversationBodyPage, error) {
	return read(ctx, c, func(r api.Reader) (contracts.ConversationBodyPage, error) {
		return r.ReadMessageBody(ctx, body, offset, limit)
	})
}

func (c *Client) Raw(ctx context.Context, raw contracts.ConversationRawRef, offset int64, limit int) (contracts.ConversationRawPage, error) {
	return read(ctx, c, func(r api.Reader) (contracts.ConversationRawPage, error) { return r.ReadRaw(ctx, raw, offset, limit) })
}

func (c *Client) Capabilities(ctx context.Context, f api.CapabilityFilter) ([]contracts.ConversationCapabilityReport, error) {
	return read(ctx, c, func(r api.Reader) ([]contracts.ConversationCapabilityReport, error) {
		reports, err := r.ReadCapabilities(ctx, f.Backend, f.Protocol, f.TargetID, 100)
		if err != nil {
			return nil, err
		}
		filtered := make([]contracts.ConversationCapabilityReport, 0, len(reports))
		for _, v := range reports {
			if f.ExecutionID != "" && v.ExecutionID != f.ExecutionID || f.AttemptID != "" && v.AttemptID != f.AttemptID || f.Source != "" && v.Source != f.Source || f.SourceIdentity != "" && v.SourceIdentity != f.SourceIdentity {
				continue
			}
			filtered = append(filtered, v)
		}
		return filtered, nil
	})
}

func (c *Client) Command(ctx context.Context, id string) (storageports.CommandRecord, error) {
	return read(ctx, c, func(r api.Reader) (storageports.CommandRecord, error) { return r.Command(ctx, id) })
}

func (c *Client) WaitCommand(ctx context.Context, id string, timeout time.Duration) (storageports.CommandRecord, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	t := time.NewTicker(150 * time.Millisecond)
	defer t.Stop()
	for {
		if waitCtx.Err() != nil {
			return storageports.CommandRecord{}, fmt.Errorf("command %s remains unconfirmed; query this same command ID later", id)
		}
		r, err := c.Command(waitCtx, id)
		if err == nil && (r.Status == "accepted" || r.Status == "rejected" || r.Status == "failed" || r.Status == "indeterminate") {
			return r, nil
		}
		if waitCtx.Err() != nil {
			continue
		}
		if err != nil && !errors.Is(err, productapi.ErrRecordNotFound) && !os.IsNotExist(err) {
			return storageports.CommandRecord{}, err
		}
		select {
		case <-waitCtx.Done():
		case <-t.C:
		}
	}
}

func NewRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (c *Client) SubmitControl(ctx context.Context, in productapi.ControlInput) (productapi.ControlRequest, error) {
	if err := c.begin(ctx); err != nil {
		return productapi.ControlRequest{}, err
	}
	defer c.mu.RUnlock()
	if c.resources.Control == nil {
		return productapi.ControlRequest{}, errors.New("control is unavailable")
	}
	if in.RequestID == "" {
		return productapi.ControlRequest{}, errors.New("control requires a stable request ID before submission")
	}
	if in.RequestedBy == "" {
		in.RequestedBy = c.resources.DefaultRequestedBy
	}
	if in.Payload.Environment != nil {
		target := *in.Payload.Environment
		in.Payload.Environment = &target
	}
	old, err := c.resources.Control.Request(ctx, in.RequestID)
	if err == nil {
		var payload productapi.ControlPayload
		if old == nil || old.RepositoryIdentity != c.Project().Identity || old.Action != string(in.Action) || contracts.DecodeJSON(old.Payload, &payload) != nil && len(old.Payload) != 0 || !reflect.DeepEqual(payload, in.Payload) || old.RequestedBy != in.RequestedBy || in.InstanceID != "" && old.InstanceID != in.InstanceID || in.ExpectedRevision != nil && old.ExpectedControlRevision != *in.ExpectedRevision {
			return productapi.ControlRequest{}, errors.New("request ID conflict: immutable request has different content or identity")
		}
		return *old, nil
	}
	if !os.IsNotExist(err) {
		return productapi.ControlRequest{}, err
	}
	return c.resources.Control.Submit(ctx, in)
}

func (c *Client) Receipt(ctx context.Context, id string) (*productapi.ControlReceipt, error) {
	if err := c.begin(ctx); err != nil {
		return nil, err
	}
	defer c.mu.RUnlock()
	if c.resources.Control == nil {
		return nil, errors.New("control is unavailable")
	}
	return c.resources.Control.Receipt(ctx, id)
}

func (c *Client) Request(ctx context.Context, id string) (*productapi.ControlRequest, error) {
	if err := c.begin(ctx); err != nil {
		return nil, err
	}
	defer c.mu.RUnlock()
	if c.resources.Control == nil {
		return nil, errors.New("control is unavailable")
	}
	return c.resources.Control.Request(ctx, id)
}

func (c *Client) WaitControl(ctx context.Context, id string, poll time.Duration) (*productapi.ControlReceipt, error) {
	if err := c.begin(ctx); err != nil {
		return nil, err
	}
	defer c.mu.RUnlock()
	if c.resources.Control == nil {
		return nil, errors.New("control is unavailable")
	}
	if poll <= 0 {
		poll = 100 * time.Millisecond
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		receipt, err := c.resources.Control.Receipt(ctx, id)
		if err == nil && receipt.Status != "applying" {
			return receipt, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Client) StopService(ctx context.Context, by string, timeout time.Duration) (productapi.ServiceStopResult, error) {
	if err := c.begin(ctx); err != nil {
		return productapi.ServiceStopResult{}, err
	}
	defer c.mu.RUnlock()
	if c.resources.Control == nil {
		return productapi.ServiceStopResult{}, errors.New("control is unavailable")
	}
	return c.resources.Control.Stop(ctx, by, timeout)
}

func (c *Client) DecisionEligibility(ctx context.Context, run *contracts.Run) error {
	if err := c.begin(ctx); err != nil {
		return err
	}
	defer c.mu.RUnlock()
	if c.resources.Decision == nil {
		return errors.New("decision commands are unavailable")
	}
	return c.resources.Decision.Eligibility(ctx, run)
}

func (c *Client) SubmitDecision(ctx context.Context, in api.DecisionInput) (storageports.CommandRecord, bool, error) {
	if err := c.begin(ctx); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	defer c.mu.RUnlock()
	if c.resources.Decision == nil {
		return storageports.CommandRecord{}, false, errors.New("decision commands are unavailable")
	}
	if in.CommandID == "" {
		return storageports.CommandRecord{}, false, errors.New("decision requires a stable command ID before submission")
	}
	if in.RequestedBy == "" {
		in.RequestedBy = c.resources.DefaultAnswerer
		if in.RequestedBy == "" {
			in.RequestedBy = c.resources.DefaultRequestedBy
		}
	}
	if in.Answer.AnsweredBy == "" {
		in.Answer.AnsweredBy = in.RequestedBy
	}
	return c.resources.Decision.Submit(ctx, in)
}

func (c *Client) Answer(ctx context.Context, in productapi.AnswerInput) (storageports.CommandRecord, bool, error) {
	if err := c.begin(ctx); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	defer c.mu.RUnlock()
	if c.resources.Decision == nil {
		return storageports.CommandRecord{}, false, errors.New("decision commands are unavailable")
	}
	if in.CommandID == "" {
		return storageports.CommandRecord{}, false, errors.New("answer requires a stable command ID before submission")
	}
	if in.RequestedBy == "" {
		in.RequestedBy = c.resources.DefaultRequestedBy
	}
	return c.resources.Decision.Answer(ctx, in)
}

func (c *Client) TaskSnapshot(ctx context.Context) (*contracts.TaskSnapshot, error) {
	if err := c.begin(ctx); err != nil {
		return nil, err
	}
	defer c.mu.RUnlock()
	if c.resources.TaskSnapshot == nil {
		return nil, errors.New("task snapshot is unavailable")
	}
	return c.resources.TaskSnapshot(ctx)
}

func (c *Client) StartTaskPayload(ctx context.Context, taskID, profileID string) (productapi.ControlPayload, error) {
	if err := c.begin(ctx); err != nil {
		return productapi.ControlPayload{}, err
	}
	defer c.mu.RUnlock()
	if c.resources.ResolveStartTask == nil {
		return productapi.ControlPayload{}, errors.New("execution profile resolver is unavailable")
	}
	return c.resources.ResolveStartTask(ctx, taskID, profileID)
}

func (c *Client) ReadProject(ctx context.Context, selected string, previous api.ProjectSnapshot) api.ProjectSnapshot {
	result := api.ProjectSnapshot{SelectedTaskID: selected, CandidateFreshnessLimit: c.resources.CandidateFreshnessLimit}
	if err := c.begin(ctx); err != nil {
		result.TaskError = err.Error()
		result.DomainError = err.Error()
		return result
	}
	defer c.mu.RUnlock()
	r := c.resources
	if r.OpenReader == nil || r.Connect == nil || r.Observe == nil {
		result.TaskError = "project reader is unavailable"
		result.DomainError = result.TaskError
		return result
	}
	reader, err := r.OpenReader(ctx)
	if err != nil {
		result.Connection = r.Connect(ctx, nil)
		result.Observation = result.Connection.Snapshot
		result.TaskError = classify(err).Error()
		result.NativeTaskError = result.TaskError
		result.DomainError = result.TaskError
		return result
	}
	if r.ProbeProcess != nil && ReusableProjectSnapshot(ctx, r.Project, selected, previous, r.ProbeProcess, func(ctx context.Context, _ string) (string, uint64, error) { return reader.DatabaseVersion(ctx) }) &&
		r.Connect(ctx, &previous.Observation).State == productapi.ConnectionConnected {
		if err := reader.Close(); err == nil {
			return previous
		} else {
			result.TaskError = err.Error()
			result.DomainError = err.Error()
			return result
		}
	}
	view, err := reader.ReadProjectView(ctx, selected, 500)
	err = errors.Join(err, reader.Close())
	if err != nil {
		result.Connection = r.Connect(ctx, nil)
		result.Observation = result.Connection.Snapshot
		result.TaskError = err.Error()
		result.NativeTaskError = result.TaskError
		result.DomainError = result.TaskError
		return result
	}
	result.Observation = r.Observe(view.Runtime, view.RuntimeError, view.SampledAt)
	result.Connection = r.Connect(ctx, &result.Observation)
	result.Observation = result.Connection.Snapshot
	result.DecisionReasons = map[string]string{}
	if r.DecisionAvailability != nil && result.Observation.Runtime != nil {
		for _, run := range result.Observation.Runtime.Runs {
			if run != nil && run.Decision != nil {
				reason := r.DecisionAvailability(ctx, run)
				if reason == "" && (result.Connection.State != productapi.ConnectionConnected || !result.Connection.CanControl) {
					reason = "服务离线或未就绪；答案不会写入，恢复服务后再通过同一命令提交"
				}
				if reason == "" && result.Connection.Snapshot.Runtime == nil {
					reason = "缺少当前服务实例快照"
				}
				if reason == "" {
					found := false
					for _, capability := range result.Connection.Snapshot.Runtime.Service.Capabilities {
						if capability == "answer_decision" {
							found = true
						}
					}
					if !found {
						reason = "服务未声明 answer_decision 能力"
					}
				}
				if reason == "" {
					reason = "提交后核对原会话接收与工作轮次开始"
				}
				result.DecisionReasons[run.RunID] = reason
			}
		}
	}
	result.NativeTasks, result.SelectedNative, result.Domain = view.Tasks, view.SelectedTask, view.Domain
	if view.TaskError != nil {
		result.TaskError = view.TaskError.Error()
		if view.Tasks == nil {
			result.NativeTaskError = result.TaskError
		}
	}
	if view.Ready != nil {
		result.Tasks = NativeCandidates(r.Project.Identity, r.ExecutableTypes, result.Connection, *view.Ready)
	}
	if view.SelectedTaskError != nil {
		result.NativeTaskError = view.SelectedTaskError.Error()
	}
	if view.DomainError != nil {
		result.DomainError = view.DomainError.Error()
	}
	if result.TaskError == "" && result.NativeTaskError == "" && result.DomainError == "" {
		result.FullReadAt = view.SampledAt
	}
	return result
}

func NativeCandidates(identity string, types []string, connection api.ServiceConnection, page contracts.NativeReadyTaskSnapshot) *contracts.TaskSnapshot {
	at := page.SampledAt
	s := &contracts.TaskSnapshot{Version: 1, RepositoryIdentity: identity, QueriedAt: at, LastSuccessAt: &at, Tasks: []contracts.TaskCandidate{}}
	if connection.State == productapi.ConnectionConnected && connection.CanControl && connection.Snapshot.Runtime != nil {
		s.InstanceID = connection.Snapshot.Runtime.Service.InstanceID
	}
	for _, t := range page.Tasks {
		allowed := false
		for _, kind := range types {
			if kind == t.Type {
				allowed = true
				break
			}
		}
		if t.Status == "open" && allowed {
			s.Tasks = append(s.Tasks, contracts.TaskCandidate{TaskID: t.TaskID, Title: t.Title, Type: t.Type})
		}
	}
	return s
}

func ReusableProjectSnapshot(ctx context.Context, ref api.ProjectRef, selected string, previous api.ProjectSnapshot, probe func(*executionbackend.ProcessIdentity) productapi.ProcessProbe, version func(context.Context, string) (string, uint64, error)) bool {
	age := time.Since(previous.FullReadAt)
	if previous.FullReadAt.IsZero() || age < 0 || age >= 5*time.Second || previous.SelectedTaskID != selected || previous.Connection.State != productapi.ConnectionConnected || previous.NativeTasks == nil || previous.Tasks == nil || previous.Domain == nil || previous.Observation.Runtime == nil || previous.TaskError != "" || previous.NativeTaskError != "" || previous.DomainError != "" {
		return false
	}
	if previous.NativeTasks.ProjectID != ref.Identity || previous.NativeTasks.DatabaseID == "" || previous.NativeTasks.DatabaseRevision == 0 {
		return false
	}
	rt := previous.Connection.Snapshot.Runtime
	if rt == nil || rt.Repository.Identity != ref.Identity || rt.Service.ProcessIdentity == nil || probe(rt.Service.ProcessIdentity).State != productapi.ProcessMatches {
		return false
	}
	if previous.Observation.FreshnessLimit > 0 && time.Since(previous.Observation.Runtime.HeartbeatAt) >= previous.Observation.FreshnessLimit {
		return false
	}
	id, rev, err := version(ctx, filepath.Join(ref.RunDir, "steerline.db"))
	return err == nil && id == previous.NativeTasks.DatabaseID && rev == previous.NativeTasks.DatabaseRevision
}

func (c *Client) Activity(ctx context.Context, runID, executionID, attemptID string, cursor productapi.ActivityCursor) (api.LogBatch, error) {
	if runID == "" || executionID == "" || attemptID == "" {
		return api.LogBatch{}, errors.New("activity query identity is missing")
	}
	if cursor.Initialized && (cursor.RunID != runID || cursor.ExecutionID != executionID || cursor.AttemptID != attemptID) {
		return api.LogBatch{}, errors.New("activity cursor belongs to another execution or attempt")
	}
	return read(ctx, c, func(reader api.Reader) (api.LogBatch, error) {
		domain, err := reader.Domain(ctx)
		if err != nil {
			return api.LogBatch{}, err
		}
		owned := false
		for _, e := range domain.Executions {
			if e.ExecutionID == executionID && e.WorkflowRunID == runID {
				owned = true
				break
			}
		}
		if !owned {
			return api.LogBatch{}, errors.New("activity execution does not belong to the requested run")
		}
		owned = false
		for _, a := range domain.Attempts {
			if a.ExecutionID == executionID && a.AttemptID == attemptID {
				owned = true
				break
			}
		}
		if !owned {
			return api.LogBatch{}, errors.New("activity attempt does not belong to the requested execution")
		}
		events, err := reader.ActivityEvents(ctx, executionID, attemptID, cursor.AfterEventID)
		if err != nil {
			return api.LogBatch{}, err
		}
		batch := api.LogBatch{Cursor: productapi.ActivityCursor{RunID: runID, ExecutionID: executionID, AttemptID: attemptID, AfterEventID: cursor.AfterEventID, Initialized: true}}
		for _, event := range events {
			if event.ProjectIdentity != c.Project().Identity || event.ExecutionID != executionID || event.AttemptID != attemptID || event.WorkflowRunID != runID || event.EventID <= batch.Cursor.AfterEventID {
				return api.LogBatch{}, errors.New("canonical activity page changed its fixed identity or order")
			}
			batch.Cursor.AfterEventID = event.EventID
			if !strings.HasPrefix(event.Type, "activity.") {
				continue
			}
			var activity contracts.ActivityEvent
			if len(event.Payload) == 0 || json.Unmarshal(event.Payload, &activity) != nil {
				batch.Diagnostics = append(batch.Diagnostics, "规范活动尚未完整采集；原始文件仅供手动诊断")
				continue
			}
			activity.Seq = uint64(event.EventID)
			batch.Activity = append(batch.Activity, activity)
		}
		if !cursor.Initialized && len(batch.Activity) == 0 {
			batch.Diagnostics = append(batch.Diagnostics, "数据库中暂无已采集活动；不会从原始日志补画")
		}
		return batch, nil
	})
}

func (c *Client) CursorAfter(filter contracts.ConversationFilter, after int64) (string, error) {
	if err := c.begin(context.Background()); err != nil {
		return "", err
	}
	defer c.mu.RUnlock()
	return query.ConversationCursorAfter(c.Project().Identity, filter, after)
}

func (c *Client) InspectRecovery(ctx context.Context, id string) (api.RecoveryInspection, error) {
	if err := c.begin(ctx); err != nil {
		return api.RecoveryInspection{}, err
	}
	defer c.mu.RUnlock()
	if c.resources.InspectRecovery == nil {
		return api.RecoveryInspection{}, errors.New("recovery inspector is unavailable")
	}
	return c.resources.InspectRecovery(ctx, id)
}

func (c *Client) CandidateAvailability(snapshot api.ProjectSnapshot, now time.Time) (bool, string) {
	if err := c.begin(context.Background()); err != nil {
		return false, err.Error()
	}
	defer c.mu.RUnlock()
	return api.CandidateFreshness(c.resources.Project.Identity, snapshot, now)
}

func (c *Client) StartTaskAvailability(snapshot api.ProjectSnapshot, now time.Time) string {
	if err := c.begin(context.Background()); err != nil {
		return err.Error()
	}
	defer c.mu.RUnlock()
	return api.StartTaskReason(c.resources.Project, snapshot, now)
}

func (c *Client) AutoDispatchAvailability(snapshot api.ProjectSnapshot) string {
	if err := c.begin(context.Background()); err != nil {
		return err.Error()
	}
	defer c.mu.RUnlock()
	return api.AutoDispatchReason(c.resources.Project, snapshot)
}
