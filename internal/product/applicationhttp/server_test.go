package applicationhttp

import (
	"context"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type offlineDirectory struct{}

func (offlineDirectory) Catalog(context.Context) (contract.Catalog, error) {
	return contract.Catalog{Projects: []contract.Project{}}, nil
}
func (offlineDirectory) Connect(context.Context, string) (*httpclient.Client, error) {
	return nil, &httpclient.ClientError{Status: 503, Code: "project_offline", Reason: "not connected"}
}
func TestWebSwitchAuthenticationRoutesAndUnavailable(t *testing.T) {
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
		if request("GET", "/v1/application/identity", alter).Code != 403 {
			t.Fatal("native authentication bypass")
		}
	}
	for _, path := range []string{"/v1/identity", "/v1/application/projects/" + strings.Repeat("a", 64) + "/arbitrary", "/v1/application/projects/" + strings.Repeat("a", 64) + "/commands/abc"} {
		if w := request("GET", path, nil); w.Code != 404 {
			t.Fatalf("%s => %d", path, w.Code)
		}
	}
	w := request("GET", "/v1/application/projects/"+strings.Repeat("a", 64)+"/commands/"+strings.Repeat("b", 32), nil)
	if w.Code != 503 || strings.Contains(w.Body.String(), "command_not_found") {
		t.Fatal("unavailable became authoritative absence", w.Body.String())
	}
	if w := request("POST", "/v1/application/projects", nil); w.Code < 400 {
		t.Fatal("catalog became writable")
	}
}
