package app

import (
	"context"
	"encoding/json"
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/environmentcheck"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

type environmentCheckPort struct {
	service    *environmentcheck.Service
	repository string
}

func newEnvironmentCheckPort(c Config, store environmentcheck.Store) *environmentCheckPort {
	s := &environmentcheck.Service{Project: c.Identity(), Selection: c.RunnerExecution.Runner, Store: store}
	for _, p := range c.RunnerExecution.Profiles {
		s.Profiles = append(s.Profiles, environmentcheck.Profile{ID: p.ID, Name: p.Name, Execution: p.Execution})
	}
	s.Open = func(ctx context.Context, selection runnerclient.Selection) (environmentcheck.Runner, error) {
		if err := runnerstatepath.Validate(c.RunDir); err != nil {
			return nil, err
		}
		cli, _, err := selection.Open(ctx, runnerstatepath.Directory(c.RunDir), positiveDuration(c.CommandTimeout, 30*time.Second))
		return cli, err
	}
	s.ResolveInput = func(ctx context.Context, execution runnerclient.ExecutionInput) (runnerclient.CheckInput, error) {
		git := commandRunner{Dir: c.Repository, Timeout: positiveDuration(c.CommandTimeout, 30*time.Second), ProcessHost: c.ProcessHost}
		remote, err := c.TaskRepositoryTarget(ctx)
		if err != nil {
			return runnerclient.CheckInput{}, err
		}
		base, err := git.Output(ctx, c.Git, "rev-parse", "--verify", "refs/remotes/"+c.Remote+"/"+c.Branch+"^{commit}")
		if err != nil {
			return runnerclient.CheckInput{}, errors.New("project target base commit is not available locally; fetch the configured target branch before checking")
		}
		publication := c.runnerPublication()
		return runnerclient.CheckInput{Repository: runnerclient.Repository{URL: remote, LocalPath: c.Repository, Remote: c.Remote, BaseBranch: c.Branch, BaseRevision: strings.TrimSpace(string(base))}, Execution: execution, Publication: &publication, Correlation: map[string]string{"project": c.Identity()}}, nil
	}
	return &environmentCheckPort{service: s, repository: c.Repository}
}

// Projection is explicit at the application boundary; HTTP never imports the
// native Runner DTOs or the process client to understand a receipt.
func projectEnvironmentCheck(r environmentcheck.Record) (api.EnvironmentCheckRecord, error) {
	b, err := json.Marshal(r)
	var view api.EnvironmentCheckRecord
	if err == nil {
		err = json.Unmarshal(b, &view)
	}
	return view, err
}
func (p *environmentCheckPort) EnvironmentCheckCatalog(ctx context.Context) (api.EnvironmentCheckCatalog, error) {
	r := p.service.Catalog(ctx)
	r.Repository = p.repository
	return json.Marshal(r)
}
func (p *environmentCheckPort) EnvironmentChecks(ctx context.Context) ([]api.EnvironmentCheckRecord, error) {
	rows, err := p.service.List(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]api.EnvironmentCheckRecord, 0, len(rows))
	for _, row := range rows {
		v, e := projectEnvironmentCheck(row)
		if e != nil {
			return nil, e
		}
		views = append(views, v)
	}
	return views, nil
}
func (p *environmentCheckPort) EnvironmentCheck(ctx context.Context, id string) (api.EnvironmentCheckRecord, error) {
	r, err := p.service.Detail(ctx, id)
	if err != nil {
		return api.EnvironmentCheckRecord{}, publicCheckError(err)
	}
	return projectEnvironmentCheck(r)
}
func (p *environmentCheckPort) EnvironmentCheckRequest(ctx context.Context, id string) (api.EnvironmentCheckRecord, error) {
	r, err := p.service.Request(ctx, id)
	if err != nil {
		return api.EnvironmentCheckRecord{}, publicCheckError(err)
	}
	return projectEnvironmentCheck(r)
}
func (p *environmentCheckPort) SubmitEnvironmentCheck(ctx context.Context, c api.EnvironmentCheckCommand) (api.EnvironmentCheckRecord, error) {
	r, err := p.service.Submit(ctx, environmentcheck.Command(c))
	view, projectionErr := projectEnvironmentCheck(r)
	return view, errors.Join(publicCheckError(err), projectionErr)
}

func publicCheckError(err error) error {
	if errors.Is(err, environmentcheck.ErrNotFound) {
		return api.ErrEnvironmentCheckNotFound
	}
	var domain *environmentcheck.Error
	if errors.As(err, &domain) {
		return &api.EnvironmentCheckError{Code: domain.Code, Message: domain.Error()}
	}
	var native *runnerclient.Failure
	if errors.As(err, &native) && !native.Unknown {
		return &api.EnvironmentCheckError{Code: native.Code, Message: native.Error()}
	}
	return err
}
