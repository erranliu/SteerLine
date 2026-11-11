package applicationclient

import (
	"context"
	"github.com/erranliu/SteerLine/internal/localservice"
	api "github.com/erranliu/SteerLine/internal/product/api"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestWebSwitchReconnectAndHome(t *testing.T) {
	if !valid("/projects/"+"a"+"/runs/run/activity?cursor=%7B%22seq%22%3A1%7D") || valid("/projects/a%2Fb/snapshot") {
		t.Fatal("query encoding changed canonical path rules")
	}
	home := t.TempDir()
	dir := filepath.Join(home, "application-api")
	start := func() (*localservice.Endpoint, *http.Server) {
		e, err := localservice.Prepare(home, dir, contract.Protocol, "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		s := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !e.Authorized(r) {
				localservice.Error(w, 403, "unauthorized", "denied")
				return
			}
			if r.URL.Path == "/v2/application/identity" {
				localservice.JSON(w, 200, e.Identity)
			} else {
				localservice.JSON(w, 200, map[string]any{"projects": []any{}})
			}
		})}
		go s.Serve(e.Listener)
		return e, s
	}
	endpoint, server := start()
	client, err := Open(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	original := client.Identity()
	server.Close()
	endpoint.Close()
	endpoint, server = start()
	defer server.Close()
	defer endpoint.Close()
	id, err := client.RefreshIdentity(context.Background())
	if err != nil || id.HomeID != original.HomeID || id.InstanceID == original.InstanceID {
		t.Fatal(id, err)
	}
	if _, err = localservice.Open(context.Background(), t.TempDir(), dir, contract.Protocol, "/v2/application/identity"); err == nil {
		t.Fatal("different home accepted")
	}
	if err = client.Post(context.Background(), "/shutdown", nil, nil); err == nil {
		t.Fatal("application shutdown exposed via forwarding client")
	}
}

func TestProjectWaitCommandReturnsConfirmed(t *testing.T) {
	home := t.TempDir()
	e, err := localservice.Prepare(home, filepath.Join(home, "application-api"), contract.Protocol, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !e.Authorized(r) {
			localservice.Error(w, 403, "unauthorized", "denied")
			return
		}
		switch r.URL.Path {
		case "/v2/application/identity":
			localservice.JSON(w, 200, e.Identity)
		case "/v2/application/projects/project/commands/request":
			localservice.JSON(w, 200, map[string]any{"command": api.CommandReceipt{CommandID: "request", Status: "confirmed"}})
		default:
			http.NotFound(w, r)
		}
	})}
	t.Cleanup(func() { _ = server.Close() })
	go server.Serve(e.Listener)
	p := &Project{client: &Client{Home: home}, identity: httpcontract.Identity{ProjectID: "project"}}
	receipt, err := p.WaitCommand(context.Background(), "request", time.Second)
	if err != nil || receipt.CommandID != "request" || receipt.Status != "confirmed" {
		t.Fatalf("confirmed recovery did not settle the original wait: %+v, %v", receipt, err)
	}
}
