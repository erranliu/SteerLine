package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
	privatefs "github.com/erranliu/SteerLine/internal/product/httpfs"
)

func TestBoundedClientBudgetPreservesAuthenticationAndCancellation(t *testing.T) {
	for _, budget := range []time.Duration{0, -1, 2*time.Minute + 1} {
		if _, err := OpenWithTimeout(context.Background(), "", "", budget); err == nil {
			t.Fatal("unbounded client budget accepted")
		}
	}
	identity := wire.Identity{Protocol: wire.Protocol, ProjectID: wire.ProjectID("repo"), RepositoryIdentity: "repo", InstanceID: "original", Ready: true}
	token := strings.Repeat("a", 64)
	entered := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token || r.Header.Get("Origin") != "" {
			t.Error("custom budget changed native authentication")
		}
		if r.URL.Path == "/v1/identity" {
			json.NewEncoder(w).Encode(identity)
			return
		}
		entered <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	identity.Address = strings.TrimPrefix(server.URL, "http://")
	runDir := t.TempDir()
	dir := filepath.Join(runDir, "local-api")
	credentialDir := filepath.Join(dir, "credentials")
	for _, path := range []string{dir, credentialDir} {
		if err := privatefs.EnsurePrivateDirectory(path); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path string, data []byte) {
		t.Helper()
		f, err := privatefs.CreatePrivateTemp(filepath.Dir(path), ".test-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(f.Name())
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if err := privatefs.ReplacePrivateFile(f.Name(), path); err != nil {
			t.Fatal(err)
		}
	}
	credential := filepath.Join(credentialDir, "original")
	write(credential, []byte(token))
	data, _ := json.Marshal(wire.Discovery{Version: 1, Protocol: wire.Protocol, ProjectID: identity.ProjectID, RepositoryIdentity: "repo", InstanceID: "original", Address: identity.Address, CredentialPath: credential, Ready: true, LeaseID: "lease"})
	write(filepath.Join(dir, "discovery.json"), data)
	client, err := OpenWithTimeout(context.Background(), runDir, "repo", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		var result any
		done <- client.Post(ctx, "/commands", map[string]string{"request_id": "original"}, &result)
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request never reached the authenticated listener")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("caller cancellation ignored")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client waited for its full budget after cancellation")
	}
	ordinary, err := Open(context.Background(), runDir, "repo")
	if err != nil {
		t.Fatal(err)
	}
	defer ordinary.Close()
	if ordinary.http.Timeout != 30*time.Second || client.http.Timeout != 2*time.Minute {
		t.Fatal("caller budget changed native defaults")
	}
}
