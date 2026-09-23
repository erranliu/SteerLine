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
	// activities holds one journal per run so PR/completion events always land in
	// their own run's file, even after the worker exited.
	activities map[string]*ActivityWriter
	// Console selects service-side rendering: "", "off", "pretty", "summary" or "json".
	Console string
}

func NewService(c Config) (*Service, error) {
	if e := c.Completion.validate(); e != nil {
		return nil, e
	}
	if e := c.prepareWorktreeRoot(); e != nil {
		return nil, e
	}
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	s := &Service{Config: c, store: &Store{Path: filepath.Join(c.RunDir, c.RuntimeFile)}, beads: Beads{Runner: r, Command: c.BD, Claimant: c.Claimant}, prs: &PRClient{Config: c.PR, Runner: r}, activities: map[string]*ActivityWriter{}}
	old, e := LoadRuntime(s.store.Path)
	if e != nil {
		return nil, e
	}
	s.State = Runtime{SchemaVersion: 3, Repository: Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.identity}, Runs: []*Run{}}
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
			if e := r.Completion.validate(); e != nil {
				return nil, e
			}
			// v1/v2 runs predate hosting; they are local by definition.
			if r.Host == "" {
				r.Host = "local"
			}
			if r.Process != nil && r.Process.Kind == "" {
				r.Process.Kind = "os"
			}
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
				r.Error = ptr("Interrupted execution: " + inspectProcessWithConfig(c, r.Process) + "; manual recovery required; never auto-resumed")
			}
			if r.WorkspaceIntent != nil && r.FinishTaskStatus != "completed" {
				err := verifyWorkspace(context.Background(), c, r.WorkspaceIntent, false)
				if err == nil && (r.WorktreePath != nil || r.WorktreeBranch != nil || r.BaseCommit != nil) {
					err = verifyRunWorkspace(context.Background(), c, r)
				}
				if err != nil {
					r.FinishTaskStatus = "blocked"
					previous := ""
					if r.Error != nil {
						previous = *r.Error + "; "
					}
					r.Error = ptr(clip(previous+"Workspace recovery check failed: "+err.Error()+"; no recreation attempted", 2000))
				}
			}
		}
	}
	s.State.Service = ServiceState{InstanceID: newID(), PID: os.Getpid(), Status: "running", Claimant: c.Claimant, Mode: "maintenance"}
	return s, nil
}
func (s *Service) save() error { return s.store.Publish(s.State) }

// activityWriter returns the lazily-opened journal for one run. Opening is
// idempotent and append-only, so a run keeps accumulating lifecycle events after
// its worker exits and after a service restart.
func (s *Service) activityWriter(r *Run) *ActivityWriter {
	if r == nil || r.LogPath == nil || r.RunID == "" {
		return nil
	}
	if w, ok := s.activities[r.RunID]; ok {
		return w
	}
	path := activityPath(*r.LogPath)
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		s.setError(fmt.Errorf("activity log unavailable: %w", e))
		return nil
	}
	w, e := OpenActivity(path, r.RunID, r.TaskID)
	if e != nil {
		s.setError(fmt.Errorf("activity log unavailable: %w", e))
		return nil
	}
	s.activities[r.RunID] = w
	return w
}

func (s *Service) closeActivity(r *Run) {
	if r == nil {
		return
	}
	w, ok := s.activities[r.RunID]
	if !ok {
		return
	}
	if e := w.Close(); e != nil {
		s.setError(fmt.Errorf("activity log close: %w", e))
	}
	delete(s.activities, r.RunID)
}

func (s *Service) closeAllActivities() {
	for id, w := range s.activities {
		if e := w.Close(); e != nil {
			s.setError(fmt.Errorf("activity log close: %w", e))
		}
		delete(s.activities, id)
	}
}

// note records a scheduler lifecycle event on that run's own journal. Activity
// failures are diagnostic: runtime.json and the per-run files stay authoritative.
func (s *Service) note(r *Run, source, kind, status, title, detail string, meta map[string]any) {
	ev := ActivityEvent{Source: source, Kind: kind, Status: status, Title: title, Detail: detail, Meta: meta}
	if w := s.activityWriter(r); w != nil {
		if e := w.Write(ev); e != nil {
			s.setError(e)
		}
	}
	s.emitConsole(ev)
}

