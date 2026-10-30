package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	task "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"path/filepath"
	"strings"
	"testing"
)

func TestPMSCandidateSnapshotCASVersionsAndReplay(t *testing.T) {
	ctx := context.Background()
	s, project, tasks := nativeAuditStore(t)
	require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error {
		_, e := tx.tx.ExecContext(ctx, `UPDATE project_task_authorities SET dispatch_paused=0 WHERE project_identity=?`, project)
		return e
	}))
	graph, e := s.TaskGraphSnapshot(ctx, project)
	require(t, e)
	original, e := s.TaskContract(ctx, project, tasks[0])
	require(t, e)
	record := tasks[0]
	var first runnerstate.Candidate
	var firstWork task.FixedWork
	var latest runnerstate.Candidate
	var latestWork task.FixedWork
	for i := 0; i < 7; i++ {
		graph, e = s.TaskGraphSnapshot(ctx, project)
		require(t, e)
		record = graph.ByID[record.TaskID]
		c := original
		commit := strings.Repeat(string("abcdef0"[i]), 40)
		c.SpecRef.Commit = commit
		c.PlanRef.Commit = commit
		c.Digest, e = task.ContractDigest(c)
		require(t, e)
		material := pm.Material{TaskID: record.TaskID, Commit: commit, Digest: strings.Repeat(string("abcdef0"[i]), 64), Status: "ready", Contract: c}
		material.Inputs = material.Digest
		material.GenerationID = "generation-" + commit
		definition := testmanagement.Definition{ID: "one", Name: "One", Level: "L1", Environment: "isolated", Command: "test", Check: "pass"}
		definitions := []testmanagement.Definition{definition}
		require(t, s.SaveGeneration(ctx, testmanagement.Generation{ID: material.GenerationID, Project: project, Revision: 1, Inputs: material.Inputs, State: "generated", Request: runnercontract.Request{RequestID: "request-" + commit}, Result: json.RawMessage(`{"tests":["one"]}`), Catalogue: testmanagement.Catalogue{Tests: definitions}}, 0))
		require(t, s.SaveMaterial(ctx, project, material))
		work := task.FixedWork{Record: record, Contract: c, Authority: graph.TaskGraph().Authority, ConfirmedDigest: original.Digest, MaterialDigest: material.Digest}
		input := runnerclient.RunInput{Title: record.Title, Repository: runnerclient.Repository{URL: c.Repository, Remote: c.Remote, BaseBranch: c.Branch, BaseRevision: commit}, Documents: runnerclient.Documents{Spec: runnerclient.Document{Path: c.SpecRef.Path}, Plan: runnerclient.Document{Path: c.PlanRef.Path}}, Publication: runnerclient.Publication{BaseBranch: c.Branch}}
		raw, _ := json.Marshal(input)
		candidate := runnerstate.Candidate{Project: project, TaskID: record.TaskID, ID: strings.Repeat(string("abcdef0"[i]), 32), Revision: 1, Explicit: true, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "test", ContractDigest: c.Digest, MaterialDigest: material.Digest, BaseRevision: commit, Participation: "active", SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "submit-" + commit, Input: raw}}
		candidate.TestPlan, _ = json.Marshal(testmanagement.Snapshot{Status: "ready", PlanPath: c.PlanRef.Path, Commit: commit, GenerationID: material.GenerationID, Inputs: material.Inputs, Definitions: definitions})
		for _, path := range []string{testmodel.CataloguePath, testmodel.RulesPath, testmodel.ProtocolPath, testmodel.WriterPath, mustArrangement(t, c.PlanRef.Path)} {
			input.Documents.References = append(input.Documents.References, runnerclient.Document{Path: path})
		}
		input.Workflow.PromptOverrides = map[string]string{"implement.execute": testPromptContext(t, candidate.TestPlan, true)}
		candidate.Submit.Input, _ = json.Marshal(input)
		if i == 0 {
			bad := material
			bad.Status = "pending"
			require(t, s.SaveMaterial(ctx, project, bad))
			if e = reserveFixtureCandidate(t, s, ctx, work, candidate, false); e == nil {
				t.Fatal("unready generation reserved")
			}
			g, e := s.TaskGraphSnapshot(ctx, project)
			require(t, e)
			if g.ByID[record.TaskID].Status != "open" {
				t.Fatal("failed reserve claimed task")
			}
			require(t, s.SaveMaterial(ctx, project, material))
			first, firstWork = candidate, work
		}
		require(t, reserveFixtureCandidate(t, s, ctx, work, candidate, i > 0))
		latest, latestWork = candidate, work
	}
	// Four further candidates on the latest version are allowed; the sixth is not.
	for i := 0; i < 5; i++ {
		next := latest
		next.ID = fmt.Sprintf("%032x", 100+i)
		next.Submit.RequestID = "submit-" + next.ID
		e := reserveFixtureCandidate(t, s, ctx, latestWork, next, true)
		if i < 4 {
			require(t, e)
		} else if e == nil {
			t.Fatal("sixth current-version candidate accepted")
		}
	}

	// Seven historical versions do not consume the five-current-version limit.
	require(t, reserveFixtureCandidate(t, s, ctx, firstWork, first, false))
	saved, e := s.Candidate(ctx, project, first.ID)
	require(t, e)
	if saved.Contract.PlanRef.Commit != first.BaseRevision || saved.MaterialDigest != first.MaterialDigest || saved.Number != 1 {
		t.Fatal("original replay upgraded candidate", saved)
	}
	if e = s.SetMaterialProfile(ctx, project, "local"); e != nil || s.MaterialProfile(ctx, project) != "local" {
		t.Fatal("profile persistence", e)
	}
}
