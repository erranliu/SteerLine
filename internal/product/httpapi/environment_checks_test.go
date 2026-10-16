package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type checkPortFixture struct {
	records                      map[string]api.EnvironmentCheckRecord
	submissions, reads, catalogs int
}

func (p *checkPortFixture) EnvironmentCheckCatalog(context.Context) (api.EnvironmentCheckCatalog, error) {
	p.catalogs++
	return json.RawMessage(`{"unavailable_reason":"fixture: mechanism unsupported"}`), nil
}
func (p *checkPortFixture) EnvironmentChecks(context.Context) ([]api.EnvironmentCheckRecord, error) {
	p.reads++
	rows := []api.EnvironmentCheckRecord{}
	for _, r := range p.records {
		rows = append(rows, r)
	}
	return rows, nil
}
func (p *checkPortFixture) EnvironmentCheck(_ context.Context, id string) (api.EnvironmentCheckRecord, error) {
	p.reads++
	if r, ok := p.records[id]; ok {
		return r, nil
	}
	return api.EnvironmentCheckRecord{}, api.ErrEnvironmentCheckNotFound
}
func (p *checkPortFixture) EnvironmentCheckRequest(ctx context.Context, id string) (api.EnvironmentCheckRecord, error) {
	return p.EnvironmentCheck(ctx, id)
}
func (p *checkPortFixture) SubmitEnvironmentCheck(_ context.Context, q api.EnvironmentCheckCommand) (api.EnvironmentCheckRecord, error) {
	p.submissions++
	r := api.EnvironmentCheckRecord{ID: q.RequestID, Project: "C:/repo", Operations: []api.EnvironmentCheckOperation{{RequestID: q.RequestID, Action: q.Action, Original: q.Original, Status: "outcome_unknown"}}}
	p.records[q.RequestID] = r
	return r, nil
}

func TestEnvironmentCheckHTTPBoundary(t *testing.T) {
	server, _, _, control := newAPIFixture(t)
	p := &checkPortFixture{records: map[string]api.EnvironmentCheckRecord{}}
	server.environmentChecks = p
	server.commands.environmentChecks = p
	base := "http://" + server.listener.Addr().String() + "/v1/projects/" + ProjectID("C:/repo")
	request := func(method, path string, body []byte, auth bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, base+path, bytes.NewReader(body))
		req.Host = server.listener.Addr().String()
		req.RemoteAddr = "127.0.0.1:39152"
		if auth {
			req.Header.Set("Authorization", "Bearer "+server.token)
		}
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		server.ServeHTTP(out, req)
		return out
	}
	for _, path := range []string{"/environment-checks", "/environment-checks/options"} {
		res := request(http.MethodGet, path, nil, true)
		if res.Code != 200 {
			t.Fatal(res.Code, res.Body.String())
		}
	}
	if p.submissions != 0 || len(control.requests) != 0 {
		t.Fatal("GET created work")
	}
	if res := request(http.MethodGet, "/environment-checks", nil, false); res.Code != 401 {
		t.Fatal("authentication bypass", res.Code)
	}
	e := CommandEnvelope{RequestID: strings.Repeat("a", 32), ProjectID: ProjectID("C:/repo"), InstanceID: "instance-a", Action: api.StartEnvironmentCheck, TargetID: ProjectID("C:/repo"), Payload: json.RawMessage(`{"profile_id":"fixture","access_mode":"sandbox"}`)}
	submit := func(e CommandEnvelope) *httptest.ResponseRecorder {
		body, _ := json.Marshal(e)
		return request(http.MethodPost, "/commands", body, true)
	}
	for _, kind := range []string{"project", "instance", "mode", "version"} {
		bad := e
		switch kind {
		case "project":
			bad.ProjectID = "wrong"
		case "instance":
			bad.InstanceID = "old"
		case "mode":
			bad.Payload = json.RawMessage(`{"profile_id":"fixture","access_mode":"auto_review"}`)
		case "version":
			bad.ExpectedVersion = 1
		}
		if res := submit(bad); res.Code < 400 {
			t.Fatal("invalid check accepted", kind, res.Body.String())
		}
	}
	if p.submissions != 0 {
		t.Fatal("invalid request dispatched")
	}
	if res := submit(e); res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	if res := submit(e); res.Code != 200 || p.submissions != 1 {
		t.Fatal("unknown request replayed", res.Code, p.submissions, res.Body.String())
	}
	changed := e
	changed.Payload = json.RawMessage(`{"profile_id":"fixture","access_mode":"full_access"}`)
	if res := submit(changed); res.Code != 409 {
		t.Fatal("same ID altered", res.Code)
	}
	res := request(http.MethodGet, "/commands/"+e.RequestID, nil, true)
	var result CommandResponse
	_ = json.Unmarshal(res.Body.Bytes(), &result)
	if res.Code != 200 || result.Action != e.Action || result.Status != "outcome_unknown" || result.Original == nil {
		t.Fatal(res.Code, res.Body.String())
	}
	// Domain association survives transport record loss; lookup still uses original intent.
	fresh := newRegistryTestOwner(newRegistryTestAuthority(), &registryTestCommands{}, newRegistryTestRecords())
	fresh.environmentChecks = p
	got, err := fresh.Read(context.Background(), e.RequestID)
	if err != nil || got.Action != e.Action || got.Original == nil {
		t.Fatal(got, err)
	}
	got, err = fresh.Submit(context.Background(), e, "tester")
	if err != nil || got.Status != "outcome_unknown" || p.submissions != 1 {
		t.Fatal("same original request did not recover after transport record loss", got, err)
	}
	normal := e
	normal.Action = "pause_dispatch"
	normal.TargetID = "dispatch"
	normal.ExpectedControlRevision = 7
	normal.Payload = json.RawMessage(`{}`)
	if _, err = fresh.Submit(context.Background(), normal, "tester"); err == nil {
		t.Fatal("check ID reused by task/control")
	}
	if p.submissions != 1 || len(control.requests) != 0 {
		t.Fatal("check created business mutation")
	}
	_ = server.SetReady("instance-a", false, "offline")
	if res = request(http.MethodGet, "/environment-checks", nil, true); res.Code != 200 {
		t.Fatal("unready history blocked", res.Code, res.Body.String())
	}
}

func TestEnvironmentCheckStopEnvelope(t *testing.T) {
	e := registryTestEnvelope(strings.Repeat("b", 32))
	e.Action = api.StopEnvironmentCheck
	e.TargetID = strings.Repeat("a", 32)
	e.ExpectedControlRevision = 0
	e.ExpectedVersion = 3
	if err := validateEnvironmentEnvelope(e); err != nil {
		t.Fatal(err)
	}
	e.ExpectedVersion = 0
	if err := validateEnvironmentEnvelope(e); err == nil {
		t.Fatal("unbound stop accepted")
	}
	e.ExpectedVersion = 3
	e.Payload = json.RawMessage(`{"cleanup":true}`)
	if err := validateEnvironmentEnvelope(e); err == nil {
		t.Fatal("invented cleanup interface accepted")
	}
	e.Payload = json.RawMessage(`null`)
	if err := validateEnvironmentEnvelope(e); err == nil {
		t.Fatal("null stop payload accepted")
	}
}
