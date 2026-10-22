package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
)

type usageQuery struct {
	QueryPort
	calls int
}

func (p *usageQuery) RunUsage(_ context.Context, id, execution string) (api.RunUsageView, error) {
	p.calls++
	return api.RunUsageView{RunID: id, ExecutionID: execution, Reason: "USAGE_UNSUPPORTED"}, nil
}
func TestRunUsageHTTPReadBoundary(t *testing.T) {
	s, _, reader, control := newAPIFixture(t)
	reader.snapshot.Connection.Snapshot.Page.Runs = []*api.CandidateSummary{{RunID: "candidate", ExecutionID: "original"}}
	p := &usageQuery{QueryPort: s.query}
	s.query = p
	request := func(method, path string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://"+s.listener.Addr().String()+"/v1/projects/"+ProjectID("C:/repo")+path, nil)
		r.RemoteAddr = "127.0.0.1:39152"
		if auth {
			r.Header.Set("Authorization", "Bearer "+s.token)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/runs/candidate/usage?execution_id=original", true); w.Code != 200 || !strings.Contains(w.Body.String(), "USAGE_UNSUPPORTED") {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, target := range []string{"", "?execution_id=foreign", "?execution_id=original&attempt_id=x", "?execution_id=original&execution_id=original"} {
		if w := request("GET", "/runs/candidate/usage"+target, true); w.Code < 400 {
			t.Fatal("invalid target accepted", target)
		}
	}
	if w := request("GET", "/runs/missing/usage?execution_id=original", true); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := request("GET", "/runs/candidate/usage?execution_id=original", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request("POST", "/runs/candidate/usage?execution_id=original", true); w.Code != 405 {
		t.Fatal(w.Code)
	}
	if p.calls != 1 || len(control.requests) != 0 || !httpcontract.AllowedProjectRead("/runs/candidate/usage") {
		t.Fatal("read created work or forwarding missing", p.calls)
	}
}
