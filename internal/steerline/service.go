package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Service struct {
	Config Config
	State  Runtime
	store  *Store
	beads  Beads
	prs    *PRClient
	worker *Worker
	active *Run
}

func NewService(c Config) (*Service, error) {
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	s := &Service{Config: c, store: &Store{Path: filepath.Join(c.RunDir, c.RuntimeFile)}, beads: Beads{Runner: r, Command: c.BD, Claimant: c.Claimant}, prs: &PRClient{Config: c.PR, Runner: r}}
	old, e := LoadRuntime(s.store.Path)
	if e != nil {
		return nil, e
	}
	s.State = Runtime{SchemaVersion: 1, Repository: Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.identity}, Runs: []*Run{}}
	if old != nil {
		if old.Repository.Identity != c.identity || old.Repository.Remote != c.Remote || old.Repository.Branch != c.Branch || old.Service.Claimant != c.Claimant {
			return nil, errors.New("runtime repository/branch/claimant mismatch; preserve state and use the original configuration")
		}
		s.State.Runs = old.Runs
		seen := map[string]bool{}
		for _, r := range s.State.Runs {
			if r == nil || r.TaskID == "" || r.RunID == "" || seen[r.TaskID] || r.Claimant != c.Claimant {
				return nil, errors.New("invalid or duplicate saved run")
			}
			seen[r.TaskID] = true
			switch r.ExecutionStatus {
			case "pending", "running", "succeeded", "failed", "unknown":
			default:
				return nil, errors.New("invalid saved execution_status")
			}
			switch r.FinishTaskStatus {
			case "pending", "waiting_pr_merge", "completed", "blocked":
			default:
				return nil, errors.New("invalid saved finish_task_status")
			}
			if r.FinishTaskStatus == "waiting_pr_merge" && r.PR.URL == nil {
				return nil, errors.New("waiting run lacks PR binding")
			}
			if r.ExecutionStatus == "pending" || r.ExecutionStatus == "running" {
				r.ExecutionStatus = "unknown"
				r.FinishTaskStatus = "blocked"
				r.Error = ptr("Interrupted execution: " + inspectProcess(r.Process) + "; manual recovery required; never auto-resumed")
			}
		}
	}
	s.State.Service = ServiceState{InstanceID: newID(), PID: os.Getpid(), Status: "running", Claimant: c.Claimant}
	return s, nil
}
func (s *Service) save() error { return s.store.Publish(s.State) }
func (s *Service) find(id string) *Run {
	for _, r := range s.State.Runs {
		if r.TaskID == id {
			return r
		}
	}
	return nil
}
func (s *Service) reconcile(ctx context.Context) error {
	owned, e := s.beads.Owned(ctx)
	if e != nil {
		return e
	}
	for _, t := range owned {
		if r := s.find(t.ID); r != nil {
			r.BeadsStatus = t.Status
			continue
		}
		if t.Status == "closed" {
			continue
		}
		s.State.Runs = append(s.State.Runs, &Run{TaskID: t.ID, Title: t.Title, RunID: newID(), Claimant: s.Config.Claimant, BeadsStatus: t.Status, ExecutionStatus: "unknown", FinishTaskStatus: "blocked", PR: PRState{State: "unknown"}, Error: ptr("Claim found without local run record; manual recovery required; execution not restarted")})
	}
	return s.save()
}
func (s *Service) setError(e error) {
	s.State.Service.Status = "error"
	s.State.Service.Error = ptr(clip(e.Error(), 2000))
}
func (s *Service) blocked(r *Run, msg string) {
	r.FinishTaskStatus = "blocked"
	r.Error = ptr(clip(msg, 2000))
}
func (s *Service) dispatch(ctx context.Context) error {
	if e := s.reconcile(ctx); e != nil {
		return e
	}
	if s.worker != nil {
		return nil
	}
	for _, r := range s.State.Runs {
		if r.ExecutionStatus == "unknown" {
			s.State.SlotOccupied = true
			return errors.New("unresolved unknown execution; dispatch paused until manual recovery")
		}
	}
	s.State.SlotOccupied = false
	tasks, e := s.beads.Ready(ctx)
	if e != nil {
		return e
	}
	for _, t := range tasks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !executable(t, s.Config.ExecutableTypes) || s.find(t.ID) != nil {
			continue
		}
		id := newID()
		dir := filepath.Join(s.Config.RunDir, "runs", id)
		r := &Run{TaskID: t.ID, Title: t.Title, RunID: id, Claimant: s.Config.Claimant, BeadsStatus: t.Status, ExecutionStatus: "pending", FinishTaskStatus: "pending", PR: PRState{State: "unknown"}, LogPath: ptr(filepath.Join(dir, "events.jsonl")), ResultPath: ptr(filepath.Join(dir, "result.json"))}
		s.State.Runs = append(s.State.Runs, r)
		// Durable intent BEFORE the external claim closes the crash window.
		if e = s.save(); e != nil {
			return e
		}
		ok, err := s.beads.Claim(ctx, t.ID)
		if err != nil {
			r.ExecutionStatus = "unknown"
			s.State.SlotOccupied = true
			s.blocked(r, "Claim uncertain: "+err.Error())
			if e = s.save(); e != nil {
				return e
			}
			return err
		}
		if !ok {
			s.State.Runs = s.State.Runs[:len(s.State.Runs)-1]
			if e = s.save(); e != nil {
				return e
			}
			continue
		}
		r.BeadsStatus = "in_progress"
		s.State.SlotOccupied = true
		s.State.CurrentTask = ptr(t.ID)
		if e = s.save(); e != nil {
			return e
		}
		if ctx.Err() != nil {
			r.ExecutionStatus = "unknown"
			s.blocked(r, "Stopped after claim, before worker launch")
			return s.save()
		}
		w, err := StartWorker(s.Config, t, r)
		if err != nil {
			r.ExecutionStatus = "failed"
			r.EndedAt = ptr(now())
			s.blocked(r, "Worker startup failed: "+err.Error())
			s.State.SlotOccupied = false
			s.State.CurrentTask = nil
			return s.save()
		}
		s.worker = w
		s.active = r
		r.ExecutionStatus = "running"
		r.StartedAt = ptr(now())
		r.Process = &w.process.identity
		return s.save()
	}
	return nil
}
func (s *Service) bind(r *Run, result Result) error {
	r.WorktreePath = result.WorktreePath
	r.BaseCommit = result.BaseCommit
	r.Summary = result.Summary
	if r.Summary != nil {
		r.Summary = ptr(clip(*r.Summary, 4096))
	}
	if result.Outcome == "blocked" {
		msg := "Repository workflow reported blocked"
		if result.Error != nil {
			msg += ": " + *result.Error
		}
		s.blocked(r, msg)
		return nil
	}
	if _, e := s.Config.PR.PRNumber(*result.PRURL); e != nil {
		return e
	}
	for _, other := range s.State.Runs {
		if other.RunID != r.RunID && other.PR.URL != nil && *other.PR.URL == *result.PRURL {
			return errors.New("PR is already bound to another run")
		}
	}
	r.PR.URL = result.PRURL
	r.PR.State = "unknown"
	r.FinishTaskStatus = "waiting_pr_merge"
	r.Error = nil
	return nil
}
func (s *Service) finished(err error) error {
	r := s.active
	s.worker = nil
	s.active = nil
	s.State.CurrentTask = nil
	s.State.SlotOccupied = false
	r.EndedAt = ptr(now())
	if err != nil {
		r.ExecutionStatus = "failed"
		s.blocked(r, "Codex execution failed: "+err.Error())
		return s.save()
	}
	r.ExecutionStatus = "succeeded"
	result, e := ReadResult(*r.ResultPath, r)
	if e == nil {
		e = s.bind(r, result)
	}
	if e != nil {
		s.blocked(r, "Invalid handoff: "+e.Error())
	}
	return s.save()
}
func (s *Service) pollPRs(ctx context.Context) error {
	var operationErr error
	for _, r := range s.State.Runs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if r.FinishTaskStatus != "waiting_pr_merge" || r.PR.URL == nil {
			continue
		}
		info, e := s.prs.Query(ctx, *r.PR.URL)
		r.PR.QueriedAt = ptr(now())
		if e != nil {
			r.PR.State = "unknown"
			r.PR.QueryError = ptr(clip(e.Error(), 1000))
			if errors.Is(e, ErrPRIdentity) {
				s.blocked(r, e.Error())
			}
			if e = s.save(); e != nil {
				return e
			}
			continue
		}
		r.PR.QueryError = nil
		r.PR.State = info.State
		r.PR.MergedAt = info.MergedAt
		r.PR.MergeCommit = info.MergeCommit
		if info.BaseBranch != s.Config.Branch {
			s.blocked(r, "PR target branch mismatch: "+info.BaseBranch)
			if e = s.save(); e != nil {
				return e
			}
			continue
		}
		if info.State == "closed_unmerged" {
			s.blocked(r, "PR closed without merge; manual recovery required")
		}
		// Persist authoritative merge evidence before attempting an external close.
		if e = s.save(); e != nil {
			return e
		}
		if info.State == "merged" {
			if e = s.beads.Close(ctx, r.TaskID, *r.PR.URL); e != nil {
				r.Error = ptr("PR merged; Beads close will retry: " + clip(e.Error(), 1000))
				operationErr = e
			} else {
				r.BeadsStatus = "closed"
				r.FinishTaskStatus = "completed"
				r.Error = nil
			}
			if e = s.save(); e != nil {
				return e
			}
		}
	}
	return operationErr
}
func (s *Service) event(ev WorkerEvent) error {
	if s.active == nil {
		return nil
	}
	s.active.LastEvent = ptr(ev.Summary)
	if ev.Session != "" {
		s.active.SessionID = ptr(ev.Session)
	}
	return s.save()
}
func (s *Service) Run(ctx context.Context) (retErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if e := s.save(); e != nil {
		return e
	}
	heartDone := make(chan struct{})
	go func() {
		defer close(heartDone)
		t := time.NewTicker(duration(s.Config.Heartbeat))
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if e := s.store.Heartbeat(); e != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		<-heartDone
		s.State.Service.Status = "stopping"
		s.save()
		s.shutdown()
		s.State.Service.Status = "stopped"
		if e := s.store.Err(); e != nil {
			retErr = e
		}
		if retErr != nil {
			s.setError(retErr)
		}
		if e := s.save(); e != nil {
			retErr = e
		}
	}()
	taskTick := time.NewTicker(duration(s.Config.TaskPoll))
	defer taskTick.Stop()
	prTick := time.NewTicker(duration(s.Config.PRPoll))
	defer prTick.Stop()
	stopTick := time.NewTicker(200 * time.Millisecond)
	defer stopTick.Stop()
	step := func(fn func(context.Context) error) error {
		e := fn(ctx)
		if s.store.Err() != nil {
			return s.store.Err()
		}
		if e != nil {
			s.setError(e)
		} else {
			s.State.Service.Status = "running"
			s.State.Service.Error = nil
		}
		return s.save()
	}
	// On startup query saved PRs first, then reconcile claims before any new dispatch.
	if e := step(s.pollPRs); e != nil {
		return e
	}
	if e := step(s.dispatch); e != nil {
		return e
	}
	for {
		var events <-chan WorkerEvent
		var done <-chan error
		if s.worker != nil {
			events = s.worker.Events
			done = s.worker.Done
		}
		select {
		case <-ctx.Done():
			return nil
		case <-stopTick.C:
			if b, e := os.ReadFile(filepath.Join(s.Config.RunDir, "stop.json")); e == nil {
				var request struct {
					InstanceID string `json:"instance_id"`
				}
				if json.Unmarshal(b, &request) == nil && request.InstanceID == s.State.Service.InstanceID {
					return nil
				}
			}
		case ev, ok := <-events:
			if ok {
				if e := s.event(ev); e != nil {
					return e
				}
			}
		case e := <-done:
			if err := s.finished(e); err != nil {
				return err
			}
		case <-taskTick.C:
			if e := step(s.dispatch); e != nil {
				return e
			}
		case <-prTick.C:
			if e := step(s.pollPRs); e != nil {
				return e
			}
		}
	}
}
func (s *Service) shutdown() {
	if s.worker == nil {
		return
	}
	t := time.NewTimer(duration(s.Config.ShutdownGrace))
	defer t.Stop()
	killed := false
	for {
		select {
		case ev, ok := <-s.worker.Events:
			if ok {
				s.event(ev)
			}
		case e := <-s.worker.Done:
			if killed {
				e = errors.New("worker process tree terminated during shutdown")
			}
			s.finished(e)
			return
		case <-t.C:
			s.worker.Kill()
			killed = true
		}
	}
}
func RequestStop(c Config) error {
	v, e := LoadRuntime(filepath.Join(c.RunDir, c.RuntimeFile))
	if e != nil {
		return e
	}
	if v == nil {
		return errors.New("no runtime snapshot")
	}
	b, _ := json.Marshal(map[string]string{"instance_id": v.Service.InstanceID})
	return AtomicWrite(filepath.Join(c.RunDir, "stop.json"), b)
}

