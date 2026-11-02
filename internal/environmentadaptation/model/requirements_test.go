package model

import (
	"encoding/json"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	"strings"
	"testing"
)

func TestProjectDetectionEnvelopeRequiresFieldsAndKeepsOpaqueDetails(t *testing.T) {
	e := runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"full_access"}`)}
	r := Record{ID: ID(), Project: "project", Execution: e, Prepared: Prepared{Commit: strings.Repeat("a", 40), Inputs: strings.Repeat("b", 64)}}
	a := Round{Number: 1}
	p := InitialReport(r, 1)
	p.Completed = true
	p.Ready = false
	p.Message = "service missing"
	p.Details = json.RawMessage(`{"arbitrary":{"ready":true,"checks":"project format"}}`)
	raw, _ := json.Marshal(p)
	got, err := ParseReport(raw, r, a)
	if err != nil || got.Ready || string(got.Details) != string(canonicalDetails(p.Details)) {
		t.Fatal(got, err)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for _, k := range []string{"ready", "completed", "project_identity", "inputs_digest", "call_context", "message"} {
		var bad map[string]json.RawMessage
		_ = json.Unmarshal(raw, &bad)
		delete(bad, k)
		b, _ := json.Marshal(bad)
		if _, err := ParseReport(b, r, a); err == nil {
			t.Fatal("missing field accepted", k)
		}
	}
	for _, v := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`"false"`), json.RawMessage(`1`)} {
		fields["ready"] = v
		b, _ := json.Marshal(fields)
		if _, err := ParseReport(b, r, a); err == nil {
			t.Fatal("invalid ready accepted", string(v))
		}
	}
}

func canonicalDetails(b []byte) []byte { c, _ := runnercontract.CanonicalJSON(b); return c }
