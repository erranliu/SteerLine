package applicationhttp

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type nativeSwitchFixture struct {
	identity         httpcontract.Identity
	started, release chan struct{}
	hold             bool
	last             chan httpcontract.CommandEnvelope
}

func newNativeSwitchFixture(t *testing.T, instance string, hold bool) *nativeSwitchFixture {
	return &nativeSwitchFixture{identity: httpcontract.Identity{Protocol: httpcontract.Protocol, ProjectID: httpcontract.ProjectID("original-repository"), RepositoryIdentity: "original-repository", InstanceID: instance, Ready: true}, started: make(chan struct{}), release: make(chan struct{}), hold: hold, last: make(chan httpcontract.CommandEnvelope, 1)}
}
func (f *nativeSwitchFixture) Identity(context.Context) httpcontract.Identity { return f.identity }
func (f *nativeSwitchFixture) Read(ctx context.Context, route string, _ url.Values) (any, error) {
	if route == "/identity" {
		return f.identity, nil
	}
	if route == "/snapshot" {
		if f.hold {
			close(f.started)
			select {
			case <-f.release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return map[string]string{"from_instance": f.identity.InstanceID}, nil
	}
	return nil, &httpclient.ClientError{Status: 404, Code: "command_not_found", Reason: "authority reports no original request"}
}
func (f *nativeSwitchFixture) Command(ctx context.Context, e httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	f.last <- e
	return httpcontract.CommandResponse{RequestID: e.RequestID, ProjectID: e.ProjectID, InstanceID: e.InstanceID, Action: e.Action, TargetID: e.TargetID, Original: &e, Status: "accepted"}, nil
}
func (f *nativeSwitchFixture) Task(context.Context, string, string, json.RawMessage) (any, error) {
	return nil, &httpclient.ClientError{Status: 501, Code: "unsupported"}
}

type switchingDirectory struct {
	current atomic.Pointer[nativeSwitchFixture]
}

func (d *switchingDirectory) Catalog(context.Context) (contract.Catalog, error) {
	return contract.Catalog{}, nil
}
func (d *switchingDirectory) Connect(context.Context, string) (ProjectPort, error) {
	return d.current.Load(), nil
}
func TestUnifiedStartupOriginalEnvelopeAndInflightIsolation(t *testing.T) {
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
	client, err := localservice.Open(ctx, home, endpoint.Dir, contract.Protocol, "/v2/application/identity")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	route := "/v2/application/projects/" + old.identity.ProjectID
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
