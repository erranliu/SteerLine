package sqlite

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"path/filepath"
	"strings"
	"testing"
)

func TestTMReservationBindsArrangementWithoutTestReferences(t *testing.T) {
	s, project, tasks := nativeAuditStore(t)
	ctx := context.Background()
	_, e := s.db.ExecContext(ctx, `UPDATE project_task_authorities SET dispatch_paused=0 WHERE project_identity=?`, project)
	require(t, e)
	graph, e := s.TaskGraphSnapshot(ctx, project)
	require(t, e)
	contract, e := s.TaskContract(ctx, project, tasks[0])
	require(t, e)
	work := taskmodel.FixedWork{Record: tasks[0], Contract: contract, Authority: graph.TaskGraph().Authority}
	snapshot, _ := json.Marshal(map[string]any{"project": json.RawMessage(`{"version":1,"requirements":["SteerlineFlow/testing.md"],"testing_catalogue":"SteerlineFlow/testing/test-list.json"}`), "plan_path": contract.PlanRef.Path, "commit": contract.PlanRef.Commit, "status": "ready", "digest": digestValue(nil)})
	input := runnercontract.RunInput{Title: work.Record.Title, Repository: runnercontract.Repository{URL: contract.Repository, Remote: contract.Remote, BaseBranch: contract.Branch, BaseRevision: strings.Repeat("a", 40)}, Documents: runnercontract.Documents{Spec: runnercontract.Document{Path: contract.SpecRef.Path}, Plan: runnercontract.Document{Path: contract.PlanRef.Path}}, Workflow: runnercontract.Workflow{PromptOverrides: map[string]string{"implement.execute": "Published instruction\n" + testPromptContext(t, snapshot)}}, Publication: runnercontract.Publication{BaseBranch: contract.Branch}, ResultFile: &runnercontract.ProgressFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString([]byte(`{"tests":[]}`))}}
	c := runnerstate.Candidate{Project: project, TaskID: tasks[0].TaskID, ID: "test-candidate", Revision: 1, Explicit: true, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "fixture", ContractDigest: contract.Digest, BaseRevision: strings.Repeat("a", 40), Participation: "active", SubmitState: "pending", Submit: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "original"}, TestPlan: snapshot}
	for _, mode := range []string{"other-path", "other-commit", "legacy-inline", "unbound-snapshot", "legacy-arrangement", "changed-snapshot", "missing-context", "wrong-context", "legacy-spec", "missing-input", "changed-input"} {
		raw, _ := json.Marshal(input)
		var bad runnercontract.RunInput
		require(t, json.Unmarshal(raw, &bad))
		next := c
		switch mode {
		case "other-path":
			bad.Documents.References = []runnercontract.Document{{Path: "private.json"}}
		case "other-commit":
			bad.Documents.References = []runnercontract.Document{{Path: "SteerlineFlow/testing.md", Commit: strings.Repeat("b", 40)}}
		case "legacy-inline":
			body := string(snapshot)
			bad.Documents.References = append(bad.Documents.References, runnercontract.Document{Kind: "inline", Path: "steerline-test-plan.json", Content: &body})
		case "unbound-snapshot":
			next.TestPlan = nil
		case "legacy-arrangement":
			bad.Documents.References = append(bad.Documents.References, runnercontract.Document{Path: strings.TrimSuffix(contract.PlanRef.Path, ".plan.md") + ".tests.json"})
		case "changed-snapshot":
			next.TestPlan = []byte(strings.Replace(string(snapshot), `"status":"ready"`, `"status":"ready","definitions":[{"id":"other"}]`, 1))
		case "missing-context":
			bad.Workflow.PromptOverrides = nil
		case "wrong-context":
			bad.Workflow.PromptOverrides["implement.execute"] = "different prompt"
		case "legacy-spec":
			bad.Documents.Spec.Kind = "git"
			bad.Documents.Spec.Commit = contract.SpecRef.Commit
		case "missing-input":
			bad.ResultFile = nil
		case "changed-input":
			bad.ResultFile.InitialContentBase64 = base64.StdEncoding.EncodeToString([]byte(`{"tests":["other"]}`))
		}
		next.Submit.Input, _ = json.Marshal(bad)
		if reserveFixtureCandidate(t, s, ctx, work, next, false) == nil {
			t.Fatalf("accepted %s", mode)
		}
	}
	c.Submit.Input, _ = json.Marshal(input)
	require(t, reserveFixtureCandidate(t, s, ctx, work, c, false))
	saved, e := s.Candidate(ctx, project, c.ID)
	require(t, e)
	if string(saved.TestPlan) != string(snapshot) {
		t.Fatal("fixed snapshot changed")
	}
}

func testPromptContext(t *testing.T, snapshot []byte) string {
	t.Helper()
	body, err := prompts.TestingContext(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
