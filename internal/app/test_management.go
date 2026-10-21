package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

type testManagementStore interface {
	testmanagement.RecordStore
	testmanagement.GenerationStore
	testmanagement.EvidenceStore
	Candidate(context.Context, string, string) (runnerstate.Candidate, error)
}
type testManagementPort struct {
	records     *testmanagement.RecordService
	maintenance *projectMaintenance
	service     *testmanagement.GenerationService
	store       testManagementStore
	evidence    *testmanagement.EvidenceService
}

func newTestManagementPort(c Config, store testManagementStore, cli runnerclient.Client, admit func(context.Context) error) *testManagementPort {
	s := &testmanagement.GenerationService{Project: c.Identity(), Repository: c.Repository, Store: store, Runner: cli, Build: cli.BuildPin(), Admit: admit}
	s.Prepare = func(ctx context.Context, id string) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
		p, err := c.RunnerExecution.Profile(id, "")
		if err != nil {
			return runnercontract.ExecutionInput{}, nil, err
		}
		e, err := runnerclient.GenerationExecution(p.Execution)
		if err != nil {
			return e, nil, err
		}
		v, err := cli.Version(ctx)
		if err != nil {
			return e, nil, err
		}
		catalog, err := cli.Catalog(ctx, v)
		return e, catalog, err
	}
	m := newProjectMaintenance(c, nil)
	s.Workspace = m.workspace
	s.ReadCommitted = m.read
	s.VerifyBinding = func(g testmanagement.Generation) error { return verifyGenerationTakeover(g, cli) }
	var evidenceChecked bool
	evidence := &testmanagement.EvidenceService{Project: c.Identity(), Store: store, Runner: cli, Admit: func(ctx context.Context) error {
		if admit != nil {
			if err := admit(ctx); err != nil {
				return err
			}
		}
		if !evidenceChecked {
			v, err := cli.Version(ctx)
			if err != nil {
				return err
			}
			cat, err := cli.Catalog(ctx, v)
			if err != nil {
				return err
			}
			if err = cat.ValidateEvidence(); err != nil {
				return err
			}
			evidenceChecked = true
		}
		return nil
	}}
	return &testManagementPort{service: s, store: store, evidence: evidence, records: &testmanagement.RecordService{Project: c.Identity(), Store: store}, maintenance: m}
}
func publicGeneration(r testmanagement.Generation) api.TestGeneration {
	v := api.TestGeneration{Plan: r.Command.Plan, Profile: r.Command.Profile, SourceCommit: r.Command.Commit, ID: r.ID, Revision: r.Revision, State: r.State, Target: r.Target, RunID: r.RunID, Diagnostic: r.Diagnostic, Result: r.Result}
	for _, a := range r.Applications {
		if a.State == "applied" {
			v.Applied = true
		}
	}
	if r.Projection != nil {
		v.StopStatus = r.Projection.StopStatus
	}
	return v
}
func generationError(err error) error {
	if errors.Is(err, testmanagement.ErrNotFound) {
		return &api.Error{Kind: api.UnknownRecord, Message: err.Error()}
	}
	return err
}
func (p *testManagementPort) GenerateTests(ctx context.Context, in api.TestGenerateRequest) (api.TestGeneration, error) {
	r, err := p.service.Generate(ctx, testmanagement.GenerateCommand{RequestID: in.RequestID, Plan: in.Plan, Profile: in.Profile, Commit: in.Commit})
	return publicGeneration(r), generationError(err)
}
func (p *testManagementPort) TestGeneration(ctx context.Context, id string) (api.TestGeneration, error) {
	r, err := p.store.Generation(ctx, p.service.Project, id)
	v := publicGeneration(r)
	if err == nil && len(r.Result) > 0 && !r.Cancelled && r.State != "applied" {
		preview, e := p.service.Preview(ctx, r)
		v.Review = &api.TestGenerationReview{ReviewDigest: preview.ReviewDigest, Before: preview.Before, After: preview.After, Diagnostic: preview.Diagnostic}
		if e != nil {
			v.Review.Diagnostic = e.Error()
		}
	}
	return v, generationError(err)
}
func (p *testManagementPort) StopTestGeneration(ctx context.Context, in api.TestStopGenerationRequest) (api.TestGeneration, error) {
	r, err := p.service.Stop(ctx, in.GenerationID, in.RequestID)
	return publicGeneration(r), generationError(err)
}
func (p *testManagementPort) PlanTests(ctx context.Context, id string) (json.RawMessage, error) {
	c, err := p.store.Candidate(ctx, p.service.Project, id)
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
	evidence, readErr := p.store.Evidence(ctx, p.service.Project, id)
	if readErr != nil && !errors.Is(readErr, testmanagement.ErrNotFound) {
		return nil, readErr
	}
	if errors.Is(readErr, testmanagement.ErrNotFound) {
		v := testmanagement.Project(s, nil, "", false, false)
		if s.Status == "ready" && len(s.Definitions) > 0 {
			v.Diagnostic = "尚无已核实的程序测试报告"
		}
		return json.Marshal(v)
	}
	if evidence.RunID != c.RunID || evidence.InputDigest != c.InputDigest {
		return nil, errors.New("test report candidate association differs")
	}
	if c.Participation == "abandoned" {
		evidence.Git.Verified = false
	}
	v := testmanagement.EvidenceView(s, evidence, time.Now().UTC())
	return json.Marshal(v)
}
func (p *testManagementPort) close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.service.Close(ctx)
}

func (p *testManagementPort) observe(ctx context.Context) error {
	generationErr := p.service.Observe(ctx)
	evidenceErr := p.evidence.Observe(ctx)
	projectionErr := p.records.ProjectEvidence(ctx)
	return errors.Join(generationErr, evidenceErr, projectionErr)
}

func (p *testManagementPort) ApplyTests(ctx context.Context, in api.TestApplyRequest) (api.TestGeneration, error) {
	r, e := p.service.Apply(ctx, testmodel.ApplyCommand{RequestID: in.RequestID, GenerationID: in.GenerationID, ExpectedRevision: in.ExpectedRevision, ReviewDigest: in.ReviewDigest, AllowEmpty: in.AllowEmpty})
	return publicGeneration(r), generationError(e)
}
