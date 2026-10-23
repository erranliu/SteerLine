package httpapi

import (
	"context"
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type gitHistoryQuery struct {
	QueryPort
	reads int
}

func (p *gitHistoryQuery) TestCatalogue(context.Context) (json.RawMessage, error) {
	p.reads++
	return json.RawMessage(`{"tests":[]}`), nil
}
func (p *gitHistoryQuery) TestRecords(context.Context, api.TestRecordFilter) (json.RawMessage, error) {
	p.reads++
	return json.RawMessage(`{"records":[],"commit":"fixed-main"}`), nil
}
func (p *gitHistoryQuery) TestRecord(context.Context, string) (json.RawMessage, error) {
	p.reads++
	return json.RawMessage(`{"status":"failed"}`), nil
}

func TestTRQueryHTTPReadOnlyBoundary(t *testing.T) {
	s, _, _, control := newAPIFixture(t)
	p := &gitHistoryQuery{QueryPort: s.query}
	s.query = p
	base := "http://" + s.listener.Addr().String() + "/v1/projects/" + ProjectID("C:/repo")
	request := func(path string, auth bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, base+path, nil)
		r.RemoteAddr = "127.0.0.1:39152"
		if auth {
			r.Header.Set("Authorization", "Bearer "+s.token)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := request("/tests/records?level=L1&outcome=failed", true); w.Code != 200 || !strings.Contains(w.Body.String(), "fixed-main") {
		t.Fatal(w.Code, w.Body.String())
	}
	before := p.reads
	for _, path := range []string{"/tests/records?retest=passed", "/tests/records?level=L1&level=L2", "/tests/records?limit=0", "/tests/records/" + strings.Repeat("a", 32) + "/evidence"} {
		if w := request(path, true); w.Code < 400 {
			t.Fatal("removed/invalid path accepted", path, w.Code)
		}
	}
	if w := request("/tests/records", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if p.reads != before || len(control.requests) != 0 {
		t.Fatal("read query created work")
	}
	if httpcontract.AllowedProjectRead("/tests/records/" + strings.Repeat("a", 32) + "/evidence") {
		t.Fatal("removed evidence route forwarded")
	}
}