func (s *Service) emitConsole(ev ActivityEvent) {
	if s.Console == "" || s.Console == "off" {
		return
	}
	switch s.Console {
	case "json":
		b, e := json.Marshal(ev)
		if e == nil {
			fmt.Fprintln(os.Stdout, string(b))
		}
	case "summary":
		if ev.Source == "steer" {
			fmt.Fprintln(os.Stdout, RenderActivity(ev, RenderOptions{Color: false}))
		}
	default:
		fmt.Fprintln(os.Stdout, RenderActivity(ev, RenderOptions{Color: stdoutColor()}))
	}
}
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
		s.State.SlotOccupied = true
		s.State.CurrentTask = ptr(t.ID)
		if e = s.save(); e != nil {
			return e
		}
		source, err := s.beads.Context(ctx)
		base := ""
		if err == nil {
			base, err = fetchBase(ctx, s.Config, id)
		}
		var intent *WorkspaceIntent
		if err == nil {
			intent, err = planWorkspace(s.Config, t.ID, id, base)
		}
		if err != nil {
			s.State.SlotOccupied = false
			s.State.CurrentTask = nil
			return errors.Join(err, s.save())
		}
		dir := filepath.Join(s.Config.RunDir, "runs", id)
		r := &Run{TaskID: t.ID, Title: t.Title, RunID: id, Claimant: s.Config.Claimant, Host: s.Config.hostName(), BeadsStatus: t.Status, ExecutionStatus: "pending", FinishTaskStatus: "pending", PR: PRState{State: "unknown"}, LogPath: ptr(filepath.Join(dir, "events.jsonl")), ResultPath: ptr(filepath.Join(dir, "result.json"))}
		r.WorkspaceIntent = intent
		r.BeadsContext = &source
		if s.Config.Completion != nil {
			copy := *s.Config.Completion
			copy.Command.Args = append([]string{}, copy.Command.Args...)
			r.Completion = &copy
		}
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
			s.State.SlotOccupied = false
			s.State.CurrentTask = nil
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
		s.note(r, "steer", "claim", "completed", "claimed "+t.ID, "", map[string]any{"beads_status": "in_progress"})
		if ctx.Err() != nil {
			r.ExecutionStatus = "unknown"
			s.blocked(r, "Stopped after claim, before worker launch")
			s.note(r, "steer", "error", "failed", "stopped after claim", "no worker launched", nil)
			s.closeActivity(r)
			return s.save()
		}
		if s.Config.hostName() == "orca" {
			s.note(r, "steer", "prepare", "started", "orca worktree create", intent.Branch, map[string]any{"base_commit": intent.BaseCommit, "agent": s.Config.agentName()})
			w, err := startOrcaRun(ctx, s.Config, t, r, source, intent, dir)
			if err != nil {
				r.ExecutionStatus = "failed"
				r.EndedAt = ptr(now())
				s.blocked(r, "Orca execution failed to start: "+err.Error())
				s.State.SlotOccupied = false
				s.State.CurrentTask = nil
				s.note(r, "steer", "error", "failed", "orca start failed", err.Error(), nil)
				s.closeActivity(r)
				return s.save()
			}
			s.worker = w
			s.active = r
			r.ExecutionStatus = "running"
			r.StartedAt = ptr(now())
			procID := w.proc.Identity()
			r.Process = &procID
			if e = s.save(); e != nil {
				return e
			}
			s.note(r, "steer", "worker_start", "started", "orca agent started", deref(r.WorktreePath), map[string]any{"terminal": procID.Terminal, "agent": s.Config.agentName()})
			return s.save()
		}
		s.note(r, "steer", "prepare", "started", "preparing worktree", intent.Path, map[string]any{"branch": intent.Branch, "base_commit": intent.BaseCommit})
		workerConfig, err := prepareWorkspace(ctx, s.Config, r, source)
		if err != nil {
			return s.preparationFailed(r, "Worktree preparation failed: "+err.Error())
		}
		r.WorktreePath = ptr(intent.Path)
		r.WorktreeBranch = ptr(intent.Branch)
		r.BaseCommit = ptr(intent.BaseCommit)
		if e = s.save(); e != nil {
			return e
		}
		s.note(r, "steer", "prepare", "completed", "worktree ready", intent.Path, map[string]any{"branch": intent.Branch, "base_commit": intent.BaseCommit})
		if ctx.Err() != nil {
			return s.preparationFailed(r, "Stopped after worktree preparation, before worker launch")
		}
		w, err := StartWorker(workerConfig, t, r)
		if err != nil {
			r.ExecutionStatus = "failed"
			r.EndedAt = ptr(now())
			s.blocked(r, "Worker startup failed: "+err.Error())
			s.State.SlotOccupied = false
			s.State.CurrentTask = nil
			s.note(r, "steer", "error", "failed", "worker startup failed", err.Error(), nil)
			s.closeActivity(r)
			return s.save()
		}
		s.worker = w
		s.active = r
		r.ExecutionStatus = "running"
		r.StartedAt = ptr(now())
		procID := w.proc.Identity()
		r.Process = &procID
		s.note(r, "steer", "worker_start", "started", "worker started", intent.Path, map[string]any{"pid": procID.PID})
		return s.save()
	}
	return nil
}

