package taskrunner

import (
	"context"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func (s *Service) observeTaskPR(ctx context.Context, c runnerstate.Candidate, branch string) (ports.RunnerPRObservation, error) {
	target, err := ports.BindRunnerPR(c, branch)
	if err != nil {
		return ports.RunnerPRObservation{}, err
	}
	return queryTaskPR(ctx, s.QueryPR, target, ports.RunnerPRObservation{CandidateID: c.ID, RunID: c.RunID, CandidateRevision: c.Revision})
}

func queryTaskPR(ctx context.Context, query ports.RunnerPRQuery, target ports.RunnerPRTarget, observation ports.RunnerPRObservation) (ports.RunnerPRObservation, error) {
	if query == nil {
		return ports.RunnerPRObservation{}, errors.New("task PR platform query is unavailable")
	}
	observation.StartedAt = time.Now().UTC()
	fact, err := query(ctx, target.Provider, target.URL)
	if err != nil {
		return ports.RunnerPRObservation{}, failure("pr_query_unavailable", "PR merge state could not be checked: "+err.Error())
	}
	if !target.Matches(fact) {
		return ports.RunnerPRObservation{}, failure("immutable_task", "platform PR differs from the frozen candidate target")
	}
	observation.Fact, observation.CheckedAt = fact, time.Now().UTC()
	return observation, nil
}
