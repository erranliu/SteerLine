package client

import (
	"context"
	"encoding/json"
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"os"
	"strings"
	"testing"
	"time"
)

type readerFixture struct {
	Reader
	closed   int
	closeErr error
	command  func(context.Context, string) (api.CommandReceipt, error)
}

func (r *readerFixture) Close() error { r.closed++; return r.closeErr }

func (r *readerFixture) Command(ctx context.Context, id string) (api.CommandReceipt, error) {
	return r.command(ctx, id)
}

func (r *readerFixture) ShowTask(context.Context, string) (api.TaskPage, error) {
	return api.TaskPage{ProjectID: "project"}, nil
}

type commandsFixture struct {
	TaskCommands
	closed    int
	discarded int
}

func (r *commandsFixture) Close() error { r.closed++; return nil }

func (r *commandsFixture) Validate(context.Context, []byte) (api.TaskBatchValidation, error) {
	return api.TaskBatchValidation{Valid: true}, nil
}
func (r *commandsFixture) Discard(context.Context, api.DiscardInput) (api.TaskDiscardReceipt, error) {
	r.discarded++
	return api.TaskDiscardReceipt{}, nil
}

func testClient(t *testing.T, r Resources) *Client {
	t.Helper()
	r.Project = api.ProjectRef{ConfigPath: "config", RunDir: "state", Identity: "project"}
	c, err := Open(context.Background(), r.Project, func(context.Context, api.ProjectRef) (Resources, error) { return r, nil })
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
	c := testClient(t, Resources{OpenReader: func(context.Context) (Reader, error) { return r, nil }, OpenCommands: func(context.Context) (TaskCommands, error) {
		return commands, nil
	}, Discard: commands, Close: func() error { closed++; return nil }})
	if _, err := c.ShowTask(context.Background(), "1"); err != nil || r.closed != 1 {
		t.Fatalf("read resource not closed: %v / %d", err, r.closed)
	}
	if _, err := c.ValidateTaskBatch(context.Background(), []byte(`{}`)); err != nil || commands.closed != 1 {
		t.Fatalf("validation factory scope: %v / %+v", err, commands)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DiscardTask(context.Background(), api.DiscardInput{}); err == nil || commands.discarded != 0 {
		t.Fatal("closed client reached discard owner", err)
	}
	_ = c.Close()
	if closed != 1 {
		t.Fatalf("client resources closed %d times", closed)
	}
	if _, err := c.ShowTask(context.Background(), "1"); err == nil {
		t.Fatal("closed client opened another reader")
	}
	r.closeErr = errors.New("reader close failed")
	c2 := testClient(t, Resources{OpenReader: func(context.Context) (Reader, error) { return r, nil }})
	if _, err := c2.DiscardTask(context.Background(), api.DiscardInput{}); err == nil {
		t.Fatal("discard without an explicit owner was accepted")
	}
	c3 := testClient(t, Resources{Discard: commands})
	if _, err := c3.DiscardTask(context.Background(), api.DiscardInput{}); err != nil || commands.discarded != 1 {
		t.Fatal("discard did not use its explicit owner", err)
	}
	if _, err := c2.ShowTask(context.Background(), "1"); !errors.Is(err, r.closeErr) {
		t.Fatalf("close failure became success: %v", err)
	}
}

func TestCM71ProjectScopeAndErrors(t *testing.T) {
	closed := 0
	ref := api.ProjectRef{ConfigPath: "first", RunDir: "state", Identity: "project"}
	_, err := Open(context.Background(), ref, func(context.Context, api.ProjectRef) (Resources, error) {
		return Resources{Project: api.ProjectRef{ConfigPath: "second", RunDir: "state", Identity: "project"}, Close: func() error { closed++; return nil }}, nil
	})
	var productErr *api.Error
	if !errors.As(err, &productErr) || productErr.Kind != api.IdentityConflict || closed != 1 {
		t.Fatalf("same repository different configuration confused: %v / %d", err, closed)
	}
	for _, tc := range []struct {
		name string
		err  error
		kind api.ErrorKind
	}{{"missing", os.ErrNotExist, api.MissingDatabase}, {"unknown", api.ErrRecordNotFound, api.UnknownRecord}, {"schema", errors.New("unsupported database schema"), api.IncompatibleFormat}, {"identity", &api.Error{Kind: api.IdentityConflict, Message: "wrong binding"}, api.IdentityConflict}, {"read failure", errors.New("disk read failed"), api.ReadFailed}} {
		t.Run(tc.name, func(t *testing.T) {
			c := testClient(t, Resources{OpenReader: func(context.Context) (Reader, error) { return nil, tc.err }})
			_, err := c.ShowTask(context.Background(), "1")
			var e *api.Error
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
	r.command = func(_ context.Context, got string) (api.CommandReceipt, error) {
		if got != id {
			t.Fatalf("request ID changed: %s", got)
		}
		calls++
		if calls < 2 {
			return api.CommandReceipt{}, api.ErrRecordNotFound
		}
		return api.CommandReceipt{CommandID: id, Status: "accepted"}, nil
	}
	c := testClient(t, Resources{OpenReader: func(context.Context) (Reader, error) { return r, nil }})
	got, err := c.WaitCommand(context.Background(), id, time.Second)
	if err != nil || got.Status != "accepted" || r.closed != 2 {
		t.Fatalf("pending/accepted receipt scope: %+v %v / %d", got, err, r.closed)
	}
	r.command = func(context.Context, string) (api.CommandReceipt, error) {
		return api.CommandReceipt{}, api.ErrRecordNotFound
	}
	_, err = c.WaitCommand(context.Background(), id, 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), id) || !strings.Contains(err.Error(), "unconfirmed") {
		t.Fatalf("unknown command lost original request: %v", err)
	}
	failure := errors.New("damaged command record")
	r.command = func(context.Context, string) (api.CommandReceipt, error) {
		return api.CommandReceipt{}, failure
	}
	if _, err = c.WaitCommand(context.Background(), id, time.Second); !errors.Is(err, failure) {
		t.Fatalf("read failure disguised as missing command: %v", err)
	}
}

type controlFixture struct {
	ControlPort
	request     *api.ControlRequest
	submits     int
	loseReceipt bool
}

func (r *controlFixture) Request(_ context.Context, id string) (*api.ControlRequest, error) {
	if r.request == nil {
		return nil, os.ErrNotExist
	}
	if id != r.request.RequestID {
		return nil, errors.New("request ID changed")
	}
	return r.request, nil
}

func (r *controlFixture) Submit(_ context.Context, in api.ControlInput) (api.ControlRequest, error) {
	r.submits++
	payload, _ := json.Marshal(in.Payload)
	r.request = &api.ControlRequest{RequestID: in.RequestID, RepositoryIdentity: "project", InstanceID: in.InstanceID, Action: string(in.Action), Payload: payload, RequestedBy: in.RequestedBy}
	if in.ExpectedRevision != nil {
		r.request.ExpectedControlRevision = *in.ExpectedRevision
	}
	if r.loseReceipt {
		return api.ControlRequest{}, context.DeadlineExceeded
	}
	return *r.request, nil
}

func (r *controlFixture) Receipt(_ context.Context, id string) (*api.ControlReceipt, error) {
	if id != r.request.RequestID {
		return nil, errors.New("request ID changed")
	}
	return &api.ControlReceipt{RequestID: id, Status: "indeterminate"}, nil
}

func TestTM05OriginalRequest(t *testing.T) {
	port := &controlFixture{loseReceipt: true}
	c := testClient(t, Resources{Control: port})
	rev := uint64(4)
	in := api.ControlInput{Action: api.SetDispatchMode, Payload: api.ControlPayload{Mode: "manual"}, RequestID: NewRequestID(), RequestedBy: "operator", InstanceID: "service", ExpectedRevision: &rev}
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
