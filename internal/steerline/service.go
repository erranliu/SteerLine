package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	Console           string
	consoleOut        io.Writer
	consoleSequence   uint64
	versionPrevious   *Runtime
	controlSeen       map[string]string
	stopControl       *ControlRequest
	stopAcceptedAt    *time.Time
	targetTask        string
	lastDispatched    string
	reservedRunID     string
	candidateSnapshot *TaskSnapshot
	explicitDispatch  bool
	controlFault      func(string)
	startupID         string
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
	s.State = Runtime{SchemaVersion: 4, Repository: Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.identity}, Runs: []*Run{}, Control: ControlState{DispatchMode: "auto", Revision: 1, Blockers: []ControlBlocker{}}}
	if old != nil {
		if old.Repository.Identity != c.identity || old.Repository.Remote != c.Remote || old.Repository.Branch != c.Branch || old.Service.Claimant != c.Claimant {
			return nil, errors.New("runtime repository/branch/claimant mismatch; preserve state and use the original configuration")
		}
		baselineBytes, _ := json.Marshal(old)
		var baseline Runtime
		_ = json.Unmarshal(baselineBytes, &baseline)
		if baseline.Control.DispatchMode == "" {
			baseline.Control.DispatchMode = "auto"
		}
		baseline.Control.Revision = 1
		if baseline.Control.Blockers == nil {
			baseline.Control.Blockers = []ControlBlocker{}
		}
		s.versionPrevious = &baseline
		s.State.Runs = old.Runs
		s.State.Control = old.Control
		if old.SchemaVersion == 4 && (s.State.Control.DispatchMode != "manual" && s.State.Control.DispatchMode != "auto" || s.State.Control.Revision == 0) {
			return nil, errors.New("invalid runtime v4 control state; preserve the snapshot for recovery")
		}
		if s.State.Control.DispatchMode == "" {
			s.State.Control.DispatchMode = "auto"
		}
		s.State.Control.Revision = 1 // A control revision is scoped to its instance.
		if s.State.Control.Blockers == nil {
			s.State.Control.Blockers = []ControlBlocker{}
		}
		if op := s.State.Control.Operation; op != nil {
			if !controlIDPattern.MatchString(op.RequestID) || !validControlAction(op.Action) || (op.Phase != "applying" && op.Phase != "completed") || len(op.RequestDigest) != 64 || op.Action == "start_task" && (!controlIDPattern.MatchString(op.RunID) || op.TaskID == "") || op.Phase == "completed" && op.Result == nil {
				return nil, errors.New("invalid runtime control operation; preserve the snapshot for recovery")
			}
			if op.Phase == "completed" && (op.Result.Version != ControlProtocolVersion || op.Result.RequestID != op.RequestID || op.Result.RequestDigest != op.RequestDigest || op.Result.Action != op.Action || !terminalControlStatus(op.Result.Status)) {
				return nil, errors.New("completed control receipt does not match its operation")
			}
		}
		seen := map[string]bool{}
		for _, r := range s.State.Runs {
			if r == nil || r.TaskID == "" || r.RunID == "" || seen[r.TaskID] || r.Claimant != c.Claimant {
				return nil, errors.New("invalid or duplicate saved run")
			}
			seen[r.TaskID] = true
			if r.AttemptID == "" {
				r.AttemptID = runAttemptID(r)
			}
			if r.ControlRevision == 0 {
				r.ControlRevision = 1
			}
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
			case "pending", "waiting_pr_merge", "waiting_decision", "completed", "blocked":
			default:
				return nil, errors.New("invalid saved finish_task_status")
			}
			if r.FinishTaskStatus == "waiting_pr_merge" && r.PR.URL == nil {
				return nil, errors.New("waiting run lacks PR binding")
			}
			// A decision is never auto-resumed on restart; an interrupted resume is
			// uncertain and must go through manual recovery.
			if r.FinishTaskStatus == "waiting_decision" {
				switch {
				case r.Decision == nil:
					r.FinishTaskStatus = "blocked"
					r.Error = ptr("waiting_decision run lacks decision state; manual recovery required")
				case r.Decision.Status == "resuming":
					r.ExecutionStatus = "unknown"
					r.FinishTaskStatus = "blocked"
					r.Error = ptr("Interrupted while resuming a decision; manual recovery required; never auto-resumed")
				}
			}
			if r.ExecutionStatus == "pending" || r.ExecutionStatus == "running" {
				r.ExecutionStatus = "unknown"
				r.FinishTaskStatus = "blocked"
				r.Error = ptr("Interrupted execution: " + s.inspectRunProcess(r) + "; manual recovery required; never auto-resumed")
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
		if s.State.Control.Operation == nil && s.State.CurrentTask != nil && s.find(*s.State.CurrentTask) == nil {
			// Automatic dispatch persists the reservation before fetch. With no run
			// record there has been no claim; clear only this pre-claim reservation.
			s.State.CurrentTask = nil
			s.State.SlotOccupied = false
		}
	}
	s.State.SchemaVersion = 4
	s.State.Service = ServiceState{InstanceID: newID(), PID: os.Getpid(), Status: "running", Claimant: c.Claimant, Mode: "maintenance", Ready: false, ControlProtocol: ControlProtocolVersion, Capabilities: []string{"start_task", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service"}, ConfigDigest: c.configDigest, HeartbeatIntervalMS: duration(c.Heartbeat).Milliseconds()}
	identity, err := currentProcessIdentity()
	if err != nil {
		return nil, fmt.Errorf("identify service process: %w", err)
	}
	s.State.Service.ProcessIdentity = identity
	return s, nil
}
func runAttemptID(r *Run) string {
	if r.LogPath != nil {
		p := filepath.Clean(*r.LogPath)
		parts := strings.Split(filepath.ToSlash(p), "/")
		for i := 0; i+1 < len(parts); i++ {
			if parts[i] == "attempts" && parts[i+1] != "" {
				return parts[i+1]
			}
		}
	}
	return r.RunID
}

func runVersionShape(r *Run) any {
	if r == nil {
		return nil
	}
	decisionID, decisionStatus := "", ""
	if r.Decision != nil {
		decisionID, decisionStatus = r.Decision.ID, r.Decision.Status
	}
	prURL := ""
	if r.PR.URL != nil {
		prURL = *r.PR.URL
	}
	return struct {
		TaskID, RunID, AttemptID, Execution, Finish, DecisionID, DecisionStatus, PRURL, Host, BeadsStatus string
		WorktreePath, WorktreeBranch, BaseCommit                                                          *string
		Process                                                                                           *ProcessIdentity
		WorkspaceIntent                                                                                   *WorkspaceIntent
	}{r.TaskID, r.RunID, r.AttemptID, r.ExecutionStatus, r.FinishTaskStatus, decisionID, decisionStatus, prURL, r.Host, r.BeadsStatus, r.WorktreePath, r.WorktreeBranch, r.BaseCommit, r.Process, r.WorkspaceIntent}
}
func controlVersionShape(r Runtime) any {
	type runShape struct{ TaskID, RunID, Execution, Finish, Decision string }
	runs := make([]runShape, 0, len(r.Runs))
	for _, run := range r.Runs {
		if run != nil {
			if run.ExecutionStatus != "pending" && run.ExecutionStatus != "running" && run.ExecutionStatus != "unknown" && run.FinishTaskStatus != "waiting_decision" {
				continue // Delivery progress without a slot reservation is unrelated.
			}
			d := ""
			if run.Decision != nil {
				d = run.Decision.Status
			}
			runs = append(runs, runShape{run.TaskID, run.RunID, run.ExecutionStatus, run.FinishTaskStatus, d})
		}
	}
	return struct {
		Slot, Paused bool
		Mode         string
		Current      *string
		Operation    *ControlOperation
		Blockers     []ControlBlocker
		Runs         []runShape
	}{r.SlotOccupied, r.Control.DispatchPaused, r.Control.DispatchMode, r.CurrentTask, r.Control.Operation, r.Control.Blockers, runs}
}
func jsonShape(v any) string { b, _ := json.Marshal(v); return string(b) }
func (s *Service) save() error {
	s.refreshSlotOccupancy()
	if s.versionPrevious != nil {
		prev := *s.versionPrevious
		if jsonShape(controlVersionShape(prev)) != jsonShape(controlVersionShape(s.State)) {
			n := prev.Control.Revision + 1
			if n == 0 {
				n = 1
			}
			s.State.Control.Revision = n
		}
		oldRuns := map[string]*Run{}
		for _, r := range prev.Runs {
			if r != nil {
				oldRuns[r.RunID] = r
			}
		}
		for _, r := range s.State.Runs {
			if r == nil {
				continue
			}
			if before := oldRuns[r.RunID]; before == nil {
				if r.ControlRevision == 0 {
					r.ControlRevision = 1
				}
			} else if jsonShape(runVersionShape(before)) != jsonShape(runVersionShape(r)) {
				r.ControlRevision = before.ControlRevision + 1
				if r.ControlRevision == 0 {
					r.ControlRevision = 1
				}
			}
		}
	} else {
		if s.State.Control.Revision == 0 {
			s.State.Control.Revision = 1
		}
		for _, r := range s.State.Runs {
			if r != nil && r.ControlRevision == 0 {
				r.ControlRevision = 1
			}
		}
	}
	if err := s.store.Publish(s.State); err != nil {
		return err
	}
	b, _ := json.Marshal(s.State)
	var prev Runtime
	if json.Unmarshal(b, &prev) == nil {
		s.versionPrevious = &prev
	}
	return nil
}

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
	out := s.consoleOut
	if out == nil {
		out = os.Stdout
	}
	switch s.Console {
	case "json":
		// The console combines service events with several per-run journals.
		// Give that output stream its own sequence without changing the journals.
		s.consoleSequence++
		ev.V, ev.Seq = ActivityVersion, s.consoleSequence
		if ev.TS.IsZero() {
			ev.TS = now()
		}
		b, e := json.Marshal(ev)
		if e == nil {
			fmt.Fprintln(out, string(b))
		}
	case "summary":
		if ev.Source == "steer" {
			fmt.Fprintln(out, RenderActivity(ev, RenderOptions{Color: false}))
		}
	default:
		fmt.Fprintln(out, RenderActivity(ev, RenderOptions{Color: stdoutColor()}))
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
	reconcileErr := s.reconcile(ctx)
	candidateErr := s.refreshCandidates(ctx)
	if reconcileErr != nil {
		return errors.Join(reconcileErr, candidateErr)
	}
	if candidateErr != nil {
		return candidateErr
	}
	if s.State.Service.Mode == "scheduler" && ctx.Err() == nil {
		s.State.Service.Ready = true
	}
	if (!s.explicitDispatch && s.State.Control.DispatchMode != "auto") || s.State.Control.DispatchPaused {
		return nil
	}
	if s.worker != nil {
		return nil
	}
	if reason := s.slotReason(s.activeControlRequestID()); reason != "" {
		s.State.SlotOccupied = true
		if reason != "waiting for a human decision" {
			return errors.New(reason)
		}
		return nil
	}
	s.State.SlotOccupied = false
	tasks, e := s.beads.Ready(ctx)
	if e != nil {
		return e
	}
	targetFound := s.targetTask == ""
	for _, t := range tasks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !executable(t, s.Config.ExecutableTypes) || s.find(t.ID) != nil {
			continue
		}
		if s.targetTask != "" && t.ID != s.targetTask {
			continue
		}
		targetFound = true
		id := newID()
		if s.reservedRunID != "" {
			id = s.reservedRunID
		}
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
		r := &Run{TaskID: t.ID, Title: t.Title, RunID: id, AttemptID: id, Claimant: s.Config.Claimant, Host: s.Config.hostName(), BeadsStatus: t.Status, ExecutionStatus: "pending", FinishTaskStatus: "pending", PR: PRState{State: "unknown"}, LogPath: ptr(filepath.Join(dir, "events.jsonl")), ResultPath: ptr(filepath.Join(dir, "result.json"))}
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
			if s.targetTask != "" {
				return errClaimConflict
			}
			continue
		}
		if s.State.Control.Operation != nil {
			s.injectControlFault("after_claim")
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
		backend := backendFor(s.Config, source)
		if local, ok := backend.(localExecutionBackend); ok && s.State.Control.Operation != nil {
			local.afterPrepare = func() { s.injectControlFault("after_worktree") }
			backend = local
		}
		backendKind, protocol := backend.Identity()
		opID := attemptOperationID(r.RunID, r.AttemptID)
		target, agent := targetFor(s.Config), agentFor(s.Config)
		spec := AttemptSpec{ProjectID: s.Config.identity, TaskID: t.ID, RunID: r.RunID, AttemptID: r.AttemptID, OperationID: opID, ConfigDigest: s.Config.configDigest, Repository: s.Config.Repository, BaseCommit: intent.BaseCommit, Workspace: *intent, Agent: agent, Protocol: protocol, Target: target, Task: t, Run: cloneRun(*r), Beads: source}
		records := executionRecords{root: s.Config.RunDir}
		payloadDigest := digestValue(spec)
		op := Operation{Version: ExecutionRecordVersion, ID: opID, RunID: r.RunID, AttemptID: r.AttemptID, Backend: backendKind, Protocol: protocol, Target: target, Action: "start", PayloadDigest: payloadDigest, Status: OperationPending, StartedAt: now(), UpdatedAt: now()}
		capabilities, probeErr := backend.Probe(ctx, target, agent)
		if probeErr != nil {
			return probeErr
		}
		if err := records.saveCapabilityReport(capabilities); err != nil {
			return err
		}
		if previous, loadErr := records.loadOperation(r.RunID, r.AttemptID, opID); loadErr == nil {
			if previous.PayloadDigest != payloadDigest || previous.Backend != backendKind || previous.Protocol != protocol || previous.Target != target {
				return errors.New("start operation identity conflicts with its persisted fixed input")
			}
			return fmt.Errorf("start operation is already %s; manual reconciliation required, refusing a second start", previous.Status)
		} else if !os.IsNotExist(loadErr) {
			return loadErr
		}
		// Persist the fixed input before any backend may create a workspace or process.
		if err := records.saveOperation(op); err != nil {
			return err
		}
		if err := s.save(); err != nil {
			return err
		}
		if backendKind == BackendOrca {
			s.note(r, "steer", "prepare", "started", "orca worktree/terminal start", intent.Branch, map[string]any{"base_commit": intent.BaseCommit, "agent": agent.Name})
		} else {
			s.note(r, "steer", "prepare", "started", "preparing local workspace and process", intent.Path, map[string]any{"branch": intent.Branch, "base_commit": intent.BaseCommit})
		}
		op.Status = OperationInFlight
		op.UpdatedAt = now()
		if err := records.saveOperation(op); err != nil {
			return err
		}
		started, startErr := backend.Start(ctx, spec)
		if startErr == nil && s.State.Control.Operation != nil {
			s.injectControlFault("after_worker_start")
		}
		if startErr != nil {
			op.ResidualResources = append([]string(nil), started.ResidualResources...)
			var backendErr *BackendError
			unknown := errors.As(startErr, &backendErr) && backendErr.Code == BackendOutcomeUnknown
			if unknown {
				op.Status, op.Diagnostic = OperationUnknown, startErr.Error()
				r.ExecutionStatus = "unknown"
				s.blocked(r, "Execution start outcome unknown; no replacement start will be attempted: "+startErr.Error())
				s.State.SlotOccupied = true
				s.State.CurrentTask = ptr(t.ID)
			} else {
				op.Status, op.Diagnostic = OperationRejected, startErr.Error()
				r.ExecutionStatus = "failed"
				r.EndedAt = ptr(now())
				s.blocked(r, "Execution backend rejected start: "+startErr.Error())
				s.State.SlotOccupied = false
				s.State.CurrentTask = nil
			}
			op.UpdatedAt = now()
			if err := records.saveOperation(op); err != nil {
				return errors.Join(startErr, err)
			}
			if backendKind == BackendLocal {
				return s.preparationFailed(r, "Local execution start failed: "+startErr.Error())
			}
			s.note(r, "steer", "error", "failed", "execution backend start failed", startErr.Error(), nil)
			s.closeActivity(r)
			return errors.Join(startErr, s.save())
		}
		if started.Disposition != "accepted" || started.Worker == nil {
			op.Status, op.Diagnostic = OperationUnknown, "backend returned no verifiable accepted worker"
			op.ResidualResources = append([]string(nil), started.ResidualResources...)
			op.UpdatedAt = now()
			if err := records.saveOperation(op); err != nil {
				return err
			}
			r.ExecutionStatus = "unknown"
			s.blocked(r, "Backend start receipt is incomplete; refusing replacement execution")
			s.State.SlotOccupied = true
			s.State.CurrentTask = ptr(t.ID)
			return s.save()
		}
		ref := started.Ref
		if ref.RunID != r.RunID || ref.AttemptID != r.AttemptID || ref.Backend != backendKind || ref.Protocol != protocol {
			identity := started.Worker.proc.Identity()
			r.Process = &identity
			r.ExecutionStatus = "unknown"
			s.worker, s.active = started.Worker, r
			s.State.SlotOccupied, s.State.CurrentTask = true, ptr(t.ID)
			op.Status, op.Diagnostic, op.UpdatedAt = OperationUnknown, "backend returned an execution ref that does not match the fixed attempt identity", now()
			opSaveErr := records.saveOperation(op)
			s.blocked(r, "Backend returned a mismatched execution identity; execution retained as unknown")
			return errors.Join(errors.New(op.Diagnostic), opSaveErr, s.save())
		}
		r.WorktreePath = started.Run.WorktreePath
		r.WorktreeBranch = started.Run.WorktreeBranch
		r.BaseCommit = started.Run.BaseCommit
		r.WorkspaceIntent = started.Run.WorkspaceIntent
		r.Process = &ref.External
		r.ExecutionStatus = "running"
		r.StartedAt = ptr(now())
		s.worker, s.active = started.Worker, r
		s.lastDispatched = t.ID
		if err := records.saveBinding(ref); err != nil {
			op.Status, op.Diagnostic = OperationUnknown, "binding persistence failed: "+err.Error()
			op.UpdatedAt = now()
			opSaveErr := records.saveOperation(op)
			r.ExecutionStatus = "unknown"
			s.blocked(r, "Execution started but its binding could not be persisted; execution retained as unknown")
			s.State.SlotOccupied, s.State.CurrentTask = true, ptr(t.ID)
			return errors.Join(err, opSaveErr, s.save())
		}
		op.Status, op.Ref, op.UpdatedAt = OperationAccepted, &ref, now()
		if err := records.saveOperation(op); err != nil {
			op.Status, op.Diagnostic, op.UpdatedAt = OperationUnknown, "accepted operation receipt persistence failed: "+err.Error(), now()
			opSaveErr := records.saveOperation(op)
			r.ExecutionStatus = "unknown"
			s.blocked(r, "Execution started but its operation receipt could not be persisted; execution retained as unknown")
			s.State.SlotOccupied, s.State.CurrentTask = true, ptr(t.ID)
			return errors.Join(err, opSaveErr, s.save())
		}
		// Apply only adapter-observed execution facts to the single service-owned Run.
		if err = s.save(); err != nil {
			return err
		}
		if s.State.Control.Operation != nil {
			s.injectControlFault("worker_persisted")
		}
		if backendKind == BackendOrca {
			s.note(r, "steer", "worker_start", "started", "orca agent started", deref(r.WorktreePath), map[string]any{"terminal": ref.External.Terminal, "agent": agent.Name})
		} else {
			s.note(r, "steer", "prepare", "completed", "local workspace ready", deref(r.WorktreePath), map[string]any{"branch": deref(r.WorktreeBranch), "base_commit": deref(r.BaseCommit)})
			s.note(r, "steer", "worker_start", "started", "local worker started", deref(r.WorktreePath), map[string]any{"pid": ref.External.PID})
		}
		op.Status, op.UpdatedAt = OperationConfirmed, now()
		if err = records.saveOperation(op); err != nil {
			return err
		}
		return s.save()
	}
	if !targetFound {
		return errTaskNotReady
	}
	return nil
}

func (s *Service) injectControlFault(point string) {
	if s.controlFault != nil {
		s.controlFault(point)
	}
}

func (s *Service) activeControlRequestID() string {
	if s.explicitDispatch && s.State.Control.Operation != nil {
		return s.State.Control.Operation.RequestID
	}
	return ""
}
func (s *Service) slotReason(ignoreRequestID string) string {
	return s.executionSlotReason(ignoreRequestID, false)
}

func (s *Service) executionSlotReason(ignoreRequestID string, resumingDecision bool) string {
	if s.worker != nil {
		return "worker is running"
	}
	if op := s.State.Control.Operation; op != nil && op.RequestID != ignoreRequestID {
		if op.Phase == "completed" {
			return "terminal control receipt is pending publication"
		}
		return "control operation is applying"
	}
	if len(s.State.Control.Blockers) > 0 {
		return s.State.Control.Blockers[0].Reason
	}
	for _, r := range s.State.Runs {
		if r == nil {
			continue
		}
		if r.ExecutionStatus == "unknown" {
			return "unresolved unknown execution; manual recovery required"
		}
		if r.ExecutionStatus == "pending" || r.ExecutionStatus == "running" {
			return "execution preparation or supervision is still active"
		}
	}
	for _, r := range s.State.Runs {
		if r != nil && !resumingDecision && r.FinishTaskStatus == "waiting_decision" {
			return "waiting for a human decision"
		}
	}
	if s.State.SlotOccupied && s.State.CurrentTask != nil && s.find(*s.State.CurrentTask) == nil {
		return "execution preparation is still active"
	}
	return ""
}

// Exported occupancy is derived from reservations; it is not itself proof of a
// worker and must not bypass pending decisions or persistent control blockers.
func (s *Service) refreshSlotOccupancy() {
	occupied := s.worker != nil || s.State.Control.Operation != nil || len(s.State.Control.Blockers) > 0
	for _, r := range s.State.Runs {
		if r != nil && (r.ExecutionStatus == "unknown" || r.ExecutionStatus == "pending" || r.ExecutionStatus == "running" || r.FinishTaskStatus == "waiting_decision") {
			occupied = true
		}
	}
	if s.State.SlotOccupied && s.State.CurrentTask != nil && s.find(*s.State.CurrentTask) == nil {
		occupied = true // Reservation saved before the first run/claim exists.
	}
	s.State.SlotOccupied = occupied
}
func (s *Service) refreshCandidates(ctx context.Context) error {
	queried := now()
	old := s.candidateSnapshot
	if old == nil {
		if b, err := os.ReadFile(filepath.Join(s.Config.RunDir, "tasks.json")); err == nil {
			var previous TaskSnapshot
			if decode(b, &previous) == nil {
				old = &previous
			}
		}
	}
	tasks, err := s.beads.Ready(ctx)
	if err != nil {
		if old == nil {
			old = &TaskSnapshot{Tasks: []TaskCandidate{}}
		}
		snapshot := &TaskSnapshot{Version: 1, InstanceID: s.State.Service.InstanceID, RepositoryIdentity: s.State.Repository.Identity, QueriedAt: queried, LastSuccessAt: old.LastSuccessAt, Error: ptr(clip(err.Error(), 1000)), BlockReason: s.slotReason(s.activeControlRequestID()), Tasks: old.Tasks}
		s.candidateSnapshot = snapshot
		return s.writeTaskSnapshot(snapshot)
	}
	candidates := make([]TaskCandidate, 0, len(tasks))
	for _, task := range tasks {
		if !executable(task, s.Config.ExecutableTypes) || s.find(task.ID) != nil {
			continue
		}
		candidates = append(candidates, TaskCandidate{TaskID: task.ID, Title: task.Title, Type: task.Type, Summary: clip(task.Description, 500)})
	}
	snapshot := &TaskSnapshot{Version: 1, InstanceID: s.State.Service.InstanceID, RepositoryIdentity: s.State.Repository.Identity, QueriedAt: queried, LastSuccessAt: &queried, BlockReason: s.slotReason(s.activeControlRequestID()), Tasks: candidates}
	s.candidateSnapshot = snapshot
	return s.writeTaskSnapshot(snapshot)
}
func (s *Service) writeTaskSnapshot(snapshot *TaskSnapshot) error {
	b, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(filepath.Join(s.Config.RunDir, "tasks.json"), append(b, '\n'))
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

// verifyHandoff checks the result's workspace/base claims against the scheduler
// record. It applies to every outcome that carries workspace facts.
func verifyHandoff(r *Run, result Result) error {
	if r.WorkspaceIntent == nil || r.WorktreePath == nil || r.WorktreeBranch == nil || r.BaseCommit == nil {
		return errors.New("run has no verified scheduler-prepared workspace; legacy/orphan result requires manual recovery")
	}
	if (result.WorktreePath != nil && canonical(*result.WorktreePath) != canonical(*r.WorktreePath)) || (result.BaseCommit != nil && *result.BaseCommit != *r.BaseCommit) {
		return errors.New("handoff workspace/base differs from scheduler record")
	}
	if result.Outcome == "pr_submitted" && (result.WorktreePath == nil || result.BaseCommit == nil) {
		return errors.New("handoff missing prepared workspace facts")
	}
	return nil
}

func blockedResultMessage(result Result) string {
	msg := "Repository workflow reported blocked"
	if result.Error != nil {
		msg += ": " + *result.Error
	}
	return msg
}

// awaitDecision persists the structured request and pauses the run. It keeps the
// worktree, terminal and Beads claim so a human answer can resume the same run.
func (s *Service) awaitDecision(r *Run, result Result) error {
	st, e := WriteDecisionRequest(r, *result.Decision)
	if e != nil {
		return e
	}
	r.Decision = st
	r.FinishTaskStatus = "waiting_decision"
	r.Error = nil
	return nil
}

func decisionMeta(r *Run) map[string]any {
	if r.Decision == nil {
		return nil
	}
	opts := make([]map[string]string, 0, len(r.Decision.Request.Options))
	for _, o := range r.Decision.Request.Options {
		opts = append(opts, map[string]string{"id": o.ID, "label": o.Label})
	}
	m := map[string]any{"decision_id": r.Decision.ID, "options": opts}
	if r.Decision.Request.Recommended != "" {
		m["recommended"] = r.Decision.Request.Recommended
	}
	return m
}

func (s *Service) bind(r *Run, result Result) error {
	if e := verifyHandoff(r, result); e != nil {
		return e
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
	if e == nil && result.Summary != nil {
		r.Summary = ptr(clip(*result.Summary, 4096))
	}
	switch {
	case e != nil:
		s.blocked(r, "Invalid handoff: "+e.Error())
		s.note(r, "steer", "result", "failed", "invalid handoff", e.Error(), nil)
		s.closeActivity(r)
	case result.Outcome == "needs_decision":
		if de := s.awaitDecision(r, result); de != nil {
			s.blocked(r, "Invalid decision request: "+de.Error())
			s.note(r, "steer", "result", "failed", "invalid decision request", de.Error(), nil)
			s.closeActivity(r)
		} else {
			s.note(r, "steer", "decision", "pending", "decision requested", r.Decision.Request.Question, decisionMeta(r))
		}
	case result.Outcome == "blocked":
		s.blocked(r, blockedResultMessage(result))
		s.note(r, "steer", "result", "completed", "blocked by workflow", deref(r.Error), nil)
		s.closeActivity(r)
	default:
		if be := s.bind(r, result); be != nil {
			s.blocked(r, "Invalid handoff: "+be.Error())
			s.note(r, "steer", "result", "failed", "invalid handoff", be.Error(), nil)
			s.closeActivity(r)
		} else {
			s.note(r, "steer", "result", "completed", "pr submitted", *r.PR.URL, nil)
		}
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
	return s.RunWithDispatch(ctx, "auto")
}
func (s *Service) RunWithDispatch(ctx context.Context, mode string) error {
	if mode != "manual" && mode != "auto" {
		return errors.New("dispatch mode must be manual or auto")
	}
	return s.run(ctx, "", mode)
}

// RetryOne is a bounded operator action. It never selects or claims another task.
func (s *Service) RetryOne(ctx context.Context, runID string) error {
	if runID == "" {
		return errors.New("retry requires a run ID")
	}
	if s.State.Control.DispatchPaused {
		return errors.New("dispatch is paused; explicit retry is refused")
	}
	if reason := s.slotReason(""); reason != "" {
		return errors.New(reason)
	}
	return s.run(ctx, runID, "manual")
}
func (s *Service) run(ctx context.Context, retryRunID, dispatchMode string) (retErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer s.closeAllActivities()
	s.State.Service.Mode = "scheduler"
	s.State.Control.DispatchMode = dispatchMode
	s.State.Service.Ready = false
	s.State.Service.Status = "running"
	s.State.Service.HeartbeatIntervalMS = duration(s.Config.Heartbeat).Milliseconds()
	if retryRunID != "" {
		s.State.Service.Mode = "retry-one"
	}
	if e := s.save(); e != nil {
		return e
	}
	heartDone := make(chan struct{})
	acceptedStops := make(chan stopAcceptance, 1)
	instanceID := s.State.Service.InstanceID
	go func() {
		defer close(heartDone)
		t := time.NewTicker(duration(s.Config.Heartbeat))
		defer t.Stop()
		stopTick := time.NewTicker(100 * time.Millisecond)
		defer stopTick.Stop()
		stopSeen := map[string]bool{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopTick.C:
				stopReq, _, checkedAt := findStopRequestCached(s.Config, instanceID, stopSeen)
				if stopRequested(s.Config, instanceID) || stopReq != nil {
					if stopReq != nil {
						acceptedStops <- stopAcceptance{Request: *stopReq, At: checkedAt}
					}
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
		if s.stopControl == nil {
			select {
			case accepted := <-acceptedStops:
				s.stopControl = &accepted.Request
				s.stopAcceptedAt = &accepted.At
			default:
			}
		}
		s.State.Service.Status = "stopping"
		s.State.Service.Ready = false
		s.save()
		if e := s.ensureStopApplyingReceipt(); e != nil {
			retErr = errors.Join(retErr, e)
		}
		s.shutdown()
		if s.State.Control.Operation != nil {
			if e := s.recoverControlOperation(); e != nil {
				retErr = errors.Join(retErr, e)
			}
		}
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
		if e := s.finalizeStopControl(); e != nil {
			retErr = errors.Join(retErr, e)
		}
	}()
	taskTick := time.NewTicker(duration(s.Config.TaskPoll))
	defer taskTick.Stop()
	prTick := time.NewTicker(duration(s.Config.PRPoll))
	defer prTick.Stop()
	decisionTick := time.NewTicker(2 * time.Second)
	defer decisionTick.Stop()
	controlTick := time.NewTicker(100 * time.Millisecond)
	defer controlTick.Stop()
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
		if e := s.save(); e != nil {
			return e
		}
		if s.State.Service.Ready {
			return s.publishStartupReady()
		}
		return nil
	}
	if retryRunID == "" {
		for s.State.Control.Operation != nil {
			if e := s.recoverControlOperation(); e != nil {
				if e = s.retainControlPublicationFailure(e); e != nil {
					return e
				}
				select {
				case <-ctx.Done():
					return nil
				case <-controlTick.C:
				}
			}
		}
	}
	// On startup query saved PRs, apply any already-answered decisions, then
	// reconcile claims before any new dispatch.
	if retryRunID == "" {
		if e := step(s.pollPRs); e != nil {
			return e
		}
		if e := step(s.applyDecisions); e != nil {
			return e
		}
		if e := s.reconcile(ctx); e != nil {
			s.setError(e)
			if saveErr := s.save(); saveErr != nil {
				return saveErr
			}
		}
		candidateErr := s.refreshCandidates(ctx)
		if candidateErr != nil {
			s.setError(candidateErr)
			if saveErr := s.save(); saveErr != nil {
				return saveErr
			}
		}
		s.State.Service.Ready = false
		if e := s.save(); e != nil {
			return e
		}
		if e := s.pollControl(ctx, cancel); e != nil {
			return e
		}
		s.State.Service.Ready = candidateErr == nil && ctx.Err() == nil && s.State.Control.Operation == nil
		if e := s.save(); e != nil {
			return e
		}
		if e := s.publishStartupReady(); e != nil {
			return e
		}
		if s.State.Control.Operation == nil {
			if e := step(s.dispatch); e != nil {
				return e
			}
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
		case <-decisionTick.C:
			if s.State.Control.Operation != nil {
				continue
			}
			if e := step(s.applyDecisions); e != nil {
				return e
			}
		case <-taskTick.C:
			if retryRunID != "" {
				continue
			}
			if e := s.pollControl(ctx, cancel); e != nil {
				return e
			}
			if s.State.Control.Operation != nil {
				continue
			}
			if e := step(s.dispatch); e != nil {
				return e
			}
		case <-controlTick.C:
			if retryRunID == "" {
				if e := s.pollControl(ctx, cancel); e != nil {
					return e
				}
			}
		case <-prTick.C:
			if s.State.Control.Operation != nil {
				continue
			}
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
	var stopOp Operation
	for {
		select {
		case ev, ok := <-s.worker.Events:
			if ok {
				s.event(ev)
			}
		case e := <-s.worker.Done:
			if killed {
				confirmed, confirmErr := s.confirmExecutionStop(stopOp)
				if !confirmed {
					s.worker = nil // the observer ended; the execution slot remains reserved.
					s.preserveUnknownStop("worker observer ended without proof that the original execution stopped", confirmErr)
					return
				}
				e = errors.New("worker process tree terminated during shutdown")
			}
			s.finished(e)
			return
		case <-t.C:
			if !killed {
				var stopErr error
				stopOp, stopErr = s.stopActiveExecution(context.Background())
				if stopErr != nil {
					s.setError(stopErr)
				}
				killed = true
				t.Reset(duration(s.Config.ShutdownGrace))
				continue
			}
			s.preserveUnknownStop("execution stop was not confirmed before shutdown timeout", nil)
			return
		}
	}
}

func (s *Service) stopActiveExecution(ctx context.Context) (Operation, error) {
	r := s.active
	if r == nil || s.worker == nil {
		return Operation{}, errors.New("there is no active execution to stop")
	}
	records := executionRecords{root: s.Config.RunDir}
	ref, err := records.loadBinding(r.RunID, r.AttemptID)
	if err != nil {
		return Operation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "active execution has no valid fixed binding", Cause: err}
	}
	if ref.RunID != r.RunID || ref.AttemptID != r.AttemptID || r.Process == nil || ref.External != *r.Process {
		return Operation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "active execution binding does not match runtime identity"}
	}
	backend := backendFor(s.Config, BeadsContext{})
	kind, protocol := backend.Identity()
	if kind != ref.Backend || protocol != ref.Protocol {
		return Operation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "configured backend/protocol differs from fixed execution"}
	}
	opID := "stop-" + r.AttemptID
	payload := struct {
		Policy string
		Grace  string
	}{"local_service_shutdown", s.Config.ShutdownGrace}
	op := Operation{Version: ExecutionRecordVersion, ID: opID, RunID: r.RunID, AttemptID: r.AttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: "stop", PayloadDigest: digestValue(payload), Status: OperationPending, StartedAt: now(), UpdatedAt: now(), Ref: &ref}
	if old, loadErr := records.loadOperation(r.RunID, r.AttemptID, opID); loadErr == nil {
		if old.Action != op.Action || old.PayloadDigest != op.PayloadDigest || old.Target != op.Target {
			return Operation{}, errors.New("stop operation id conflicts with its fixed target/policy")
		}
		return old, nil // Never resend a persisted stop operation.
	} else if !os.IsNotExist(loadErr) {
		return Operation{}, loadErr
	}
	if err = records.saveOperation(op); err != nil {
		return Operation{}, err
	}
	op.Status, op.UpdatedAt = OperationInFlight, now()
	if err = records.saveOperation(op); err != nil {
		return Operation{}, err
	}
	receipt, stopErr := backend.StopExecution(ctx, ref, op, s.worker)
	if stopErr != nil {
		var be *BackendError
		if errors.As(stopErr, &be) && (be.Code == BackendOutcomeUnknown || be.Code == BackendUnreachable || be.Code == BackendIdentityMismatch) {
			op.Status = OperationUnknown
		} else {
			op.Status = OperationRejected
		}
		op.Diagnostic, op.UpdatedAt = stopErr.Error(), now()
		if saveErr := records.saveOperation(op); saveErr != nil {
			return op, errors.Join(stopErr, saveErr)
		}
		return op, stopErr
	}
	switch receipt.Disposition {
	case "confirmed":
		op.Status = OperationConfirmed
	case "accepted":
		op.Status = OperationAccepted
	case "rejected":
		op.Status = OperationRejected
	default:
		op.Status = OperationUnknown
	}
	op.Diagnostic, op.UpdatedAt = receipt.Diagnostic, now()
	if err = records.saveOperation(op); err != nil {
		return op, err
	}
	if op.Status == OperationUnknown || op.Status == OperationRejected {
		return op, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "stop request did not confirm an accepted stop"}
	}
	return op, nil
}

func (s *Service) confirmExecutionStop(op Operation) (bool, error) {
	if op.ID == "" {
		return false, errors.New("stop operation is missing")
	}
	ref := op.Ref
	if ref == nil {
		loaded, err := (executionRecords{root: s.Config.RunDir}).loadBinding(op.RunID, op.AttemptID)
		if err != nil {
			return false, err
		}
		ref = &loaded
	}
	backend := backendFor(s.Config, BeadsContext{})
	obs, err := backend.Observe(context.Background(), *ref)
	records := executionRecords{root: s.Config.RunDir}
	if err != nil {
		op.Status, op.Diagnostic, op.UpdatedAt = OperationUnknown, err.Error(), now()
		saveErr := records.saveOperation(op)
		return false, errors.Join(err, saveErr)
	}
	if obs.Liveness != "exited" {
		op.Status, op.Diagnostic, op.UpdatedAt = OperationUnknown, "original execution has not produced stopped evidence: "+obs.Diagnostic, now()
		saveErr := records.saveOperation(op)
		return false, errors.Join(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: op.Diagnostic}, saveErr)
	}
	op.Status, op.Diagnostic, op.UpdatedAt = OperationConfirmed, obs.Diagnostic, now()
	if err = records.saveOperation(op); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) preserveUnknownStop(reason string, cause error) {
	if s.active == nil {
		return
	}
	r := s.active
	r.ExecutionStatus = "unknown"
	s.blocked(r, reason)
	if cause != nil {
		r.Error = ptr(clip(reason+": "+cause.Error(), 1000))
	}
	s.State.SlotOccupied, s.State.CurrentTask = true, ptr(r.TaskID)
	s.note(r, "steer", "stop", "unknown", "execution stop unconfirmed", reason, nil)
	s.closeActivity(r)
	if err := s.save(); err != nil {
		s.setError(err)
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
		if s.inspectRunProcess(r) == "matching process still exists" {
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
		if s.inspectRunProcess(r) == "matching process still exists" {
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
