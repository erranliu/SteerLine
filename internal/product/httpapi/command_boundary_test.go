package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

type registryTestRecords struct{ values map[string]requestRecord }

func newRegistryTestRecords() *registryTestRecords {
	return &registryTestRecords{values: make(map[string]requestRecord)}
}

func (r *registryTestRecords) Read(id string) (requestRecord, error) {
	value, ok := r.values[id]
	if !ok {
		return requestRecord{}, os.ErrNotExist
	}
	return cloneRegistryRecord(value), nil
}

func (r *registryTestRecords) Register(value requestRecord) error {
	if _, exists := r.values[value.Envelope.RequestID]; exists {
		return os.ErrExist
	}
	r.values[value.Envelope.RequestID] = cloneRegistryRecord(value)
	return nil
}

func (r *registryTestRecords) Write(value requestRecord) error {
	r.values[value.Envelope.RequestID] = cloneRegistryRecord(value)
	return nil
}

func cloneRegistryRecord(value requestRecord) requestRecord {
	data, _ := json.Marshal(value)
	var copy requestRecord
	_ = json.Unmarshal(data, &copy)
	return copy
}

type registryTestAuthority struct {
	commands   map[string]api.CommandReceipt
	requests   map[string]api.ControlRequest
	receipts   map[string]*api.ControlReceipt
	commandErr error
	requestErr error
}

func newRegistryTestAuthority() *registryTestAuthority {
	return &registryTestAuthority{commands: map[string]api.CommandReceipt{}, requests: map[string]api.ControlRequest{}, receipts: map[string]*api.ControlReceipt{}}
}

func (a *registryTestAuthority) Command(_ context.Context, id string) (api.CommandReceipt, error) {
	if a.commandErr != nil {
		return api.CommandReceipt{}, a.commandErr
	}
	if value, ok := a.commands[id]; ok {
		return value, nil
	}
	return api.CommandReceipt{}, &api.Error{Kind: api.UnknownRecord, Message: "command not found"}
}

func (a *registryTestAuthority) Request(_ context.Context, id string) (*api.ControlRequest, error) {
	if a.requestErr != nil {
		return nil, a.requestErr
	}
	value, ok := a.requests[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &value, nil
}

func (a *registryTestAuthority) Receipt(_ context.Context, id string) (*api.ControlReceipt, error) {
	value, ok := a.receipts[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return value, nil
}

func (a *registryTestAuthority) WaitControl(context.Context, string, time.Duration) (*api.ControlReceipt, error) {
	return nil, os.ErrNotExist
}

type registryTestCommands struct {
	authority *registryTestAuthority
	controlIn []api.ControlInput
	onSubmit  func(api.ControlInput) (api.ControlRequest, error)
}

func (c *registryTestCommands) DiscardTask(context.Context, api.DiscardInput) (api.TaskDiscardReceipt, error) {
	return api.TaskDiscardReceipt{}, errors.New("unexpected discard")
}
func (c *registryTestCommands) NormalizeDecision(_ context.Context, value api.DecisionInput) (api.DecisionInput, error) {
	return value, nil
}
func (c *registryTestCommands) SubmitDecision(context.Context, api.DecisionInput) (api.CommandReceipt, bool, error) {
	return api.CommandReceipt{}, false, errors.New("unexpected decision")
}
func (c *registryTestCommands) StartTaskPayload(_ context.Context, taskID, profileID string) (api.ControlPayload, error) {
	return api.ControlPayload{TaskID: taskID, ExecutionProfileID: profileID, ExecutionProfileDigest: "digest"}, nil
}
func (c *registryTestCommands) SubmitControlInbox(_ context.Context, input api.ControlInput) (api.ControlRequest, error) {
	c.controlIn = append(c.controlIn, input)
	if c.onSubmit != nil {
		return c.onSubmit(input)
	}
	request := registryTestControlRequest(input)
	c.authority.requests[input.RequestID] = request
	return request, nil
}

type registryTestNativeViewer struct{ calls int }

func (n *registryTestNativeViewer) OpenNativeSession(context.Context, api.NativeSessionTarget) error {
	n.calls++
	return nil
}

func registryTestControlRequest(input api.ControlInput) api.ControlRequest {
	payload, _ := json.Marshal(input.Payload)
	revision := uint64(0)
	if input.ExpectedRevision != nil {
		revision = *input.ExpectedRevision
	}
	return api.ControlRequest{ProtocolVersion: 1, RequestID: input.RequestID, RepositoryIdentity: "C:/repo", InstanceID: input.InstanceID,
		Action: string(input.Action), ExpectedControlRevision: revision, Payload: payload, RequestedBy: input.RequestedBy,
		CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute)}
}

func registryTestEnvelope(id string) CommandEnvelope {
	return CommandEnvelope{RequestID: id, ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: "pause_dispatch",
		TargetID: "dispatch", ExpectedVersion: 7, ExpectedControlRevision: 7, Payload: json.RawMessage(`{}`)}
}

func newRegistryTestOwner(authority *registryTestAuthority, commands *registryTestCommands, records requestRecordAccess) *commandRegistry {
	readiness := &readinessState{status: "ready"}
	readiness.Set(true, "", "ready")
	return newCommandRegistry(Options{ProjectIdentity: "C:/repo", InstanceID: "instance-a", Capabilities: []string{"pause_dispatch"}},
		Ports{Commands: commands, Authority: authority}, readiness, records)
}

func TestHTTPCommandBoundary(t *testing.T) {
	authority := newRegistryTestAuthority()
	commands := &registryTestCommands{authority: authority}
	owner := newRegistryTestOwner(authority, commands, newRegistryTestRecords())
	server := &Server{options: Options{ProjectIdentity: "C:/repo", InstanceID: "instance-a"}, commands: owner}
	envelope := registryTestEnvelope(strings.Repeat("a", 32))
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/projects/commands", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.submitCommand(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("handler returned status %d: %s", recorder.Code, recorder.Body.String())
	}
	var response CommandResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "submitted" || response.Original == nil || !sameEnvelope(*response.Original, envelope) {
		t.Fatalf("handler did not preserve the original envelope: %+v", response)
	}
	if len(commands.controlIn) != 1 || commands.controlIn[0].Action != api.PauseDispatch || commands.controlIn[0].RequestedBy == "" {
		t.Fatalf("handler did not call the command port with the OS caller identity: %+v", commands.controlIn)
	}
}

