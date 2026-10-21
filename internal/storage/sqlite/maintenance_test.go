package sqlite

import (
	"context"
	"encoding/json"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"path/filepath"
	"strings"
	"testing"
)

func TestPMFreezeApprovalCASAndOriginalCandidate(t *testing.T) {
	ctx := context.Background()
	s, project, tasks := nativeAuditStore(t)
	require(t, s.transactSQLite(ctx, func(tx *sqliteTx) error {
		_, e := tx.tx.ExecContext(ctx, `UPDATE project_task_authorities SET dispatch_paused=0 WHERE project_identity=?`, project)
		return e
	}))
	g, e := s.TaskGraphSnapshot(ctx, project)
	require(t, e)
	old, e := s.TaskContract(ctx, project, tasks[0])
	require(t, e)
	c := old
	c.SpecRef.Commit = strings.Repeat("f", 40)
	c.PlanRef.Commit = c.SpecRef.Commit
	c.AcceptanceRefs = append([]taskmodel.FileReference(nil), c.AcceptanceRefs...)
	for i := range c.AcceptanceRefs {
		c.AcceptanceRefs[i].Commit = c.SpecRef.Commit
	}
	c.Digest, e = taskmodel.ContractDigest(c)
	require(t, e)
	a, e := s.SaveMaterialApproval(ctx, pm.Approval{Project: project, Plan: c.PlanRef.Path, Inputs: strings.Repeat("a", 64), Output: strings.Repeat("b", 64), Commit: c.SpecRef.Commit, By: "test", RequestID: strings.Repeat("a", 32)}, 0)
	require(t, e)
	w := taskmodel.FixedWork{Record: tasks[0], Contract: c, ConfirmedDigest: old.Digest, Authority: g.TaskGraph().Authority, MaterialPlan: a.Plan, MaterialInputs: a.Inputs, MaterialOutput: a.Output, MaterialRevision: a.Revision}
	input := runnerclient.RunInput{Title: w.Record.Title, Repository: runnerclient.Repository{URL: c.Repository, Remote: c.Remote, BaseBranch: c.Branch, BaseRevision: c.SpecRef.Commit}, Documents: runnerclient.Documents{Spec: runnerclient.Document{Kind: "git", Path: c.SpecRef.Path, Commit: c.SpecRef.Commit}, Plan: runnerclient.Document{Kind: "git", Path: c.PlanRef.Path, Commit: c.PlanRef.Commit}}, Publication: runnerclient.Publication{BaseBranch: c.Branch}}
	raw, _ := json.Marshal(input)
	candidate := runnerbridge.Candidate{Project: project, TaskID: w.Record.TaskID, ID: "first", Revision: 1, Explicit: true, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "test", ContractDigest: c.Digest, BaseRevision: c.SpecRef.Commit, Participation: "active", SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "first-submit", Input: raw}}
	next := a
	next.RequestID = strings.Repeat("b", 32)
	next, e = s.SaveMaterialApproval(ctx, next, a.Revision)
	require(t, e)
	if e = s.ReserveRunnerCandidate(ctx, w, candidate, false); e == nil {
		t.Fatal("stale approval reserved a candidate")
	}
	after, e := s.TaskGraphSnapshot(ctx, project)
	require(t, e)
	if after.ByID[w.Record.TaskID].Status != "open" {
		t.Fatal("failed reservation claimed task")
	}
	w.MaterialRevision = next.Revision
	require(t, s.ReserveRunnerCandidate(ctx, w, candidate, false))
	frozen, e := s.TaskExecution(ctx, project, w.Record.TaskID)
	require(t, e)
	if frozen.Contract.Digest != c.Digest || frozen.Base != c.SpecRef.Commit {
		t.Fatal("new code retained old contract")
	}
	stored, e := s.TaskContract(ctx, project, w.Record)
	require(t, e)
	if stored.Digest != old.Digest {
		t.Fatal("freeze rewrote confirmed source")
	}
	// Unrelated/new maintenance facts cannot change the fixed execution.
	next.RequestID = strings.Repeat("c", 32)
	next.Output = strings.Repeat("c", 64)
	_, e = s.SaveMaterialApproval(ctx, next, next.Revision)
	require(t, e)
	g, e = s.TaskGraphSnapshot(ctx, project)
	require(t, e)
	w.Record = g.ByID[w.Record.TaskID]
	w.Authority = g.TaskGraph().Authority
	candidate.ID = "second"
	candidate.Submit.RequestID = "second-submit"
	require(t, s.ReserveRunnerCandidate(ctx, w, candidate, true))
	if _, e = s.ConfirmTaskMaterials(ctx, project, w.Record.TaskID, w.Record.Version, c, strings.Repeat("d", 32)); e == nil {
		t.Fatal("reserved Task was re-confirmed")
	}
}
func TestPMTaskConfirmationIdentityAndReplay(t *testing.T) {
	ctx := context.Background()
	s, project, tasks := nativeAuditStore(t)
	old, e := s.TaskContract(ctx, project, tasks[0])
	require(t, e)
	c := old
	c.PlanRef.Commit = strings.Repeat("f", 40)
	c.SpecRef.Commit = c.PlanRef.Commit
	c.AcceptanceRefs = append([]taskmodel.FileReference(nil), c.AcceptanceRefs...)
	for i := range c.AcceptanceRefs {
		c.AcceptanceRefs[i].Commit = c.PlanRef.Commit
	}
	c.Digest, e = taskmodel.ContractDigest(c)
	require(t, e)
	bad := c
	bad.Branch = "other"
	bad.Digest, _ = taskmodel.ContractDigest(bad)
	if _, e = s.ConfirmTaskMaterials(ctx, project, tasks[0].TaskID, tasks[0].Version, bad, strings.Repeat("a", 32)); e == nil {
		t.Fatal("changed target")
	}
	id := strings.Repeat("b", 32)
	got, e := s.ConfirmTaskMaterials(ctx, project, tasks[0].TaskID, tasks[0].Version, c, id)
	require(t, e)
	again, e := s.ConfirmTaskMaterials(ctx, project, tasks[0].TaskID, tasks[0].Version, c, id)
	require(t, e)
	if again.Version != got.Version {
		t.Fatal("confirmation replay advanced version")
	}
	if _, e = s.ConfirmTaskMaterials(ctx, project, tasks[0].TaskID, tasks[0].Version, old, id); e == nil {
		t.Fatal("same request changed input")
	}
}
