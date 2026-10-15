package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
	model "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/execution/scheduler"
	api "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type runnerActorHost struct {
	ready   chan struct{}
	mu      sync.Mutex
	receipt api.ControlReceipt
}

func (*runnerActorHost) ValidateStateLocation() error         { return nil }
func (*runnerActorHost) ControlRequestIDs() ([]string, error) { return nil, nil }
func (*runnerActorHost) ReadControlRequest(string) (api.ControlRequest, error) {
	return api.ControlRequest{}, errors.New("no request")
}
func (*runnerActorHost) ReadControlReceipt(string) (*api.ControlReceipt, error) { return nil, nil }
func (*runnerActorHost) ControlRequestDigest(string) string {
	return "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
}
func (h *runnerActorHost) WriteControlReceipt(r api.ControlReceipt) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.receipt = r
	return nil
}
func (*runnerActorHost) FindStopRequest(string, map[string]bool) (*api.ControlRequest, string, time.Time) {
	return nil, "", time.Time{}
}
func (h *runnerActorHost) PublishReady(string, *b.ProcessIdentity, string) error {
	h.ready <- struct{}{}
	return nil
}
func (*runnerActorHost) Emit(model.ActivityEvent) {}
func (*runnerActorHost) CurrentProcessIdentity() (*b.ProcessIdentity, error) {
	return process.NewServiceFacts().CurrentIdentity()
}

func TestRunnerActorUsesHostScheduler(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "stopped", true: "native_stop_unknown"}[uncertain], func(t *testing.T) {
			s, project, _ := nativeAuditStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			host := &runnerActorHost{ready: make(chan struct{}, 1)}
			calls := 0
			actor, err := taskrunner.NewActor(taskrunner.ActorOptions{Tasks: &taskrunner.Service{Store: s, Project: project}, Store: s, Host: host, Repository: storageports.CheckpointRepository{Identity: project, Path: "fixture", Remote: "origin", Branch: "main"}, Mode: "manual", Heartbeat: 50 * time.Millisecond,
				Control: func(context.Context, api.ControlRequest) (api.ControlReceipt, error) {
					return api.ControlReceipt{}, errors.New("unexpected control")
				},
				ShutdownRunner: func(context.Context) error {
					calls++
					if uncertain {
						return errors.New("native shutdown unverified")
					}
					return nil
				},
			})
			require(t, err)
			for _, required := range []string{"stop_service", "pause_dispatch", "resume_dispatch", "set_dispatch_mode", "runner_reply", "runner_resume", "answer_decision"} {
				found := false
				for _, value := range actor.State.Service.Capabilities {
					found = found || value == required
				}
				if !found {
					t.Fatal("service did not advertise its implemented control", required)
				}
			}
			// RecoverInputs requires only caller persistence, with no Runner needed
			// when no candidate has been reserved.
			actorTasks := actor
			stop := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				done <- scheduler.Run(ctx, actorTasks, scheduler.Schedule{TaskPoll: time.Hour, PRPoll: time.Hour, Heartbeat: 50 * time.Millisecond, FindStop: func(instance string, _ map[string]bool) (*api.ControlRequest, string, time.Time) {
					select {
					case <-stop:
						now := time.Now().UTC()
						return &api.ControlRequest{ProtocolVersion: 1, RequestID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", InstanceID: instance, RepositoryIdentity: project, Action: "stop_service", RequestedBy: "fixture", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", now
					default:
						return nil, "", time.Time{}
					}
				}}, "manual")
			}()
			select {
			case <-host.ready:
			case <-ctx.Done():
				t.Fatal("service never became ready")
			}
			commandRan := false
			require(t, actor.ExecuteTaskMutation(ctx, func(actorContext context.Context) error {
				if actorContext == nil || actorContext.Err() != nil {
					t.Error("task mutation lost its live actor context")
				}
				commandRan = true
				return nil
			}))
			if !commandRan {
				t.Fatal("HTTP command was not serialized through the actor")
			}
			close(stop)
			var runErr error
			select {
			case runErr = <-done:
			case <-ctx.Done():
				t.Fatal("service did not settle its stop")
			}
			if (runErr != nil) != uncertain || calls != 1 {
				t.Fatal("shutdown outcome was guessed or replayed", runErr, calls)
			}
			if (actor.ResourceSettlementError() != nil) != uncertain {
				t.Fatal("resource settlement differs from the original shutdown evidence")
			}
			checkpoint, err := s.LoadCheckpoint(context.Background(), project, true)
			require(t, err)
			if checkpoint.Service.Ready || checkpoint.Service.Status != "stopped" {
				t.Fatal("service checkpoint stayed ready after actor exit")
			}
			host.mu.Lock()
			receipt := host.receipt
			host.mu.Unlock()
			want := "applied"
			if uncertain {
				want = "indeterminate"
			}
			if receipt.Status != want || receipt.RequestDigest == "" {
				t.Fatal("stop receipt lost its request or native uncertainty", receipt)
			}
		})
	}
}

// Host transport hashes the immutable file bytes, while the caller's command
// store deduplicates the parsed value. Neither may substitute for the other.
type runnerControlHost struct {
	*runnerActorHost
	request    api.ControlRequest
	fileDigest string
}

func (h *runnerControlHost) ControlRequestIDs() ([]string, error) {
	return []string{h.request.RequestID}, nil
}
func (h *runnerControlHost) ReadControlRequest(string) (api.ControlRequest, error) {
	return h.request, nil
}
func (h *runnerControlHost) ControlRequestDigest(string) string { return h.fileDigest }
func TestRunnerActorBindsReceiptToOriginalHostFile(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "same", true: "changed"}[changed], func(t *testing.T) {
			s, project, _ := nativeAuditStore(t)
			host := &runnerControlHost{runnerActorHost: &runnerActorHost{ready: make(chan struct{}, 1)}, fileDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
			actor, err := taskrunner.NewActor(taskrunner.ActorOptions{Tasks: &taskrunner.Service{Store: s, Project: project}, Store: s, Host: host, Repository: storageports.CheckpointRepository{Identity: project}, Mode: "manual", Heartbeat: time.Second, ShutdownRunner: func(context.Context) error { return nil }, Control: func(_ context.Context, r api.ControlRequest) (api.ControlReceipt, error) {
				if changed {
					host.fileDigest = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
				}
				return api.ControlReceipt{Version: 1, RequestID: r.RequestID, InstanceID: r.InstanceID, Action: r.Action, Status: "applied", RequestDigest: taskrunner.RequestDigest(r)}, nil
			}})
			require(t, err)
			defer actor.Close()
			host.request = api.ControlRequest{RequestID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", InstanceID: actor.InstanceID(), Action: "pause_dispatch"}
			err = actor.TickControl(context.Background(), func() {})
			if changed {
				if err == nil || host.receipt.RequestID != "" {
					t.Fatal("changed original file received a verified receipt", err)
				}
			} else if err != nil || host.receipt.RequestDigest != host.fileDigest || host.receipt.RequestDigest == taskrunner.RequestDigest(host.request) {
				t.Fatal("host receipt used the parsed-value digest", host.receipt, err)
			}
		})
	}
}
