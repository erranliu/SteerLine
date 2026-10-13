package httpclient

import (
	"context"
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/product/api"
	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTUIHTTPContract(t *testing.T) {
	t.Run("render metadata remains responsive during HTTP IO", func(t *testing.T) {
		entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(entered)
			<-release
			json.NewEncoder(w).Encode(api.ProjectSnapshot{})
		}))
		defer server.Close()
		defer close(release)
		c := &Console{ref: api.ProjectRef{Identity: "repo"}, client: &Client{baseURL: server.URL, project: wire.ProjectID("repo"), http: server.Client()}}
		go func() { c.ReadProject(context.Background(), "", api.ProjectSnapshot{}); close(finished) }()
		<-entered
		metadata := make(chan api.ProjectRef, 1)
		go func() { metadata <- c.Project() }()
		select {
		case ref := <-metadata:
			if ref.Identity != "repo" {
				t.Fatal("wrong cached metadata")
			}
		case <-time.After(250 * time.Millisecond):
			t.Fatal("render metadata blocked behind network IO")
		}
	})
	t.Run("all TUI actions retain typed targets and versions", testConsoleActions)
	t.Run("lost stop receipt remains unverified", testStopUnknown)
	t.Run("lost response, duplicate and reopened client preserve original intent", func(t *testing.T) {
		ctx := context.Background()
		home := t.TempDir()
		identity := wire.Identity{Protocol: wire.Protocol, ProjectID: wire.ProjectID("repository"), RepositoryIdentity: "repository", InstanceID: "instance", Ready: true}
		var mu sync.Mutex
		calls := 0
		lost := true
		foreign := false
		receipts := map[string]wire.CommandResponse{}
		var view *Console
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			if r.Header.Get("Authorization") != "Bearer private" {
				t.Error("missing authenticated transport")
				w.WriteHeader(401)
				return
			}
			if r.URL.Path == "/v1/identity" {
				json.NewEncoder(w).Encode(identity)
				return
			}
			if strings.HasSuffix(r.URL.Path, "/commands") {
				var envelope wire.CommandEnvelope
				if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
					t.Error(err)
					return
				}
				// The server must never see a request before its original intent is durable.
				path, _ := view.outgoingPath(envelope.RequestID)
				data, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(data), envelope.RequestID) {
					t.Errorf("POST preceded durable intent: %v", err)
				}
				if envelope.InstanceID != "instance" || envelope.TargetID != "task" || envelope.ExpectedVersion != 7 || envelope.ExpectedControlRevision != 9 {
					t.Errorf("original target/version changed: %+v", envelope)
				}
				calls++
				receipt := api.ControlReceipt{RequestID: envelope.RequestID, InstanceID: identity.InstanceID, Action: envelope.Action, Status: "applied"}
				request := api.ControlRequest{RequestID: envelope.RequestID, InstanceID: identity.InstanceID, RepositoryIdentity: "repository", Action: envelope.Action}
				result := wire.CommandResponse{RequestID: envelope.RequestID, ProjectID: identity.ProjectID, InstanceID: identity.InstanceID, Action: envelope.Action, TargetID: envelope.TargetID, ExpectedVersion: envelope.ExpectedVersion, Status: "applied", Original: &envelope, ControlRequest: &request, ControlReceipt: &receipt}
				receipts[envelope.RequestID] = result
				if lost {
					lost = false
					connection, _, _ := w.(http.Hijacker).Hijack()
					connection.Close()
					return
				}
				json.NewEncoder(w).Encode(result)
				return
			}
			id := filepath.Base(r.URL.Path)
			if receipt, ok := receipts[id]; ok {
				if foreign {
					receipt.TargetID = "another-task"
				}
				json.NewEncoder(w).Encode(receipt)
			} else {
				w.WriteHeader(404)
				json.NewEncoder(w).Encode(wire.ErrorResponse{Code: "command_not_found"})
			}
		}))
		defer server.Close()
		identity.Address = strings.TrimPrefix(server.URL, "http://")
		low := &Client{baseURL: server.URL, token: "private", project: identity.ProjectID, identity: identity, http: server.Client()}
		ref := api.ProjectRef{Identity: "repository", ApplicationHome: home, RunDir: filepath.Join(home, "state")}
		view = &Console{ref: ref, client: low}
		revision := uint64(9)
		input := api.ControlInput{RequestID: strings.Repeat("a", 32), Action: api.StartTask, InstanceID: "instance", ExpectedRevision: &revision, Payload: api.ControlPayload{TaskID: "task", ExpectedTaskVersion: 7, ExecutionProfileID: "profile"}}
		if _, err := view.SubmitControl(ctx, input); err == nil {
			t.Fatal("lost response reported success")
		}
		// Reopening only reads this client's intent, then reconciles through HTTP.
		reopened := &Console{ref: ref, client: low}
		receipt, err := reopened.Receipt(ctx, input.RequestID)
		if err != nil || receipt == nil || receipt.Status != "applied" {
			t.Fatalf("original result not recovered: %+v %v", receipt, err)
		}
		request, err := reopened.SubmitControl(ctx, input)
		if err != nil || request.RequestID != input.RequestID {
			t.Fatalf("duplicate failed: %+v %v", request, err)
		}
		mu.Lock()
		foreign = true
		mu.Unlock()
		if _, err = reopened.Receipt(ctx, input.RequestID); err == nil {
			t.Fatal("foreign response became original receipt")
		}
		mu.Lock()
		foreign = false
		mu.Unlock()
		input.Payload.ExpectedTaskVersion = 8
		if _, err = reopened.SubmitControl(ctx, input); err == nil {
			t.Fatal("changed original envelope accepted")
		}
		mu.Lock()
		count := calls
		mu.Unlock()
		if count != 1 {
			t.Fatalf("side effects = %d", count)
		}
		original, _ := reopened.readOutgoing(input.RequestID)
		if original.Envelope.ExpectedVersion != 7 {
			t.Fatal("persisted version changed")
		}
	})
	t.Run("offline observation never grants mutation", func(t *testing.T) {
		c := NewConsole(context.Background(), api.ProjectRef{Identity: "repo", RunDir: filepath.Join(t.TempDir(), "missing"), ApplicationHome: t.TempDir()})
		previous := api.ProjectSnapshot{NativeTasks: &api.TaskPage{}, FullReadAt: time.Now(), Connection: api.ServiceConnection{CanControl: true, CanStop: true, State: api.ConnectionConnected}}
		got := c.ReadProject(context.Background(), "task", previous)
		if got.NativeTasks != previous.NativeTasks || got.Connection.CanControl || got.Connection.CanStop || got.Connection.State != api.ConnectionOffline {
			t.Fatalf("unsafe offline view: %+v", got)
		}
		if _, err := c.SubmitControl(context.Background(), api.ControlInput{Action: api.StartTask, RequestID: strings.Repeat("b", 32), InstanceID: "old"}); err == nil {
			t.Fatal("offline mutation accepted")
		}
		if _, err := os.Stat(c.ref.RunDir); !os.IsNotExist(err) {
			t.Fatalf("connection initialized business state: %v", err)
		}
	})
	t.Run("missing request in replacement instance cannot replay", func(t *testing.T) {
		identity := wire.Identity{Protocol: wire.Protocol, ProjectID: wire.ProjectID("repo"), RepositoryIdentity: "repo", InstanceID: "new", Ready: true}
		posts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/identity" {
				json.NewEncoder(w).Encode(identity)
				return
			}
			if r.Method == "POST" {
				posts++
			}
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(wire.ErrorResponse{Code: "command_not_found"})
		}))
		defer server.Close()
		identity.Address = strings.TrimPrefix(server.URL, "http://")
		c := &Console{ref: api.ProjectRef{Identity: "repo", ApplicationHome: t.TempDir()}, client: &Client{baseURL: server.URL, token: "private", identity: identity, project: identity.ProjectID, http: server.Client()}}
		original := wire.CommandEnvelope{RequestID: strings.Repeat("c", 32), ProjectID: identity.ProjectID, InstanceID: "old", Action: "stop_service", TargetID: "old", Payload: json.RawMessage(`{}`)}
		if err := c.saveOutgoing(outgoing{Version: 1, Envelope: original}); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Request(context.Background(), original.RequestID); err == nil {
			t.Fatal("old request reported absent/replayed")
		}
		if posts != 0 {
			t.Fatalf("replacement replayed %d times", posts)
		}
	})
}
