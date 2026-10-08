// Package client owns the single product adapter. Concrete resource
// construction is injected by the application; it never imports SQL or app wiring.
package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"
)

type Client struct {
	mu        sync.RWMutex
	resources Resources
	closed    bool
	closeErr  error
}

func Open(ctx context.Context, ref api.ProjectRef, factory Factory) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if factory == nil {
		return nil, &api.Error{Kind: api.Unavailable, Message: "product client resource factory is required"}
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
		return nil, &api.Error{Kind: api.IdentityConflict, Message: "product client project identity or configuration does not match"}
	}
	r.Project = api.CloneProjectRef(r.Project)
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
		return &api.Error{Kind: api.Closed, Message: "product client is closed"}
	}
	return nil
}

type classifiedError struct {
	public api.Error
	cause  error
}

func (e *classifiedError) Error() string   { return e.public.Message }
func (e *classifiedError) Unwrap() []error { return []error{&e.public, e.cause} }

func classify(err error) error {
	if err == nil {
		return nil
	}
	var existing *api.Error
	if errors.As(err, &existing) {
		return err
	}
	kind := api.ReadFailed
	switch {
	case errors.Is(err, api.ErrRecordNotFound):
		kind = api.UnknownRecord
	case os.IsNotExist(err):
		kind = api.MissingDatabase
	case strings.Contains(err.Error(), "schema") || strings.Contains(err.Error(), "unsupported") || strings.Contains(err.Error(), "older configurations"):
		kind = api.IncompatibleFormat
	case strings.Contains(err.Error(), "identity") || strings.Contains(err.Error(), "does not match"):
		kind = api.IdentityConflict
	}
	return &classifiedError{public: api.Error{Kind: kind, Message: err.Error()}, cause: err}
}

func read[T any](ctx context.Context, c *Client, fn func(Reader) (T, error)) (result T, err error) {
	if err = c.begin(ctx); err != nil {
		return result, err
	}
	defer c.mu.RUnlock()
	if c.resources.OpenReader == nil {
		return result, unsupported("product reader")
	}
	r, err := c.resources.OpenReader(ctx)
	if err != nil {
		return result, classify(err)
	}
	defer func() { err = classify(errors.Join(err, r.Close())) }()
	return fn(r)
}

func command[T any](ctx context.Context, c *Client, mutation bool, fn func(TaskCommands) (T, error)) (result T, err error) {
	if err = c.begin(ctx); err != nil {
		return result, err
	}
	defer c.mu.RUnlock()
	if c.resources.OpenCommands == nil {
		return result, unsupported("product commands")
	}
	r, err := c.resources.OpenCommands(ctx, mutation)
	if err != nil {
		return result, classify(err)
	}
	defer func() { err = errors.Join(err, r.Close()) }()
	return fn(r)
}

func (c *Client) ShowTask(ctx context.Context, id string) (api.TaskPage, error) {
	return read(ctx, c, func(r Reader) (api.TaskPage, error) { return r.ShowTask(ctx, id) })
}

func (c *Client) ListTasks(ctx context.Context, cursor string, limit int) (api.TaskPage, error) {
	return read(ctx, c, func(r Reader) (api.TaskPage, error) { return r.ListTasks(ctx, cursor, limit) })
}

func (c *Client) TaskGraph(ctx context.Context, cursor string, limit int) (api.GraphPage, error) {
	return read(ctx, c, func(r Reader) (api.GraphPage, error) { return r.TaskGraph(ctx, cursor, limit) })
}

func (c *Client) ValidateTaskBatch(ctx context.Context, payload []byte) (api.TaskBatchValidation, error) {
	return command(ctx, c, false, func(r TaskCommands) (api.TaskBatchValidation, error) { return r.Validate(ctx, payload) })
}

func (c *Client) CreateTaskBatch(ctx context.Context, in api.CreateInput) (api.TaskBatchReceipt, error) {
	return command(ctx, c, true, func(r TaskCommands) (api.TaskBatchReceipt, error) { return r.Create(ctx, in) })
}

func (c *Client) DiscardTask(ctx context.Context, in api.DiscardInput) (api.TaskDiscardReceipt, error) {
	return command(ctx, c, true, func(r TaskCommands) (api.TaskDiscardReceipt, error) { return r.Discard(ctx, in) })
}

func (c *Client) AddPredecessors(ctx context.Context, in api.TaskPredecessorAddInput) (api.TaskPredecessorAddReceipt, error) {
	return command(ctx, c, true, func(r TaskCommands) (api.TaskPredecessorAddReceipt, error) {
		return r.AddPredecessors(ctx, in)
	})
}

func (c *Client) Conversation(ctx context.Context, filter api.ConversationFilter, cursor string) (api.ConversationPage, error) {
	return c.ConversationPage(ctx, filter, cursor, 1000)
}

