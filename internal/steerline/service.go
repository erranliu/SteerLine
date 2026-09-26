package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Service struct {
	Config      Config
	State       Runtime
	store       *Store
	domainStore *SQLiteStore
	prs         *PRClient
	worker      *Worker
	active      *Run
	// activities holds one journal per run so PR and delivery events always land in
	// their own run's file, even after the worker exited.
	activities                 map[string]*ActivityWriter
	conversationMappers        map[string]*ConversationMapper
	conversationReplayRequests chan []conversationJournalSource
	conversationReplayErrors   chan error
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
	if err := requireNativeTaskAuthority(c); err != nil {
		return nil, err
	}
	if _, err := c.validateNativeWorkflow(); err != nil {
		return nil, err
	}
	if err := c.ResolveIdentity(context.Background()); err != nil {
		return nil, err
	}
	if err := validateNativeStateLocation(c); err != nil {
		return nil, err
	}
	if err := preflightNativeStatePath(c); err != nil {
		return nil, err
	}
	if e := c.prepareWorktreeRoot(); e != nil {
		return nil, e
	}
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	db, e := OpenNativeTaskCommandStore(context.Background(), &c)
	if e != nil {
		return nil, e
	}
	defer func() {
		if retErr != nil {
			_ = db.Close()
		}
	}()
	s := &Service{Config: c, domainStore: db, store: &Store{Path: filepath.Join(c.RunDir, c.RuntimeFile), Domain: db, Project: c.identity}, prs: &PRClient{Config: c.PR, Runner: r}, activities: map[string]*ActivityWriter{}, conversationMappers: map[string]*ConversationMapper{}}
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
	}
	s.State = Runtime{SchemaVersion: 5, Repository: Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.identity}, Tasks: []TaskRecord{}, WorkflowRuns: []WorkflowRunRecord{}, Executions: []ExecutionRecord{}, Attempts: []StepAttemptRecord{}, Decisions: []DecisionRecord{}, Runs: []*Run{}, Control: ControlState{DispatchMode: "auto", Revision: 1, Blockers: []ControlBlocker{}}}
	authority, authErr := db.nativeAuthority(context.Background(), c.identity)
	if authErr != nil {
		return nil, authErr
	}
	s.State.Control = ControlState{DispatchMode: authority.DispatchMode, DispatchPaused: authority.DispatchPaused, Revision: authority.AuthorityVersion, Blockers: []ControlBlocker{}}
	if old != nil {
		if old.Repository.Identity != c.identity || old.Repository.Remote != c.Remote || old.Repository.Branch != c.Branch {
			return nil, nativeErr("authority_mismatch", "runtime repository or branch differs from the native authority; preserve state")
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
		if s.State.Control.DispatchMode != "manual" && s.State.Control.DispatchMode != "auto" || s.State.Control.Revision == 0 {
			return nil, nativeErr("unsupported_schema", "invalid native runtime v5 control state; preserve the snapshot")
		}
		s.State.Control.DispatchMode = authority.DispatchMode
		s.State.Control.DispatchPaused = authority.DispatchPaused
		s.State.Control.Revision = authority.AuthorityVersion
		if s.State.Control.Blockers == nil {
			s.State.Control.Blockers = []ControlBlocker{}
		}
		if err := s.hydrateDecisionProjection(context.Background()); err != nil {
			return nil, fmt.Errorf("read authoritative decision state: %w", err)
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
			if r.ManualStop != nil && (!manuallyStopped(r) || r.ExecutionStatus != "failed" && r.ExecutionStatus != "succeeded") {
				return nil, errors.New("invalid manual stop confirmation for current attempt")

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
				return nil, fmt.Errorf("runtime v5 run %s is missing execution identity fields: attempt_id=%t control_revision=%t task_id=%t backend=%t protocol=%t workflow_version=%t execution_id=%t", r.RunID, r.AttemptID != "", r.ControlRevision != 0, r.DomainTaskID != "", r.Backend != "", r.Protocol != "", r.WorkflowVersion != "", r.ExecutionID != "")
			}
			if r.Decision != nil && r.Decision.Version == 0 {
				return nil, errors.New("runtime v5 decision is missing its version")
			}
			if r.Decision != nil && (r.Decision.RequestPath != "" || r.Decision.AnswerPath != "") {
				return nil, errors.New("decision state uses the removed sidecar-answer format; preserve it for manual recovery, no answer file will be imported")
			}
			if r.BeadsContext != nil || r.Completion != nil || r.BeadsStatus != "" || r.IdentityMapDigest != "" || r.DomainTaskID != r.TaskID || r.ResultPath != nil {
				return nil, nativeErr("authority_mismatch", "runtime contains a retired task-source or whole-run result binding; preserve it for manual recovery")
			}
			if r.WorkflowVersion != MinimalWorkflowVersion {
				return nil, nativeErr("unsupported_schema", "runtime contains an execution workflow outside native minimal-v1; preserve it for manual recovery")
			}
			if r.Host != "local" && r.Host != "orca" {
				r.ExecutionStatus = "unknown"
				s.blocked(r, "Run lacks current backend identity; preserve state without migration")
				s.State.SlotOccupied = true
				continue
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
			// A decision is never auto-resumed on restart after its resuming
			// checkpoint. The fixed Operation and external target need reconciliation.
			if r.Decision != nil && r.Decision.Status == "resuming" {
				r.ExecutionStatus = "unknown"
				r.FinishTaskStatus = "blocked"
				r.Error = ptr("Interrupted while resuming a decision; manual recovery required; never auto-resumed")
				s.State.SlotOccupied = true
				s.State.CurrentTask = ptr(r.TaskID)
			} else if r.FinishTaskStatus == "waiting_decision" {
				switch {
				case r.Decision == nil:
					r.FinishTaskStatus = "blocked"
					r.Error = ptr("waiting_decision run lacks decision state; manual recovery required")
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
					r.Error = ptr("Interrupted execution: " + s.inspectRunProcess(r) + "; manual recovery required; never auto-resumed")
				}
			}
			if r.ExecutionStatus == "unknown" {
				s.State.SlotOccupied = true
				s.State.CurrentTask = ptr(r.TaskID)
			}
			if r.WorkspaceIntent != nil && r.Process != nil && r.FinishTaskStatus != "completed" {
				err := s.inspectRunWorkspace(context.Background(), r)
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
	if err := s.refreshNativeSnapshot(context.Background(), false); err != nil {
		return nil, err
	}
	s.State.SchemaVersion = 5
	s.State.Service = ServiceState{InstanceID: newID(), PID: os.Getpid(), Status: "running", Claimant: c.Claimant, Mode: "maintenance", Ready: false, ControlProtocol: ControlProtocolVersion, Capabilities: []string{"answer_decision", "retry_run", "mark_stopped", "resolve_command", "start_task", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service"}, ConfigDigest: c.configDigest, HeartbeatIntervalMS: duration(c.Heartbeat).Milliseconds()}
	identity, err := currentProcessIdentity()
	if err != nil {
		return nil, fmt.Errorf("identify service process: %w", err)
	}
	s.State.Service.ProcessIdentity = identity
	if err := db.ValidateConversationCheckpointFormat(context.Background(), c.identity); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Close() error {
	if s == nil || s.domainStore == nil {
		return nil
	}
	return s.domainStore.Close()
}

func (s *Service) hydrateDecisionProjection(ctx context.Context) error {
	snapshot, err := s.domainStore.Query(ctx, s.Config.identity)
	if err != nil {
		return err
	}
	s.State.Decisions = snapshot.Decisions
	records := make(map[string]DecisionRecord, len(snapshot.Decisions))
	for _, record := range snapshot.Decisions {
		records[record.DecisionID] = record
	}
	for _, run := range s.State.Runs {
		if run == nil || run.Decision == nil {
			continue
		}
		record, ok := records[run.Decision.ID]
		if !ok {
			continue
		}
		request, marshalErr := json.Marshal(run.Decision.Request)
		if marshalErr != nil {
			return marshalErr
		}
		if record.WorkflowRunID != run.RunID || record.ExecutionID != run.ExecutionID || run.Decision.AttemptID != run.AttemptID || record.AttemptID != run.Decision.AttemptID || !bytes.Equal(bytes.TrimSpace(record.Request), bytes.TrimSpace(request)) {
			return fmt.Errorf("decision %s conflicts with the runtime execution, attempt or question identity", run.Decision.ID)
		}
		if record.Version == 0 || !validDecisionStatus(record.Status) {
			return fmt.Errorf("decision %s has invalid authoritative version or status", run.Decision.ID)
		}
		run.Decision.Version = record.Version
		run.Decision.Status = record.Status
		if len(record.Answer) > 0 {
			var answer DecisionAnswer
			if err = decode(record.Answer, &answer); err != nil {
				return fmt.Errorf("decision %s has an invalid saved answer: %w", record.DecisionID, err)
			}
			if answer.DecisionID != record.DecisionID || answer.RunID != run.RunID || validateDecisionAnswer(&run.Decision.Request, &answer) != nil {
				return fmt.Errorf("decision %s saved answer does not match its fixed identity or question", record.DecisionID)
			}
			run.Decision.OptionID = answer.OptionID
			run.Decision.Note = answer.Note
			run.Decision.AnsweredBy = answer.AnsweredBy
			if !answer.AnsweredAt.IsZero() {
				answeredAt := answer.AnsweredAt
				run.Decision.AnsweredAt = &answeredAt
			}
		}
	}
	return nil
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
		TaskID, RunID, AttemptID, Execution, Finish, DecisionID, DecisionStatus, PRURL, Host string
		WorktreePath, WorktreeBranch, BaseCommit                                             *string
		Process                                                                              *ProcessIdentity
		WorkspaceIntent                                                                      *WorkspaceIntent
		ManualStop                                                                           *ManualStopConfirmation
	}{r.TaskID, r.RunID, r.AttemptID, r.ExecutionStatus, r.FinishTaskStatus, decisionID, decisionStatus, prURL, r.Host, r.WorktreePath, r.WorktreeBranch, r.BaseCommit, r.Process, r.WorkspaceIntent, r.ManualStop}
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
	authority, err := s.domainStore.nativeAuthority(context.Background(), s.Config.identity)
	if err != nil {
		return err
	}
	s.State.Control.DispatchMode, s.State.Control.DispatchPaused, s.State.Control.Revision = authority.DispatchMode, authority.DispatchPaused, authority.AuthorityVersion
	oldRuns := map[string]*Run{}
	if s.versionPrevious != nil {
		for _, run := range s.versionPrevious.Runs {
			if run != nil {
				oldRuns[run.RunID] = run
			}
		}
	}
	for _, run := range s.State.Runs {
		if run == nil {
			continue
		}
		if before := oldRuns[run.RunID]; before == nil {
			if run.ControlRevision == 0 {
				run.ControlRevision = 1
			}
		} else if jsonShape(runVersionShape(before)) != jsonShape(runVersionShape(run)) {
			run.ControlRevision = before.ControlRevision + 1
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
	if domainTask == "" || executionID == "" || !stableExecutionID(r.RunID) || !stableExecutionID(r.AttemptID) {
		s.setError(nativeErr("unsupported_schema", "native activity lacks its fixed task, run, execution or attempt identity"))
		return
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
	if err := s.refreshNativeSnapshot(ctx, true); err != nil {
		return err
	}
	if err := s.reconcileNativeCleanup(ctx); err != nil {
		return err
	}
	return s.reconcileExitedNativeStep(ctx)
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
	return s.dispatchNative(ctx)
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
	if operation := s.State.Control.Operation; operation != nil && operation.RequestID != ignoreRequestID {
		if operation.Phase == "completed" {
			return "terminal control receipt is pending publication"
		}
		return "control operation is applying"
	}
	if len(s.State.Control.Blockers) > 0 {
		return s.State.Control.Blockers[0].Reason
	}
	for _, run := range s.State.Runs {
		if run == nil {
			continue
		}
		if run.ExecutionStatus == "unknown" {
			return "unresolved unknown execution; manual recovery required"
		}
		if run.ExecutionStatus == "pending" || run.ExecutionStatus == "running" {
			return "execution preparation or supervision is still active"
		}
	}
	for _, run := range s.State.Runs {
		if run != nil && !resumingDecision && run.FinishTaskStatus == "waiting_decision" {
			return "waiting for a human decision"
		}
	}
	if s.State.SlotOccupied && s.State.CurrentTask != nil && s.find(*s.State.CurrentTask) == nil {
		return "execution preparation is still active"
	}
	return ""
}

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
	tasks, err := (QueryService{Store: s.domainStore, Project: s.Config.identity}).ReadyNativeTasks(ctx)
	if err != nil {
		return err
	}
	candidates := make([]TaskCandidate, 0, len(tasks))
	for _, task := range tasks {
		if !contains(s.Config.ExecutableTypes, task.Type) || s.find(task.TaskID) != nil {
			continue
		}
		candidates = append(candidates, TaskCandidate{TaskID: task.TaskID, Title: task.Title, Type: task.Type})
	}
	s.candidateSnapshot = &TaskSnapshot{Version: 1, InstanceID: s.State.Service.InstanceID, RepositoryIdentity: s.State.Repository.Identity, QueriedAt: queried, LastSuccessAt: &queried, BlockReason: s.slotReason(s.activeControlRequestID()), Tasks: candidates}
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

// verifyHandoff checks the result's workspace/base claims against the scheduler
// record. It applies to every outcome that carries workspace facts.
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

func (s *Service) finished(err error) error {
	run := s.active
	if s.worker != nil {
		for ev := range s.worker.Events {
			if eventErr := s.event(ev); eventErr != nil {
				s.setError(eventErr)
			}
		}
	}
	if run == nil || run.WorkflowVersion != MinimalWorkflowVersion {
		return nativeErr("unsupported_schema", "native service observed a run without the current minimal-v1 workflow; preserve its execution identity")
	}
	if replayErr := s.reconcileConversationRun(run); replayErr != nil {
		s.setError(fmt.Errorf("completed conversation source replay failed: %w", replayErr))
	}
	return s.finishMinimalStep(err)
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
	return s.pollNativePRs(ctx, runID)
}
func (s *Service) event(ev WorkerEvent) error {
	if ev.RunID != "" && ev.AttemptID != "" && (s.active == nil || s.active.RunID != ev.RunID || s.active.AttemptID != ev.AttemptID) {
		err := s.recordHistoricalWorkerEvent(ev)
		ackWorkerEventPersistence(ev, err)
		return err
	}
	if s.active == nil {
		if ev.Persisted != nil {
			err := errors.New("conversation source event has no active run binding")
			ackWorkerEventPersistence(ev, err)
			return err
		}
		return nil
	}
	s.active.LastEvent = ptr(ev.Summary)
	if ev.Session != "" && ev.Source != "orca-terminal-v2" {
		s.active.SessionID = ptr(ev.Session)
	}
	if ev.Activity.Kind != "" {
		conversationErr := s.recordConversationEvent(s.active, ev)
		if conversationErr != nil {
			s.setError(fmt.Errorf("conversation event persistence failed: %w", conversationErr))
		}
		ackWorkerEventPersistence(ev, conversationErr)
		if ev.Persisted != nil && conversationErr != nil {
			return s.save()
		}
		s.recordActivity(s.active, ev.Activity)
		s.emitConsole(ev.Activity)
	} else if ev.Persisted != nil {
		err := errors.New("conversation source event has no activity classification")
		ackWorkerEventPersistence(ev, err)
		return err
	}
	return s.save()
}

func ackWorkerEventPersistence(ev WorkerEvent, err error) {
	if ev.Persisted == nil {
		return
	}
	select {
	case ev.Persisted <- err:
	default:
	}
}

func (s *Service) recordHistoricalWorkerEvent(ev WorkerEvent) error {
	if !stableExecutionID(ev.RunID) || !stableExecutionID(ev.AttemptID) || ev.Activity.Kind == "" {
		if ev.Persisted != nil {
			return errors.New("conversation source identity is incomplete")
		}
		return nil
	}
	var run *Run
	for _, candidate := range s.State.Runs {
		if candidate != nil && candidate.RunID == ev.RunID {
			run = candidate
			break
		}
	}
	if run == nil {
		if ev.Persisted != nil {
			return errors.New("conversation source run binding is unavailable")
		}
		return nil
	}
	// Keep late facts attached to their original attempt in the domain journal.
	attempt := (*StepAttemptRecord)(nil)
	for i := range s.State.Attempts {
		if s.State.Attempts[i].AttemptID == ev.AttemptID && s.State.Attempts[i].ExecutionID == run.ExecutionID {
			attempt = &s.State.Attempts[i]
			break
		}
	}
	if attempt == nil {
		if ev.Persisted != nil {
			return errors.New("conversation source attempt binding is unavailable")
		}
		return nil
	}
	if err := s.recordConversationEvent(run, ev); err != nil {
		return err
	}
	dir := filepath.Join(s.Config.RunDir, "runs", ev.RunID)
	if run.WorkflowVersion == MinimalWorkflowVersion {
		dir = filepath.Join(dir, "steps", ev.AttemptID)
	} else if ev.AttemptID != ev.RunID {
		dir = filepath.Join(dir, "attempts", ev.AttemptID)
	}
	ev.Activity.V, ev.Activity.RunID, ev.Activity.TaskID = ActivityVersion, run.RunID, run.TaskID
	if ev.Activity.TS.IsZero() {
		ev.Activity.TS = now()
	}
	payload, err := json.Marshal(ev.Activity)
	if err != nil {
		return err
	}
	s.State.PendingEvents = append(s.State.PendingEvents, DomainEvent{ProjectIdentity: s.Config.identity, TaskID: run.DomainTaskID, WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: ev.AttemptID, ExternalSource: "center:" + run.RunID, ExternalID: newID(), Type: "activity." + ev.Activity.Kind, ReceivedAt: ev.Activity.TS, Payload: payload})
	if err = s.save(); err != nil {
		return err
	}
	w, err := OpenActivity(filepath.Join(dir, "activity.jsonl"), ev.RunID, run.TaskID)
	if err != nil {
		return err
	}
	return w.Write(ev.Activity)
}
func (s *Service) Run(ctx context.Context) (retErr error) {
	return s.RunWithDispatch(ctx, "auto")
}
func (s *Service) RunWithDispatch(ctx context.Context, mode string) error {
	if mode != "manual" && mode != "auto" {
		return errors.New("dispatch mode must be manual or auto")
	}
	return s.run(ctx, mode)
}

func (s *Service) run(ctx context.Context, dispatchMode string) (retErr error) {
	ctx, cancel := context.WithCancel(ctx)
	var conversationReplayDone <-chan struct{}
	var codexHistoryDone <-chan struct{}
	defer func() {
		cancel()
		if conversationReplayDone != nil {
			<-conversationReplayDone
		}
		if codexHistoryDone != nil {
			<-codexHistoryDone
		}
		s.conversationReplayRequests = nil
		s.conversationReplayErrors = nil
	}()
	defer s.closeAllActivities()
	s.State.Service.Mode = "scheduler"
	if err := s.refreshNativeSnapshot(ctx, true); err != nil {
		return err
	}
	s.State.Service.Ready = false
	s.State.Service.Status = "running"
	s.State.Service.HeartbeatIntervalMS = duration(s.Config.Heartbeat).Milliseconds()
	if e := s.save(); e != nil {
		return e
	}
	s.conversationReplayRequests = make(chan []conversationJournalSource, 16)
	s.conversationReplayErrors = make(chan error, 16)
	replayDone := make(chan struct{})
	conversationReplayDone = replayDone
	replaySources := s.conversationJournalSources()
	go func() {
		defer close(replayDone)
		s.runConversationReplayQueue(ctx, s.conversationReplayRequests, s.conversationReplayErrors)
	}()
	if len(replaySources) > 0 {
		s.conversationReplayRequests <- replaySources
	}
	historyRequests := make(chan []codexHistoryBinding, 1)
	historyDone := make(chan struct{})
	codexHistoryDone = historyDone
	go func() {
		defer close(historyDone)
		s.runCodexHistoryCollector(ctx, historyRequests, s.conversationReplayErrors)
	}()
	historyRequests <- s.codexHistorySources()
	historyTick := time.NewTicker(2 * time.Second)
	defer historyTick.Stop()
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
				if stopReq != nil {
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
	// On startup query saved PRs, apply any already-answered decisions, then
	// reconcile claims before any new dispatch.
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
	var observedWorker *Worker
	var events <-chan WorkerEvent
	for {
		if s.worker != observedWorker {
			observedWorker = s.worker
			events = nil
			if s.worker != nil {
				events = s.worker.Events
			}
		}
		var done <-chan error
		if s.worker != nil {
			done = s.worker.Done
		}
		select {
		case <-ctx.Done():
			return nil
		case <-historyTick.C:
			select {
			case historyRequests <- s.codexHistorySources():
			default:
			}
		case replayErr := <-s.conversationReplayErrors:
			if replayErr != nil {
				if e := step(func(context.Context) error { return replayErr }); e != nil {
					return e
				}
			}
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if e := s.event(ev); e != nil {
				return e
			}
		case e := <-done:
			if err := s.finished(e); err != nil {
				return err
			}
		case <-decisionTick.C:
			if s.State.Control.Operation != nil {
				continue
			}
			if e := step(s.processCommandsAndDecisions); e != nil {
				return e
			}
		case <-taskTick.C:
			if e := step(s.processCommandsAndDecisions); e != nil {
				return e
			}
			if e := step(s.reconcile); e != nil {
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
			if e := s.pollControl(ctx, cancel); e != nil {
				return e
			}
		case <-prTick.C:
			if s.State.Control.Operation != nil {
				continue
			}
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
	var stopOp Operation
	events := (<-chan WorkerEvent)(s.worker.Events)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			s.event(ev)
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
	backend := backendFor(s.Config)
	kind, protocol := backend.Identity()
	if kind != ref.Backend || protocol != ref.Protocol {
		return Operation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "configured backend/protocol differs from fixed execution"}
	}
	opID := "stop-" + r.AttemptID
	payload := struct {
		Policy string
		Grace  string
	}{"fixed_local_execution_stop", s.Config.ShutdownGrace}
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
	records := executionRecords{root: s.Config.RunDir}
	// The adapter already persisted an exact stop receipt. Orca's terminal show
	// may later report an orphaned/disconnected tab even after ptyKilled=true;
	// that weaker observation must not erase the durable close evidence.
	if op.Status == OperationConfirmed {
		persisted, err := records.loadOperation(op.RunID, op.AttemptID, op.ID)
		if err != nil {
			return false, err
		}
		if persisted.Status != OperationConfirmed || persisted.PayloadDigest != op.PayloadDigest || persisted.Target != op.Target || persisted.Ref == nil || op.Ref == nil || digestValue(*persisted.Ref) != digestValue(*op.Ref) {
			return false, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "exact stop confirmation was not durably saved"}
		}
		return true, nil
	}
	ref := op.Ref
	if ref == nil {
		loaded, err := (executionRecords{root: s.Config.RunDir}).loadBinding(op.RunID, op.AttemptID)
		if err != nil {
			return false, err
		}
		ref = &loaded
	}
	backend := backendFor(s.Config)
	obs, err := backend.Observe(context.Background(), *ref)
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
