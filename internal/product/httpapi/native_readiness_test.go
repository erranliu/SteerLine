package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

// Signal the lock attempt while retaining the exact shared arbitration lock.
// This makes the queue -> readiness loss -> lock acquisition order explicit.
type nativeReadinessBarrier struct {
	sync.Locker
	once    sync.Once
	entered chan struct{}
}

func (b *nativeReadinessBarrier) Lock() {
	b.once.Do(func() { close(b.entered) })
	b.Locker.Lock()
}

func nativeReadinessEnvelope(t *testing.T, server *Server, reader *apiReader) CommandEnvelope {
	t.Helper()
	run := &api.CandidateSummary{RunID: strings.Repeat("a", 32), ExecutionID: strings.Repeat("b", 32), AttemptID: strings.Repeat("c", 32), SessionID: "original", SessionGeneration: "generation"}
	reader.snapshot.Connection.Snapshot.Page.Runs = []*api.CandidateSummary{run}
	payload, err := json.Marshal(api.NativeSessionTarget{RunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, SessionID: run.SessionID, SessionGeneration: run.SessionGeneration})
	if err != nil {
		t.Fatal(err)
	}
	return CommandEnvelope{RequestID: strings.Repeat("d", 32), ProjectID: ProjectID(server.options.ProjectIdentity), InstanceID: server.options.InstanceID, Action: "open_native_session", TargetID: run.RunID, Payload: payload}
}

func submitNativeReadinessCommand(t *testing.T, server *Server, envelope CommandEnvelope) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/commands", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.submitCommand(recorder, request)
	return recorder
}

func assertNativeReadinessResponse(t *testing.T, recorder *httptest.ResponseRecorder, status string) CommandResponse {
	t.Helper()
	var response CommandResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != status || recorder.Code != responseHTTPStatus(status) {
		t.Fatalf("native response: HTTP %d %+v, want %s", recorder.Code, response, status)
	}
	return response
}

func TestHTTPNativeQueuedReadiness(t *testing.T) {
	for _, mode := range []string{"local_not_ready", "server_closed", "runtime_not_ready"} {
		t.Run(mode, func(t *testing.T) {
			server, _, reader, _ := newAPIFixture(t)
			envelope := nativeReadinessEnvelope(t, server, reader)
			opened := 0
			reader.onOpenNative = func(api.NativeSessionTarget) error { opened++; return nil }
			shared := server.commands.mu
			shared.Lock()
			locked := true
			defer func() {
				if locked {
					shared.Unlock()
				}
			}()
			barrier := &nativeReadinessBarrier{Locker: shared, entered: make(chan struct{})}
			server.commands.mu = barrier
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() { done <- submitNativeReadinessCommand(t, server, envelope) }()
			select {
			case <-barrier.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("native request did not reach shared arbitration")
			}
			// The request is queued behind the shared lock. Lose readiness before
			// allowing it to acquire that lock; no query hook or sleep is needed.
			switch mode {
			case "local_not_ready":
				if err := server.SetReady(server.options.InstanceID, false, "stopping"); err != nil {
					t.Fatal(err)
				}
			case "server_closed":
				if err := server.Close(context.Background()); err != nil {
					t.Fatal(err)
				}
			case "runtime_not_ready":
				reader.snapshot.Connection.Snapshot.Page.Service.Ready = false
			}
			shared.Unlock()
			locked = false
			var recorder *httptest.ResponseRecorder
			select {
			case recorder = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("queued native request did not complete after arbitration release")
			}
			response := assertNativeReadinessResponse(t, recorder, "rejected")
			if response.Code != "stale_target" || opened != 0 {
				t.Fatalf("queued request opened after readiness loss: opened=%d response=%+v", opened, response)
			}
			// The original rejection is durable even when readiness recovers.
			if mode != "server_closed" {
				reader.snapshot.Connection.Snapshot.Page.Service.Ready = true
				if err := server.SetReady(server.options.InstanceID, true, ""); err != nil {
					t.Fatal(err)
				}
			}
			assertNativeReadinessResponse(t, submitNativeReadinessCommand(t, server, envelope), "rejected")
			if opened != 0 {
				t.Fatal("replayed a rejected native operation")
			}
		})
	}
}

func TestHTTPNativeRuntimeReadiness(t *testing.T) {
	for _, condition := range []string{"missing_snapshot", "stale_instance", "runtime_not_ready", "disconnected", "identity_unmatched", "config_unmatched"} {
		t.Run(condition, func(t *testing.T) {
			server, _, reader, _ := newAPIFixture(t)
			envelope := nativeReadinessEnvelope(t, server, reader)
			opened := 0
			reader.onOpenNative = func(api.NativeSessionTarget) error { opened++; return nil }
			original := reader.snapshot
			page := *original.Connection.Snapshot.Page
			reader.snapshot.Connection.Snapshot.Page = &page
			switch condition {
			case "missing_snapshot":
				reader.snapshot.Connection.Snapshot.Page = nil
			case "stale_instance":
				page.Service.InstanceID = "instance-old"
			case "runtime_not_ready":
				page.Service.Ready = false
			case "disconnected":
				reader.snapshot.Connection.State = api.ConnectionOffline
			case "identity_unmatched":
				reader.snapshot.Connection.IdentityMatched = false
			case "config_unmatched":
				reader.snapshot.Connection.ConfigMatched = false
			}
			if !server.readiness.Ready() {
				t.Fatal("test must retain local readiness while runtime readiness changes")
			}
			assertNativeReadinessResponse(t, submitNativeReadinessCommand(t, server, envelope), "rejected")
			if opened != 0 {
				t.Fatal("unready runtime opened native viewer")
			}
			reader.snapshot = original
			assertNativeReadinessResponse(t, submitNativeReadinessCommand(t, server, envelope), "rejected")
			envelope.RequestID = strings.Repeat("e", 32)
			assertNativeReadinessResponse(t, submitNativeReadinessCommand(t, server, envelope), "applied")
			reader.snapshot.Connection.Snapshot.Page = nil
			assertNativeReadinessResponse(t, submitNativeReadinessCommand(t, server, envelope), "applied")
			if opened != 1 {
				t.Fatalf("native replay changed side effect count: %d", opened)
			}
		})
	}
}