// AcceptResult is an explicit operator action, never an automatic retry/resume.
func (s *Service) AcceptResult(ctx context.Context, runID, path string) error {
	for _, r := range s.State.Runs {
		if r.RunID != runID {
			continue
		}
		if r.FinishTaskStatus == "completed" {
			return errors.New("completed runs cannot be changed")
		}
		if inspectProcess(r.Process) == "matching process still exists" {
			return errors.New("original process still exists; stop it first")
		}
		result, e := ReadResult(path, r)
		if e != nil {
			return e
		}
		if result.Outcome != "pr_submitted" {
			return errors.New("manual acceptance requires a submitted PR")
		}
		info, e := s.prs.Query(ctx, *result.PRURL)
		if e != nil {
			return e
		}
		if info.BaseBranch != s.Config.Branch || info.State == "closed_unmerged" {
			return errors.New("PR must target configured branch and be open or merged")
		}
		if e = s.bind(r, result); e != nil {
			return e
		}
		r.ExecutionStatus = "succeeded"
		r.ResultPath = ptr(path)
		r.EndedAt = ptr(now())
		s.State.SlotOccupied = false
		s.State.CurrentTask = nil
		s.State.Service.Status = "stopped"
		return s.save()
	}
	return fmt.Errorf("run %s not found", runID)
}

func (s *Service) MarkStopped(runID string) error {
	for _, r := range s.State.Runs {
		if r.RunID != runID {
			continue
		}
		if r.ExecutionStatus != "unknown" {
			return errors.New("mark-stopped only applies to unknown execution")
		}
		if inspectProcess(r.Process) == "matching process still exists" {
			return errors.New("original process still exists; stop it first")
		}
		r.ExecutionStatus = "failed"
		r.EndedAt = ptr(now())
		s.blocked(r, "Operator confirmed old process tree stopped; task remains claimed for manual repair")
		s.State.SlotOccupied = false
		s.State.CurrentTask = nil
		s.State.Service.Status = "stopped"
		return s.save()
	}
	return fmt.Errorf("run %s not found", runID)
}
