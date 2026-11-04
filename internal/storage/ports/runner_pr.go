package ports

import (
	"context"
	"encoding/json"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/backend"
	model "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type RunnerPRQuery func(context.Context, model.PRProviderConfig, string) (model.PRInfo, error)

type RunnerPRTarget struct {
	Provider model.PRProviderConfig
	URL      string
	Number   int
	Base     string
	Head     string
}

func BindRunnerPR(c runnerstate.Candidate, branch string) (RunnerPRTarget, error) {
	var input runnercontract.RunInput
	if json.Unmarshal(c.Submit.Input, &input) != nil {
		return RunnerPRTarget{}, &taskmodel.Error{Code: "contract_mismatch", Message: "frozen publication configuration is invalid"}
	}
	var frozen struct {
		BaseURL string          `json:"base_url"`
		Query   backend.Command `json:"query"`
		Auth    []string        `json:"authentication_refs"`
	}
	if json.Unmarshal(input.Publication.Settings, &frozen) != nil {
		return RunnerPRTarget{}, &taskmodel.Error{Code: "contract_mismatch", Message: "frozen publication configuration is invalid"}
	}
	p := model.PRProviderConfig{Kind: input.Publication.Provider, BaseURL: frozen.BaseURL, Repository: input.Publication.Repository, Query: frozen.Query}
	if len(frozen.Auth) > 0 {
		p.TokenEnv = frozen.Auth[0]
	}
	r := c.Projection
	if r == nil || c.RunID == "" || c.InputDigest == "" || r.RunID != c.RunID || r.InputDigest != c.InputDigest || c.Branch == "" || r.Execution.Branch != c.Branch || r.Execution.Workspace == "" || r.Execution.SessionID == "" || r.PR == nil {
		return RunnerPRTarget{}, &taskmodel.Error{Code: "merge_state_unknown", Message: "candidate lacks its verified Run/workspace/session/PR binding"}
	}
	f := r.PR
	number, err := p.PRNumber(f.URL)
	if err != nil || number != f.Number || f.URL != c.PRURL || f.Provider != p.Kind || f.Repository != p.Repository || f.HeadRepository != p.Repository || f.BaseBranch != branch || input.Repository.BaseBranch != branch || f.HeadBranch != c.Branch {
		return RunnerPRTarget{}, &taskmodel.Error{Code: "immutable_task", Message: "PR differs from the frozen candidate target"}
	}
	return RunnerPRTarget{Provider: p, URL: f.URL, Number: number, Base: branch, Head: c.Branch}, nil
}

func (t RunnerPRTarget) Matches(f model.PRInfo) bool {
	return f.URL == t.URL && f.Repository == t.Provider.Repository && f.HeadRepository == t.Provider.Repository && f.BaseBranch == t.Base && f.HeadBranch == t.Head
}

// VerifyRunnerPR rebinds the platform observation to the candidate read in the transaction.
func VerifyRunnerPR(c runnerstate.Candidate, branch string, started time.Time, observed RunnerPRObservation) (RunnerPRTarget, error) {
	target, err := BindRunnerPR(c, branch)
	if err != nil {
		return RunnerPRTarget{}, err
	}
	if observed.CandidateID != c.ID || observed.CandidateRevision != c.Revision || observed.RunID != c.RunID || started.IsZero() ||
		observed.StartedAt.Before(started) || observed.CheckedAt.Before(observed.StartedAt) || !target.Matches(observed.Fact) {
		return RunnerPRTarget{}, &taskmodel.Error{Code: "merge_state_unknown", Message: "PR requires a fresh platform observation of the original candidate"}
	}
	f := observed.Fact
	switch f.State {
	case "merged":
		if f.Merged && f.MergedAt != nil && !f.MergedAt.IsZero() && f.MergeCommit != nil && taskmodel.ValidCommit(*f.MergeCommit) {
			return target, nil
		}
	case "open", "closed_unmerged":
		if !f.Merged {
			return target, nil
		}
	}
	return RunnerPRTarget{}, &taskmodel.Error{Code: "pr_query_unavailable", Message: "platform PR merge state is incomplete or inconsistent"}
}
