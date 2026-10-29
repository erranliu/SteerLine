package environmentadaptation_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation"
	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestCallerPromptConvergencePreviousFacts(t *testing.T) {
	p := &envmodel.Report{ActivityID: "original", Round: 2, Commit: "fixed", ScriptDigest: "script", Message: "original diagnosis",
		Checks: []envmodel.Check{{ID: "ok", Status: "passed", Message: strings.Repeat("successful detail", 500)}, {ID: "bad", Status: "failed", Message: "完整失败原因\n不可截断"}, {ID: "pending", Status: "incomplete", Message: "缺依赖未执行"}},
		L2:     []envmodel.Item{{ID: "passed", Status: "passed", Message: strings.Repeat("passed detail", 500)}, {ID: "failed", Status: "failed", Message: "独立断言失败"}},
		PR:     envmodel.PR{URL: "https://host/pull/1", Base: "validation", Head: "original", OutcomeUnknown: true, CleanupCompleted: false}}
	r := envmodel.Record{ID: "original", Profile: "remote", Diagnostic: p.Message}
	a := envmodel.Round{Number: 2, Report: p}
	original, _ := json.Marshal(p)
	got := env.PreviousFacts(r, a)
	var view struct {
		Activity string           `json:"activity_id"`
		Round    int              `json:"round"`
		Report   *envmodel.Report `json:"report"`
	}
	if err := json.Unmarshal(got, &view); err != nil {
		t.Fatal(err)
	}
	if view.Activity != r.ID || view.Round != 2 || len(view.Report.Checks) != 3 || len(view.Report.L2) != 2 || view.Report.Checks[1] != p.Checks[1] || view.Report.Checks[2] != p.Checks[2] || view.Report.L2[1] != p.L2[1] || view.Report.PR != p.PR || view.Report.Commit != p.Commit || view.Report.ScriptDigest != p.ScriptDigest {
		t.Fatal("fact projection lost identity/failure/PR facts")
	}
	after, _ := json.Marshal(p)
	if string(after) != string(original) || string(mustReportJSON(t, view.Report)) != string(original) {
		t.Fatal("complete original report was lost or mutated")
	}
	if string(got) != string(env.PreviousFacts(r, a)) {
		t.Fatal("nondeterministic projection")
	}
	nilReport := env.PreviousFacts(envmodel.Record{ID: "no-report", Diagnostic: "failed before report"}, envmodel.Round{Number: 3})
	if !strings.Contains(string(nilReport), "failed before report") || !strings.Contains(string(nilReport), `"report":null`) {
		t.Fatal("missing report hidden")
	}
	t.Logf("UTF-8 previous report: original=%d projected=%d", len(original), len(got))
}

func TestCallerPromptConvergenceStageContextAndFrozenRecovery(t *testing.T) {
	s, remote, _, _ := setup(t)
	r := start(t, s)
	var input runnercontract.RunInput
	if err := json.Unmarshal(r.Rounds[0].Request.Input, &input); err != nil {
		t.Fatal(err)
	}
	initial, _ := base64.StdEncoding.DecodeString(input.ResultFile.InitialContentBase64)
	if !strings.Contains(string(initial), r.ID) || !strings.Contains(string(initial), r.Prepared.ScriptDigest) {
		t.Fatal("phase identity lost")
	}
	for _, step := range []string{"implement.execute", "publish"} {
		if !strings.Contains(input.Workflow.PromptOverrides[step], envmodel.ExecutionRulesPath) || strings.Contains(input.Workflow.PromptOverrides[step], r.Prepared.ScriptDigest) {
			t.Fatal("duplicated identity or missing read instruction")
		}
	}
	original := string(r.Rounds[0].Request.Input)
	// Recreate the service with a different preparation function. Observing an
	// already frozen operation must never render a new request or submit again.
	s.Prepare = func(_ context.Context, _ string) (envmodel.Prepared, error) {
		t.Fatal("reprepared frozen request")
		return envmodel.Prepared{}, nil
	}
	if err := s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	saved := read(t, s, r.ID)
	if string(saved.Rounds[0].Request.Input) != original || remote.submits != 1 {
		t.Fatal("frozen request changed or duplicated")
	}
	setReport(remote, report(r, remote, false))
	observe(t, s)
	observe(t, s)
	next := read(t, s, r.ID)
	if len(next.Rounds) != 2 || next.State != "running" || remote.submits != 1 {
		t.Fatal("next round was not prepared after settlement", next)
	}
	var second runnercontract.RunInput
	if err := json.Unmarshal(next.Rounds[1].Request.Input, &second); err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(second.ResultFile.InitialContentBase64)
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Previous json.RawMessage `json:"previous"`
		Round    int             `json:"round"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if file.Round != 2 || string(file.Previous) != string(env.PreviousFacts(next, next.Rounds[0])) {
		t.Fatalf("complete previous report differs: %s", raw)
	}
	if strings.Contains(second.Workflow.PromptOverrides["implement.execute"], "previous") {
		t.Fatal("previous report leaked into instruction")
	}
}
func mustReportJSON(t *testing.T, r *envmodel.Report) []byte {
	t.Helper()
	b, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestProjectEnvironmentPreviousFileLimitDoesNotTruncateOrSubmit(t *testing.T) {
	s, remote, _, _ := setup(t)
	r := start(t, s)
	observe(t, s)
	p := report(r, remote, false)
	original, _ := json.Marshal(p)
	p.Message = strings.Repeat("x", runnercontract.MaxProgressBytes-len(original)-16)
	body, _ := json.Marshal(p)
	if len(body) >= runnercontract.MaxProgressBytes {
		t.Fatal("fixture report already exceeds original file limit")
	}
	setReport(remote, p)
	observe(t, s)
	observe(t, s)
	got := read(t, s, r.ID)
	if got.State != "failed" || !got.Settled() || len(got.Rounds) != 1 || remote.submits != 1 || !strings.Contains(got.Diagnostic, "上限") || got.Rounds[0].Report.Message != p.Message {
		t.Fatal("oversized next initial file was truncated or submitted", got.State, got.Diagnostic)
	}
}
