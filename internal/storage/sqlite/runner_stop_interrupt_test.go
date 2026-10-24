package sqlite

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/hostport"
	"github.com/erranliu/SteerLine/internal/execution/scheduler"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type stopHost struct {
	hostport.HostPort
	mu      sync.Mutex
	request *api.ControlRequest
	receipt *api.ControlReceipt
	done    chan api.ControlReceipt
}

func (h *stopHost) CurrentProcessIdentity() (*hostcontract.ProcessIdentity, error) {
	return &hostcontract.ProcessIdentity{Kind: "os", PID: 1}, nil
}
func (h *stopHost) ControlRequestIDs() ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.request == nil {
		return nil, nil
	}
	return []string{h.request.RequestID}, nil
}
func (h *stopHost) ReadControlRequest(string) (api.ControlRequest, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return *h.request, nil
}
func (h *stopHost) ReadControlReceipt(string) (*api.ControlReceipt, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.receipt == nil {
		return nil, os.ErrNotExist
	}
	r := *h.receipt
	return &r, nil
}
func (h *stopHost) ControlRequestDigest(string) string { return strings.Repeat("d", 64) }
func (h *stopHost) WriteControlReceipt(r api.ControlReceipt) error {
	h.mu.Lock()
	h.receipt = &r
	h.mu.Unlock()
	h.done <- r
	return nil
}

type blockedObservation struct {
	runnerbridge.CLI
	run     runnerclient.Run
	entered chan struct{}
	once    sync.Once
}

func (c *blockedObservation) Inspect(context.Context, string, string, bool) (runnerclient.Inspection, error) {
	return runnerclient.Inspection{Run: &c.run}, nil
}
func (*blockedObservation) SupportsObservation() bool { return true }
func (c *blockedObservation) Observe(ctx context.Context, _, _ string) (runnerclient.RunObservation, error) {
	c.once.Do(func() { close(c.entered) })
	<-ctx.Done()
	return runnerclient.RunObservation{}, ctx.Err()
}

func TestCandidateStopReliabilityBlockedObservation(t *testing.T) {
	for _, mode := range []string{"candidate", "generation", "ordinary_control"} {
		t.Run(mode, func(t *testing.T) { testBlockedCandidateStop(t, mode) })
	}
}
func testBlockedCandidateStop(t *testing.T, mode string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, project, reserve := candidateStopFixture(t)
	first := reserve(strings.Repeat("b", 32), false)
	reserve("second", true)
	first.RunID, first.InputDigest, first.SubmitState = "run", "input", "accepted"
	first.Projection = &runnerclient.Run{RunID: "run", InputDigest: "input", Revision: 1, Status: "running"}
	first.Revision++
	require(t, s.SaveCandidate(ctx, first, 1, nil))
	cli := &blockedObservation{run: *first.Projection, entered: make(chan struct{})}
	bridge := &runnerbridge.Bridge{Store: s, Client: func(string) runnerbridge.CLI { return cli }}
	svc := &taskrunner.Service{Project: project, Store: s, Bridge: bridge}
	controls := taskrunner.Controls{Tasks: svc, Store: s, ResolveProfile: func(string, string) (string, error) { t.Fatal("stop consulted execution profile"); return "", nil }}
	h := &stopHost{done: make(chan api.ControlReceipt, 1)}
	if mode == "ordinary_control" {
		h.request = &api.ControlRequest{RequestID: strings.Repeat("f", 32), Action: "start_candidate"}
	}
	var actor *taskrunner.Actor
	actor, err := taskrunner.NewActor(taskrunner.ActorOptions{Tasks: svc, Store: s, Host: h, Repository: hostcontract.Repository{Identity: project}, Mode: "manual", Heartbeat: time.Second, Control: func(ctx context.Context, r api.ControlRequest) (api.ControlReceipt, error) {
		if mode == "ordinary_control" && r.Action == "start_candidate" {
			cli.once.Do(func() { close(cli.entered) })
			<-ctx.Done()
			return api.ControlReceipt{}, ctx.Err()
		}
		return controls.Handle(ctx, r, actor.InstanceID(), 1)
	}})
	require(t, err)
	if mode == "generation" {
		actor.ObserveAuxiliary = func(ctx context.Context) error {
			cli.once.Do(func() { close(cli.entered) })
			<-ctx.Done()
			return ctx.Err()
		}
	}
	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx, actor, scheduler.Schedule{TaskPoll: time.Hour, PRPoll: time.Hour, Heartbeat: time.Hour}, "manual")
	}()
	defer func() {
		cancel()
		select {
		case err := <-done:
			require(t, err)
		case <-time.After(5 * time.Second):
			t.Error("actor did not release work")
		}
	}()
	select {
	case <-cli.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("observation did not start")
	}
	payload, _ := json.Marshal(api.ControlPayload{RunID: first.ID, TaskID: first.TaskID, ExpectedRunRevision: 1})
	now := time.Now().UTC()
	req := api.ControlRequest{ProtocolVersion: 1, RequestID: strings.Repeat("e", 32), RepositoryIdentity: project, InstanceID: actor.InstanceID(), Action: "stop_candidate", ExpectedControlRevision: 99, RequestedBy: "operator", CreatedAt: now, ExpiresAt: now.Add(time.Minute), Payload: payload}
	h.mu.Lock()
	h.request = &req
	h.mu.Unlock()
	started := time.Now()
	actor.NotifyControl()
	select {
	case receipt := <-h.done:
		if receipt.Status != "applied" {
			t.Fatal(receipt)
		}
		t.Logf("blocked Runner stop settled in %s", time.Since(started))
	case <-time.After(5 * time.Second):
		t.Fatal("blocked observation retained stop")
	}
	current, err := s.Candidate(ctx, project, first.ID)
	require(t, err)
	if current.Participation != "abandoned" || !current.Withdrawal.Matches(req.RequestID, 1, "operator") {
		t.Fatal("wrong withdrawal", current)
	}
	if ctx.Err() != nil {
		t.Fatal("stop killed service")
	}
}
