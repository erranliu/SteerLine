package server

import (
	"context"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSwitchDevelopmentProxyAndLease(t *testing.T) {
	vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("vite:" + r.URL.Path)) }))
	defer vite.Close()
	home := t.TempDir()
	id, _ := localservice.HomeID(home)
	client := &fakeClient{appIdentity: contract.Identity{Protocol: contract.Protocol, HomeID: id}}
	options := Options{Home: home, Address: freeAddress(t), LauncherDir: LauncherDirectory(home), Client: client, DevOrigin: vite.URL}
	s, err := Prepare(options)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(context.Background())
	if w := request(s, "GET", "/projects/"+strings.Repeat("a", 64)+"/dag", "", nil, ""); w.Code != 200 || !strings.HasPrefix(w.Body.String(), "vite:") {
		t.Fatal(w.Body.String())
	}
	if w := request(s, "GET", "/api/v1/application/projects", "", nil, ""); w.Code != 401 || strings.Contains(w.Body.String(), "vite:") {
		t.Fatal("API reached Vite")
	}
	options.Address = freeAddress(t)
	if other, err := Prepare(options); err == nil {
		other.Close(context.Background())
		t.Fatal("same home acquired second Web lease")
	}
}