func (s *Service) preparationFailed(r *Run, reason string) error {
	r.ExecutionStatus = "failed"
	r.EndedAt = ptr(now())
	s.blocked(r, reason)
	s.State.SlotOccupied = false
	s.State.CurrentTask = nil
	s.note(r, "steer", "prepare", "failed", "worktree preparation failed", reason, nil)
	s.closeActivity(r)
	return s.save()
}
func (s *Service) bind(r *Run, result Result) error {
	if r.WorkspaceIntent == nil || r.WorktreePath == nil || r.WorktreeBranch == nil || r.BaseCommit == nil {
		return errors.New("run has no verified scheduler-prepared workspace; legacy/orphan result requires manual recovery")
	}
	if (result.WorktreePath != nil && canonical(*result.WorktreePath) != canonical(*r.WorktreePath)) || (result.BaseCommit != nil && *result.BaseCommit != *r.BaseCommit) {
		return errors.New("handoff workspace/base differs from scheduler record")
	}
	if result.Outcome == "pr_submitted" && (result.WorktreePath == nil || result.BaseCommit == nil) {
		return errors.New("handoff missing prepared workspace facts")
	}
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
	if s.worker != nil {
		// The host closes Events before signalling Done, so draining here always
		// terminates and no tail event is lost by the Done-first select race.
		for ev := range s.worker.Events {
			if e := s.event(ev); e != nil {
				s.setError(e)
			}
		}
	}
	s.worker = nil
	s.active = nil
	s.State.CurrentTask = nil
	s.State.SlotOccupied = false
	r.EndedAt = ptr(now())
	if err != nil {
		r.ExecutionStatus = "failed"
		s.blocked(r, "Codex execution failed: "+err.Error())
		s.note(r, "steer", "error", "failed", "worker exited", err.Error(), map[string]any{"execution_status": "failed"})
		s.closeActivity(r)
		return s.save()
	}
	r.ExecutionStatus = "succeeded"
	s.note(r, "steer", "worker_exit", "completed", "worker exited", "", map[string]any{"exit_code": 0})
	result, e := ReadResult(*r.ResultPath, r)
	if e == nil {
		e = verifyRunWorkspace(context.Background(), s.Config, r)
	}
	if e == nil {
		e = s.bind(r, result)
	}
	if e != nil {
		s.blocked(r, "Invalid handoff: "+e.Error())
		s.note(r, "steer", "result", "failed", "invalid handoff", e.Error(), nil)
		s.closeActivity(r)
	} else if r.PR.URL != nil {
		s.note(r, "steer", "result", "completed", "pr submitted", *r.PR.URL, nil)
	} else {
		s.note(r, "steer", "result", "completed", "blocked by workflow", deref(r.Error), nil)
		s.closeActivity(r)
	}
	return s.save()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func (s *Service) pollPRs(ctx context.Context) error {
	return s.pollPRsFor(ctx, "")
}
func (s *Service) pollPRsFor(ctx context.Context, runID string) error {
	var operationErr error
	for _, r := range s.State.Runs {
		if runID != "" && r.RunID != runID {
			continue
		}
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
		previous := r.PR.State
		r.PR.QueryError = nil
		r.PR.State = info.State
		r.PR.MergedAt = info.MergedAt
		r.PR.MergeCommit = info.MergeCommit
		if info.State != previous {
			s.note(r, "steer", "pr", "info", "pr "+info.State, *r.PR.URL, map[string]any{"previous": previous})
		}
		if info.BaseBranch != s.Config.Branch {
			s.blocked(r, "PR target branch mismatch: "+info.BaseBranch)
			s.note(r, "steer", "pr", "failed", "pr target mismatch", info.BaseBranch, nil)
			if e = s.save(); e != nil {
				return e
			}
			continue
		}
		if info.State == "closed_unmerged" {
			s.blocked(r, "PR closed without merge; manual recovery required")
			s.note(r, "steer", "pr", "failed", "pr closed unmerged", *r.PR.URL, nil)
		}
		// Persist authoritative merge evidence before attempting an external close.
		if e = s.save(); e != nil {
			return e
		}
		if info.State == "merged" {
			if e = s.complete(ctx, r, info); e != nil {
				r.Error = ptr("PR merged; completion will retry: " + clip(e.Error(), 1000))
				operationErr = e
			} else {
				r.BeadsStatus = "closed"
				r.FinishTaskStatus = "completed"
				r.Error = nil
				s.note(r, "steer", "completion", "completed", "task closed", r.TaskID, nil)
				s.closeActivity(r)
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
	if ev.Activity.Kind != "" {
		if w := s.activityWriter(s.active); w != nil {
			if e := w.Write(ev.Activity); e != nil {
				s.setError(e)
			}
		}
		s.emitConsole(ev.Activity)
	}
	return s.save()
}
func (s *Service) Run(ctx context.Context) (retErr error) {
	return s.run(ctx, "")
}

// RetryOne is a bounded operator action. It never selects or claims another task.
func (s *Service) RetryOne(ctx context.Context, runID string) error {
	if runID == "" {
		return errors.New("retry requires a run ID")
	}
	return s.run(ctx, runID)
}
func (s *Service) run(ctx context.Context, retryRunID string) (retErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer s.closeAllActivities()
	s.State.Service.Mode = "scheduler"
	if retryRunID != "" {
		s.State.Service.Mode = "retry-one"
	}
	if e := s.save(); e != nil {
		return e
	}
	heartDone := make(chan struct{})
	instanceID := s.State.Service.InstanceID
	go func() {
		defer close(heartDone)
		t := time.NewTicker(duration(s.Config.Heartbeat))
		defer t.Stop()
		stopTick := time.NewTicker(200 * time.Millisecond)
		defer stopTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopTick.C:
				if stopRequested(s.Config, instanceID) {
					cancel()
					return
				}
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
	if retryRunID == "" {
		if e := step(s.pollPRs); e != nil {
			return e
		}
		if e := step(s.dispatch); e != nil {
			return e
		}
	} else {
		if e := s.startRetry(ctx, retryRunID); e != nil {
			return e
		}
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
			if retryRunID != "" && !s.retryWaiting(retryRunID) {
				return nil
			}
		case <-taskTick.C:
			if retryRunID != "" {
				continue
			}
			if e := step(s.dispatch); e != nil {
				return e
			}
		case <-prTick.C:
			if e := step(func(ctx context.Context) error { return s.pollPRsFor(ctx, retryRunID) }); e != nil {
				return e
			}
			if retryRunID != "" && s.worker == nil && !s.retryWaiting(retryRunID) {
				return nil
			}
		}
	}
}

func (s *Service) retryWaiting(id string) bool {
	for _, r := range s.State.Runs {
		if r.RunID == id {
			return r.FinishTaskStatus == "waiting_pr_merge"
		}
	}
	return false
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

func stopRequested(c Config, instanceID string) bool {
	b, e := os.ReadFile(filepath.Join(c.RunDir, "stop.json"))
	if e != nil {
		return false
	}
	var request struct {
		InstanceID string `json:"instance_id"`
	}
	return json.Unmarshal(b, &request) == nil && request.InstanceID == instanceID
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
		if inspectProcessWithConfig(s.Config, r.Process) == "matching process still exists" {
			return errors.New("original process still exists; stop it first")
		}
		result, e := ReadResult(path, r)
		if e != nil {
			return e
		}
		if e = verifyRunWorkspace(ctx, s.Config, r); e != nil {
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
		if inspectProcessWithConfig(s.Config, r.Process) == "matching process still exists" {
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
