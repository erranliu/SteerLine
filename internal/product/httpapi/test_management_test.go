package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
)

type tmQuery struct {
	QueryPort
	api.TestManagementPort
	reads int
}

func (p *tmQuery) PlanTests(_ context.Context, id string) (json.RawMessage, error) {
	p.reads++
	if id != "original-candidate" {
		return nil, &api.Error{Kind: api.UnknownRecord, Message: "candidate not found"}
	}
	return json.RawMessage(`{"status":"ready","total":1,"passed":0,"failed":0,"pending":1,"rows":[]}`), nil
}

func TestTMPlanTestsReadBoundary(t *testing.T) {
	s, _, _, control := newAPIFixture(t)
	p := &tmQuery{QueryPort: s.query}
	s.query = p
	base := "http://" + s.listener.Addr().String() + "/v1/projects/" + ProjectID("C:/repo")
	request := func(method, path string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, base+path, nil)
		r.RemoteAddr = "127.0.0.1:39152"
		if auth {
			r.Header.Set("Authorization", "Bearer "+s.token)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := request(http.MethodGet, "/runs/original-candidate/tests", true); w.Code != 200 || !strings.Contains(w.Body.String(), `"pending":1`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(http.MethodGet, "/runs/missing/tests", true); w.Code != 404 {
		t.Fatal("missing candidate changed meaning", w.Code, w.Body.String())
	}
	before := p.reads
	if w := request(http.MethodGet, "/runs/original-candidate/tests", false); w.Code != 401 {
		t.Fatal("authentication bypass", w.Code)
	}
	if w := request(http.MethodGet, "/runs/original-candidate/tests?arbitrary=1", true); w.Code < 400 {
		t.Fatal("unexpected query accepted", w.Code)
	}
	if p.reads != before || len(control.requests) != 0 {
		t.Fatal("query created work or invalid request reached the port")
	}
	if !httpcontract.AllowedProjectRead("/runs/original-candidate/tests") {
		t.Fatal("test query omitted from browser forwarding")
	}
	if !httpcontract.AllowedProjectRead("/tests/generations/" + strings.Repeat("a", 32)) {
		t.Fatal("maintenance preview read omitted")
	}
	for _, route := range []string{"/tests/generate", "/tests/stop-generation", "/tests/apply"} {
		if httpcontract.AllowedProjectRead(route) {
			t.Fatal("native generation operation exposed in browser", route)
		}
	}
}
