package applicationhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
)

type offlineDirectory struct{}

func (offlineDirectory) Catalog(context.Context) (contract.Catalog, error) {
	return contract.Catalog{Projects: []contract.Project{}}, nil
}

func TestLegacyCheckRetirementReadRoutes(t *testing.T) {
	for _, path := range []string{"environment-checks", "environment-checks/options", "environment-checks/" + strings.Repeat("a", 32)} {
		if httpcontract.AllowedProjectRead("/" + path) {
			t.Fatal("retired check route still forwarded", path)
		}
	}
	for _, path := range []string{"environment-checks/start", "environment-checks/stop", "environment-checks/cleanup", "environment-checks/../tasks"} {
		if httpcontract.AllowedProjectRead("/" + path) {
			t.Fatal("invented check mutation route", path)
		}
	}
}
func (offlineDirectory) Connect(context.Context, string) (ProjectPort, error) {
	return nil, &httpclient.ClientError{Status: 503, Code: "project_offline", Reason: "not connected"}
}
func TestUnifiedStartupAuthenticationRoutesAndUnavailable(t *testing.T) {
	home := t.TempDir()
	endpoint, err := localservice.Prepare(home, filepath.Join(home, "application-api"), contract.Protocol, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.Close()
	s := &Server{Endpoint: endpoint, Directory: offlineDirectory{}, Shutdown: func() {}}
	request := func(method, path string, alter func(*http.Request)) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://"+endpoint.Identity.Address+path, nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Authorization", "Bearer "+endpoint.Token)
		if alter != nil {
			alter(r)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, alter := range []func(*http.Request){func(r *http.Request) { r.Host = "evil" }, func(r *http.Request) { r.Header.Set("Origin", "http://"+endpoint.Identity.Address) }, func(r *http.Request) { r.Header.Del("Authorization") }, func(r *http.Request) { r.RemoteAddr = "192.168.1.2:1234" }} {
		if request("GET", "/v2/application/identity", alter).Code != 403 {
			t.Fatal("native authentication bypass")
		}
	}
	for _, path := range []string{"/v1/identity", "/v2/application/projects/" + strings.Repeat("a", 64) + "/arbitrary", "/v2/application/projects/" + strings.Repeat("a", 64) + "/commands/abc"} {
		if w := request("GET", path, nil); w.Code != 404 {
			t.Fatalf("%s => %d", path, w.Code)
		}
	}
	w := request("GET", "/v2/application/projects/"+strings.Repeat("a", 64)+"/commands/"+strings.Repeat("b", 32), nil)
	if w.Code != 503 || strings.Contains(w.Body.String(), "command_not_found") {
		t.Fatal("unavailable became authoritative absence", w.Body.String())
	}
	if w := request("GET", "/v2/application/projects/"+strings.Repeat("a", 64)+"/runs/candidate/last-message?execution_id=run&attempt_id=attempt", nil); w.Code != 503 {
		t.Fatal("last-message route was not forwarded", w.Code, w.Body.String())
	}
	if w := request("POST", "/v2/application/projects", nil); w.Code < 400 {
		t.Fatal("catalog became writable")
	}
}

func TestPMReadRoutesKeepMaintenanceWritesOnCommands(t *testing.T) {
	for _, route := range []string{"/maintenance", "/maintenance/tasks/1", "/tests/generations/" + strings.Repeat("a", 32)} {
		if !httpcontract.AllowedProjectRead(route) {
			t.Fatal("missing read", route)
		}
	}
	for _, route := range []string{"/maintenance/apply", "/maintenance/generate/extra", "/tests/apply", "/maintenance/tasks/../1"} {
		if httpcontract.AllowedProjectRead(route) {
			t.Fatal("mutation or traversal read", route)
		}
	}
}