func (c *Client) ConversationPage(ctx context.Context, filter api.ConversationFilter, cursor string, limit int) (api.ConversationPage, error) {
	return read(ctx, c, func(r Reader) (api.ConversationPage, error) {
		return r.Conversation(ctx, filter, cursor, limit)
	})
}

func (c *Client) Body(ctx context.Context, body api.ConversationBodyRef, offset int64, limit int) (api.ConversationBodyPage, error) {
	return read(ctx, c, func(r Reader) (api.ConversationBodyPage, error) {
		return r.Body(ctx, body, offset, limit)
	})
}

func (c *Client) Command(ctx context.Context, id string) (api.CommandReceipt, error) {
	return read(ctx, c, func(r Reader) (api.CommandReceipt, error) { return r.Command(ctx, id) })
}

func (c *Client) WaitCommand(ctx context.Context, id string, timeout time.Duration) (api.CommandReceipt, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	t := time.NewTicker(150 * time.Millisecond)
	defer t.Stop()
	for {
		if waitCtx.Err() != nil {
			return api.CommandReceipt{}, fmt.Errorf("command %s remains unconfirmed; query this same command ID later", id)
		}
		r, err := c.Command(waitCtx, id)
		if err == nil && (r.Status == "accepted" || r.Status == "rejected" || r.Status == "failed" || r.Status == "indeterminate") {
			return r, nil
		}
		if waitCtx.Err() != nil {
			continue
		}
		if err != nil && !errors.Is(err, api.ErrRecordNotFound) && !os.IsNotExist(err) {
			return api.CommandReceipt{}, err
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

func (c *Client) SubmitControl(ctx context.Context, in api.ControlInput) (api.ControlRequest, error) {
	if err := c.begin(ctx); err != nil {
		return api.ControlRequest{}, err
	}
	defer c.mu.RUnlock()
	if c.resources.Control == nil {
		return api.ControlRequest{}, unsupported("control")
	}
	if in.RequestID == "" {
		return api.ControlRequest{}, errors.New("control requires a stable request ID before submission")
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
		var payload api.ControlPayload
		if old == nil || old.RepositoryIdentity != c.Project().Identity || old.Action != string(in.Action) || decodeJSON(old.Payload, &payload) != nil && len(old.Payload) != 0 || !reflect.DeepEqual(payload, in.Payload) || old.RequestedBy != in.RequestedBy || in.InstanceID != "" && old.InstanceID != in.InstanceID || in.ExpectedRevision != nil && old.ExpectedControlRevision != *in.ExpectedRevision {
			return api.ControlRequest{}, errors.New("request ID conflict: immutable request has different content or identity")
		}
		return *old, nil
	}
	if !os.IsNotExist(err) {
		return api.ControlRequest{}, err
	}
	return c.resources.Control.Submit(ctx, in)
}

func (c *Client) SubmitControlInbox(ctx context.Context, in api.ControlInput) (api.ControlRequest, error) {
	if err := c.begin(ctx); err != nil {
		return api.ControlRequest{}, err
	}
	defer c.mu.RUnlock()
	if c.resources.Control == nil {
		return api.ControlRequest{}, unsupported("control")
	}
	port, ok := c.resources.Control.(ControlInboxPort)
	if !ok {
		return api.ControlRequest{}, unsupported("control request inbox")
	}
	if in.RequestID == "" {
		return api.ControlRequest{}, errors.New("control requires a stable request ID before submission")
	}
	if in.RequestedBy == "" {
		in.RequestedBy = c.resources.DefaultRequestedBy
	}
	if in.Payload.Environment != nil {
		target := *in.Payload.Environment
		in.Payload.Environment = &target
	}
	if in.InstanceID == "" {
		return api.ControlRequest{}, &api.Error{Kind: api.IdentityConflict, Message: "control request requires the discovered instance identity"}
	}
	if in.Action != api.StopService && in.ExpectedRevision == nil {
		return api.ControlRequest{}, &api.Error{Kind: api.IdentityConflict, Message: "control request requires its displayed revision"}
	}
	return port.SubmitInbox(ctx, in)
}

func (c *Client) Receipt(ctx context.Context, id string) (*api.ControlReceipt, error) {
	if err := c.begin(ctx); err != nil {
		return nil, err
	}
	defer c.mu.RUnlock()
	if c.resources.Control == nil {
		return nil, unsupported("control")
	}
	return c.resources.Control.Receipt(ctx, id)
}

func (c *Client) Request(ctx context.Context, id string) (*api.ControlRequest, error) {
	if err := c.begin(ctx); err != nil {
		return nil, err
	}
	defer c.mu.RUnlock()
	if c.resources.Control == nil {
		return nil, unsupported("control")
	}
	return c.resources.Control.Request(ctx, id)
}

func (c *Client) WaitControl(ctx context.Context, id string, poll time.Duration) (*api.ControlReceipt, error) {
	if err := c.begin(ctx); err != nil {
		return nil, err
	}
	defer c.mu.RUnlock()
	if c.resources.Control == nil {
		return nil, unsupported("control")
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

func (c *Client) StopService(ctx context.Context, by string, timeout time.Duration) (api.ServiceStopResult, error) {
	if err := c.begin(ctx); err != nil {
		return api.ServiceStopResult{}, err
	}
	defer c.mu.RUnlock()
	if c.resources.Control == nil {
		return api.ServiceStopResult{}, unsupported("control")
	}
	return c.resources.Control.Stop(ctx, by, timeout)
}

func (c *Client) DecisionEligibility(ctx context.Context, run *api.CandidateSummary) error {
	if err := c.begin(ctx); err != nil {
		return err
	}
	defer c.mu.RUnlock()
	if c.resources.Decision == nil {
		return unsupported("decision commands")
	}
	return c.resources.Decision.Eligibility(ctx, run)
}

// NormalizeDecision maps the original answer through the existing domain adapter
// before HTTP records its immutable submission. It performs no mutation.
func (c *Client) NormalizeDecision(ctx context.Context, in api.DecisionInput) (api.DecisionInput, error) {
	if err := c.begin(ctx); err != nil {
		return in, err
	}
	defer c.mu.RUnlock()
	if c.resources.Decision == nil {
		return in, unsupported("decision commands")
	}
	return c.resources.Decision.Normalize(ctx, in)
}

func (c *Client) SubmitDecision(ctx context.Context, in api.DecisionInput) (api.CommandReceipt, bool, error) {
	if err := c.begin(ctx); err != nil {
		return api.CommandReceipt{}, false, err
	}
	defer c.mu.RUnlock()
	if c.resources.Decision == nil {
		return api.CommandReceipt{}, false, unsupported("decision commands")
	}
	if in.CommandID == "" {
		return api.CommandReceipt{}, false, errors.New("decision requires a stable command ID before submission")
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

func (c *Client) Answer(ctx context.Context, in api.AnswerInput) (api.CommandReceipt, bool, error) {
	if err := c.begin(ctx); err != nil {
		return api.CommandReceipt{}, false, err
	}
	defer c.mu.RUnlock()
	if c.resources.Decision == nil {
		return api.CommandReceipt{}, false, unsupported("decision commands")
	}
	if in.CommandID == "" {
		return api.CommandReceipt{}, false, errors.New("answer requires a stable command ID before submission")
	}
	if in.RequestedBy == "" {
		in.RequestedBy = c.resources.DefaultRequestedBy
	}
	return c.resources.Decision.Answer(ctx, in)
}

func (c *Client) TaskSnapshot(ctx context.Context) (*api.TaskCandidates, error) {
	if err := c.begin(ctx); err != nil {
		return nil, err
	}
	defer c.mu.RUnlock()
	if c.resources.TaskSnapshot == nil {
		return nil, unsupported("task snapshot")
	}
	return c.resources.TaskSnapshot(ctx)
}

func (c *Client) StartTaskPayload(ctx context.Context, taskID, profileID string) (api.ControlPayload, error) {
	if err := c.begin(ctx); err != nil {
		return api.ControlPayload{}, err
	}
	defer c.mu.RUnlock()
	if c.resources.ResolveStartTask == nil {
		return api.ControlPayload{}, unsupported("execution profile resolver")
	}
	return c.resources.ResolveStartTask(ctx, taskID, profileID)
}

func (c *Client) ReadProject(ctx context.Context, selected string, previous api.ProjectSnapshot) api.ProjectSnapshot {
	result, err := read(ctx, c, func(r Reader) (api.ProjectSnapshot, error) { return r.ReadProject(ctx, selected, previous), nil })
	if err == nil {
		return result
	}
	if c.resources.ReadFailure != nil {
		return c.resources.ReadFailure(ctx, selected, err)
	}
	result.SelectedTaskID = selected
	result.TaskError, result.HistoryError, result.NativeTaskError = err.Error(), err.Error(), err.Error()
	return result
}

func (c *Client) RecentActivity(ctx context.Context, run, execution, attempt string) (api.LogBatch, error) {
	return read(ctx, c, func(r Reader) (api.LogBatch, error) { return r.RecentActivity(ctx, run, execution, attempt) })
}

func (c *Client) Activity(ctx context.Context, run, execution, attempt string, cursor api.ActivityCursor) (api.LogBatch, error) {
	return read(ctx, c, func(r Reader) (api.LogBatch, error) { return r.Activity(ctx, run, execution, attempt, cursor) })
}

func (c *Client) InspectRecovery(ctx context.Context, id string) (api.RecoveryInspection, error) {
	if err := c.begin(ctx); err != nil {
		return api.RecoveryInspection{}, err
	}
	defer c.mu.RUnlock()
	if c.resources.InspectRecovery == nil {
		return api.RecoveryInspection{}, unsupported("recovery inspector")
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

func unsupported(capability string) error {
	return &api.Error{Kind: api.Unsupported, Message: capability + " is unsupported"}
}
func decodeJSON(data []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("JSON must contain one value")
	}
	return nil
}
