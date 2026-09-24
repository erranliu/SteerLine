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
	Config         Config
	State          Runtime
	store          *Store
	domainStore    *SQLiteStore
	beads          Beads
	prs            *PRClient
	worker         *Worker
	legacyAdapters *legacyAdapterRegistry
	active         *Run
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

func NewService(c Config) (_ *Service, retErr error) {
	if e := c.Completion.validate(); e != nil {
		return nil, e
	}
	if e := preflightExistingState(c.RunDir, c.RuntimeFile); e != nil {
		return nil, e
	}
	if e := c.prepareWorktreeRoot(); e != nil {
		return nil, e
	}
	databasePath := filepath.Join(c.RunDir, "steerline.db")
	if _, statErr := os.Stat(databasePath); os.IsNotExist(statErr) {
		old, loadErr := LoadRuntime(filepath.Join(c.RunDir, c.RuntimeFile))
		if loadErr != nil {
			return nil, loadErr
		}
		if old != nil {
			return nil, errors.New("runtime v5 projection exists without its authoritative SQLite database; restore the database before starting")
		}
	}
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	db, e := OpenSQLiteStore(databasePath)
	if e != nil {
		return nil, e
	}
	defer func() {
		if retErr != nil {
			_ = db.Close()
		}
	}()
	s := &Service{Config: c, domainStore: db, store: &Store{Path: filepath.Join(c.RunDir, c.RuntimeFile), Domain: db, Project: c.identity}, beads: Beads{Runner: r, Command: c.BD, Claimant: c.Claimant}, prs: &PRClient{Config: c.PR, Runner: r}, activities: map[string]*ActivityWriter{}, legacyAdapters: newLegacyAdapterRegistry()}
	projection, _, e := db.LoadProjection(context.Background(), c.identity)
	if e != nil {
		return nil, e
	}
	var old *Runtime
	if len(projection) > 0 {
		var projected Runtime
		if e = decode(projection, &projected); e != nil {
			return nil, fmt.Errorf("invalid SQLite runtime projection: %w", e)
		}
		if projected.SchemaVersion != 5 {
			return nil, fmt.Errorf("unsupported SQLite runtime projection schema %d", projected.SchemaVersion)
		}
		old = &projected
	} else {
		path := filepath.Join(c.RunDir, c.RuntimeFile)
		old, e = LoadRuntime(path)
		if e != nil {
			return nil, e
		}
		if old != nil {
			return nil, errors.New("runtime v5 projection exists without its authoritative SQLite database; restore the database before starting")
		}
	}
	s.State = Runtime{SchemaVersion: 5, Repository: Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.identity}, Tasks: []TaskRecord{}, WorkflowRuns: []WorkflowRunRecord{}, Executions: []ExecutionRecord{}, Attempts: []StepAttemptRecord{}, Decisions: []DecisionRecord{}, Runs: []*Run{}, Control: ControlState{DispatchMode: "auto", Revision: 1, Blockers: []ControlBlocker{}}}
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
		s.State.Revision = old.Revision
		s.State.Tasks = old.Tasks
		s.State.WorkflowRuns = old.WorkflowRuns
		s.State.Executions = old.Executions
		s.State.Attempts = old.Attempts
		s.State.Decisions = old.Decisions
		s.State.Operations = old.Operations
		if old.SchemaVersion != 5 || s.State.Control.DispatchMode != "manual" && s.State.Control.DispatchMode != "auto" || s.State.Control.Revision == 0 {
			return nil, errors.New("invalid runtime v5 control state; preserve the snapshot for recovery")
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
			if r.DomainTaskID == "" {
				for _, task := range s.State.Tasks {
					if task.ExternalID == r.TaskID {
						r.DomainTaskID = task.TaskID
						break
					}
				}
			}
			if r.Backend == "" || r.Protocol == "" || r.ExecutionID == "" {
				for _, execution := range s.State.Executions {
					if execution.WorkflowRunID == r.RunID {
						if r.ExecutionID == "" {
							r.ExecutionID = execution.ExecutionID
						}
						if r.Backend == "" {
							r.Backend = execution.Backend
						}
						if r.Protocol == "" {
							r.Protocol = execution.Protocol
						}
						if r.Agent == "" {
							r.Agent = execution.Agent
						}
						if r.Target.ID == "" {
							r.Target = execution.Target
						}
						break
					}
				}
			}
			if r.WorkflowVersion == "" {
				for _, workflow := range s.State.WorkflowRuns {
					if workflow.WorkflowRunID == r.RunID {
						r.WorkflowVersion = workflow.WorkflowVersion
						break
					}
				}
			}
			if r.AttemptID == "" || r.ControlRevision == 0 || r.DomainTaskID == "" || r.Backend == "" || r.Protocol == "" || r.WorkflowVersion == "" || r.ExecutionID == "" {
				return nil, errors.New("runtime v5 run is missing required execution identity fields")
			}
			if r.Decision != nil && r.Decision.Version == 0 {
				return nil, errors.New("runtime v5 decision is missing its version")
			}
			if e := r.Completion.validate(); e != nil {
				return nil, e
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
				recoverPlannedStep := false
				if r.WorkflowVersion == MinimalWorkflowVersion && r.ExecutionStatus == "pending" && r.FinishTaskStatus == "pending" {
					for _, workflow := range s.State.WorkflowRuns {
						if workflow.WorkflowRunID != r.RunID || workflow.Phase != "executing" {
							continue
						}
						for _, execution := range s.State.Executions {
							if execution.ExecutionID != r.ExecutionID || execution.CurrentStep == "" {
								continue
							}
							for _, attempt := range s.State.Attempts {
								if attempt.ExecutionID == execution.ExecutionID && attempt.AttemptID == r.AttemptID && attempt.StepID == execution.CurrentStep && attempt.Status == "planned" {
									recoverPlannedStep = true
									break
								}
							}
						}
					}
				}
				if !recoverPlannedStep {
					r.ExecutionStatus = "unknown"
					r.FinishTaskStatus = "blocked"
					r.Error = ptr("Interrupted execution: " + inspectProcessWithConfig(c, r.Process) + "; manual recovery required; never auto-resumed")
				}
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
	s.State.SchemaVersion = 5
	s.State.Service = ServiceState{InstanceID: newID(), PID: os.Getpid(), Status: "running", Claimant: c.Claimant, Mode: "maintenance", Ready: false, ControlProtocol: ControlProtocolVersion, Capabilities: []string{"start_task", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service"}, ConfigDigest: c.configDigest, HeartbeatIntervalMS: duration(c.Heartbeat).Milliseconds()}
	identity, err := currentProcessIdentity()
	if err != nil {
		return nil, fmt.Errorf("identify service process: %w", err)
	}
	s.State.Service.ProcessIdentity = identity
	s.reconcileOperationsAfterRestart()
	return s, nil
}

func (s *Service) Close() error {
	if s == nil || s.domainStore == nil {
		return nil
	}
	return s.domainStore.Close()
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
	if err := s.store.Publish(&s.State); err != nil {
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
	if s.activities == nil {
		s.activities = map[string]*ActivityWriter{}
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

// note queues a canonical event for the next domain transaction and mirrors it
// to the optional display journal.
func (s *Service) note(r *Run, source, kind, status, title, detail string, meta map[string]any) {
	ev := ActivityEvent{Source: source, Kind: kind, Status: status, Title: title, Detail: detail, Meta: meta}
	s.recordActivity(r, ev)
	s.emitConsole(ev)
}

func (s *Service) recordActivity(r *Run, ev ActivityEvent) {
	if r == nil {
		return
	}
	ev.V, ev.RunID, ev.TaskID = ActivityVersion, r.RunID, r.TaskID
	if ev.TS.IsZero() {
		ev.TS = now()
	}
	metadata := map[string]any{}
	for key, value := range ev.Meta {
		metadata[key] = value
	}
	metadata["attempt_id"], metadata["execution_id"] = r.AttemptID, r.ExecutionID
	ev.Meta = metadata
	if w := s.activityWriter(r); w != nil {
		written, e := w.WriteEvent(ev)
		if e != nil {
			s.setError(e)
		} else {
			ev.Seq = written.Seq
		}
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		s.setError(err)
		return
	}
	domainTask, executionID := r.DomainTaskID, r.ExecutionID
	if domainTask == "" {
		domainTask = legacyTaskID(s.Config.identity, r)
	}
	if executionID == "" {
		executionID = legacyExecutionID(r.RunID)
	}
	s.State.PendingEvents = append(s.State.PendingEvents, DomainEvent{ProjectIdentity: s.Config.identity, TaskID: domainTask, WorkflowRunID: r.RunID, ExecutionID: executionID, AttemptID: r.AttemptID, ExternalSource: "center:" + r.RunID, ExternalID: newID(), Type: "activity." + ev.Kind, ReceivedAt: ev.TS, Payload: payload})
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
	backendName, protocolName, backendErr := ExecutionBackendKind(s.Config.hostName())
	if backendErr != nil {
		return backendErr
	}
	minimalConfig, minimalErr := s.Config.minimalConfig()
	if minimalErr != nil {
		return minimalErr
	}
	if minimalConfig != nil && backendName != "local_process" {
		return UnsupportedAdapterOperation("minimal-v1", "target-side step execution is not verified for this backend")
	}
	if minimalConfig != nil {
		if err := s.Config.validateMinimalPreflight(*minimalConfig); err != nil {
			return err
		}
	}
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
		domainTaskID, identityMapDigest := "", ""
		if err == nil {
			domainTaskID, identityMapDigest, err = resolveDomainTaskIdentity(s.Config.Repository, s.Config.RunDir, s.Config.identity, source.ProjectID, t.ID)
		}
		if err == nil {
			for _, savedTask := range s.State.Tasks {
				if savedTask.TaskID == domainTaskID && savedTask.ExternalID != t.ID {
					return fmt.Errorf("business task %q is already bound to a different Beads source ID", domainTaskID)
				}
			}
		}
		base := ""
		if err == nil {
			base, err = fetchBase(ctx, s.Config, id)
		}
		if err == nil && minimalConfig != nil {
			err = s.Config.validateMinimalEntriesAtBase(ctx, *minimalConfig, base)
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
		target := ExecutionTarget{ID: "local"}
		if backendName == "orca" {
			target = ExecutionTarget{ID: "local-orca", ConnectionID: "local-orca"}
		}
		r := &Run{TaskID: t.ID, DomainTaskID: domainTaskID, IdentityMapDigest: identityMapDigest, Title: t.Title, RunID: id, AttemptID: id, DomainAttemptNumber: 1, Claimant: s.Config.Claimant, Host: s.Config.hostName(), Backend: backendName, Protocol: protocolName, Agent: s.Config.agentName(), Target: target, BeadsStatus: t.Status, ExecutionStatus: "pending", FinishTaskStatus: "pending", PR: PRState{State: "unknown"}, LogPath: ptr(filepath.Join(dir, "events.jsonl")), ResultPath: ptr(filepath.Join(dir, "result.json"))}
		r.WorkspaceIntent = intent
		r.BeadsContext = &source
		if minimalConfig != nil {
			taskRecord, workflowRecord, executionRecord, prepareAttempt, initErr := InitializeMinimalWorkflow(s.Config.identity, s.Config, t, source, r)
			if initErr != nil {
				return initErr
			}
			r.WorkflowVersion = MinimalWorkflowVersion
			s.State.Tasks = append(s.State.Tasks, taskRecord)
			s.State.WorkflowRuns = append(s.State.WorkflowRuns, workflowRecord)
			s.State.Executions = append(s.State.Executions, executionRecord)
			s.State.Attempts = append(s.State.Attempts, prepareAttempt)
		} else {
			r.WorkflowVersion = "legacy-v1"
		}
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
		claimOperationID, opErr := s.startOperation("claim_task", "claim:"+s.Config.identity+":"+t.ID+":"+id, map[string]any{"task_id": t.ID, "run_id": id, "claimant": s.Config.Claimant, "source_project": source.ProjectID})
		if opErr != nil {
			return opErr
		}
		ok, err := s.beads.Claim(ctx, t.ID)
		if err != nil {
			_ = s.setOperationState(claimOperationID, "outcome_unknown", "", nil, err.Error())
			r.ExecutionStatus = "unknown"
			s.State.SlotOccupied = true
			s.blocked(r, "Claim uncertain: "+err.Error())
			if e = s.save(); e != nil {
				return e
			}
			return err
		}
		if !ok {
			_ = s.setOperationState(claimOperationID, "rejected", "", []byte(`{"claimed":false}`), "claim conflict")
			s.State.RejectedClaimRuns = append(s.State.RejectedClaimRuns, r.RunID)
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
		claimReceipt, _ := json.Marshal(map[string]any{"claimed": true, "task_id": t.ID, "claimant": s.Config.Claimant})
		if err = s.setOperationState(claimOperationID, "confirmed", "", claimReceipt, ""); err != nil {
			return err
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
		if r.WorkflowVersion == MinimalWorkflowVersion {
			if err = s.setTaskStatus(r.DomainTaskID, "in_progress"); err != nil {
				return err
			}
			workflow, execution := s.minimalRecords(r)
			workflow.Phase = "executing"
			workflow.UpdatedAt = now()
			execution.Status = "active"
			execution.UpdatedAt = now()
			prepareAttempt := s.minimalAttempt(r, r.AttemptID)
			if prepareAttempt == nil {
				return errors.New("minimal workflow is missing its prepare attempt")
			}
			if err = s.save(); err != nil {
				return err
			}
			s.note(r, "steer", "prepare", "started", "preparing minimal-v1 workspace", intent.Path, map[string]any{"base_commit": intent.BaseCommit})
			return s.prepareMinimalExecution(ctx, t, r, source)
		}
		adapter, err := s.legacyAdapters.Resolve(backendName, protocolName)
		if err != nil {
			return err
		}
		startOperationID, opErr := s.startOperation("start_execution", "start:"+s.Config.identity+":"+r.RunID+":"+r.AttemptID, map[string]any{"task_id": t.ID, "workflow_run_id": r.RunID, "execution_id": legacyExecutionID(r.RunID), "attempt_id": r.AttemptID, "backend": backendName, "protocol": protocolName, "agent": r.Agent, "target": r.Target, "workspace_intent": r.WorkspaceIntent})
		if opErr != nil {
			return opErr
		}
		if backendName == "orca" {
			s.note(r, "steer", "prepare", "started", "orca worktree create", intent.Branch, map[string]any{"base_commit": intent.BaseCommit, "agent": s.Config.agentName()})
		} else {
			s.note(r, "steer", "prepare", "started", "preparing worktree", intent.Path, map[string]any{"branch": intent.Branch, "base_commit": intent.BaseCommit})
		}
		checkpoint := func(path, branch, base string) error {
			if s.State.Control.Operation != nil {
				s.injectControlFault("after_worktree")
			}
			r.WorkspaceIntent.Path, r.WorkspaceIntent.Branch, r.WorkspaceIntent.BaseCommit = path, branch, base
			r.WorktreePath, r.WorktreeBranch, r.BaseCommit = ptr(path), ptr(branch), ptr(base)
			if saveErr := s.save(); saveErr != nil {
				return saveErr
			}
			title := "worktree ready"
			if backendName == "orca" {
				title = "orca worktree ready"
			}
			s.note(r, "steer", "prepare", "completed", title, path, map[string]any{"branch": branch, "base_commit": base, "backend": backendName, "protocol": protocolName})
			return nil
		}
		started, startErr := adapter.Start(ctx, s.Config, t, r, source, intent, dir, checkpoint)
		if started.Worker != nil {
			s.worker, s.active = started.Worker, r
		}
		if startErr != nil {
			if s.store.Err() != nil || started.Worker != nil {
				return startErr
			}
			if started.Uncertain {
				_ = s.setOperationState(startOperationID, "outcome_unknown", "", nil, startErr.Error())
				r.ExecutionStatus = "unknown"
				s.blocked(r, "Execution start outcome is uncertain: "+startErr.Error())
				s.State.SlotOccupied, s.State.CurrentTask = true, ptr(t.ID)
				s.note(r, "steer", "error", "unknown", "execution start outcome unknown", startErr.Error(), map[string]any{"backend": backendName, "protocol": protocolName})
				return errors.Join(startErr, s.save())
			}
			if !started.Prepared {
				_ = s.setOperationState(startOperationID, "rejected", "", nil, startErr.Error())
				return s.preparationFailed(r, "Worktree preparation failed: "+startErr.Error())
			}
			_ = s.setOperationState(startOperationID, "rejected", "", nil, startErr.Error())
			r.ExecutionStatus = "failed"
			r.EndedAt = ptr(now())
			s.blocked(r, "Worker startup failed: "+startErr.Error())
			s.State.SlotOccupied, s.State.CurrentTask = false, nil
			s.note(r, "steer", "error", "failed", "worker startup failed", startErr.Error(), nil)
			s.closeActivity(r)
			return s.save()
		}
		if started.Worker == nil {
			return errors.New("legacy adapter accepted start without returning a worker handle")
		}
		if s.State.Control.Operation != nil {
			s.injectControlFault("after_worker_start")
		}
		r.ExecutionStatus, r.StartedAt, r.Process, r.AttemptID = "running", ptr(now()), &started.Process, id
		s.lastDispatched = t.ID
		startReceipt, _ := json.Marshal(started.Process)
		if err = s.setOperationState(startOperationID, "confirmed", "", startReceipt, ""); err != nil {
			return err
		}
		if e = s.save(); e != nil {
			return e
		}
		if s.State.Control.Operation != nil {
			s.injectControlFault("worker_persisted")
		}
		metadata := map[string]any{"backend": backendName, "protocol": protocolName, "agent": s.Config.agentName()}
		if started.Process.PID > 0 {
			metadata["pid"] = started.Process.PID
		}
		if started.Process.Terminal != "" {
			metadata["terminal"] = started.Process.Terminal
		}
		startedTitle := "worker started"
		if backendName == "orca" {
			startedTitle = "orca agent started"
		}
		s.note(r, "steer", "worker_start", "started", startedTitle, deref(r.WorktreePath), metadata)
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
	if r != nil && r.WorkflowVersion == MinimalWorkflowVersion {
		return s.finishMinimalStep(err)
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
		if r.WorkflowVersion == MinimalWorkflowVersion && info.State != "closed_unmerged" {
			workflow, execution := s.minimalRecords(r)
			if workflow == nil || execution == nil || workflow.SelectedResult != execution.ExecutionID ||
				execution.Status != "succeeded" || execution.CurrentStep != "waiting_pr_merge" ||
				!revisionPattern.MatchString(execution.CandidateRevision) || info.HeadRevision != execution.CandidateRevision ||
				execution.VerifiedRevision != execution.CandidateRevision || (workflow.Definition.ReviewRequired && execution.ReviewedRevision != execution.CandidateRevision) {
				s.blocked(r, "PR head or selected execution no longer matches the verified candidate; completion refused")
				if e = s.save(); e != nil {
					return e
				}
				continue
			}
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
				if r.WorkflowVersion == MinimalWorkflowVersion {
					workflow, execution := s.minimalRecords(r)
					var task *TaskRecord
					for i := range s.State.Tasks {
						if s.State.Tasks[i].TaskID == r.DomainTaskID {
							task = &s.State.Tasks[i]
							break
						}
					}
					if workflow == nil || execution == nil || task == nil || info.HeadRevision != execution.CandidateRevision {
						e = errors.New("merged PR delivery evidence does not match the selected candidate")
					} else {
						e = CompleteMergedDelivery(task, workflow, execution, DeliveryEvidence{PRURL: info.URL, PublishedHead: info.HeadRevision, MergeCommit: deref(info.MergeCommit), TargetBranch: info.BaseBranch, Merged: info.Merged, ProviderTaskStatus: "closed", ObservedAt: now()})
					}
				}
				if e != nil {
					r.Error = ptr("PR merged; completion will retry: " + clip(e.Error(), 1000))
					operationErr = e
				} else {
					r.BeadsStatus = "closed"
					r.FinishTaskStatus = "completed"
					r.Error = nil
					s.note(r, "steer", "completion", "completed", "task closed", r.TaskID, nil)
					s.closeActivity(r)
				}
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
		s.recordActivity(s.active, ev.Activity)
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
		if e := step(s.processCommandsAndDecisions); e != nil {
			return e
		}
		if e := s.reconcile(ctx); e != nil {
			s.setError(e)
			if saveErr := s.save(); saveErr != nil {
				return saveErr
			}
		}
		if e := step(s.resumeMinimalSteps); e != nil {
			return e
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
			if e := step(s.processCommandsAndDecisions); e != nil {
				return e
			}
		case <-taskTick.C:
			if retryRunID != "" {
				continue
			}
			if e := step(s.processCommandsAndDecisions); e != nil {
				return e
			}
			if e := s.pollControl(ctx, cancel); e != nil {
				return e
			}
			if s.State.Control.Operation != nil {
				continue
			}
			if e := step(s.resumeMinimalSteps); e != nil {
				return e
			}
			if s.worker != nil {
				continue
			}
			if e := step(s.dispatch); e != nil {
				return e
			}
		case <-controlTick.C:
			if e := step(s.processPendingCommands); e != nil {
				return e
			}
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
		if r.WorkflowVersion == MinimalWorkflowVersion {
			return errors.New("minimal-v1 results must pass their current step gates; single-workflow accept-result is unsupported")
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
		if r.WorkflowVersion == MinimalWorkflowVersion {
			return errors.New("minimal-v1 requires reconciliation of the recorded attempt and operation; mark-stopped cannot settle step side effects")
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
