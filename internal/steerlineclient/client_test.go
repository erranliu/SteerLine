package steerlineclient

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type readerFixture struct {
	api.Reader
	closed   int
	closeErr error
	command  func(context.Context, string) (storageports.CommandRecord, error)
	activity []storageports.DomainEvent
	domain   contracts.DomainSnapshot
}

func (r *readerFixture) Close() error { r.closed++; return r.closeErr }

func (r *readerFixture) Command(ctx context.Context, id string) (storageports.CommandRecord, error) {
	return r.command(ctx, id)
}

func (r *readerFixture) ShowNativeTask(context.Context, string) (contracts.NativeTaskPage, error) {
	return contracts.NativeTaskPage{ProjectID: "project"}, nil
}

func (r *readerFixture) Domain(context.Context) (contracts.DomainSnapshot, error) {
	return r.domain, nil
}

type commandsFixture struct {
	api.TaskCommands
	closed int
}

func (r *commandsFixture) Close() error { r.closed++; return nil }

func (r *commandsFixture) Validate(context.Context, []byte) (taskmodel.TaskBatchValidation, error) {
	return taskmodel.TaskBatchValidation{Valid: true}, nil
}

func testClient(t *testing.T, r api.Resources) *Client {
	t.Helper()
	r.Project = api.ProjectRef{ConfigPath: "config", RunDir: "state", Identity: "project"}
	c, err := Open(context.Background(), r.Project, func(context.Context, api.ProjectRef) (api.Resources, error) { return r, nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestCM71ResourceOwnershipAndClosedClient(t *testing.T) {
	r := &readerFixture{}
	commands := &commandsFixture{}
	closed := 0
	mutations := 0
	c := testClient(t, api.Resources{OpenReader: func(context.Context) (api.Reader, error) { return r, nil }, OpenCommands: func(_ context.Context, mutation bool) (api.TaskCommands, error) {
		if mutation {
			mutations++
		}
		return commands, nil
	}, Close: func() error { closed++; return nil }})
	if _, err := c.ShowTask(context.Background(), "1"); err != nil || r.closed != 1 {
		t.Fatalf("read resource not closed: %v / %d", err, r.closed)
	}
	if _, err := c.ValidateTaskBatch(context.Background(), []byte(`{}`)); err != nil || commands.closed != 1 || mutations != 0 {
		t.Fatalf("validation factory scope: %v / %+v", err, commands)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
	if closed != 1 {
		t.Fatalf("client resources closed %d times", closed)
	}
	if _, err := c.ShowTask(context.Background(), "1"); err == nil {
		t.Fatal("closed client opened another reader")
	}
	r.closeErr = errors.New("reader close failed")
	c2 := testClient(t, api.Resources{OpenReader: func(context.Context) (api.Reader, error) { return r, nil }})
	if _, err := c2.ShowTask(context.Background(), "1"); !errors.Is(err, r.closeErr) {
		t.Fatalf("close failure became success: %v", err)
	}
}

func TestCM71ProjectScopeAndErrors(t *testing.T) {
	closed := 0
	ref := api.ProjectRef{ConfigPath: "first", RunDir: "state", Identity: "project"}
	_, err := Open(context.Background(), ref, func(context.Context, api.ProjectRef) (api.Resources, error) {
		return api.Resources{Project: api.ProjectRef{ConfigPath: "second", RunDir: "state", Identity: "project"}, Close: func() error { closed++; return nil }}, nil
	})
	var productErr *productapi.Error
	if !errors.As(err, &productErr) || productErr.Kind != productapi.IdentityConflict || closed != 1 {
		t.Fatalf("same repository different configuration confused: %v / %d", err, closed)
	}
	for _, tc := range []struct {
		name string
		err  error
		kind productapi.ErrorKind
	}{{"missing", os.ErrNotExist, productapi.MissingDatabase}, {"unknown", productapi.ErrRecordNotFound, productapi.UnknownRecord}, {"schema", errors.New("unsupported database schema"), productapi.IncompatibleFormat}, {"identity", &taskmodel.Error{Code: "authority_mismatch", Message: "wrong binding"}, productapi.IdentityConflict}, {"read failure", errors.New("disk read failed"), productapi.ReadFailed}} {
		t.Run(tc.name, func(t *testing.T) {
			c := testClient(t, api.Resources{OpenReader: func(context.Context) (api.Reader, error) { return nil, tc.err }})
			_, err := c.ShowTask(context.Background(), "1")
			var e *productapi.Error
			if !errors.As(err, &e) || e.Kind != tc.kind || !errors.Is(err, tc.err) {
				t.Fatalf("error classified as %v", err)
			}
		})
	}
}

func TestWaitServiceCommandResolvesIdentityAndKeepsPolling(t *testing.T) {
	id := NewRequestID()
	calls := 0
	r := &readerFixture{}
	r.command = func(_ context.Context, got string) (storageports.CommandRecord, error) {
		if got != id {
			t.Fatalf("request ID changed: %s", got)
		}
		calls++
		if calls < 2 {
			return storageports.CommandRecord{}, productapi.ErrRecordNotFound
		}
		return storageports.CommandRecord{CommandID: id, Status: "accepted"}, nil
	}
	c := testClient(t, api.Resources{OpenReader: func(context.Context) (api.Reader, error) { return r, nil }})
	got, err := c.WaitCommand(context.Background(), id, time.Second)
	if err != nil || got.Status != "accepted" || r.closed != 2 {
		t.Fatalf("pending/accepted receipt scope: %+v %v / %d", got, err, r.closed)
	}
	r.command = func(context.Context, string) (storageports.CommandRecord, error) {
		return storageports.CommandRecord{}, productapi.ErrRecordNotFound
	}
	_, err = c.WaitCommand(context.Background(), id, 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), id) || !strings.Contains(err.Error(), "unconfirmed") {
		t.Fatalf("unknown command lost original request: %v", err)
	}
	failure := errors.New("damaged command record")
	r.command = func(context.Context, string) (storageports.CommandRecord, error) {
		return storageports.CommandRecord{}, failure
	}
	if _, err = c.WaitCommand(context.Background(), id, time.Second); !errors.Is(err, failure) {
		t.Fatalf("read failure disguised as missing command: %v", err)
	}
}

type controlFixture struct {
	api.ControlPort
	request     *productapi.ControlRequest
	submits     int
	loseReceipt bool
}

func (r *controlFixture) Request(_ context.Context, id string) (*productapi.ControlRequest, error) {
	if r.request == nil {
		return nil, os.ErrNotExist
	}
	if id != r.request.RequestID {
		return nil, errors.New("request ID changed")
	}
	return r.request, nil
}

func (r *controlFixture) Submit(_ context.Context, in productapi.ControlInput) (productapi.ControlRequest, error) {
	r.submits++
	payload, _ := json.Marshal(in.Payload)
	r.request = &productapi.ControlRequest{RequestID: in.RequestID, RepositoryIdentity: "project", InstanceID: in.InstanceID, Action: string(in.Action), Payload: payload, RequestedBy: in.RequestedBy}
	if in.ExpectedRevision != nil {
		r.request.ExpectedControlRevision = *in.ExpectedRevision
	}
	if r.loseReceipt {
		return productapi.ControlRequest{}, context.DeadlineExceeded
	}
	return *r.request, nil
}

func (r *controlFixture) Receipt(_ context.Context, id string) (*productapi.ControlReceipt, error) {
	if id != r.request.RequestID {
		return nil, errors.New("request ID changed")
	}
	return &productapi.ControlReceipt{RequestID: id, Status: "indeterminate"}, nil
}

func TestCM71UnknownControlUsesOriginalRequestAndRejectsConflict(t *testing.T) {
	port := &controlFixture{loseReceipt: true}
	c := testClient(t, api.Resources{Control: port})
	rev := uint64(4)
	in := productapi.ControlInput{Action: productapi.SetDispatchMode, Payload: productapi.ControlPayload{Mode: "manual"}, RequestID: NewRequestID(), RequestedBy: "operator", InstanceID: "service", ExpectedRevision: &rev}
	if _, err := c.SubmitControl(context.Background(), in); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost receipt was accepted as success: %v", err)
	}
	req, err := c.Request(context.Background(), in.RequestID)
	if err != nil || req.InstanceID != in.InstanceID || req.ExpectedControlRevision != rev {
		t.Fatalf("original target lost: %+v / %v", req, err)
	}
	receipt, err := c.Receipt(context.Background(), in.RequestID)
	if err != nil || receipt.Status != "indeterminate" {
		t.Fatalf("unknown result became success: %+v / %v", receipt, err)
	}
	if _, err = c.SubmitControl(context.Background(), in); err != nil || port.submits != 1 {
		t.Fatalf("replayed request republished: %v / %d", err, port.submits)
	}
	in.Payload.Mode = "auto"
	if _, err = c.SubmitControl(context.Background(), in); err == nil || port.submits != 1 {
		t.Fatal("same ID different content accepted")
	}
	in.Payload.Mode = "manual"
	in.InstanceID = "replacement"
	if _, err = c.SubmitControl(context.Background(), in); err == nil {
		t.Fatal("unknown request retargeted to replacement instance")
	}
	_ = c.Close()
	if port.submits != 1 {
		t.Fatal("closing client performed a control mutation")
	}
}

func TestCM71ActivityChecksOriginalAttemptBeforeReadingLog(t *testing.T) {
	r := &readerFixture{domain: contracts.DomainSnapshot{Executions: []executionmodel.ExecutionRecord{{ExecutionID: "exec", WorkflowRunID: "run"}}, Attempts: []executionmodel.StepAttemptRecord{{ExecutionID: "exec", AttemptID: "old"}, {ExecutionID: "exec", AttemptID: "new"}}}}
	c := testClient(t, api.Resources{OpenReader: func(context.Context) (api.Reader, error) { return r, nil }})
	if _, err := c.Activity(context.Background(), "run", "exec", "old", productapi.ActivityCursor{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Activity(context.Background(), "run", "exec", "old", productapi.ActivityCursor{RunID: "run", ExecutionID: "exec", AttemptID: "new", Initialized: true}); err == nil {
		t.Fatal("current cursor overrode original attempt")
	}
	if _, err := c.Activity(context.Background(), "other", "exec", "old", productapi.ActivityCursor{}); err == nil {
		t.Fatal("cross-run activity read")
	}
}

func (r *readerFixture) ActivityEvents(_ context.Context, _, _ string, after int64) ([]storageports.DomainEvent, error) {
	var events []storageports.DomainEvent
	for _, e := range r.activity {
		if e.EventID > after {
			events = append(events, e)
		}
	}
	return events, nil
}
