package model_test

import (
	"encoding/base64"
	"encoding/json"
	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"testing"
)

func TestEnvironmentRoundFileRetainsVerifiedInputMetadata(t *testing.T) {
	previous := json.RawMessage(`{"profile":"remote","report":{"message":"完整原失败","checks":[{"id":"git","status":"passed","message":"original successful check"}]}}`)
	report := envmodel.Report{Version: 1, ActivityID: "activity", Round: 2, Commit: "fixed", ScriptDigest: "script", Completed: true, Checks: []envmodel.Check{{ID: "tool", Status: "failed", Message: "current failure"}}, L2: []envmodel.Item{{ID: "one", Status: "incomplete", Message: "requirements failed"}}}
	body, _ := json.Marshal(struct {
		envmodel.Report
		Previous json.RawMessage `json:"previous"`
	}{report, previous})
	input, _ := json.Marshal(runnercontract.RunInput{ResultFile: &runnercontract.ProgressFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString(body)}})
	record := envmodel.Record{ID: "activity", Prepared: envmodel.Prepared{Commit: "fixed", ScriptDigest: "script", L2: []string{"one"}}}
	round := envmodel.Round{Number: 2, Request: runnercontract.Request{Input: input}}
	if got, err := envmodel.ParseReport(body, record, round); err != nil || got.Message != report.Message {
		t.Fatalf("original script retains input metadata: %+v / %v", got, err)
	}
	var object map[string]json.RawMessage
	_ = json.Unmarshal(body, &object)
	for _, change := range []string{"previous", "unexpected"} {
		object[change] = json.RawMessage(`{"report":"tampered"}`)
		altered, _ := json.Marshal(object)
		if _, err := envmodel.ParseReport(altered, record, round); err == nil {
			t.Fatal("changed metadata/unknown report property accepted", change)
		}
		delete(object, change)
	}
}
