package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

type trQuery struct {
	QueryPort
	api.TestRecordsPort
	calls       int
	report      tm.Report
	registerErr error
	receiptErr  error
}

func TestTRRegisterUnknownCommitReconcilesWithoutRepeatOrDetail(t *testing.T) {
	s, _, _, _ := newAPIFixture(t)
	p := &trQuery{QueryPort: s.query, registerErr: errors.New("storage commit outcome unknown")}
	s.query = p
	s.commands.query = p
	id := strings.Repeat("a", 32)
	payload, _ := json.Marshal(api.TestRecordRegistration{Report: json.RawMessage(`{"id":"` + id + `"}`)})
	envelope := CommandEnvelope{RequestID: strings.Repeat("b", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: api.RegisterTestRecord, TargetID: id, Payload: payload}
	body, _ := json.Marshal(envelope)
	w := trRequest(s, "POST", "/commands", body, true)
	var result CommandResponse
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != 200 || result.Status != "outcome_unknown" || result.Code != "record_unverified" {
		t.Fatal(w.Code, w.Body.String())
	}
	p.report.ID = id // authority confirms the original commit did land
	w = trRequest(s, "GET", "/commands/"+envelope.RequestID, nil, true)
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != 200 || result.Status != "applied" || strings.Contains(w.Body.String(), `"code"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	p.receiptErr = &api.Error{Kind: api.InvalidInput, Message: "supplied summary mismatch"}
	w = trRequest(s, "GET", "/commands/"+envelope.RequestID, nil, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"rejected"`) {
		t.Fatal("bad original evidence matched an existing record", w.Code, w.Body.String())
	}
	p.receiptErr = &api.Error{Kind: api.IdentityConflict, Message: "immutable content differs"}
	if w = trRequest(s, "GET", "/commands/"+envelope.RequestID, nil, true); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if p.calls != 1 {
		t.Fatal("receipt lookup repeated mutation", p.calls)
	}
}

func (p *trQuery) RegisterTestRecord(_ context.Context, in api.TestRecordRegistration) (json.RawMessage, error) {
	p.calls++
	if p.registerErr != nil {
		return nil, p.registerErr
	}
	if err := json.Unmarshal(in.Report, &p.report); err != nil {
		return nil, err
	}
	return json.RawMessage(`{"id":"` + p.report.ID + `","evidence_status":"available"}`), nil
}
func (p *trQuery) TestRecord(context.Context, string) (json.RawMessage, error) {
	panic("registration must not query the UI detail")
}
func (p *trQuery) TestRecordReceipt(context.Context, api.TestRecordRegistration) (json.RawMessage, error) {
	if p.receiptErr != nil {
		return nil, p.receiptErr
	}
	return json.Marshal(map[string]any{"id": p.report.ID, "status": "applied", "evidence_status": "available"})
}

func (p *trQuery) TestRecords(_ context.Context, f api.TestRecordFilter) (json.RawMessage, error) {
	p.calls++
	if f.TestID != "rules" {
		return nil, &api.Error{Kind: api.InvalidInput, Message: "invalid test"}
	}
	return json.RawMessage(`{"records":[],"summary":{"total":0}}`), nil
}
func trRequest(s *Server, method, path string, body []byte, auth bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+s.listener.Addr().String()+"/v1/projects/"+ProjectID("C:/repo")+path, strings.NewReader(string(body)))
	r.RemoteAddr = "127.0.0.1:39152"
	r.Header.Set("Content-Type", "application/json")
	if auth {
		r.Header.Set("Authorization", "Bearer "+s.token)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestTRRegisterAuthenticatedReplayAndWireBoundary(t *testing.T) {
	s, _, _, control := newAPIFixture(t)
	p := &trQuery{QueryPort: s.query}
	s.query = p
	s.commands.query = p
	// The producer's timestamp and omitted/default fields differ textually from
	// the typed stored projection. A receipt replay must still match.
	raw := json.RawMessage(`{"id":"` + strings.Repeat("a", 32) + `","started_at":"2026-10-21T00:00:00.0000000Z"}`)
	payload, _ := json.Marshal(api.TestRecordRegistration{Report: raw, Summary: []byte("{}")})
	e := CommandEnvelope{RequestID: strings.Repeat("b", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: api.RegisterTestRecord, TargetID: strings.Repeat("a", 32), Payload: payload}
	body, _ := json.Marshal(e)
	if w := trRequest(s, "POST", "/commands", body, false); w.Code != 401 {
		t.Fatal(w.Code, w.Body.String())
	}
	for i := 0; i < 2; i++ {
		if w := trRequest(s, "POST", "/commands", body, true); w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"applied"`) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if p.calls != 1 || len(control.requests) != 0 {
		t.Fatal("replay executed mutation or Runner work", p.calls, control.requests)
	}
	if w := trRequest(s, "GET", "/commands/"+e.RequestID, nil, true); w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"applied"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	e.Payload = json.RawMessage(`{"report":{"id":"` + e.TargetID + `"}}`)
	conflict, _ := json.Marshal(e)
	if w := trRequest(s, "POST", "/commands", conflict, true); w.Code != 409 {
		t.Fatal("request collision accepted", w.Code, w.Body.String())
	}
	e.RequestID = strings.Repeat("c", 32)
	e.ExpectedVersion = 1
	invalid, _ := json.Marshal(e)
	if w := trRequest(s, "POST", "/commands", invalid, true); w.Code != 422 {
		t.Fatal("task version accepted", w.Code, w.Body.String())
	}
	if w := trRequest(s, "POST", "/commands", []byte(strings.Repeat("x", (1<<20)+1)), true); w.Code != 413 {
		t.Fatal("wire budget bypassed", w.Code)
	}
	if p.calls != 1 {
		t.Fatal("rejected input reached mutation")
	}
	if !p.report.StartedAt.Equal(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)) {
		t.Fatal(p.report)
	}
	// Exactly the shared wire limit is legal; one byte over was rejected above.
	e.RequestID = strings.Repeat("d", 32)
	e.ExpectedVersion = 0
	e.Payload = payload
	boundary, _ := json.Marshal(e)
	boundary = append(boundary, []byte(strings.Repeat(" ", (1<<20)-len(boundary)))...)
	if w := trRequest(s, "POST", "/commands", boundary, true); w.Code != 200 {
		t.Fatal("wire limit rejected valid registration", w.Code, w.Body.String())
	}
}
func TestTRQueryReadOnlyFiltersAndAuthentication(t *testing.T) {
	s, _, _, control := newAPIFixture(t)
	p := &trQuery{QueryPort: s.query}
	s.query = p
	if w := trRequest(s, "GET", "/tests/records?test_id=rules", nil, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, path := range []string{"/tests/records?test_id=rules&arbitrary=x", "/tests/records?limit=101", "/tests/records?test_id=rules&test_id=other", "/tests/records/" + strings.Repeat("a", 32) + "?x=y"} {
		if w := trRequest(s, "GET", path, nil, true); w.Code != 422 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	if w := trRequest(s, "GET", "/tests/records?test_id=rules", nil, false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := trRequest(s, "GET", "/tests/records?test_id=invalid", nil, true); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := trRequest(s, "POST", "/tests/records", nil, true); w.Code != 405 {
		t.Fatal(w.Code, w.Body.String())
	}
	if p.calls != 2 || len(control.requests) != 0 {
		t.Fatal("query side effect", p.calls, control.requests)
	}
}
