package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestCandidateStopHTTPContract(t *testing.T) {
	server, _, reader, control := newAPIFixture(t)
	reader.onResolveStart = func(string, string) { t.Fatal("stop resolved a startup profile") }
	client, err := Open(context.Background(), server.options.RunDir, server.options.ProjectIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	envelope := CommandEnvelope{RequestID: strings.Repeat("e", 32), ProjectID: client.Identity().ProjectID, InstanceID: client.Identity().InstanceID,
		Action: "stop_candidate", TargetID: strings.Repeat("b", 32), ExpectedVersion: 13, ExpectedControlRevision: 7, Payload: json.RawMessage(`{"task_id":"38"}`)}
	response, err := client.SubmitCommand(context.Background(), envelope)
	if err != nil || response.ControlRequest == nil {
		t.Fatal("stop submission failed", err, response)
	}
	req := control.requests[envelope.RequestID]
	var payload api.ControlPayload
	if err = json.Unmarshal(req.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload != (api.ControlPayload{TaskID: "38", RunID: envelope.TargetID, ExpectedRunRevision: 13}) {
		t.Fatal("stop mapping lost candidate identity", payload)
	}
	if !matchesControl(envelope, &requestRecord{Control: &api.ControlInput{RequestID: req.RequestID, InstanceID: req.InstanceID, Action: api.StopCandidate, Payload: payload, RequestedBy: req.RequestedBy, ExpectedRevision: &req.ExpectedControlRevision}}, req) {
		t.Fatal("mapped stop cannot be reconciled")
	}
	_, err = client.SubmitCommand(context.Background(), envelope)
	if err != nil || control.submits != 1 {
		t.Fatal("replay submitted a second stop", err, control.submits)
	}
	changed := envelope
	changed.TargetID = strings.Repeat("c", 32)
	if _, err = client.SubmitCommand(context.Background(), changed); err == nil {
		t.Fatal("original request retargeted")
	}
	for _, raw := range []string{`{}`, `{"task_id":"38","reason":"unused"}`, `{"task_id":"38","expected_task_version":2}`} {
		bad := envelope
		bad.RequestID = strings.Repeat("f", 32)
		bad.Payload = json.RawMessage(raw)
		if _, err = client.SubmitCommand(context.Background(), bad); err == nil {
			t.Fatal("unsupported stop payload accepted", raw)
		}
	}
	control.receipts[req.RequestID] = &api.ControlReceipt{RequestID: req.RequestID, InstanceID: req.InstanceID, Action: req.Action, Status: "applied", RunID: envelope.TargetID}
	response, err = client.Command(context.Background(), req.RequestID)
	if err != nil || response.Status != "applied" || response.TargetID != envelope.TargetID {
		t.Fatal("fixed stop receipt not recoverable", err, response)
	}
}
