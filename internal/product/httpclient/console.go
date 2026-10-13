package httpclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
	fs "github.com/erranliu/SteerLine/internal/product/httpfs"
)

// Console is a reconnectable HTTP consumer. Its only persistent files are
// connection discovery and its own immutable outgoing envelopes/observations.
type Console struct {
	mu       sync.Mutex
	ref      api.ProjectRef
	client   *Client
	snapshot api.ProjectSnapshot
	closed   bool
	identity wire.Identity
}

func NewConsole(ctx context.Context, ref api.ProjectRef) *Console {
	c := &Console{ref: ref}
	c.mu.Lock()
	_, _ = c.connect(ctx)
	c.mu.Unlock()
	return c
}
func (c *Console) Project() api.ProjectRef {
	c.mu.Lock()
	defer c.mu.Unlock()
	return api.CloneProjectRef(c.ref)
}
func (c *Console) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}
func (c *Console) connect(ctx context.Context) (*Client, error) {
	if c.closed {
		return nil, errors.New("HTTP console client closed")
	}
	if c.client != nil {
		// The per-instance credential authenticates every request. Handshake
		// once on connection; avoid a second full product query for each call.
		return c.client, nil
	}
	client, err := Open(ctx, c.ref.RunDir, c.ref.Identity)
	if err != nil {
		return nil, err
	}
	presentation, err := client.ProjectPresentation(ctx)
	if err != nil {
		client.Close()
		return nil, err
	}
	c.client = client
	c.identity = client.Identity()
	instruction := c.ref.Presentation.ServiceInstruction
	c.ref.Presentation = presentation.Presentation
	c.ref.Presentation.ServiceInstruction = instruction
	return client, nil
}
func (c *Console) ReadProject(ctx context.Context, selected string, previous api.ProjectSnapshot) api.ProjectSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	client, err := c.connect(ctx)
	var snapshot api.ProjectSnapshot
	if err == nil {
		snapshot, err = client.Snapshot(ctx, selected)
		if err == nil && snapshot.Connection.Snapshot.Page != nil && snapshot.Connection.Snapshot.Page.Service.InstanceID != client.Identity().InstanceID {
			err = &api.Error{Kind: api.IdentityConflict, Message: "HTTP snapshot belongs to a replacement service instance"}
		}
	}
	if err != nil {
		if c.client != nil {
			_ = c.client.Close()
			c.client = nil
		}
		snapshot = previous
		snapshot.SelectedTaskID = selected
		snapshot.Connection.State = api.ConnectionOffline
		snapshot.Connection.CanControl = false
		snapshot.Connection.CanStop = false
		snapshot.Connection.CanStart = false
		snapshot.Connection.Diagnostic = "HTTP service unavailable; last observation only: " + err.Error()
		snapshot.TaskError = err.Error()
		snapshot.NativeTaskError = err.Error()
		snapshot.HistoryError = err.Error()
	} else {
		c.snapshot = snapshot
	}
	return snapshot
}
func (c *Console) RecentActivity(ctx context.Context, run, execution, attempt string) (api.LogBatch, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	client, err := c.connect(ctx)
	if err != nil {
		return api.LogBatch{}, err
	}
	page, err := client.Activity(ctx, run, execution, attempt, api.ActivityCursor{})
	return page.Page, err
}
func (c *Console) Conversation(ctx context.Context, filter api.ConversationFilter, cursor string) (api.ConversationPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	client, err := c.connect(ctx)
	if err != nil {
		return api.ConversationPage{}, err
	}
	page, err := client.Conversation(ctx, filter.WorkflowID, filter, cursor, 200)
	return page.Page, err
}
func (c *Console) Body(ctx context.Context, ref api.ConversationBodyRef, offset int64, limit int) (api.ConversationBodyPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	client, err := c.connect(ctx)
	if err != nil {
		return api.ConversationBodyPage{}, err
	}
	// A body reference binds Execution/Attempt, while the public route also needs
	// its workflow. Resolve that association from the authenticated snapshot.
	snapshot, err := client.Snapshot(ctx, "")
	if err != nil {
		return api.ConversationBodyPage{}, err
	}
	if snapshot.Connection.Snapshot.Page != nil {
		for _, run := range snapshot.Connection.Snapshot.Page.Runs {
			if run != nil && run.ExecutionID == ref.ExecutionID {
				return client.Body(ctx, run.RunID, ref, offset, limit)
			}
		}
	}
	return api.ConversationBodyPage{}, errors.New("body workflow binding unavailable")
}
func (c *Console) InspectRecovery(ctx context.Context, run string) (api.RecoveryInspection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	client, err := c.connect(ctx)
	if err != nil {
		return api.RecoveryInspection{}, err
	}
	candidate, err := client.Run(ctx, run)
	if err != nil {
		return api.RecoveryInspection{}, err
	}
	var result api.RecoveryInspection
	err = client.Get(ctx, "/runs/"+run+"/recovery?execution_id="+candidate.ExecutionID+"&attempt_id="+candidate.AttemptID, &result)
	return result, err
}
func (c *Console) CandidateAvailability(s api.ProjectSnapshot, now time.Time) (bool, string) {
	return api.CandidateFreshness(c.Project().Identity, s, now)
}
func (c *Console) StartTaskAvailability(s api.ProjectSnapshot, now time.Time) string {
	return api.StartTaskReason(c.Project(), s, now)
}
func (c *Console) AutoDispatchAvailability(s api.ProjectSnapshot) string {
	return api.AutoDispatchReason(c.Project(), s)
}
func (c *Console) DecisionEligibility(ctx context.Context, run *api.CandidateSummary) error {
	if run == nil {
		return errors.New("no selected candidate")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	client, err := c.connect(ctx)
	if err != nil {
		return err
	}
	return client.Get(ctx, "/runs/"+run.RunID+"/decision-eligibility?execution_id="+run.ExecutionID+"&attempt_id="+run.AttemptID, nil)
}
func (c *Console) StartTaskPayload(ctx context.Context, task, profile string) (api.ControlPayload, error) {
	ref := c.Project()
	for _, p := range ref.Presentation.Profiles {
		if p.ID == profile {
			return api.ControlPayload{TaskID: task, ExecutionProfileID: profile, ExecutionProfileDigest: p.Digest}, nil
		}
	}
	return api.ControlPayload{}, errors.New("profile unavailable")
}

type outgoing struct {
	Version  int                   `json:"version"`
	Envelope wire.CommandEnvelope  `json:"envelope"`
	Response *wire.CommandResponse `json:"response,omitempty"`
}

func validateResponse(e wire.CommandEnvelope, r wire.CommandResponse) error {
	if r.RequestID != e.RequestID || r.ProjectID != e.ProjectID || r.InstanceID != e.InstanceID || r.Action != e.Action || r.TargetID != e.TargetID || r.ExpectedVersion != e.ExpectedVersion {
		return errors.New("HTTP response does not match original request target/version")
	}
	if r.Original != nil && !reflect.DeepEqual(*r.Original, e) {
		return errors.New("HTTP response changed original envelope")
	}
	if r.ControlRequest != nil && (r.ControlRequest.RequestID != e.RequestID || r.ControlRequest.InstanceID != e.InstanceID || r.ControlRequest.Action != e.Action) {
		return errors.New("HTTP control request binding mismatch")
	}
	if r.ControlReceipt != nil && (r.ControlReceipt.RequestID != e.RequestID || r.ControlReceipt.InstanceID != e.InstanceID || r.ControlReceipt.Action != e.Action) {
		return errors.New("HTTP control receipt binding mismatch")
	}
	if r.Command != nil && (r.Command.CommandID != e.RequestID || r.Command.TargetID != e.TargetID || r.Command.ExpectedVersion != e.ExpectedVersion) {
		return errors.New("HTTP command receipt binding mismatch")
	}
	return nil
}

func (c *Console) outgoingPath(id string) (string, error) {
	b, err := hex.DecodeString(id)
	if err != nil || len(b) != 16 || hex.EncodeToString(b) != id {
		return "", errors.New("invalid request ID")
	}
	return filepath.Join(c.ref.ApplicationHome, "ui", "http-requests", wire.ProjectID(c.ref.Identity), id+".json"), nil
}
func (c *Console) readOutgoing(id string) (outgoing, error) {
	var value outgoing
	path, err := c.outgoingPath(id)
	if err != nil {
		return value, err
	}
	if err = fs.VerifyPrivateFile(path); err != nil {
		return value, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if len(data) > 4<<20 {
		return value, errors.New("outgoing envelope too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&value)
	if err == nil && decoder.Decode(new(any)) != io.EOF {
		err = errors.New("outgoing record has trailing JSON")
	}
	if err == nil && (value.Version != 1 || value.Envelope.RequestID != id || value.Envelope.ProjectID != wire.ProjectID(c.ref.Identity)) {
		err = errors.New("outgoing envelope identity/format invalid")
	}
	if err == nil && value.Response != nil {
		err = validateResponse(value.Envelope, *value.Response)
	}
	return value, err
}
func (c *Console) saveOutgoing(value outgoing) error {
	path, err := c.outgoingPath(value.Envelope.RequestID)
	if err != nil {
		return err
	}
	previous, previousErr := c.readOutgoing(value.Envelope.RequestID)
	if previousErr != nil && !os.IsNotExist(previousErr) {
		return previousErr
	}
	if previousErr == nil {
		if !reflect.DeepEqual(previous.Envelope, value.Envelope) {
			return errors.New("request ID conflict: durable outgoing intent differs")
		}
		if value.Response == nil {
			return nil
		}
		if previous.Response != nil && (previous.Response.Status == "applied" || previous.Response.Status == "rejected") && value.Response.Status != previous.Response.Status {
			return nil
		}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > 4<<20 {
		return errors.New("outgoing envelope too large")
	}
	if err = fs.EnsurePrivateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	file, err := fs.CreatePrivateTemp(filepath.Dir(path), ".outgoing-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if os.IsNotExist(previousErr) {
		// Publish the first immutable intent without replacing another window's
		// record. The temporary link is removed before this method returns.
		if err = os.Link(name, path); err != nil {
			if os.IsExist(err) {
				return c.saveOutgoing(value)
			}
			return err
		}
		return nil
	}
	return os.Rename(name, path)
}
func notFound(err error) bool {
	var failure *ClientError
	return errors.As(err, &failure) && failure.Code == "command_not_found"
}
func (c *Console) submit(ctx context.Context, envelope wire.CommandEnvelope) (wire.CommandResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	client, err := c.connect(ctx)
	if err != nil {
		return wire.CommandResponse{}, err
	}
	if envelope.ProjectID == "" {
		envelope.ProjectID = wire.ProjectID(c.ref.Identity)
	}
	if envelope.InstanceID == "" {
		return wire.CommandResponse{}, errors.New("original service instance required")
	}
	stored, err := c.readOutgoing(envelope.RequestID)
	if err == nil {
		if !reflect.DeepEqual(stored.Envelope, envelope) {
			return wire.CommandResponse{}, errors.New("request ID conflict: immutable envelope changed")
		}
	} else if os.IsNotExist(err) {
		stored = outgoing{Version: 1, Envelope: envelope}
		if err = c.saveOutgoing(stored); err != nil {
			return wire.CommandResponse{}, err
		}
	} else {
		return wire.CommandResponse{}, err
	}
	response, err := client.Command(ctx, envelope.RequestID)
	if notFound(err) {
		if client.Identity().InstanceID != envelope.InstanceID {
			return response, errors.New("stale_instance: original request belongs to another service")
		}
		response, err = client.SubmitCommand(ctx, envelope)
	}
	if err == nil {
		if err = validateResponse(envelope, response); err != nil {
			return response, err
		}
		stored.Response = &response
		if saveErr := c.saveOutgoing(stored); saveErr != nil {
			return response, saveErr
		}
	}
	return response, err
}
func (c *Console) lookup(ctx context.Context, id string) (wire.CommandResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	stored, storedErr := c.readOutgoing(id)
	if storedErr != nil && !os.IsNotExist(storedErr) {
		return wire.CommandResponse{}, storedErr
	}
	client, err := c.connect(ctx)
	if err == nil {
		var result wire.CommandResponse
		result, err = client.Command(ctx, id)
		if err == nil {
			if storedErr == nil {
				if err = validateResponse(stored.Envelope, result); err != nil {
					return result, err
				}
				stored.Response = &result
				if err = c.saveOutgoing(stored); err != nil {
					return result, err
				}
			}
			return result, nil
		}
	}
	if storedErr == nil && stored.Response != nil {
		switch stored.Response.Status {
		case "applied", "rejected":
			return *stored.Response, nil
		}
	}
	if notFound(err) && storedErr == nil {
		if client.Identity().InstanceID != stored.Envelope.InstanceID {
			return wire.CommandResponse{}, errors.New("stale_instance: original request not replayed")
		}
		response, sendErr := client.SubmitCommand(ctx, stored.Envelope)
		if sendErr == nil {
			if sendErr = validateResponse(stored.Envelope, response); sendErr != nil {
				return response, sendErr
			}
			stored.Response = &response
			sendErr = c.saveOutgoing(stored)
		}
		return response, sendErr
	}
	if notFound(err) {
		return wire.CommandResponse{}, os.ErrNotExist
	}
	return wire.CommandResponse{}, err
}
func (c *Console) SubmitControl(ctx context.Context, in api.ControlInput) (api.ControlRequest, error) {
	p := in.Payload
	envelope := wire.CommandEnvelope{RequestID: in.RequestID, InstanceID: in.InstanceID, Action: string(in.Action)}
	if in.ExpectedRevision != nil {
		envelope.ExpectedControlRevision = *in.ExpectedRevision
	}
	var payload any = map[string]any{}
	switch in.Action {
	case api.StartTask, api.StartCandidate:
		envelope.TargetID = p.TaskID
		envelope.ExpectedVersion = p.ExpectedTaskVersion
		envelope.ExpectedGraphRevision = p.ExpectedGraphRevision
		payload = map[string]any{"execution_profile_id": p.ExecutionProfileID}
	case api.AbandonCandidate:
		envelope.TargetID = p.WorkflowRunID
		envelope.ExpectedVersion = p.ExpectedCandidateVersion
		payload = map[string]any{"task_id": p.TaskID, "expected_task_version": p.ExpectedTaskVersion, "reason": p.Reason}
	case api.SetDispatchMode:
		envelope.TargetID = "dispatch"
		envelope.ExpectedVersion = envelope.ExpectedControlRevision
		payload = map[string]any{"mode": p.Mode}
	case api.PauseDispatch, api.ResumeDispatch:
		envelope.TargetID = "dispatch"
		envelope.ExpectedVersion = envelope.ExpectedControlRevision
	case api.OpenTerminal, api.RecheckEnvironment:
		if p.Environment == nil {
			return api.ControlRequest{}, errors.New("environment target required")
		}
		envelope.TargetID = p.Environment.RunID
		envelope.ExpectedVersion = p.Environment.Version
		payload = map[string]any{"target": p.Environment}
	case api.StopService:
		envelope.TargetID = in.InstanceID
	default:
		return api.ControlRequest{}, errors.New("unsupported control")
	}
	envelope.Payload, _ = json.Marshal(payload)
	response, err := c.submit(ctx, envelope)
	if err != nil {
		return api.ControlRequest{}, err
	}
	if response.ControlRequest != nil {
		return *response.ControlRequest, nil
	}
	return api.ControlRequest{}, fmt.Errorf("%s: %s", response.Code, response.Diagnostic)
}
func (c *Console) Request(ctx context.Context, id string) (*api.ControlRequest, error) {
	response, err := c.lookup(ctx, id)
	if err != nil {
		return nil, err
	}
	if response.ControlRequest == nil {
		return nil, os.ErrNotExist
	}
	return response.ControlRequest, nil
}
func (c *Console) Receipt(ctx context.Context, id string) (*api.ControlReceipt, error) {
	response, err := c.lookup(ctx, id)
	if err != nil {
		return nil, err
	}
	if response.ControlReceipt != nil {
		if response.Action == "stop_service" && response.ControlReceipt.Status == "applied" {
			copy := *response.ControlReceipt
			c.mu.Lock()
			identity := c.identity
			c.mu.Unlock()
			exited, locks, _ := observeExit(identity)
			if identity.InstanceID == response.InstanceID && exited && locks {
				copy.Message = "原停服请求已应用，原进程退出与锁释放已核实"
			} else {
				copy.Code = "stop_exit_unverified"
				copy.Message = "原停服请求已应用；进程退出与锁释放结果未核实"
			}
			return &copy, nil
		}
		return response.ControlReceipt, nil
	}
	if response.Status == "rejected" || response.Status == "outcome_unknown" {
		return &api.ControlReceipt{RequestID: id, InstanceID: response.InstanceID, Action: response.Action, Status: response.Status, Code: response.Code, Message: response.Diagnostic}, nil
	}
	return nil, os.ErrNotExist
}
func (c *Console) Command(ctx context.Context, id string) (api.CommandReceipt, error) {
	response, err := c.lookup(ctx, id)
	if err != nil {
		return api.CommandReceipt{}, err
	}
	if response.Command != nil {
		return *response.Command, nil
	}
	return api.CommandReceipt{CommandID: id, ProjectIdentity: c.Project().Identity, Kind: response.Action, TargetID: response.TargetID, ExpectedVersion: response.ExpectedVersion, Status: response.Status, ResultCode: response.Code, ResultError: response.Diagnostic}, nil
}
func (c *Console) SubmitDecision(ctx context.Context, in api.DecisionInput) (api.CommandReceipt, bool, error) {
	a := in.Answer
	payload, _ := json.Marshal(map[string]any{"run_id": a.RunID, "decision_id": a.DecisionID, "option_id": a.OptionID, "note": a.Note, "answers": a.Answers, "expected_run_revision": a.ExpectedRunRevision, "source_attempt_id": a.SourceAttemptID, "expected_recovery_version": a.ExpectedRecoveryVersion})
	response, err := c.submit(ctx, wire.CommandEnvelope{RequestID: in.CommandID, InstanceID: a.ServiceInstanceID, Action: "answer_decision", TargetID: a.DecisionID, ExpectedVersion: a.ExpectedDecisionVersion, Payload: payload})
	if err != nil {
		return api.CommandReceipt{}, false, err
	}
	if response.Command != nil {
		return *response.Command, false, nil
	}
	return api.CommandReceipt{CommandID: in.CommandID, ProjectIdentity: c.Project().Identity, Kind: "answer_decision", TargetID: a.DecisionID, ExpectedVersion: a.ExpectedDecisionVersion, Status: response.Status, ResultCode: response.Code, ResultError: response.Diagnostic}, false, nil
}
func (c *Console) Answer(ctx context.Context, in api.AnswerInput) (api.CommandReceipt, bool, error) {
	c.mu.Lock()
	client, err := c.connect(ctx)
	var run api.CandidateSummary
	if err == nil {
		run, err = client.Run(ctx, in.RunID)
	}
	c.mu.Unlock()
	if err != nil {
		return api.CommandReceipt{}, false, err
	}
	return c.SubmitDecision(ctx, api.DecisionInput{CommandID: in.CommandID, Answer: api.DecisionAnswer{RunID: in.RunID, DecisionID: in.DecisionID, OptionID: in.Option, Note: in.Note, ExpectedDecisionVersion: in.ExpectedVersion, ExpectedRunRevision: run.ControlRevision, SourceAttemptID: run.AttemptID, ExpectedRecoveryVersion: run.RecoveryVersion, ServiceInstanceID: client.Identity().InstanceID}})
}
func (c *Console) WaitControl(ctx context.Context, id string, poll time.Duration) (*api.ControlReceipt, error) {
	if poll <= 0 {
		poll = 100 * time.Millisecond
	}
	for {
		receipt, err := c.Receipt(ctx, id)
		if err == nil && receipt.Status != "applying" {
			return receipt, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(poll):
		}
	}
}
func (c *Console) WaitCommand(ctx context.Context, id string, timeout time.Duration) (api.CommandReceipt, error) {
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		receipt, err := c.Command(wait, id)
		if err == nil && (receipt.Status == "accepted" || receipt.Status == "rejected" || receipt.Status == "failed" || receipt.Status == "indeterminate" || receipt.Status == "applied" || receipt.Status == "outcome_unknown") {
			return receipt, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return api.CommandReceipt{}, err
		}
		select {
		case <-wait.Done():
			return api.CommandReceipt{}, fmt.Errorf("command %s remains unconfirmed; query this same command ID later: %w", id, wait.Err())
		case <-time.After(150 * time.Millisecond):
		}
	}
}
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (c *Console) OpenNativeSession(ctx context.Context, target api.NativeSessionTarget) error {
	c.mu.Lock()
	client, err := c.connect(ctx)
	instance := ""
	if err == nil {
		instance = client.Identity().InstanceID
	}
	c.mu.Unlock()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(target)
	response, err := c.submit(ctx, wire.CommandEnvelope{RequestID: newID(), InstanceID: instance, Action: "open_native_session", TargetID: target.RunID, ExpectedVersion: 0, Payload: payload})
	if err != nil {
		return err
	}
	if response.Status != "applied" {
		return fmt.Errorf("%s: %s", response.Status, response.Diagnostic)
	}
	return nil
}
func (c *Console) StopService(ctx context.Context, by string, timeout time.Duration) (api.ServiceStopResult, error) {
	c.mu.Lock()
	client, err := c.connect(ctx)
	var identity wire.Identity
	if err == nil {
		identity = client.Identity()
	}
	c.mu.Unlock()
	if err != nil {
		return api.ServiceStopResult{}, err
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	id := newID()
	_, err = c.SubmitControl(wait, api.ControlInput{Action: api.StopService, RequestID: id, InstanceID: identity.InstanceID})
	result := api.ServiceStopResult{Status: "pending", RequestID: id, InstanceID: identity.InstanceID, Diagnostic: "结果未核实：原停服回执/进程退出/锁释放尚未齐备"}
	// A lost POST result is not proof that the stop was not applied.
	if err != nil && wait.Err() != nil {
		return result, nil
	}
	for wait.Err() == nil {
		receipt, readErr := c.Receipt(wait, id)
		if readErr == nil {
			result.Receipt = receipt
			if receipt.Status == "rejected" {
				result.Status = "rejected"
				result.Diagnostic = receipt.Message
				return result, nil
			}
		}
		exited, locks, probeErr := observeExit(identity)
		result.ProcessExited, result.LocksReleased = exited, locks
		if probeErr != nil {
			result.Diagnostic = probeErr.Error()
		}
		if exited && locks && result.Receipt != nil && result.Receipt.Status == "applied" {
			result.Status = "stopped"
			result.Diagnostic = "original applied receipt, process exit and lock release verified"
			return result, nil
		}
		select {
		case <-wait.Done():
		case <-time.After(50 * time.Millisecond):
		}
	}
	return result, nil
}
