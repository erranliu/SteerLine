package sqlite

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	model "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/process"
	"github.com/erranliu/SteerLine/internal/execution/scheduler"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type runnerActorHost struct {
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
func (*runnerActorHost) Emit(model.ActivityEvent) {}
func (*runnerActorHost) CurrentProcessIdentity() (*hostcontract.ProcessIdentity, error) {
	return process.NewServiceFacts().CurrentIdentity()
}

func TestRunnerActorUsesHostScheduler(t *testing.T) {
	s, project, _ := nativeAuditStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	host := &runnerActorHost{}
	actor, err := taskrunner.NewActor(taskrunner.ActorOptions{Tasks: &taskrunner.Service{Store: s, Project: project}, Store: s, Host: host, Repository: hostcontract.Repository{Identity: project, Path: "fixture", Remote: "origin", Branch: "main"}, Mode: "manual", Heartbeat: 50 * time.Millisecond, NativeSessionAdapters: []string{"codex-app-server"}, Control: func(context.Context, api.ControlRequest) (api.ControlReceipt, error) {
		return api.ControlReceipt{}, errors.New("unexpected control")
	}})
	require(t, err)
	for _, required := range []string{"pause_dispatch", "resume_dispatch", "set_dispatch_mode", "runner_reply", "runner_resume", "answer_decision", "open_native_session", "open_native_session:codex-app-server"} {
		found := false
		for _, value := range actor.State.Service.Capabilities {
			found = found || value == required
		}
		if !found {
			t.Fatal("missing implemented control", required)
		}
	}
	done := make(chan error, 1)
	go func() {
		done <- scheduler.Run(ctx, actor, scheduler.Schedule{TaskPoll: time.Hour, PRPoll: time.Hour, Heartbeat: 50 * time.Millisecond}, "manual")
	}()
	for {
		checkpoint, readErr := s.LoadCheckpoint(ctx, project, true)
		if readErr == nil && checkpoint.Service.Ready {
			break
		}
		select {
		case err := <-done:
			t.Fatal("actor exited before ready", err)
		case <-ctx.Done():
			t.Fatal("actor never ready")
		case <-time.After(5 * time.Millisecond):
		}
	}
	commandRan := false
	require(t, actor.ExecuteTaskMutation(ctx, func(actorContext context.Context) error {
		if actorContext.Err() != nil {
			t.Error("task mutation lost live actor context")
		}
		commandRan = true
		return nil
	}))
	if !commandRan {
		t.Fatal("command did not run through actor")
	}
	cancel()
	select {
	case err := <-done:
		require(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("actor did not finish")
	}
	// EndActor publishes the checkpoint but leaves the borrowed store usable.
	checkpoint, err := s.LoadCheckpoint(context.Background(), project, true)
	require(t, err)
	if checkpoint.Service.Ready || checkpoint.Service.Status != "stopped" {
		t.Fatal("checkpoint stayed ready", checkpoint.Service)
	}
	if host.receipt.RequestID != "" {
		t.Fatal("actor fabricated a retired stop receipt")
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
			host := &runnerControlHost{runnerActorHost: &runnerActorHost{}, fileDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
			actor, err := taskrunner.NewActor(taskrunner.ActorOptions{Tasks: &taskrunner.Service{Store: s, Project: project}, Store: s, Host: host, Repository: hostcontract.Repository{Identity: project}, Mode: "manual", Heartbeat: time.Second, Control: func(_ context.Context, r api.ControlRequest) (api.ControlReceipt, error) {
				if changed {
					host.fileDigest = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
				}
				return api.ControlReceipt{Version: 1, RequestID: r.RequestID, InstanceID: r.InstanceID, Action: r.Action, Status: "applied", RequestDigest: taskrunner.RequestDigest(r)}, nil
			}})
			require(t, err)

			host.request = api.ControlRequest{RequestID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", InstanceID: actor.InstanceID(), Action: "pause_dispatch"}
			err = actor.TickControl(context.Background(), func() {})
			if changed {
				if err != nil || host.receipt.RequestID != "" || actor.State.Service.Error == nil || !strings.Contains(*actor.State.Service.Error, "host control request changed") {
					t.Fatal("changed original file must retain a diagnostic without a verified receipt or service shutdown", err, actor.State.Service.Error)
				}
			} else if err != nil || host.receipt.RequestDigest != host.fileDigest || host.receipt.RequestDigest == taskrunner.RequestDigest(host.request) {
				t.Fatal("host receipt used the parsed-value digest", host.receipt, err)
			}
		})
	}
}