func TestHTTPCommandRegistry(t *testing.T) {
	t.Run("concurrent same ID shares one registered intent and dispatch", func(t *testing.T) {
		authority := newRegistryTestAuthority()
		commands := &registryTestCommands{authority: authority}
		owner := newRegistryTestOwner(authority, commands, newRegistryTestRecords())
		envelope := registryTestEnvelope(strings.Repeat("e", 32))
		const callers = 8
		start := make(chan struct{})
		responses := make(chan CommandResponse, callers)
		errorsFound := make(chan error, callers)
		var wait sync.WaitGroup
		for i := 0; i < callers; i++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				response, err := owner.Submit(context.Background(), envelope, "tester")
				if err != nil {
					errorsFound <- err
					return
				}
				responses <- response
			}()
		}
		close(start)
		wait.Wait()
		close(responses)
		close(errorsFound)
		for err := range errorsFound {
			t.Errorf("concurrent replay failed: %v", err)
		}
		count := 0
		for response := range responses {
			count++
			if response.Status != "submitted" || response.Original == nil || !sameEnvelope(*response.Original, envelope) {
				t.Errorf("concurrent replay lost the original response: %+v", response)
			}
		}
		if count != callers || len(commands.controlIn) != 1 {
			t.Fatalf("same request ID was not coalesced: responses=%d dispatches=%d", count, len(commands.controlIn))
		}
	})

	t.Run("same ID replays the original authority result and conflicts across actions", func(t *testing.T) {
		authority := newRegistryTestAuthority()
		commands := &registryTestCommands{authority: authority}
		owner := newRegistryTestOwner(authority, commands, newRegistryTestRecords())
		envelope := registryTestEnvelope(strings.Repeat("b", 32))
		first, err := owner.Submit(context.Background(), envelope, "tester")
		if err != nil || first.Status != "submitted" {
			t.Fatalf("first request failed: %+v %v", first, err)
		}
		second, err := owner.Submit(context.Background(), envelope, "tester")
		if err != nil || second.Status != first.Status || len(commands.controlIn) != 1 {
			t.Fatalf("same request was dispatched more than once: %+v %v calls=%d", second, err, len(commands.controlIn))
		}
		conflict := envelope
		conflict.Action = "set_dispatch_mode"
		conflict.Payload = json.RawMessage(`{"mode":"manual"}`)
		_, err = owner.Submit(context.Background(), conflict, "tester")
		var protocolErr *protocolError
		if !errors.As(err, &protocolErr) || protocolErr.code != "request_id_conflict" || len(commands.controlIn) != 1 {
			t.Fatalf("cross-action request ID was not rejected before dispatch: %v calls=%d", err, len(commands.controlIn))
		}
	})

	t.Run("authority read failures remain errors before registration", func(t *testing.T) {
		authority := newRegistryTestAuthority()
		authority.requestErr = errors.New("control authority read failed")
		commands := &registryTestCommands{authority: authority}
		records := newRegistryTestRecords()
		owner := newRegistryTestOwner(authority, commands, records)
		_, err := owner.Submit(context.Background(), registryTestEnvelope(strings.Repeat("c", 32)), "tester")
		if err == nil || len(records.values) != 0 || len(commands.controlIn) != 0 {
			t.Fatalf("authority read failure was treated as absence: %v records=%d dispatches=%d", err, len(records.values), len(commands.controlIn))
		}
	})

	t.Run("formal consumer winning submit is reconciled by the exact mapped intent", func(t *testing.T) {
		authority := newRegistryTestAuthority()
		commands := &registryTestCommands{authority: authority}
		commands.onSubmit = func(input api.ControlInput) (api.ControlRequest, error) {
			request := registryTestControlRequest(input)
			authority.requests[input.RequestID] = request
			return api.ControlRequest{}, errors.New("formal consumer already owns this request ID")
		}
		owner := newRegistryTestOwner(authority, commands, newRegistryTestRecords())
		response, err := owner.Submit(context.Background(), registryTestEnvelope(strings.Repeat("d", 32)), "tester")
		if err != nil || response.Status != "submitted" || response.ControlRequest == nil || len(commands.controlIn) != 1 {
			t.Fatalf("formal consumer result was not reconciled: %+v %v calls=%d", response, err, len(commands.controlIn))
		}
	})
}
