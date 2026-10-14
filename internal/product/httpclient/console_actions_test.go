package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
)

func testConsoleActions(t *testing.T) {
	identity := wire.Identity{ProjectID: wire.ProjectID("repo"), InstanceID: "original"}
	var seen []wire.CommandEnvelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(wire.ErrorResponse{Code: "command_not_found"})
			return
		}
		var e wire.CommandEnvelope
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			t.Error(err)
			return
		}
		seen = append(seen, e)
		response := wire.CommandResponse{RequestID: e.RequestID, ProjectID: e.ProjectID, InstanceID: e.InstanceID, Action: e.Action, TargetID: e.TargetID, ExpectedVersion: e.ExpectedVersion, Original: &e, Status: "accepted"}
		if e.Action == "answer_decision" {
			response.Command = &api.CommandReceipt{CommandID: e.RequestID, Kind: e.Action, TargetID: e.TargetID, ExpectedVersion: e.ExpectedVersion, Status: "accepted"}
		} else {
			response.ControlRequest = &api.ControlRequest{RequestID: e.RequestID, InstanceID: e.InstanceID, Action: e.Action}
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	c := &Console{ref: api.ProjectRef{Identity: "repo", ApplicationHome: t.TempDir()}, client: &Client{baseURL: server.URL, project: identity.ProjectID, identity: identity, http: server.Client()}}
	revision := uint64(21)
	payload := api.ControlPayload{TaskID: "task", ExpectedTaskVersion: 7, ExpectedGraphRevision: 11, Mode: "auto", ExecutionProfileID: "profile", Environment: &api.HostActionTarget{RunID: "run", ExecutionID: "execution", AttemptID: "attempt", TurnID: "turn", BlockerID: "blocker", Version: 17}}
	cases := []struct {
		action  api.ControlAction
		target  string
		version uint64
	}{
		{api.StartTask, "task", 7}, {api.StartCandidate, "task", 7}, {api.SetDispatchMode, "dispatch", 21}, {api.PauseDispatch, "dispatch", 21}, {api.ResumeDispatch, "dispatch", 21}, {api.OpenTerminal, "run", 17}, {api.RecheckEnvironment, "run", 17},
	}
	for i, test := range cases {
		id := fmt.Sprintf("%032x", i+1)
		_, err := c.SubmitControl(context.Background(), api.ControlInput{Action: test.action, InstanceID: "original", RequestID: id, ExpectedRevision: &revision, Payload: payload})
		if err != nil {
			t.Fatal(test.action, err)
		}
		e := seen[len(seen)-1]
		if e.RequestID != id || e.InstanceID != "original" || e.TargetID != test.target || e.ExpectedVersion != test.version || e.ExpectedControlRevision != revision {
			t.Fatalf("changed %s: %+v", test.action, e)
		}
		var p map[string]json.RawMessage
		json.Unmarshal(e.Payload, &p)
		if test.action == api.StartCandidate && (e.ExpectedGraphRevision != 11 || string(p["execution_profile_id"]) != `"profile"`) {
			t.Fatal("candidate intent changed")
		}
		if test.action == api.OpenTerminal || test.action == api.RecheckEnvironment {
			var target api.HostActionTarget
			json.Unmarshal(p["target"], &target)
			if target != *payload.Environment {
				t.Fatal("environment target changed")
			}
		}
	}
	answer := api.DecisionAnswer{RunID: "run", DecisionID: "decision", OptionID: "option", Note: "原备注\n第二行", Answers: map[string][]string{"question": {"原回复"}}, ExpectedDecisionVersion: 19, ExpectedRunRevision: 23, SourceAttemptID: "attempt", ExpectedRecoveryVersion: 29, ServiceInstanceID: "original"}
	_, _, err := c.SubmitDecision(context.Background(), api.DecisionInput{CommandID: strings.Repeat("b", 32), Answer: answer})
	if err != nil {
		t.Fatal(err)
	}
	e := seen[len(seen)-1]
	var p struct {
		Run      string              `json:"run_id"`
		Note     string              `json:"note"`
		Revision uint64              `json:"expected_run_revision"`
		Attempt  string              `json:"source_attempt_id"`
		Recovery uint64              `json:"expected_recovery_version"`
		Answers  map[string][]string `json:"answers"`
	}
	json.Unmarshal(e.Payload, &p)
	if e.Action != "answer_decision" || e.TargetID != "decision" || e.ExpectedVersion != 19 || p.Run != "run" || p.Note != answer.Note || p.Revision != 23 || p.Attempt != "attempt" || p.Recovery != 29 || len(p.Answers["question"]) != 1 || p.Answers["question"][0] != "原回复" {
		t.Fatalf("feedback target/content changed: %+v", e)
	}
}

func testStopUnknown(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			connection, _, _ := w.(http.Hijacker).Hijack()
			connection.Close()
			return
		}
		// The authority cannot confirm an outcome. Never fabricate not-found.
		w.WriteHeader(503)
		json.NewEncoder(w).Encode(wire.ErrorResponse{Code: "outcome_unknown"})
	}))
	defer server.Close()
	identity := wire.Identity{ProjectID: wire.ProjectID("repo"), InstanceID: "original"}
	c := &Console{ref: api.ProjectRef{Identity: "repo", ApplicationHome: t.TempDir()}, client: &Client{baseURL: server.URL, project: identity.ProjectID, identity: identity, http: server.Client()}}
	// Persist the original request first, then simulate the lost POST independently
	// of StopService's initial existence query.
	id := strings.Repeat("c", 32)
	e := wire.CommandEnvelope{RequestID: id, ProjectID: identity.ProjectID, InstanceID: identity.InstanceID, Action: "stop_service", TargetID: identity.InstanceID, Payload: json.RawMessage(`{}`)}
	if err := c.saveOutgoing(outgoing{Version: 1, Envelope: e}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.client.SubmitCommand(context.Background(), e); err == nil {
		t.Fatal("lost receipt reported success")
	}
	if _, err := c.Receipt(context.Background(), id); err == nil {
		t.Fatal("unknown stop manufactured an applied receipt")
	}
	result, err := c.StopService(context.Background(), "test", 20*time.Millisecond)
	if err != nil || result.Status != "pending" || result.Receipt != nil || result.ProcessExited || result.LocksReleased || posts != 1 {
		t.Fatalf("unknown stop/replay: %+v %v posts=%d", result, err, posts)
	}
}
