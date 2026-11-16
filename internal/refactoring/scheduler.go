package refactoring

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/refactoring/model"
)

func observationTime(observed, now time.Time) time.Time {
	if observed.IsZero() || observed.After(now) {
		return now
	}
	return observed
}

func (s *Service) scheduled(ctx context.Context, includeHistory bool) ([]model.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.Store.Refactorings(ctx, s.Project)
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].EnqueuedAt.Equal(rows[j].EnqueuedAt) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].EnqueuedAt.Before(rows[j].EnqueuedAt)
	})
	work := make([]model.Record, 0, model.Concurrency)
	for _, r := range rows {
		if r.Slot {
			work = append(work, r)
		}
	}
	now := s.now()
	for _, r := range rows {
		if len(work) >= model.Concurrency {
			break
		}
		if r.Slot || !r.Active() || r.Phase == "waiting_input" || r.RetryAt != nil && now.Before(*r.RetryAt) {
			continue
		}
		r.Slot = true
		r.WaitReason = ""
		if !r.StopRequested && r.RetryUntil == nil {
			r.Resume(now)
		}
		if err = s.save(ctx, &r, nil); err != nil {
			return work, err
		}
		work = append(work, r)
	}
	// Historical observations do not consume an execution slot.
	for _, r := range rows {
		if includeHistory && !r.Slot && s.investigationDue(r) >= 0 {
			work = append(work, r)
			return work, nil
		}
	}
	for _, r := range rows {
		if includeHistory && !r.Active() && s.reviewDue(r, now) {
			work = append(work, r)
			return work, nil
		}
	}
	for _, r := range rows {
		if includeHistory && !r.Active() && s.costDue(r, now) >= 0 {
			work = append(work, r)
			break
		}
	}
	return work, nil
}

// Run keeps each action independently observable while a sibling calls Runner.
func (s *Service) Run(ctx context.Context) {
	_ = s.run(ctx, false)
}

