package orcaworker

import (
	"context"
	"errors"
	"testing"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func TestBoundTerminalGateReleasesCanceledWaiter(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	client := New(Config{Output: func(ctx context.Context, _ time.Duration, _ ...string) ([]byte, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return []byte(`{"ok":true,"result":{"terminal":{"handle":"agent","incarnationId":"inc"}},"_meta":{"runtimeId":"runtime"}}`), nil
	}})
	identity := executionapi.ProcessIdentity{Terminal: "agent", RuntimeID: "runtime", Incarnation: "inc"}
	target := executionapi.ExecutionTarget{ID: "test-gate"}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := client.InspectBoundTerminal(ctx, target, identity); done <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waiter, cancelWaiter := context.WithCancel(ctx)
	cancelWaiter()
	if _, err := client.InspectBoundTerminal(waiter, target, identity); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter was accepted: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	orcaObservationGates.Lock()
	remaining := len(orcaObservationGates.byIdentity)
	orcaObservationGates.Unlock()
	if remaining != 0 {
		t.Fatalf("observation gates leaked: %d", remaining)
	}
}

func TestTerminalObservationRejectsFailedTransportWithValidPayload(t *testing.T) {
	failure := errors.New("transport interrupted")
	client := New(Config{Output: func(context.Context, time.Duration, ...string) ([]byte, error) {
		return []byte(`{"ok":true,"result":{"terminal":{"handle":"agent"}},"_meta":{"runtimeId":"runtime"}}`), failure
	}})
	if _, err := client.ShowTerminal(context.Background(), "agent"); !errors.Is(err, failure) {
		t.Fatalf("failed observation was accepted: %v", err)
	}
}

func TestMutationRejectionDoesNotHideTransportTimeout(t *testing.T) {
	for _, code := range []string{"inject_rejected", "dispatch_inactive"} {
		t.Run(code, func(t *testing.T) {
			client := New(Config{Output: func(context.Context, time.Duration, ...string) ([]byte, error) {
				return []byte(`{"ok":false,"error":{"code":"` + code + `","message":"rejected","data":{"reason":"no_agent_detected"}}}`), context.DeadlineExceeded
			}})
			result, err := client.MetaCall(context.Background(), "orchestration", "dispatch")
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("timeout evidence was lost")
			}
			if NoAgentDetected(result) || FeedbackDispatchInactive(&CallError{Cause: err, Result: result}) {
				t.Fatal("uncertain transport was treated as a conclusive rejection")
			}
			client = New(Config{Output: func(context.Context, time.Duration, ...string) ([]byte, error) {
				return []byte(`{"ok":false,"error":{"code":"` + code + `","message":"rejected","data":{"reason":"no_agent_detected"}}}`), nil
			}})
			result, err = client.MetaCall(context.Background(), "orchestration", "dispatch")
			if code == "inject_rejected" && !NoAgentDetected(result) || code == "dispatch_inactive" && !FeedbackDispatchInactive(&CallError{Cause: err, Result: result}) {
				t.Fatal("verified platform rejection was lost")
			}
		})
	}
}
