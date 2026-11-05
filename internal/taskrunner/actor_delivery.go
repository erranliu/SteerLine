package taskrunner

import (
	"context"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type deliveryCandidate struct{ task, candidate string }

type automaticDeliveryResult struct {
	input ports.RunnerResult
	err   error
}

// Only the actor reads or mutates the cycle. A worker publishes one result
// through the actor's mailbox and never reads or writes the store.
type automaticDeliveryCycle struct {
	remaining []deliveryCandidate
	running   bool
	result    *automaticDeliveryResult
	failures  []error
}

func automaticDeliveryEligible(task taskmodel.TaskRecord, c runnerstate.Candidate) bool {
	return task.Status == "in_progress" && c.TaskID == task.TaskID && c.Participation == "active" && !c.StopRequested &&
		c.RunID != "" && c.SubmitState == "accepted" && c.Claimant == task.Claimant && c.Projection != nil && c.Projection.PR != nil
}

func (a *Actor) TickPR(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}
	if a.delivery != nil {
		return a.TickDelivery(ctx)
	}
	snapshot, err := a.options.Store.ReadRunnerSnapshot(ctx, a.options.Tasks.Project)
	if err != nil {
		return err
	}
	a.delivery = &automaticDeliveryCycle{}
	for _, c := range snapshot.Candidates {
		if automaticDeliveryEligible(snapshot.Graph.ByID[c.TaskID], c) {
			a.delivery.remaining = append(a.delivery.remaining, deliveryCandidate{c.TaskID, c.ID})
		}
	}
	return a.advanceAutomaticDelivery(ctx)
}

func (a *Actor) DeliveryWakeup() <-chan struct{} { return a.deliveryWakeup }

// Observation checkpoints only yield for actionable merge evidence. The
// scheduler consumes open PRs and query errors after the observation tick.
func (a *Actor) handoffReadyDelivery(ctx context.Context) error {
	a.workMu.Lock()
	ready := a.readyDelivery.canYieldObservation()
	a.workMu.Unlock()
	if !ready {
		return nil
	}
	return a.TickDelivery(ctx)
}

func (a *Actor) TickDelivery(ctx context.Context) error {
	if a.delivery == nil {
		return nil
	}
	a.workMu.Lock()
	if a.controlPending || ctx.Err() != nil {
		a.workMu.Unlock()
		return nil
	}
	if a.readyDelivery != nil {
		a.delivery.running = false
		a.delivery.result, a.readyDelivery = a.readyDelivery, nil
	}
	a.workMu.Unlock()
	return a.advanceAutomaticDelivery(ctx)
}

