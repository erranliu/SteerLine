package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestTaskStartPreviewAndReservationUseCurrentMaterialLimit(t *testing.T) {
	ctx := context.Background()
	store, project, tasks := nativeAuditStore(t)
	path := store.path
	require(t, store.Close())
	store, err := OpenSQLiteStoreWithAdmissionPolicy(path, env.NewAdmissionPolicy(false))
	require(t, err)
	defer store.Close()

	contract, err := store.TaskContract(ctx, project, tasks[0])
	require(t, err)
	input := runnerclient.RunInput{
		Title: tasks[0].Title, Execution: readinessRecord(project).Execution,
		Repository:  runnerclient.Repository{URL: contract.Repository, Remote: contract.Remote, BaseBranch: contract.Branch, BaseRevision: strings.Repeat("a", 40)},
		Documents:   runnerclient.Documents{Spec: runnerclient.Document{Path: contract.SpecRef.Path}, Plan: runnerclient.Document{Path: contract.PlanRef.Path}},
		Publication: runnerclient.Publication{BaseBranch: contract.Branch},
	}
	body, err := json.Marshal(input)
	require(t, err)
	template := runnerstate.Candidate{
		Project: project, TaskID: tasks[0].TaskID, Claimant: "fixture", Revision: 1,
		Explicit: true, StateDir: filepath.Join(t.TempDir(), "runner"), ContractDigest: contract.Digest,
		BaseRevision: input.Repository.BaseRevision, Participation: "active", SubmitState: "pending",
		Submit: runnerclient.Request{Protocol: runnerclient.Protocol, Input: body},
	}
	reserve := func(id string, extra bool) error {
		graph, err := store.TaskGraphSnapshot(ctx, project)
		require(t, err)
		work := taskmodel.FixedWork{Record: graph.ByID[template.TaskID], Contract: contract, Authority: graph.TaskGraph().Authority}
		candidate := template
		candidate.ID, candidate.Submit.RequestID = id, id
		return store.ReserveRunnerCandidate(ctx, work, candidate, extra)
	}
	require(t, reserve("first", false))
	for i := 2; i <= 4; i++ {
		require(t, reserve(fmt.Sprintf("current-%d", i), true))
	}
	previous := template
	previous.ID, previous.Submit.RequestID, previous.Number = "previous-material", "previous-material", 5
	previous.MaterialDigest = "previous-digest"
	oldBody, err := json.Marshal(previous)
	require(t, err)
	_, err = store.db.ExecContext(ctx, `INSERT INTO runner_candidates(project_identity,candidate_id,task_id,revision,request_id,run_id,payload) VALUES(?,?,?,?,?,'',?)`,
		project, previous.ID, previous.TaskID, previous.Revision, previous.Submit.RequestID, oldBody)
	require(t, err)

	preview := func() int {
		page, err := store.NativeTaskPage(ctx, project, template.TaskID, "", 0, 0, 1)
		require(t, err)
		return page.Tasks[0].CurrentMaterialCandidates
	}
	if active := preview(); active != 4 {
		t.Fatalf("preview counted candidates from another material version: %d", active)
	}
	require(t, reserve("other-confirmation", true))
	err = reserve("stale-preview", true)
	var blocked *taskmodel.Error
	if !errors.As(err, &blocked) || blocked.Code != "candidate_limit_reached" {
		t.Fatalf("reservation trusted the older preview: %v", err)
	}
	if active := preview(); active != 5 {
		t.Fatalf("rejected reservation changed the candidate count: %d", active)
	}
}
