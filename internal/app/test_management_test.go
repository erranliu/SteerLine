package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

type tmSnapshotStore struct {
	testManagementStore
	candidate runnerstate.Candidate
}

func (s tmSnapshotStore) Candidate(_ context.Context, project, id string) (runnerstate.Candidate, error) {
	if project != s.candidate.Project || id != s.candidate.ID {
		return runnerstate.Candidate{}, storageports.ErrNotFound
	}
	return s.candidate, nil
}

func TestTMPlanTestsRequiresPublicReports(t *testing.T) {
	raw, _ := json.Marshal(testmanagement.Snapshot{Status: "ready", PlanPath: "task.plan.md", Definitions: []testmanagement.Definition{{ID: "rules", Name: "规则"}}})
	p := &testManagementPort{service: &testmanagement.GenerationService{Project: "project"}, store: tmSnapshotStore{candidate: runnerstate.Candidate{Project: "project", ID: "candidate", TestPlan: raw}}}
	body, err := p.PlanTests(context.Background(), "candidate")
	var view testmanagement.View
	if err != nil || json.Unmarshal(body, &view) != nil || view.Total != 1 || view.Pending != 1 || view.Passed != 0 || view.Diagnostic == "" {
		t.Fatal("unverified result counted", string(body), err)
	}
	_, err = p.PlanTests(context.Background(), "other")
	var missing *api.Error
	if !errors.As(err, &missing) || missing.Kind != api.UnknownRecord {
		t.Fatal("missing candidate became storage failure", err)
	}
}

func (s tmSnapshotStore) Progress(context.Context, string, string) (testmodel.Progress, error) {
	return testmodel.Progress{}, testmanagement.ErrNotFound
}
