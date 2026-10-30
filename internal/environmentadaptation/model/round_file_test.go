package model

import (
	"encoding/base64"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"testing"
)

func TestEnvironmentRoundFileRetainsVerifiedInputMetadata(t *testing.T) {
	previous := json.RawMessage(`{"profile":"remote","report":{"message":"完整原失败","checks":[{"id":"git","status":"passed","message":"original successful check"}]}}`)
	report := Report{Version: ReportVersion, ActivityID: "activity", Round: 2, Commit: "fixed", ScriptDigest: "script", Completed: true, Checks: []Check{{ID: "tool", Status: "failed", Message: "current failure"}}, L2: []Item{{ID: "one", Status: "incomplete", Message: "requirements failed"}}}
	body, _ := json.Marshal(struct {
		Report
		Previous json.RawMessage `json:"previous"`
	}{report, previous})
	input, _ := json.Marshal(runnercontract.RunInput{ResultFile: &runnercontract.ProgressFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString(body)}})
	record := Record{ID: "activity", Prepared: Prepared{Commit: "fixed", ScriptDigest: "script", L2: []string{"one"}}}
	round := Round{Number: 2, Request: runnercontract.Request{Input: input}}
	if got, err := ParseReport(body, record, round); err != nil || got.Message != report.Message {
		t.Fatalf("original script retains input metadata: %+v / %v", got, err)
	}
	var object map[string]json.RawMessage
	_ = json.Unmarshal(body, &object)
	for _, change := range []string{"previous", "unexpected"} {
		object[change] = json.RawMessage(`{"report":"tampered"}`)
		altered, _ := json.Marshal(object)
		if _, err := ParseReport(altered, record, round); err == nil {
			t.Fatal("changed metadata/unknown report property accepted", change)
		}
		delete(object, change)
	}
}
