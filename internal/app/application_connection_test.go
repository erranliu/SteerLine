package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/localservice"
	api "github.com/erranliu/SteerLine/internal/product/api"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	fs "github.com/erranliu/SteerLine/internal/product/httpfs"
	"github.com/erranliu/SteerLine/internal/projectconnection"
)

func TestWebSwitchNativeConnectionLeaseAndRestart(t *testing.T) {
	home := t.TempDir()
	repo := switchRepo(t, home, "repo")
	switchConfig(t, home, "one", repo)
	ref := api.ProjectRef{ConfigPath: filepath.Join(home, "etc", "one.json"), ApplicationHome: home}
	if err := projectconnection.Resolve(context.Background(), &ref); err != nil {
		t.Fatal(err)
	}
	process, err := httpclient.CurrentProcessIdentity()
	if err != nil {
		t.Fatal(err)
	}
	var identity atomic.Value
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := identity.Load().(httpcontract.Identity)
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 64) {
			w.WriteHeader(403)
			return
		}
		if r.URL.Path == "/v1/identity" {
			calls.Add(1)
			localservice.JSON(w, 200, id)
			return
		}
		if r.URL.Query().Get("hold") == "1" {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		localservice.JSON(w, 200, map[string]string{"instance": id.InstanceID})
	}))
	defer native.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	id := httpcontract.Identity{Protocol: httpcontract.Protocol, ProjectID: httpcontract.ProjectID(ref.Identity), RepositoryIdentity: ref.Identity, InstanceID: "original", Process: process, Address: strings.TrimPrefix(native.URL, "http://"), Ready: true}
	identity.Store(id)
	dir := filepath.Join(ref.RunDir, "local-api")
	credentials := filepath.Join(dir, "credentials")
	if err = fs.EnsurePrivateDirectory(credentials); err != nil {
		t.Fatal(err)
	}
	token := filepath.Join(credentials, "native.token")
	if err = os.WriteFile(token, []byte(strings.Repeat("a", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	if err = fs.SecurePrivateFile(token); err != nil {
		t.Fatal(err)
	}
	publish := func() {
		t.Helper()
		if err := localservice.Write(dir, "discovery.json", httpcontract.Discovery{Version: 1, Protocol: httpcontract.Protocol, ProjectID: id.ProjectID, RepositoryIdentity: ref.Identity, InstanceID: id.InstanceID, Process: process, Address: id.Address, CredentialPath: token, LeaseID: id.InstanceID, Ready: true}); err != nil {
			t.Fatal(err)
		}
	}
	publish()
	d := NewApplicationDirectory(home)
	defer d.Close()
	switchCatalog(t, d, func(c contract.Catalog) bool { return len(c.Projects) == 1 && c.Projects[0].Online })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	original, err := d.Connect(ctx, id.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	reused, err := d.Connect(ctx, id.ProjectID)
	if err != nil || reused != original || calls.Load() != 1 {
		t.Fatal("verified connection was unnecessarily rediscovered", err, calls.Load())
	}
	done := make(chan error, 1)
	var oldResponse map[string]string
	go func() { done <- original.Get(ctx, "/snapshot?hold=1", &oldResponse) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	id.InstanceID = "replacement"
	identity.Store(id)
	publish()
	current, err := d.Connect(ctx, id.ProjectID)
	if err != nil || current == original || current.Identity().InstanceID != id.InstanceID {
		t.Fatal("native restart did not replace connection", err)
	}
	close(release)
	if err = <-done; err != nil || oldResponse["instance"] != "original" {
		t.Fatal("retiring connection interrupted or changed inflight result", oldResponse, err)
	}
	if calls.Load() != 2 {
		t.Fatal("unexpected identity handshakes", calls.Load())
	}
}
