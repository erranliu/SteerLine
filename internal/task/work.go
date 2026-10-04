// Package task owns task selection and task-only change declarations.
package task

import (
	"context"
	"time"

	"github.com/erranliu/SteerLine/internal/task/model"
)

type Work struct {
	Record    model.TaskRecord
	Contract  model.TaskContract
	Authority model.GraphAuthority
}

type Reader interface {
	TaskSnapshot(context.Context, string) (model.Snapshot, error)
	TaskContract(context.Context, string, model.TaskRecord) (model.TaskContract, error)
}

type WorkPort interface {
	Ready(context.Context, string) ([]model.TaskRecord, error)
	Fixed(context.Context, string, string, uint64, bool) (Work, error)
	Claim(Work, string, time.Time) (model.TaskRecord, error)
}

// ResultPort returns a task-only declaration; it provides no execution writes.
type ResultPort interface {
	Declare(model.TaskRecord, model.TaskContract, model.Result, time.Time) (model.ResultDeclaration, error)
	Intent(model.DiscardIntent) (model.DiscardIntent, error)
	Discard(model.TaskRecord, model.DiscardIntent, string, []string, time.Time) (model.TaskRecord, error)
}

type Service struct{ Reader Reader }

func (s Service) Ready(ctx context.Context, project string) ([]model.TaskRecord, error) {
	graph, err := s.Reader.TaskSnapshot(ctx, project)
	if err != nil {
		return nil, err
	}
	return model.ReadyTasks(graph), nil
}

func (s Service) Fixed(ctx context.Context, project, id string, version uint64, candidate bool) (Work, error) {
	graph, err := s.Reader.TaskSnapshot(ctx, project)
	if err != nil {
		return Work{}, err
	}
	record, found := graph.ByID[id]
	if !found {
		return Work{}, &model.Error{Code: "task_not_found", Message: "fixed task was not found"}
	}
	status := "open"
	if candidate {
		status = "in_progress"
	}
	if record.Status != status || record.Version != version {
		return Work{}, &model.Error{Code: "stale_version", Message: "fixed task version changed"}
	}
	eligibility := record
	if candidate {
		eligibility.Status = "open"
	}
	if ready, blockers := model.DependencyStateForTask(eligibility, graph); !ready {
		return Work{}, model.DependencyClaimError(blockers)
	}
	contract, err := s.Reader.TaskContract(ctx, project, record)
	if err != nil {
		return Work{}, err
	}
	return Work{Record: record, Contract: contract, Authority: graph.Authority}, nil
}

func (s Service) Claim(work Work, claimant string, at time.Time) (model.TaskRecord, error) {
	return model.Claim(work.Record, claimant, at)
}

func (s Service) Declare(record model.TaskRecord, contract model.TaskContract, input model.Result, at time.Time) (model.ResultDeclaration, error) {
	return model.DeclareResult(record, contract, input, at)
}

func (s Service) Intent(input model.DiscardIntent) (model.DiscardIntent, error) {
	return input, model.ValidateDiscardIntent(input)
}

func (s Service) Discard(record model.TaskRecord, input model.DiscardIntent, requestID string, cleanup []string, at time.Time) (model.TaskRecord, error) {
	if err := model.ValidateDiscardIntent(input); err != nil {
		return model.TaskRecord{}, err
	}
	if input.TaskID != record.TaskID || input.ExpectedVersion != record.Version {
		return model.TaskRecord{}, &model.Error{Code: "stale_version", Message: "discard intent differs from task version"}
	}
	return model.Discard(record, input.Reason, input.RequestedBy, requestID, cleanup, at)
}