func (s *Service) run(ctx context.Context, drain bool) error {
	interval := time.Second
	if drain {
		interval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var workers sync.WaitGroup
	defer workers.Wait()
	var advancing sync.Map
	for ctx.Err() == nil {
		rows, err := s.scheduled(ctx, !drain)
		if drain && len(rows) == 0 {
			return err
		}
		for _, r := range rows {
			if _, busy := advancing.LoadOrStore(r.ID, true); busy {
				continue
			}
			workers.Go(func() {
				defer advancing.Delete(r.ID)
				step, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				_ = s.advanceAction(step, r)
			})
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return ctx.Err()
}

// Tick waits for one bounded pass for deterministic callers and shutdown.
func (s *Service) Tick(ctx context.Context) error {
	s.advance.Lock()
	defer s.advance.Unlock()
	rows, err := s.scheduled(ctx, true)
	var workers sync.WaitGroup
	outcomes := make([]error, len(rows))
	for i, r := range rows {
		workers.Go(func() { outcomes[i] = s.advanceAction(ctx, r) })
	}
	workers.Wait()
	return errors.Join(err, errors.Join(outcomes...))
}

func (s *Service) advanceAction(ctx context.Context, r model.Record) error {
	if i := s.investigationDue(r); i >= 0 {
		op := r.Operations[i]
		if err := s.ReleaseInvestigation(op.ID); err != nil {
			return err
		}
		_, err := s.update(ctx, r.ID, func(current *model.Record) error {
			current.Operations[i].Investigation = ""
			return nil
		})
		return err
	}
	if !r.Active() {
		if s.claimReviewCheck(r) {
			if err := s.refreshReview(ctx, r); err != nil {
				return err
			}
		}
		return s.refreshCost(ctx, r)
	}
	err := s.tick(ctx, r)
	if err != nil && ctx.Err() == nil {
		_, _ = s.update(ctx, r.ID, func(current *model.Record) error {
			current.Diagnostic = err.Error()
			return nil
		})
	}
	return err
}

func (s *Service) investigationDue(r model.Record) int {
	if s.ReleaseInvestigation != nil {
		for i, op := range r.Operations {
			if op.Settled() && op.Investigation != "" {
				return i
			}
		}
	}
	return -1
}

// reviewDue runs under mu.
func (s *Service) reviewDue(r model.Record, now time.Time) bool {
	return s.Verify != nil && r.Phase == "delivered" && r.PR != nil && !r.PR.Merged &&
		r.PR.State == "open" && now.Sub(s.reviewChecked[r.ID]) >= time.Minute
}

func (s *Service) claimReviewCheck(r model.Record) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !s.reviewDue(r, now) {
		return false
	}
	if s.reviewChecked == nil {
		s.reviewChecked = make(map[string]time.Time)
	}
	s.reviewChecked[r.ID] = now
	return true
}

func (s *Service) refreshReview(ctx context.Context, r model.Record) error {
	pr, checks, err := s.verifyPR(ctx, r, model.Result{PR: r.PR.URL})
	if err != nil {
		return err
	}
	checksChanged := len(r.Reports) > 0 && r.Reports[len(r.Reports)-1].Checks != checks
	if !checksChanged && matchesVersion(r.PR, pr.URL, pr.HeadRevision, pr.BaseRevision) && pr.State == r.PR.State && pr.Merged == r.PR.Merged {
		return nil
	}
	_, err = s.update(ctx, r.ID, func(current *model.Record) error {
		if current.Revision != r.Revision {
			return nil
		}
		current.PR = pr
		if pr.State == "open" && !pr.Merged {
			current.Phase = "report"
			current.EnqueuedAt = s.now()
			current.FinishedAt = nil
		}
		return nil
	})
	return err
}

func (s *Service) costDue(r model.Record, now time.Time) int {
	if s.ReadCost != nil && r.Frozen.Pricing != nil {
		for i, o := range r.Operations {
			if o.State == "complete" && !o.Cost.Complete && now.Sub(o.Cost.ObservedAt) > 30*time.Second {
				return i
			}
		}
	}
	return -1
}

func (s *Service) refreshCost(ctx context.Context, r model.Record) error {
	i := s.costDue(r, s.now())
	if i < 0 {
		return nil
	}
	o := r.Operations[i]
	cost, err := s.ReadCost(ctx, r, o)
	if err != nil {
		cost = o.Cost
		cost.Diagnostic = err.Error()
	}
	cost.ObservedAt = s.now()
	_, err = s.update(ctx, r.ID, func(current *model.Record) error {
		current.Operations[i].Cost = cost
		return nil
	})
	return err
}

func (s *Service) markSubmitted(ctx context.Context, id string, operation model.Operation, proof []env.Admission) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.Store.Refactoring(ctx, s.Project, id)
	if err != nil || !r.Slot || r.StopRequested || len(r.Operations) == 0 {
		return false, err
	}
	last := &r.Operations[len(r.Operations)-1]
	if last.ID != operation.ID || last.State != "pending" {
		return false, nil
	}
	queued := last.Content(r).WorkflowID == "generate" && s.Queue != nil
	key := "refactoring:" + last.ID
	if queued && !s.Queue.AcquireContent(s.Project, key) {
		if r.WaitReason == model.WaitSharedCapacity {
			return false, nil
		}
		r.WaitReason = model.WaitSharedCapacity
		r.Pause(s.now())
		return false, s.save(ctx, &r, nil)
	}
	r.WaitReason = ""
	r.RetryAt, r.RetryUntil = nil, nil
	r.Resume(s.now())
	r.Authorized = true
	if last.Purpose == "conflict" {
		r.ConflictPending = false
	}
	last.State = "submitted"
	if err = s.save(ctx, &r, nil, proof...); err != nil {
		if queued {
			s.Queue.UpdateContent(s.Project, key, last.CreatedAt, false, false)
		}
		return false, err
	}
	return true, nil
}

func (s *Service) deferFirstDispatch(ctx context.Context, id, operation string, qualificationErr error) error {
	_, err := s.update(ctx, id, func(r *model.Record) error {
		last := &r.Operations[len(r.Operations)-1]
		if last.ID != operation || last.State != "pending" || r.StopRequested {
			return nil
		}
		now := s.now()
		r.Slot = false
		r.Pause(now)
		r.Diagnostic = qualificationErr.Error()
		if r.RetryUntil == nil {
			until := now.Add(30 * time.Second)
			r.RetryUntil = &until
		}
		if errors.Is(qualificationErr, model.ErrEnvironmentChanged) || !now.Before(*r.RetryUntil) {
			last.State = "cancelled"
			r.Phase = "blocked"
			r.WaitReason = ""
			return nil
		}
		retry := now.Add(2 * time.Second)
		r.RetryAt = &retry
		r.WaitReason = model.WaitEnvironment
		return nil
	})
	return err
}
