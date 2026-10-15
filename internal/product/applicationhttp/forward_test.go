package applicationhttp

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	privatefs "github.com/erranliu/SteerLine/internal/product/httpfs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type nativeSwitchFixture struct {
	run, repository  string
	identity         httpcontract.Identity
	started, release chan struct{}
	hold             bool
	last             chan httpcontract.CommandEnvelope
}

func newNativeSwitchFixture(t *testing.T, instance string, hold bool) *nativeSwitchFixture {
	t.Helper()
	f := &nativeSwitchFixture{run: t.TempDir(), repository: "original-repository", hold: hold, started: make(chan struct{}), release: make(chan struct{}), last: make(chan httpcontract.CommandEnvelope, 1)}
	process, err := httpclient.CurrentProcessIdentity()
	if err != nil {
		t.Fatal(err)
	}
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("c", 64) {
			w.WriteHeader(403)
			return
		}
		if r.URL.Path == "/v1/identity" {
			localservice.JSON(w, 200, f.identity)
			return
		}
		if r.Method == "POST" {
			var envelope httpcontract.CommandEnvelope
			if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
				t.Error(err)
			}
			f.last <- envelope
			localservice.JSON(w, 200, httpcontract.CommandResponse{RequestID: envelope.RequestID, ProjectID: envelope.ProjectID, InstanceID: envelope.InstanceID, Action: envelope.Action, TargetID: envelope.TargetID, ExpectedVersion: envelope.ExpectedVersion, Original: &envelope, Status: "accepted"})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/snapshot") {
			if f.hold {
				close(f.started)
				select {
				case <-f.release:
				case <-r.Context().Done():
					return
				}
			}
			localservice.JSON(w, 200, map[string]string{"from_instance": instance})
			return
		}
		localservice.Error(w, 404, "command_not_found", "authority reports no original request")
	}))
	t.Cleanup(native.Close)
	f.identity = httpcontract.Identity{Protocol: httpcontract.Protocol, ProjectID: httpcontract.ProjectID(f.repository), RepositoryIdentity: f.repository, InstanceID: instance, Process: process, Address: strings.TrimPrefix(native.URL, "http://"), Ready: true}
	dir := filepath.Join(f.run, "local-api")
	credentials := filepath.Join(dir, "credentials")
	if err = privatefs.EnsurePrivateDirectory(credentials); err != nil {
		t.Fatal(err)
	}
	token := filepath.Join(credentials, "test.token")
	if err = os.WriteFile(token, []byte(strings.Repeat("c", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	if err = privatefs.SecurePrivateFile(token); err != nil {
		t.Fatal(err)
	}
	if err = localservice.Write(dir, "discovery.json", httpcontract.Discovery{Version: 1, Protocol: httpcontract.Protocol, ProjectID: f.identity.ProjectID, RepositoryIdentity: f.repository, InstanceID: instance, Process: process, Address: f.identity.Address, CredentialPath: token, LeaseID: "lease", Ready: true}); err != nil {
		t.Fatal(err)
	}
	return f
}

type switchingDirectory struct {
	current atomic.Pointer[nativeSwitchFixture]
}

func (d *switchingDirectory) Catalog(context.Context) (contract.Catalog, error) {
	return contract.Catalog{}, nil
}
func (d *switchingDirectory) Connect(ctx context.Context, _ string) (*httpclient.Client, error) {
	f := d.current.Load()
	return httpclient.Open(ctx, f.run, f.repository)
}
func TestWebSwitchOriginalEnvelopeAndInflightReplacement(t *testing.T) {
	old := newNativeSwitchFixture(t, "native-original", true)
	replacement := newNativeSwitchFixture(t, "native-replacement", false)
	d := &switchingDirectory{}
	d.current.Store(old)
	home := t.TempDir()
	endpoint, err := localservice.Prepare(home, filepath.Join(home, "application-api"), contract.Protocol, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.Close()
	server := &http.Server{Handler: &Server{Endpoint: endpoint, Directory: d, Shutdown: func() {}}}
	go server.Serve(endpoint.Listener)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := localservice.Open(ctx, home, endpoint.Dir, contract.Protocol, "/v1/application/identity")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	route := "/v1/application/projects/" + old.identity.ProjectID
	envelope := httpcontract.CommandEnvelope{RequestID: strings.Repeat("d", 32), ProjectID: old.identity.ProjectID, InstanceID: old.identity.InstanceID, Action: "start_task", TargetID: "1", ExpectedVersion: 5, ExpectedGraphRevision: 7, Payload: json.RawMessage(`{"exact_number":9007199254740993}`)}
	var receipt httpcontract.CommandResponse
	if err = client.Request(ctx, "POST", route+"/commands", envelope, &receipt); err != nil {
		t.Fatal(err)
	}
	got := <-old.last
	before, _ := json.Marshal(envelope)
	after, _ := json.Marshal(got)
	if string(before) != string(after) || receipt.Original == nil || receipt.InstanceID != envelope.InstanceID {
		t.Fatal("original envelope/receipt changed")
	}
	done := make(chan error, 1)
	var result map[string]string
	go func() { done <- client.Request(ctx, "GET", route+"/snapshot", nil, &result) }()
	select {
	case <-old.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	d.current.Store(replacement)
	var fresh map[string]string
	if err = client.Request(ctx, "GET", route+"/snapshot", nil, &fresh); err != nil || fresh["from_instance"] != "native-replacement" {
		t.Fatal(fresh, err)
	}
	close(old.release)
	if err = <-done; err != nil || result["from_instance"] != "native-original" {
		t.Fatal("inflight call interrupted or retargeted", result, err)
	}
	if err = client.Request(ctx, "POST", route+"/commands", envelope, &receipt); err == nil || !strings.Contains(err.Error(), "stale_target") {
		t.Fatal("old command rebound", err)
	}
	if err = client.Request(ctx, "GET", route+"/commands/"+envelope.RequestID, nil, &receipt); err == nil || !strings.Contains(err.Error(), "command_not_found") {
		t.Fatal("native authoritative absence changed", err)
	}
}
