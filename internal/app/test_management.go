package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	api "github.com/erranliu/SteerLine/internal/product/api"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

type testManagementStore interface {
	testmanagement.ProgressStore
	Candidate(context.Context, string, string) (runnerstate.Candidate, error)
}

type testHistoryBaseline interface {
	current() pm.Observation
	testRecordTree(context.Context, string) ([]byte, error)
}

type testManagementPort struct {
	historyMu sync.Mutex
	history   *testmanagement.RecordSnapshot
	project   string
	store     testManagementStore
	progress  *testmanagement.ProgressService
	baseline  testHistoryBaseline
	materials committedMaterialReader
	freshness materialFreshnessReader
}

func newTestManagementPort(c Config, store testManagementStore, cli runnerclient.Client, admit func(context.Context) error, baseline testHistoryBaseline, materials committedMaterialReader, freshness materialFreshnessReader) *testManagementPort {
	c = bindNativeConfiguration(c, cli)
	return &testManagementPort{project: c.Identity(), store: store, baseline: baseline, materials: materials, freshness: freshness, progress: &testmanagement.ProgressService{Project: c.Identity(), Store: store, Runner: cli, Admit: admit}}
}
func (p *testManagementPort) PlanTests(ctx context.Context, id string) (json.RawMessage, error) {
	c, err := p.store.Candidate(ctx, p.project, id)
	if err != nil {
		if errors.Is(err, storageports.ErrNotFound) {
			return nil, &api.Error{Kind: api.UnknownRecord, Message: "candidate not found"}
		}
		return nil, err
	}
	s := testmanagement.Snapshot{Status: "missing", Definitions: []testmanagement.Definition{}}
	if len(c.TestPlan) > 0 {
		if err = json.Unmarshal(c.TestPlan, &s); err != nil {
			return nil, err
		}
	}
	freshness := "current"
	if p.freshness != nil {
		freshness = p.freshness.freshness(ctx, c)
	}
	encode := func(v testmanagement.View) (json.RawMessage, error) {
		if freshness == "unknown" {
			v.Passed = 0
			v.Pending = 0
			v.Diagnostic = "当前测试关联不可用：" + freshness + "；以下为原候选历史"
			for i := range v.Rows {
				v.Rows[i].Status = "stale"
			}
		}
		return json.Marshal(struct {
			testmanagement.View
			MaterialStatus       string `json:"material_status"`
			AssociationAvailable bool   `json:"association_available"`
		}{v, freshness, freshness == "current"})
	}
	progress, readErr := p.store.Progress(ctx, p.project, id)
	if readErr != nil && !errors.Is(readErr, testmanagement.ErrNotFound) {
		return nil, readErr
	}
	if errors.Is(readErr, testmanagement.ErrNotFound) {
		progress.Diagnostic = "尚未收到本次运行的测试进度"
	} else if progress.RunID != c.RunID || progress.InputDigest != c.InputDigest {
		return nil, errors.New("test progress candidate association differs")
	}
	v := testmanagement.ProgressView(s, progress)
	return encode(v)
}
