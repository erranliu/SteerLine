package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

func TestTMReservationAdmitsOnlyBoundOptionalTestReferences(t *testing.T) {
	s, project, tasks := nativeAuditStore(t)
	ctx := context.Background()
	_, err := s.db.ExecContext(ctx, `UPDATE project_task_authorities SET dispatch_paused=0 WHERE project_identity=?`, project)
	require(t, err)
	graph, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	contract, err := s.TaskContract(ctx, project, tasks[0])
	require(t, err)
	// This fixture's validated Plan path is supplied by the normal Task contract.
	if !strings.HasSuffix(contract.PlanRef.Path, ".plan.md") {
		t.Fatal("fixture Plan must use the conventional suffix", contract.PlanRef)
	}
	work := taskmodel.FixedWork{Record: tasks[0], Contract: contract, Authority: graph.TaskGraph().Authority}
	instruction := testmodel.TestingInstructions
	refs := []runnercontract.Document{{Kind: "git", Path: strings.TrimSuffix(contract.PlanRef.Path, ".plan.md") + ".tests.json", Commit: contract.PlanRef.Commit}, {Kind: "git", Path: testmodel.CataloguePath, Commit: contract.PlanRef.Commit}, {Kind: "inline", Path: "steerline-testing.md", Content: &instruction}}
	input := runnercontract.RunInput{Title: work.Record.Title, Repository: runnercontract.Repository{URL: contract.Repository, Remote: contract.Remote, BaseBranch: contract.Branch, BaseRevision: strings.Repeat("a", 40)}, Documents: runnercontract.Documents{Spec: runnercontract.Document{Kind: "git", Path: contract.SpecRef.Path, Commit: contract.SpecRef.Commit}, Plan: runnercontract.Document{Kind: "git", Path: contract.PlanRef.Path, Commit: contract.PlanRef.Commit}, References: refs}, Publication: runnercontract.Publication{BaseBranch: contract.Branch}}
	snapshot, _ := json.Marshal(map[string]string{"plan_path": contract.PlanRef.Path, "commit": contract.PlanRef.Commit, "status": "ready"})
	c := runnerstate.Candidate{Project: project, TaskID: tasks[0].TaskID, ID: "test-candidate", Revision: 1, Explicit: true, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "fixture", ContractDigest: contract.Digest, BaseRevision: strings.Repeat("a", 40), Participation: "active", SubmitState: "pending", Submit: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: "original"}, TestPlan: snapshot}
	for _, mode := range []string{"other-path", "other-commit", "changed-inline", "duplicate", "unbound-snapshot"} {
		raw, _ := json.Marshal(input)
		var bad runnercontract.RunInput
		require(t, json.Unmarshal(raw, &bad))
		next := c
		switch mode {
		case "other-path":
			bad.Documents.References[0].Path = "private.json"
		case "other-commit":
			bad.Documents.References[0].Commit = strings.Repeat("b", 40)
		case "changed-inline":
			body := "changed"
			bad.Documents.References[2].Content = &body
		case "duplicate":
			bad.Documents.References = append(bad.Documents.References, bad.Documents.References[0])
		case "unbound-snapshot":
			next.TestPlan = nil
		}
		next.Submit.Input, _ = json.Marshal(bad)
		if s.ReserveRunnerCandidate(ctx, work, next, false) == nil {
			t.Fatalf("accepted %s", mode)
		}
	}
	c.Submit.Input, _ = json.Marshal(input)
	require(t, s.ReserveRunnerCandidate(ctx, work, c, false))
	saved, err := s.Candidate(ctx, project, c.ID)
	require(t, err)
	if string(saved.TestPlan) != string(snapshot) {
		t.Fatal("fixed testing snapshot changed")
	}
}
