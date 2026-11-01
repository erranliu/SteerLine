package app

import (
	"context"
	"errors"
	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"os"
)

type invalidEnvironmentBasis struct{ error }

func (e *invalidEnvironmentBasis) Unwrap() error { return e.error }
func (p *environmentPort) observeBasis(ctx context.Context, commit string) error {
	if p.store == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, profile := range p.config.RunnerExecution.Profiles {
		if profile.Workflow.ID != "development" && profile.Workflow.ID != "development-audited" {
			continue
		}
		key, err := env.Key(profile.Execution)
		if err != nil || seen[key] {
			continue
		}
		seen[key] = true
		current, record, err := p.current(ctx, profile.ID)
		if err != nil {
			return err
		}
		if current.ActivityID == "" || current.Invalidated {
			continue
		}
		material, err := p.materialAt(ctx, profile.ID, commit)
		var invalid *invalidEnvironmentBasis
		var nonregular *nonRegularCommittedFileError
		confirmed := errors.Is(err, os.ErrNotExist) || errors.As(err, &invalid) || errors.As(err, &nonregular)
		if err != nil && !confirmed {
			return err
		}
		if err == nil && material.Inputs == record.Prepared.Inputs {
			continue
		}
		reason := "已确认项目环境依据变化，须重新检测"
		if err != nil {
			reason = err.Error()
		}
		if err := p.store.RevokeEnvironmentBasis(ctx, current.Project, key, current.ActivityID, reason, current.Revision); err != nil {
			return err
		}
	}
	return nil
}
