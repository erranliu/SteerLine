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
func (*runnerActorHost) ControlRequestDigest(string) string                     { return "" }
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
						return &api.ControlRequest{ProtocolVersion: 1, RequestID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", InstanceID: instance, RepositoryIdentity: project, Action: "stop_service", RequestedBy: "fixture", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}, "", now
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
			require(t, actor.Call(ctx, func() error { commandRan = true; return nil }))
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
