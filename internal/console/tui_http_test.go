package console

import (
	"context"
	"encoding/hex"
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
	fs "github.com/erranliu/SteerLine/internal/product/httpfs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestTUIHTTPInteraction(t *testing.T) {
	home := t.TempDir()
	ref := testProject("http")
	ref.ApplicationHome = home
	ref.RunDir = filepath.Join(home, "state")
	snapshot := readySnapshot(ref, time.Now())
	snapshot.Connection.Snapshot.Page.Runs = []*api.CandidateSummary{{RunID: "run", ExecutionID: "execution", AttemptID: "current-attempt"}}
	queries := 0
	text := strings.Repeat("完整回复\n", 20000)
	event := readerFinal(text, "original-item", "old-attempt", "old-turn")
	event.ProjectID = ref.Identity
	cursors, bodyReads := 0, 0
	expired := false
	identity := wire.Identity{Protocol: wire.Protocol, ProjectID: wire.ProjectID(ref.Identity), RepositoryIdentity: ref.Identity, InstanceID: snapshot.Connection.Snapshot.Page.Service.InstanceID, Ready: true}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 64) {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.URL.Path == "/v1/identity":
			json.NewEncoder(w).Encode(identity)
		case strings.HasSuffix(r.URL.Path, "/presentation"):
			json.NewEncoder(w).Encode(wire.ProjectPresentation{ProjectID: identity.ProjectID, Presentation: ref.Presentation})
		case strings.HasSuffix(r.URL.Path, "/snapshot"):
			queries++
			json.NewEncoder(w).Encode(snapshot)
		case strings.HasSuffix(r.URL.Path, "/conversation"):
			q := r.URL.Query()
			if q.Get("execution_id") != "execution" || q.Get("session_id") != "session" || q.Get("attempt_id") != "" || q.Get("finals_only") != "true" {
				t.Error("full history filter changed")
			}
			if expired {
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(wire.ErrorResponse{Code: "cursor_invalidated"})
				return
			}
			page := api.ConversationPage{}
			if q.Get("cursor") == "" {
				page.Events = []api.ConversationEvent{event}
				page.HasMore = true
				page.NextCursor = "original-next"
			} else if q.Get("cursor") != "original-next" {
				t.Error("cursor changed")
			}
			cursors++
			json.NewEncoder(w).Encode(wire.ConversationPage{ProjectID: identity.ProjectID, Page: page})
		case strings.HasSuffix(r.URL.Path, "/body"):
			q := r.URL.Query()
			data, _ := hex.DecodeString(q.Get("ref"))
			var body api.ConversationBodyRef
			json.Unmarshal(data, &body)
			if body != *event.Body {
				t.Error("body original metadata changed")
			}
			offset, _ := strconv.Atoi(q.Get("offset"))
			limit, _ := strconv.Atoi(q.Get("limit"))
			end := min(offset+limit, len(text))
			for !utf8.ValidString(text[offset:end]) {
				end--
			}
			bodyReads++
			json.NewEncoder(w).Encode(api.ConversationBodyPage{ItemKey: body.ItemKey, Revision: body.Revision, SHA256: body.SHA256, Completeness: body.Completeness, Offset: int64(offset), NextOffset: int64(end), TotalBytes: body.Bytes, Complete: end == len(text), Data: []byte(text[offset:end])})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	identity.Address = strings.TrimPrefix(server.URL, "http://")
	apiDir := filepath.Join(ref.RunDir, "local-api")
	credentials := filepath.Join(apiDir, "credentials")
	for _, path := range []string{apiDir, credentials} {
		if err := fs.EnsurePrivateDirectory(path); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path string, value []byte) {
		t.Helper()
		if err := os.WriteFile(path, value, 0600); err != nil {
			t.Fatal(err)
		}
		if err := fs.SecurePrivateFile(path); err != nil {
			t.Fatal(err)
		}
	}
	token := filepath.Join(credentials, "lease.token")
	write(token, []byte(strings.Repeat("a", 64)))
	data, _ := json.Marshal(wire.Discovery{Version: 1, Protocol: wire.Protocol, ProjectID: identity.ProjectID, RepositoryIdentity: ref.Identity, InstanceID: identity.InstanceID, Address: identity.Address, CredentialPath: token, Ready: true, LeaseID: "lease"})
	write(filepath.Join(apiDir, "discovery.json"), data)
	client := httpclient.NewConsole(context.Background(), ref, httpclient.NewPlatformStopObserver())
	defer client.Close()
	ref = client.Project()
	model, err := NewModel(context.Background(), []ProjectRef{ref}, Options{ApplicationHome: home, Clients: map[string]ConsoleClient{ref.Key: client}})
	if err != nil {
		t.Fatal(err)
	}
	defer model.Close()
	// Execute the actual model query command; its result carries the original
	// project/generation and goes through the same update handler as Bubble Tea.
	updated, _ := model.Update(model.readProject(0)())
	model = updated.(Model)
	if queries != 1 || model.projects[0].snapshot.Connection.State != api.ConnectionConnected || model.projects[0].snapshot.Tasks.Tasks[0].TaskID != "task-B" {
		t.Fatal("model did not consume the authenticated HTTP snapshot")
	}
	// The actual TUI final reader consumes HTTP pages and historical body ranges.
	var reader finalReplyReader
	route := finalReplyRoute{ref.Key, ref.Identity}
	_, cmd := reader.refresh(context.Background(), client, time.Now(), route, "candidate", readerFilter(), true, "current-attempt", "current-turn")
	index := cmd().(finalRepliesMsg)
	bodyCmd, changed := reader.acceptReplies(context.Background(), client, route, true, index)
	if index.err != nil || !changed || bodyCmd == nil {
		t.Fatalf("HTTP history: %+v", index)
	}
	bodyMsg := bodyCmd().(bodyMsg)
	foreign := bodyMsg
	foreign.identity = "another-project"
	reader.acceptBody(route.identity, foreign)
	if reader.bodyText != "" {
		t.Fatal("late body crossed project binding")
	}
	reader.acceptBody(route.identity, bodyMsg)
	if reader.bodyError != "" || reader.bodyText != text || cursors != 2 || bodyReads < 2 {
		t.Fatal("HTTP full final pagination failed", reader.bodyError, cursors, bodyReads)
	}
	expired = true
	if msg := reader.read(context.Background(), client, time.Now(), route, true, "", "", false)().(finalRepliesMsg); msg.err == nil {
		t.Fatal("expired HTTP cursor accepted")
	}
	server.Close()
	updated, _ = model.Update(model.readProject(0)())
	model = updated.(Model)
	if model.projects[0].snapshot.Connection.CanControl || model.projects[0].snapshot.Connection.State != api.ConnectionOffline || len(model.projects[0].snapshot.Tasks.Tasks) == 0 {
		t.Fatal("disconnect discarded observation or retained mutation authority")
	}
	id := strings.Repeat("d", 32)
	model.projects[0].actions[id] = &actionState{RequestID: id, Action: "stop_service", TargetInstance: identity.InstanceID}
	model.projects[0].view.Outstanding[id] = "stop_service"
	msg := actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: id, receipt: &api.ControlReceipt{RequestID: id, InstanceID: identity.InstanceID, Action: "stop_service", Status: "applied", Code: "stop_exit_unverified", Message: "结果未核实"}}
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if model.projects[0].actions[id].Resolved || model.projects[0].view.Outstanding[id] == "" {
		t.Fatal("TUI equated applied stop with verified exit")
	}
	msg.receipt.Code = "applied"
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if !model.projects[0].actions[id].Resolved || model.projects[0].view.Outstanding[id] != "" {
		t.Fatal("verified stop did not finish original observation")
	}
}