func (a *Actor) advanceAutomaticDelivery(ctx context.Context) error {
	cycle := a.delivery
	if cycle.running || ctx.Err() != nil {
		return nil
	}
	if result := cycle.result; result != nil {
		cycle.result = nil
		if result.err == nil {
			deadline := result.input.PR.CheckedAt.Add(ports.AutomaticDeliveryCommitWindow)
			if time.Now().Before(deadline) {
				commit, cancel := context.WithDeadline(ctx, deadline)
				_, result.err = a.options.Store.CommitRunnerTaskResult(commit, result.input)
				cancel()
			}
		}
		cycle.recordFailure(result.err)
	}
	for len(cycle.remaining) > 0 && ctx.Err() == nil {
		key := cycle.remaining[0]
		input, target, err := a.prepareAutomaticDelivery(ctx, key)
		if ctx.Err() != nil {
			return nil
		}
		cycle.remaining = cycle.remaining[1:]
		if err != nil {
			cycle.recordFailure(err)
			continue
		}
		if input.AutomaticBinding != nil {
			cycle.running = true
			a.startAutomaticDeliveryQuery(input, target)
			return nil
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	a.delivery = nil
	return a.observeError(ctx, errors.Join(cycle.failures...))
}

func (cycle *automaticDeliveryCycle) recordFailure(err error) {
	var domain *taskmodel.Error
	if err != nil && !errors.Is(err, context.Canceled) && !(errors.As(err, &domain) && domain.Code == "merge_state_unknown") {
		cycle.failures = append(cycle.failures, err)
	}
}

func (a *Actor) prepareAutomaticDelivery(ctx context.Context, key deliveryCandidate) (ports.RunnerResult, ports.RunnerPRTarget, error) {
	project := a.options.Tasks.Project
	graph, err := a.options.Store.TaskGraphSnapshot(ctx, project)
	if err != nil {
		return ports.RunnerResult{}, ports.RunnerPRTarget{}, err
	}
	task := graph.ByID[key.task]
	if task.Status != "in_progress" {
		return ports.RunnerResult{}, ports.RunnerPRTarget{}, nil
	}
	c, err := a.options.Store.Candidate(ctx, project, key.candidate)
	if err != nil || !automaticDeliveryEligible(task, c) {
		return ports.RunnerResult{}, ports.RunnerPRTarget{}, err
	}
	binding, target, err := ports.BindAutomaticDelivery(c, graph.Authority.Branch)
	if err != nil {
		return ports.RunnerResult{}, target, err
	}
	input := ports.RunnerResult{Project: project, TaskID: key.task, CandidateID: c.ID, TaskVersion: task.Version,
		CandidateRevision: c.Revision, AuthorityVersion: graph.Authority.AuthorityVersion, GraphRevision: graph.Authority.GraphRevision,
		VerificationStarted: time.Now().UTC(), AutomaticBinding: &binding,
		PR: ports.RunnerPRObservation{CandidateID: c.ID, RunID: c.RunID, CandidateRevision: c.Revision}}
	return input, target, nil
}

func (a *Actor) startAutomaticDeliveryQuery(input ports.RunnerResult, target ports.RunnerPRTarget) {
	ctx, query := a.ctx, a.options.Tasks.QueryPR
	a.deliveryQueries.Go(func() {
		observation, err := queryTaskPR(ctx, query, target, input.PR)
		input.PR = observation
		a.workMu.Lock()
		if ctx.Err() != nil {
			a.workMu.Unlock()
			return
		}
		a.readyDelivery = &automaticDeliveryResult{input: input, err: err}
		if a.readyDelivery.canYieldObservation() && a.yieldAuxiliary != nil {
			a.yieldAuxiliary(errDeliveryReady)
		}
		a.workMu.Unlock()
		select {
		case a.deliveryWakeup <- struct{}{}:
		default:
		}
	})
}

var errDeliveryReady = errors.New("automatic delivery result is ready")

func (r *automaticDeliveryResult) canYieldObservation() bool {
	if r == nil || r.err != nil {
		return false
	}
	fact := r.input.PR.Fact
	return fact.State == "merged" && fact.Merged && fact.MergedAt != nil && !fact.MergedAt.IsZero() &&
		fact.MergeCommit != nil && taskmodel.ValidCommit(*fact.MergeCommit)
}

func (a *Actor) observeAuxiliaryForDecision(ctx context.Context) error {
	budget, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	auxiliary, yield := context.WithCancelCause(budget)
	defer yield(nil)
	a.workMu.Lock()
	a.yieldAuxiliary = yield
	if a.readyDelivery.canYieldObservation() {
		yield(errDeliveryReady)
	}
	a.workMu.Unlock()
	var err error
	if auxiliary.Err() == nil {
		err = a.ObserveAuxiliary(auxiliary)
	}
	a.workMu.Lock()
	a.yieldAuxiliary = nil
	a.workMu.Unlock()
	if context.Cause(auxiliary) == errDeliveryReady && auxiliaryCanceledOnly(err) {
		return nil
	}
	return err
}

// Auxiliary stages can join failures. Keep any independent failure when the
// remaining wait was deliberately yielded to a ready delivery.
func auxiliaryCanceledOnly(err error) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			if !auxiliaryCanceledOnly(child) {
				return false
			}
		}
		return true
	}
	if wrapped := errors.Unwrap(err); wrapped != nil {
		return auxiliaryCanceledOnly(wrapped)
	}
	return err == nil || err == context.Canceled
}
