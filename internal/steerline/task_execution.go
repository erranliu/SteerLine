package steerline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TaskClaimReceipt records reservation, not worker start. The existing
// start_task applied receipt remains gated on a persisted worker identity.
type TaskClaimReceipt struct {
	RequestID        string `json:"request_id"`
	TaskID           string `json:"task_id"`
	TaskVersion      uint64 `json:"task_version"`
	WorkflowRunID    string `json:"workflow_run_id"`
	ExecutionID      string `json:"execution_id"`
	AttemptID        string `json:"attempt_id"`
	BaseCommit       string `json:"base_commit"`
	Status           string `json:"status"`
	StartTaskApplied bool   `json:"start_task_applied"`
}

type TaskClaimOptions struct {
	TaskID                   string
	RequestID                string
	ExpectedTaskVersion      uint64
	ExpectedAuthorityVersion uint64
	RequestedBy              string
	Explicit                 bool
	WorkflowRunID            string
	ExecutionProfile         *ExecutionProfile
}

type NativeDispatchUpdate struct {
	RequestID                string
	ExpectedAuthorityVersion uint64
	Mode                     *string
	Paused                   *bool
	RequestedBy              string
}

func (c CommandService) SetNativeDispatchState(ctx context.Context, update NativeDispatchUpdate) (ProjectTaskAuthority, error) {
	sqlite, ok := c.Store.(*SQLiteStore)
	if !ok || c.Project == "" {
		return ProjectTaskAuthority{}, nativeErr("authority_mismatch", "dispatch control requires the center SQLite project")
	}
	if !controlIDPattern.MatchString(update.RequestID) || update.ExpectedAuthorityVersion == 0 {
		return ProjectTaskAuthority{}, nativeErr("invalid_batch", "dispatch control requires request ID and sampled authority version")
	}
	if update.Mode == nil && update.Paused == nil {
		return ProjectTaskAuthority{}, nativeErr("invalid_batch", "dispatch control must change mode or pause state")
	}
	if update.Mode != nil && *update.Mode != "manual" && *update.Mode != "auto" {
		return ProjectTaskAuthority{}, nativeErr("invalid_batch", "dispatch mode must be auto or manual")
	}
	if strings.TrimSpace(update.RequestedBy) == "" {
		update.RequestedBy = DefaultRequestedBy()
	}
	requestBytes, err := json.Marshal(struct {
		Mode        *string `json:"mode,omitempty"`
		Paused      *bool   `json:"paused,omitempty"`
		RequestedBy string  `json:"requested_by"`
	}{update.Mode, update.Paused, update.RequestedBy})
	if err != nil {
		return ProjectTaskAuthority{}, err
	}
	requestPayload, err := CanonicalJSON(requestBytes)
	if err != nil {
		return ProjectTaskAuthority{}, err
	}
	if old, found, lookupErr := lookupTaskBatchCommand(ctx, sqlite, c.Project, update.RequestID); lookupErr != nil {
		return ProjectTaskAuthority{}, lookupErr
	} else if found {
		return replayNativeDispatchState(old, requestPayload, c.Project, update.RequestedBy)
	}
	graph, err := sqlite.taskGraphSnapshot(ctx, c.Project)
	if err != nil {
		return ProjectTaskAuthority{}, err
	}
	if graph.Authority.AuthorityVersion != update.ExpectedAuthorityVersion {
		return ProjectTaskAuthority{}, nativeErr("stale_version", fmt.Sprintf("expected authority version %d, found %d", update.ExpectedAuthorityVersion, graph.Authority.AuthorityVersion))
	}
	next := graph.Authority
	next.AuthorityVersion++
	if update.Mode != nil {
		next.DispatchMode = *update.Mode
	}
	if update.Paused != nil {
		next.DispatchPaused = *update.Paused
	}
	next.UpdatedAt = time.Now().UTC()
	var result ProjectTaskAuthority
	err = sqlite.Transact(ctx, func(domainTx DomainTx) error {
		tx, ok := domainTx.(*sqliteTx)
		if !ok {
			return nativeErr("authority_mismatch", "dispatch control requires a center SQLite transaction")
		}
		if old, found, lookupErr := lookupTaskBatchCommandTx(ctx, tx, c.Project, update.RequestID); lookupErr != nil {
			return lookupErr
		} else if found {
			value, replayErr := replayNativeDispatchState(old, requestPayload, c.Project, update.RequestedBy)
			if replayErr != nil {
				return replayErr
			}
			result = value
			return taskBatchReplay{Receipt: TaskBatchReceipt{RequestID: update.RequestID, Status: "accepted"}}
		}
		current, readErr := taskGraphSnapshotTx(ctx, tx, c.Project)
		if readErr != nil {
			return readErr
		}
		if current.Authority.AuthorityVersion != update.ExpectedAuthorityVersion {
			return nativeErr("stale_version", "native dispatch authority changed during control preflight")
		}
		command := CommandRecord{ProjectIdentity: c.Project, CommandID: update.RequestID, Kind: "set_dispatch_state", TargetID: c.Project, ExpectedVersion: update.ExpectedAuthorityVersion, IdempotencyKey: update.RequestID, RequestedBy: update.RequestedBy, Payload: requestPayload, Status: "in_flight", CreatedAt: next.UpdatedAt}
		if _, made, enqueueErr := tx.EnqueueCommand(command); enqueueErr != nil {
			return nativeErr("idempotency_conflict", enqueueErr.Error())
		} else if !made {
			return nativeErr("idempotency_conflict", "dispatch request ID is already in use")
		}
		operation := OperationRecord{ProjectIdentity: c.Project, OperationID: update.RequestID, IdempotencyKey: update.RequestID, Kind: "set_dispatch_state", Status: "confirmed", PayloadDigest: digestCanonical(requestPayload), Request: requestPayload, CreatedAt: next.UpdatedAt, UpdatedAt: next.UpdatedAt}
		if _, made, putErr := tx.PutOperation(operation); putErr != nil {
			return putErr
		} else if !made {
			return nativeErr("idempotency_conflict", "dispatch operation request ID is already in use")
		}
		if writeErr := tx.UpdateProjectTaskAuthorityCAS(next, update.ExpectedAuthorityVersion); writeErr != nil {
			return writeErr
		}
		command.Status = "accepted"
		command.Result, _ = json.Marshal(next)
		command.UpdatedAt = next.UpdatedAt
		if writeErr := tx.UpdateCommand(command); writeErr != nil {
			return writeErr
		}
		result = next
		body, _ := json.Marshal(next)
		_, writeErr := tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, ExternalSource: "native-task-command", ExternalID: update.RequestID, Type: "project.dispatch.updated", ReceivedAt: next.UpdatedAt, Payload: body})
		return writeErr
	})
	var replay taskBatchReplay
	if errors.As(err, &replay) {
		return result, nil
	}
	return result, err
}

func replayNativeDispatchState(old CommandRecord, requestPayload []byte, project, requestedBy string) (ProjectTaskAuthority, error) {
	if old.ProjectIdentity != project || old.Kind != "set_dispatch_state" || old.IdempotencyKey == "" || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, requestPayload) || old.Status != "accepted" || len(old.Result) == 0 {
		return ProjectTaskAuthority{}, nativeErr("idempotency_conflict", "dispatch request ID was used for a different command")
	}
	var result ProjectTaskAuthority
	if err := decode(old.Result, &result); err != nil || result.ProjectIdentity != project {
		return ProjectTaskAuthority{}, nativeErr("unsupported_schema", "saved dispatch command receipt is invalid")
	}
	return result, nil
}

// ClaimNativeTask reserves one ready task after verifying the pinned contract,
// the fetched target base, and every predecessor merge commit. It does not
// wait for a workspace or worker inside SQLite.
func (c CommandService) ClaimNativeTask(ctx context.Context, config Config, options TaskClaimOptions) (TaskClaimReceipt, error) {
	sqlite, ok := c.Store.(*SQLiteStore)
	if !ok || c.Project == "" || config.Identity() != c.Project {
		return TaskClaimReceipt{}, nativeErr("authority_mismatch", "claim requires the configured native project")
	}
	if config.hostName() == "orca_supervised" && !config.WorkspaceDirtyIgnore.empty() {
		return TaskClaimReceipt{}, nativeErr("host_unsupported", "supervised Orca cannot independently verify workspace_dirty_ignore paths")
	}
	if err := config.WorkspaceDirtyIgnore.validate(); err != nil {
		return TaskClaimReceipt{}, nativeErr("contract_mismatch", err.Error())
	}
	if err := ValidateNativeTaskID(options.TaskID); err != nil {
		return TaskClaimReceipt{}, err
	}
	if !controlIDPattern.MatchString(options.RequestID) || options.ExpectedTaskVersion == 0 || options.ExpectedAuthorityVersion == 0 || options.WorkflowRunID != "" && !stableExecutionID(options.WorkflowRunID) {
		return TaskClaimReceipt{}, nativeErr("invalid_batch", "claim requires request ID and current task/project versions")
	}
	if options.ExecutionProfile != nil && !options.ExecutionProfile.MatchesConfig(config) {
		return TaskClaimReceipt{}, nativeErr("contract_mismatch", "selected execution profile differs from the claim configuration")
	}
	if strings.TrimSpace(options.RequestedBy) == "" {
		options.RequestedBy = DefaultRequestedBy()
	}
	requestBody := struct {
		TaskID        string `json:"task_id"`
		RequestedBy   string `json:"requested_by"`
		Explicit      bool   `json:"explicit"`
		WorkflowRunID string `json:"workflow_run_id,omitempty"`
		ProfileID     string `json:"execution_profile_id,omitempty"`
		ProfileDigest string `json:"execution_profile_digest,omitempty"`
	}{TaskID: options.TaskID, RequestedBy: options.RequestedBy, Explicit: options.Explicit, WorkflowRunID: options.WorkflowRunID}
	if options.ExecutionProfile != nil {
		requestBody.ProfileID = options.ExecutionProfile.ID
		requestBody.ProfileDigest = options.ExecutionProfile.Digest()
	}
	requestRaw, _ := json.Marshal(requestBody)
	requestPayload, err := CanonicalJSON(requestRaw)
	if err != nil {
		return TaskClaimReceipt{}, err
	}
	if old, found, e := lookupTaskBatchCommand(ctx, sqlite, c.Project, options.RequestID); e != nil {
		return TaskClaimReceipt{}, e
	} else if found {
		return replayTaskClaim(old, requestPayload, options.TaskID, options.RequestedBy)
	}

	graph, err := sqlite.taskGraphSnapshot(ctx, c.Project)
	if err != nil {
		return TaskClaimReceipt{}, err
	}
	if graph.Authority.AuthorityVersion != options.ExpectedAuthorityVersion {
		return TaskClaimReceipt{}, nativeErr("stale_version", fmt.Sprintf("expected authority version %d, found %d", options.ExpectedAuthorityVersion, graph.Authority.AuthorityVersion))
	}
	if err = validateNativeConfiguredTarget(ctx, config, graph.Authority); err != nil {
		return TaskClaimReceipt{}, err
	}
	task, exists := graph.ByID[options.TaskID]
	if !exists {
		return TaskClaimReceipt{}, nativeErr("task_not_found", "native task was not found")
	}
	if task.Status == "discarded" {
		return TaskClaimReceipt{}, nativeErr("task_discarded", "discarded task cannot be claimed")
	}
	if task.Status != "open" || task.Version != options.ExpectedTaskVersion {
		return TaskClaimReceipt{}, nativeErr("stale_version", fmt.Sprintf("task is %s at version %d", task.Status, task.Version))
	}
	if nativeDeliveryInconsistent(task, graph) {
		return TaskClaimReceipt{}, nativeErr("inconsistent_delivery", "task state conflicts with its delivery record")
	}
	if graph.Authority.DispatchPaused {
		return TaskClaimReceipt{}, nativeErr("dispatch_paused", "dispatch is paused")
	}
	if options.Explicit && graph.Authority.DispatchMode != "manual" || !options.Explicit && graph.Authority.DispatchMode != "auto" {
		return TaskClaimReceipt{}, nativeErr("wrong_mode", "claim mode does not match project dispatch mode")
	}
	if ready, blockers := taskDependencyState(task, graph); !ready {
		return TaskClaimReceipt{}, nativeDependencyClaimError(blockers)
	}
	allowed := map[string]bool{}
	types := config.ExecutableTypes
	if len(types) == 0 {
		types = []string{"task", "bug", "feature", "chore"}
	}
	for _, typ := range types {
		allowed[typ] = true
	}
	if !allowed[task.Type] {
		return TaskClaimReceipt{}, nativeErr("task_not_ready", "task type is not executable in this project")
	}
	contract, err := nativeContractForTask(ctx, sqlite, c.Project, task)
	if err != nil {
		return TaskClaimReceipt{}, err
	}
	if err = VerifyTaskContractFiles(ctx, config, contract); err != nil {
		return TaskClaimReceipt{}, err
	}
	var workflow *MinimalWorkflowConfig
	if config.WorkflowProtocol != WorkerFlowVersion {
		workflow, err = config.validateNativeWorkflow()
		if err != nil || workflow == nil {
			return TaskClaimReceipt{}, nativeErr("task_not_ready", "native claim requires project_steps with the native-task/v1 worker context")
		}
		if err = config.validateMinimalPreflight(*workflow); err != nil {
			return TaskClaimReceipt{}, nativeErr("task_not_ready", "fixed workflow capability preflight failed: "+err.Error())
		}
	}
	base, err := acquireNativeBase(ctx, config)
	if err != nil {
		return TaskClaimReceipt{}, nativeErr("task_not_ready", "fixed target branch base could not be acquired: "+err.Error())
	}
	if workflow != nil {
		if err = config.validateMinimalEntriesAtBase(ctx, *workflow, base); err != nil {
			return TaskClaimReceipt{}, nativeErr("task_not_ready", "workflow step entry is not present at the fixed base: "+err.Error())
		}
	}
	predecessorMerges := map[string]string{}
	for _, edge := range graph.Edges {
		if edge.SuccessorID != task.TaskID {
			continue
		}
		pred, ok := graph.ByID[edge.PredecessorID]
		delivery, delivered := graph.Deliveries[edge.PredecessorID]
		if !ok || !delivered || pred.Status != "closed" {
			return TaskClaimReceipt{}, nativeErr("task_not_ready", "predecessor delivery changed during claim preflight")
		}
		predecessorMerges[edge.PredecessorID] = delivery.MergeCommit
		r := Runner{Dir: config.Repository, Timeout: duration(config.CommandTimeout)}
		if _, e := r.Output(ctx, config.Git, "merge-base", "--is-ancestor", delivery.MergeCommit, base); e != nil {
			return TaskClaimReceipt{}, nativeErr("base_missing_delivery", "acquired branch base does not contain a verified predecessor merge commit")
		}
	}
	backend, protocol, err := ExecutionBackendKind(config.hostName())
	if err != nil {
		return TaskClaimReceipt{}, err
	}
	if config.WorkflowProtocol == WorkerFlowVersion && backend == string(BackendLocal) {
		protocol = string(ProtocolLocalCodexAppServer)
	}
	target := targetFor(config)
	target.Branch, target.BaseRevision = config.Branch, base
	claimant := strings.TrimSpace(config.Claimant)
	if claimant == "" {
		claimant = options.RequestedBy
	}
	created := time.Now().UTC()
	workflowRunID, executionID, attemptID := options.WorkflowRunID, newID(), newID()
	if workflowRunID == "" {
		workflowRunID = newID()
	}
	runner := StepRunnerSettings{Codex: config.Codex, Sandbox: config.Sandbox, Model: config.Model, ReasoningEffort: config.ReasoningEffort, Profile: config.Profile}
	runner.Codex.Args = append([]string(nil), config.Codex.Args...)
	wfr := WorkflowRunRecord{ProjectIdentity: c.Project, WorkflowRunID: workflowRunID, TaskID: task.TaskID, WorkflowVersion: MinimalWorkflowVersion, ConfigDigest: config.configDigest, Runner: runner, Phase: "queued", Repository: config.Repository, Branch: config.Branch, CreatedAt: created, UpdatedAt: created}
	if workflow != nil {
		wfr.Definition, wfr.ProjectSteps, wfr.WorkspaceDirtyIgnore = workflow.Definition, workflow.Steps, config.WorkspaceDirtyIgnore
	} else {
		wfr.WorkflowVersion, wfr.SelectedResult = WorkerFlowVersion, executionID
	}
	if options.ExecutionProfile != nil {
		fixed := options.ExecutionProfile.Freeze()
		wfr.ExecutionProfile = &fixed
	}
	execution := ExecutionRecord{ProjectIdentity: c.Project, ExecutionID: executionID, WorkflowRunID: workflowRunID, Agent: config.agentName(), Backend: string(backend), Protocol: string(protocol), Target: target, Workspace: WorkspaceEvidence{Repository: config.identity, Branch: config.Branch, Base: base}, Status: "pending", CurrentStep: "prepare", CreatedAt: created, UpdatedAt: created}
	attempt := StepAttemptRecord{ProjectIdentity: c.Project, AttemptID: attemptID, ExecutionID: executionID, StepID: "prepare", AttemptNumber: 1, Status: "planned", InputRevision: base, ContractDigest: contract.Digest, ExternalBinding: target, CreatedAt: created, UpdatedAt: created}
	if workflow == nil {
		execution.CurrentStep, attempt.StepID = "worker", "worker"
	}
	nextTask := task
	nextTask.Status, nextTask.Claimant, nextTask.Version, nextTask.UpdatedAt = "in_progress", claimant, task.Version+1, created
	operationRequest, _ := json.Marshal(map[string]any{"task_id": task.TaskID, "task_version": options.ExpectedTaskVersion, "authority_version": options.ExpectedAuthorityVersion, "workflow_run_id": workflowRunID, "execution_id": executionID, "attempt_id": attemptID, "base_commit": base, "predecessor_merge_commits": predecessorMerges, "requested_by": options.RequestedBy})
	var receipt TaskClaimReceipt
	err = sqlite.Transact(ctx, func(domainTx DomainTx) error {
		tx, ok := domainTx.(*sqliteTx)
		if !ok {
			return errors.New("claim requires the center SQLite transaction")
		}
		if old, found, e := lookupTaskBatchCommandTx(ctx, tx, c.Project, options.RequestID); e != nil {
			return e
		} else if found {
			r, e := replayTaskClaim(old, requestPayload, options.TaskID, options.RequestedBy)
			if e != nil {
				return e
			}
			receipt = r
			return taskBatchReplay{Receipt: TaskBatchReceipt{RequestID: options.RequestID, Status: "accepted"}}
		}
		current, e := taskGraphSnapshotTx(ctx, tx, c.Project)
		if e != nil {
			return e
		}
		currentTask, ok := current.ByID[task.TaskID]
		if !ok {
			return nativeErr("task_not_found", "native task was not found")
		}
		if current.Authority.AuthorityVersion != options.ExpectedAuthorityVersion || current.Authority.GraphRevision != graph.Authority.GraphRevision || currentTask.Version != options.ExpectedTaskVersion || currentTask.Status != "open" {
			return nativeErr("stale_version", "task or project authority changed during claim preflight")
		}
		if nativeDeliveryInconsistent(currentTask, current) {
			return nativeErr("inconsistent_delivery", "task state conflicts with its delivery record")
		}
		if current.Authority.DispatchPaused {
			return nativeErr("dispatch_paused", "dispatch was paused before claim")
		}
		if options.Explicit && current.Authority.DispatchMode != "manual" || !options.Explicit && current.Authority.DispatchMode != "auto" {
			return nativeErr("wrong_mode", "project dispatch mode changed before claim")
		}
		ready, blockers := taskDependencyState(currentTask, current)
		if !ready {
			return nativeDependencyClaimError(blockers)
		}
		for pred, merge := range predecessorMerges {
			if current.Deliveries[pred].MergeCommit != merge {
				return nativeErr("base_missing_delivery", "predecessor delivery identity changed during claim preflight")
			}
		}
		var occupied int
		if e = tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM executions e JOIN workflow_runs w ON w.project_identity=e.project_identity AND w.workflow_run_id=e.workflow_run_id WHERE e.project_identity=? AND e.status IN ('pending','active','waiting_decision','blocked')`, c.Project).Scan(&occupied); e != nil {
			return e
		}
		if occupied > 0 && !options.Explicit {
			return nativeErr("slot_busy", "native execution slot is occupied or uncertain")
		}
		command := CommandRecord{ProjectIdentity: c.Project, CommandID: options.RequestID, Kind: "claim_task", TargetID: task.TaskID, ExpectedVersion: options.ExpectedTaskVersion, IdempotencyKey: options.RequestID, RequestedBy: options.RequestedBy, Payload: requestPayload, Status: "in_flight", CreatedAt: created}
		var made bool
		command, made, e = tx.EnqueueCommand(command)
		if e != nil {
			return nativeErr("idempotency_conflict", e.Error())
		}
		if !made {
			return nativeErr("idempotency_conflict", "claim request ID is already in use")
		}
		operation := OperationRecord{ProjectIdentity: c.Project, OperationID: options.RequestID, IdempotencyKey: options.RequestID, Kind: "claim_task", Status: "pending", PayloadDigest: digestCanonical(operationRequest), Request: operationRequest, CreatedAt: created, UpdatedAt: created}
		if _, made, e = tx.PutOperation(operation); e != nil {
			return e
		} else if !made {
			return nativeErr("idempotency_conflict", "claim operation request ID is already in use")
		}
		if e = tx.UpdateNativeTaskCAS(nextTask, options.ExpectedTaskVersion); e != nil {
			return e
		}
		if e = tx.PutWorkflowRun(wfr); e != nil {
			return e
		}
		if e = tx.PutExecution(execution); e != nil {
			return e
		}
		if e = tx.PutStepAttempt(attempt); e != nil {
			return e
		}
		receipt = TaskClaimReceipt{RequestID: options.RequestID, TaskID: task.TaskID, TaskVersion: nextTask.Version, WorkflowRunID: workflowRunID, ExecutionID: executionID, AttemptID: attemptID, BaseCommit: base, Status: "claim_reserved", StartTaskApplied: false}
		command.Result, _ = json.Marshal(receipt)
		command.Status = "accepted"
		if e = tx.UpdateCommand(command); e != nil {
			return e
		}
		body, _ := json.Marshal(receipt)
		_, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: task.TaskID, WorkflowRunID: workflowRunID, ExecutionID: executionID, AttemptID: attemptID, ExternalSource: "native-task-command", ExternalID: options.RequestID, Type: "task.claim.reserved", ReceivedAt: created, Payload: body})
		return e
	})
	var replay taskBatchReplay
	if errors.As(err, &replay) {
		return receipt, nil
	}
	return receipt, err
}

func nativeDependencyClaimError(blockers []TaskDependencyStatus) error {
	for _, blocker := range blockers {
		switch blocker.Blocker {
		case "discarded_predecessor":
			return nativeErr("discarded_predecessor", "a discarded predecessor blocks this task")
		case "inconsistent_delivery":
			return nativeErr("inconsistent_delivery", "a predecessor's state conflicts with its delivery record")
		}
	}
	return nativeErr("task_not_ready", "one or more predecessors lack verified delivery")
}

func replayTaskClaim(old CommandRecord, payload []byte, taskID string, requestedBy string) (TaskClaimReceipt, error) {
	if old.Kind != "claim_task" || old.TargetID != taskID || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, payload) {
		return TaskClaimReceipt{}, nativeErr("idempotency_conflict", "request ID was reused with different claim content")
	}
	if old.Status != "accepted" || len(old.Result) == 0 {
		return TaskClaimReceipt{}, nativeErr("idempotency_conflict", "claim request has no committed result")
	}
	var receipt TaskClaimReceipt
	if err := json.Unmarshal(old.Result, &receipt); err != nil {
		return receipt, nativeErr("unsupported_schema", "saved claim receipt is unreadable")
	}
	return receipt, nil
}

func acquireNativeBase(ctx context.Context, c Config) (string, error) {
	ref := fmt.Sprintf("refs/remotes/%s/%s", c.Remote, c.Branch)
	spec := fmt.Sprintf("+refs/heads/%s:%s", c.Branch, ref)
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	if _, err := r.Output(ctx, c.Git, "fetch", "--no-tags", "--", c.Remote, spec); err != nil {
		return "", err
	}
	output, err := r.Output(ctx, c.Git, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	base := strings.TrimSpace(string(output))
	if !validCommit(base) {
		return "", errors.New("remote branch did not resolve to a full commit")
	}
	return base, nil
}

func nativeContractForTask(ctx context.Context, s *SQLiteStore, project string, task TaskRecord) (TaskContract, error) {
	var body []byte
	var digest string
	err := s.db.QueryRowContext(ctx, `SELECT digest,payload FROM task_contracts WHERE project_identity=? AND contract_id=? AND task_id=?`, project, task.ContractID, task.TaskID).Scan(&digest, &body)
	if err != nil {
		return TaskContract{}, nativeErr("contract_mismatch", "native task has no matching fixed contract")
	}
	var contract TaskContract
	if err = json.Unmarshal(body, &contract); err != nil {
		return contract, err
	}
	if contract.Digest != digest {
		return contract, nativeErr("contract_mismatch", "fixed task contract digest differs from its stored row")
	}
	if err = ValidateTaskContract(contract); err != nil {
		return contract, err
	}
	return contract, nil
}

type TaskDeliveryVerifier interface {
	Query(context.Context, string) (PRInfo, error)
}

type TaskDeliveryReceipt struct {
	TaskID        string             `json:"task_id"`
	TaskVersion   uint64             `json:"task_version"`
	WorkflowRunID string             `json:"workflow_run_id"`
	GraphRevision uint64             `json:"graph_revision"`
	Status        string             `json:"status"`
	Delivery      TaskDeliveryRecord `json:"delivery"`
}

// CompleteNativeTask verifies the exact selected PR outside SQLite, then
// rechecks the task, selected workflow result and fixed gate evidence in one
// transaction before writing delivery, closed state and graph revision.
func (c CommandService) CompleteNativeTask(ctx context.Context, config Config, taskID string, expectedTaskVersion uint64, workflowRunID string, verifier TaskDeliveryVerifier) (TaskDeliveryReceipt, error) {
	sqlite, ok := c.Store.(*SQLiteStore)
	if !ok || c.Project == "" || config.Identity() != c.Project {
		return TaskDeliveryReceipt{}, nativeErr("authority_mismatch", "delivery requires the configured native project")
	}
	if err := ValidateNativeTaskID(taskID); err != nil {
		return TaskDeliveryReceipt{}, err
	}
	if expectedTaskVersion == 0 || workflowRunID == "" {
		return TaskDeliveryReceipt{}, nativeErr("immutable_task", "delivery requires task version and workflow identity")
	}
	graph, err := sqlite.taskGraphSnapshot(ctx, c.Project)
	if err != nil {
		return TaskDeliveryReceipt{}, err
	}
	task, exists := graph.ByID[taskID]
	if !exists {
		return TaskDeliveryReceipt{}, nativeErr("task_not_found", "native task was not found")
	}
	if existing, ok := graph.Deliveries[taskID]; ok {
		if task.Status != "closed" {
			return TaskDeliveryReceipt{}, nativeErr("inconsistent_delivery", "task has a delivery without closed state")
		}
		if existing.SelectedWorkflowRunID != workflowRunID {
			return TaskDeliveryReceipt{}, nativeErr("inconsistent_delivery", "task is already delivered by a different workflow")
		}
		return TaskDeliveryReceipt{TaskID: taskID, TaskVersion: task.Version, WorkflowRunID: workflowRunID, GraphRevision: graph.Authority.GraphRevision, Status: "closed", Delivery: existing}, nil
	}
	if verifier == nil {
		return TaskDeliveryReceipt{}, nativeErr("merge_state_unknown", "PR verifier is required")
	}
	if err = validateNativeConfiguredTarget(ctx, config, graph.Authority); err != nil {
		return TaskDeliveryReceipt{}, err
	}
	if task.Status == "closed" {
		return TaskDeliveryReceipt{}, nativeErr("inconsistent_delivery", "closed task has no delivery record")
	}
	if task.Status == "discarded" {
		return TaskDeliveryReceipt{}, c.recordDiscardedMergeConflict(ctx, sqlite, config, task, workflowRunID, verifier, graph.Authority)
	}
	if task.Status != "in_progress" || task.Version != expectedTaskVersion {
		return TaskDeliveryReceipt{}, nativeErr("stale_version", fmt.Sprintf("task is %s at version %d", task.Status, task.Version))
	}
	snapshot, err := sqlite.Query(ctx, c.Project)
	if err != nil {
		return TaskDeliveryReceipt{}, err
	}
	workflow, execution, attempts, err := selectedNativeDeliveryState(snapshot, taskID, workflowRunID)
	if err != nil {
		return TaskDeliveryReceipt{}, err
	}
	contract, err := nativeContractForTask(ctx, sqlite, c.Project, task)
	if err != nil {
		return TaskDeliveryReceipt{}, err
	}
	publish, err := validateNativeDeliveryGates(task, contract, workflow, execution, attempts)
	if err != nil {
		return TaskDeliveryReceipt{}, err
	}
	publication, _, err := taskDiscardPlanFromSnapshot(snapshot, taskID)
	if err != nil {
		return TaskDeliveryReceipt{}, err
	}
	if publication.UncertainPR || publication.PRURL != publish.PRURL {
		return TaskDeliveryReceipt{}, nativeErr("merge_state_unknown", "task has another unresolved or mismatched PR publication")
	}
	if info, err := verifier.Query(ctx, publish.PRURL); err != nil {
		return TaskDeliveryReceipt{}, nativeErr("merge_state_unknown", "PR merge state could not be verified: "+clip(err.Error(), 1000))
	} else if info.State == "unknown" {
		return TaskDeliveryReceipt{}, nativeErr("merge_state_unknown", "PR provider returned an unknown merge state")
	} else if !info.Merged || info.State != "merged" {
		return TaskDeliveryReceipt{}, nativeErr("task_not_ready", "selected PR does not have complete verified merged evidence")
	} else if info.MergedAt == nil || info.MergedAt.IsZero() || info.MergeCommit == nil || !validCommit(*info.MergeCommit) {
		return TaskDeliveryReceipt{}, nativeErr("merge_state_unknown", "merged PR evidence is incomplete")
	} else {
		target, targetErr := configuredRepositoryTarget(ctx, config)
		if targetErr != nil || !sameRepositoryTarget(target, graph.Authority.Repository) || config.PR.ValidateRemote(target) != nil {
			return TaskDeliveryReceipt{}, nativeErr("authority_mismatch", "PR configuration does not match the fixed native repository")
		}
		if workflow.WorkflowVersion == WorkerFlowVersion {
			if err := validateWorkerPRIdentity(info, publish.PRURL, config.PR.Repository, graph.Authority.Branch, execution.Workspace.Branch); err != nil {
				return TaskDeliveryReceipt{}, nativeErr("immutable_task", "merged PR does not belong to the selected worker execution branch")
			}
		} else if info.URL != publish.PRURL || info.Repository != config.PR.Repository || info.BaseBranch != graph.Authority.Branch || info.HeadRevision != execution.CandidateRevision {
			return TaskDeliveryReceipt{}, nativeErr("immutable_task", "merged PR identity, branch or head differs from the selected task result")
		}
		number, err := config.PR.PRNumber(info.URL)
		if err != nil {
			return TaskDeliveryReceipt{}, nativeErr("immutable_task", "verified PR URL does not match configured platform identity")
		}
		evidence := TaskDeliveryEvidence{PRInstance: strings.TrimSuffix(config.PR.BaseURL, "/"), URL: info.URL, Repository: info.Repository, PRNumber: int64(number), BaseBranch: info.BaseBranch, State: info.State, Merged: true, MergedAt: *info.MergedAt, MergeCommit: *info.MergeCommit}
		evidenceBytes, _ := json.Marshal(evidence)
		record := TaskDeliveryRecord{ProjectIdentity: c.Project, TaskID: taskID, SelectedWorkflowRunID: workflowRunID, PRInstance: evidence.PRInstance, PRRepository: evidence.Repository, PRNumber: evidence.PRNumber, PRURL: evidence.URL, MergeCommit: evidence.MergeCommit, MergedAt: evidence.MergedAt, RecordedAt: time.Now().UTC(), Evidence: evidenceBytes}
		return c.commitNativeDelivery(ctx, sqlite, graph.Authority.AuthorityVersion, task, workflow, execution, attempts, contract, publish, record)
	}
}

// recordDiscardedMergeConflict keeps a late platform merge as audit evidence
// without converting a discarded task into a normal delivery.
func (c CommandService) recordDiscardedMergeConflict(ctx context.Context, s *SQLiteStore, config Config, task TaskRecord, workflowRunID string, verifier TaskDeliveryVerifier, authority ProjectTaskAuthority) error {
	var owned int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_runs WHERE project_identity=? AND workflow_run_id=? AND task_id=?`, c.Project, workflowRunID, task.TaskID).Scan(&owned); err != nil {
		return err
	}
	if owned != 1 {
		return nativeErr("task_discarded", "discarded task cannot be completed by an unrelated workflow")
	}
	plan, current, err := taskDiscardPlanFromStore(ctx, s, c.Project, task.TaskID)
	if err != nil {
		return err
	}
	if current.Status != "discarded" || current.Version != task.Version || plan.PRURL == "" || plan.UncertainPR {
		return nativeErr("task_discarded", "discarded task has no conclusive PR binding to reconcile")
	}
	target, err := configuredRepositoryTarget(ctx, config)
	if err != nil || !sameRepositoryTarget(target, authority.Repository) || config.PR.ValidateRemote(target) != nil {
		return nativeErr("authority_mismatch", "PR configuration does not match the discarded task repository")
	}
	info, err := verifier.Query(ctx, plan.PRURL)
	if err != nil || info.State == "unknown" {
		return nativeErr("merge_state_unknown", "discarded task PR merge state could not be verified")
	}
	if !info.Merged || info.State != "merged" {
		return nativeErr("task_discarded", "discarded task PR is not verified merged")
	}
	if info.URL != plan.PRURL || info.Repository != config.PR.Repository || info.BaseBranch != authority.Branch || info.MergedAt == nil || info.MergedAt.IsZero() || info.MergeCommit == nil || !validCommit(*info.MergeCommit) {
		return nativeErr("merge_state_unknown", "discarded task PR merge identity or evidence is incomplete")
	}
	if _, err = config.PR.PRNumber(info.URL); err != nil {
		return nativeErr("merge_state_unknown", "discarded task PR URL does not match its fixed platform identity")
	}
	conflict := struct {
		TaskID           string     `json:"task_id"`
		WorkflowRunID    string     `json:"workflow_run_id"`
		DiscardRequestID string     `json:"discard_request_id"`
		DiscardedAt      *time.Time `json:"discarded_at,omitempty"`
		PRURL            string     `json:"pr_url"`
		PRHead           string     `json:"pr_head"`
		MergeCommit      string     `json:"merge_commit"`
		MergedAt         time.Time  `json:"merged_at"`
	}{task.TaskID, workflowRunID, task.DiscardRequestID, task.DiscardedAt, info.URL, info.HeadRevision, *info.MergeCommit, *info.MergedAt}
	body, err := json.Marshal(conflict)
	if err != nil {
		return err
	}
	err = s.Transact(ctx, func(domainTx DomainTx) error {
		tx := domainTx.(*sqliteTx)
		graph, e := taskGraphSnapshotTx(ctx, tx, c.Project)
		if e != nil {
			return e
		}
		stored, exists := graph.ByID[task.TaskID]
		if !exists || stored.Status != "discarded" || stored.Version != task.Version || graph.Authority.AuthorityVersion != authority.AuthorityVersion {
			return nativeErr("stale_version", "discarded task changed during PR merge reconciliation")
		}
		if _, delivered := graph.Deliveries[task.TaskID]; delivered {
			return nativeErr("inconsistent_delivery", "discarded task already has a normal delivery")
		}
		fixed, _, e := taskDiscardPlanTx(ctx, tx, c.Project, task.TaskID)
		if e != nil {
			return e
		}
		if fixed.PRURL != plan.PRURL || fixed.UncertainPR {
			return nativeErr("merge_state_unknown", "discarded task PR binding changed during reconciliation")
		}
		_, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: task.TaskID, WorkflowRunID: workflowRunID, ExternalSource: "native-task-delivery", ExternalID: task.TaskID + ":late-merge:" + *info.MergeCommit, Type: "task.delivery.conflict", ReceivedAt: time.Now().UTC(), Payload: body})
		return e
	})
	if err != nil {
		return err
	}
	return nativeErr("inconsistent_delivery", "discarded task has a merged PR; manual verification is required")
}

func selectedNativeDeliveryState(snapshot DomainSnapshot, taskID, workflowRunID string) (WorkflowRunRecord, ExecutionRecord, []StepAttemptRecord, error) {
	var workflow *WorkflowRunRecord
	for i := range snapshot.WorkflowRuns {
		if snapshot.WorkflowRuns[i].WorkflowRunID == workflowRunID && snapshot.WorkflowRuns[i].TaskID == taskID {
			workflow = &snapshot.WorkflowRuns[i]
			break
		}
	}
	if workflow == nil {
		return WorkflowRunRecord{}, ExecutionRecord{}, nil, nativeErr("immutable_task", "selected workflow run does not belong to this task")
	}
	var execution *ExecutionRecord
	for i := range snapshot.Executions {
		if snapshot.Executions[i].WorkflowRunID == workflowRunID && snapshot.Executions[i].ExecutionID == workflow.SelectedResult {
			execution = &snapshot.Executions[i]
			break
		}
	}
	if execution == nil {
		return *workflow, ExecutionRecord{}, nil, nativeErr("inconsistent_delivery", "selected workflow result is missing")
	}
	attempts := []StepAttemptRecord{}
	for _, attempt := range snapshot.Attempts {
		if attempt.ExecutionID == execution.ExecutionID {
			attempts = append(attempts, attempt)
		}
	}
	return *workflow, *execution, attempts, nil
}

func latestDeliveryAttempt(attempts []StepAttemptRecord, step string) (StepAttemptRecord, StepGateResult, bool) {
	var selected StepAttemptRecord
	var result StepGateResult
	found := false
	for _, attempt := range attempts {
		if attempt.StepID != step || attempt.Status != "succeeded" || len(attempt.Result) == 0 {
			continue
		}
		var candidate StepGateResult
		if json.Unmarshal(attempt.Result, &candidate) != nil || candidate.StepID != step {
			continue
		}
		if !found || attempt.AttemptNumber > selected.AttemptNumber {
			selected = attempt
			result = candidate
			found = true
		}
	}
	return selected, result, found
}

func validateNativeDeliveryGates(task TaskRecord, contract TaskContract, workflow WorkflowRunRecord, execution ExecutionRecord, attempts []StepAttemptRecord) (StepGateResult, error) {
	if workflow.WorkflowVersion == WorkerFlowVersion {
		if task.ContractID != contract.ContractID || contract.Digest == "" {
			return StepGateResult{}, nativeErr("contract_mismatch", "selected task contract is unavailable")
		}
		if workflow.TaskID != task.TaskID || workflow.SelectedResult != execution.ExecutionID || execution.WorkflowRunID != workflow.WorkflowRunID ||
			workflow.PRURL == "" || execution.SessionID == "" || execution.Workspace.Branch == "" ||
			workflow.Phase != "executing" && workflow.Phase != "waiting_decision" && workflow.Phase != "waiting_pr_merge" {
			return StepGateResult{}, nativeErr("task_not_ready", "selected worker flow has no bound session, execution branch or PR")
		}
		return StepGateResult{PRURL: workflow.PRURL}, nil
	}
	if err := (MinimalWorkflowConfig{Definition: workflow.Definition, Steps: workflow.ProjectSteps}).Validate(); err != nil {
		return StepGateResult{}, nativeErr("task_not_ready", "selected workflow definition or fixed steps are invalid: "+err.Error())
	}
	if workflow.WorkflowVersion != MinimalWorkflowVersion || workflow.Phase != "waiting_pr_merge" || workflow.SelectedResult != execution.ExecutionID || execution.WorkflowRunID != workflow.WorkflowRunID || execution.Status != "succeeded" || execution.CurrentStep != "waiting_pr_merge" {
		return StepGateResult{}, nativeErr("task_not_ready", "selected workflow result is not awaiting verified PR merge")
	}
	if !revisionPattern.MatchString(execution.CandidateRevision) || execution.VerifiedRevision != execution.CandidateRevision || execution.Workspace.Dirty || execution.Workspace.Head != execution.CandidateRevision {
		return StepGateResult{}, nativeErr("task_not_ready", "selected execution lacks clean verification evidence for its candidate")
	}
	verifyAttempt, verify, ok := latestDeliveryAttempt(attempts, "verify")
	if !ok || verifyAttempt.ContractDigest != nativeStepContractDigest(workflow.ProjectSteps, "verify") || !verify.Succeeded || verify.ResultID == "" || verify.Revision != execution.CandidateRevision || verify.Workspace.Dirty || verify.Workspace.Head != execution.CandidateRevision || verify.Workspace.VerifiedAt.IsZero() {
		return StepGateResult{}, nativeErr("task_not_ready", "required verification result is missing or for a different revision")
	}
	if len(workflow.ProjectSteps.Verify.RequiredChecks) == 0 {
		return StepGateResult{}, nativeErr("task_not_ready", "selected workflow has no fixed required verification checks")
	}
	failed, err := validateChecks(workflow.ProjectSteps.Verify.RequiredChecks, verify.Checks, execution.CandidateRevision)
	if err != nil || failed {
		return StepGateResult{}, nativeErr("task_not_ready", "fixed verification gates are not all passed for the published revision")
	}
	if workflow.Definition.ReviewRequired {
		reviewAttempt, review, ok := latestDeliveryAttempt(attempts, "review")
		if !ok || reviewAttempt.ContractDigest != nativeStepContractDigest(workflow.ProjectSteps, "review") || !review.Succeeded || review.ResultID == "" || review.ReviewVerdict != "passed" || review.Revision != execution.CandidateRevision || review.Workspace.Dirty || review.Workspace.Head != execution.CandidateRevision || execution.ReviewedRevision != execution.CandidateRevision {
			return StepGateResult{}, nativeErr("task_not_ready", "required review is not passed for the published revision")
		}
	}
	publishAttempt, publish, ok := latestDeliveryAttempt(attempts, "publish_pr")
	if !ok || publishAttempt.ContractDigest != nativeStepContractDigest(workflow.ProjectSteps, "publish_pr") || !publish.Succeeded || publish.ResultID == "" || publish.Workspace.Dirty || publish.Workspace.Head != execution.CandidateRevision || publish.PRURL == "" || publish.PublishedHead != execution.CandidateRevision || publish.Revision != execution.CandidateRevision {
		return StepGateResult{}, nativeErr("task_not_ready", "selected workflow has no fixed PR publication result")
	}
	if contract.Digest == "" || task.ContractID != contract.ContractID {
		return StepGateResult{}, nativeErr("contract_mismatch", "selected task contract is unavailable")
	}
	return publish, nil
}

func nativeStepContractDigest(steps ProjectSteps, step string) string {
	data, err := json.Marshal(minimalStepContract(steps, step))
	if err != nil {
		return ""
	}
	return operationDigest(data)
}

func (c CommandService) commitNativeDelivery(ctx context.Context, s *SQLiteStore, expectedAuthorityVersion uint64, task TaskRecord, workflow WorkflowRunRecord, execution ExecutionRecord, attempts []StepAttemptRecord, contract TaskContract, publish StepGateResult, delivery TaskDeliveryRecord) (TaskDeliveryReceipt, error) {
	var receipt TaskDeliveryReceipt
	err := s.Transact(ctx, func(domainTx DomainTx) error {
		tx, ok := domainTx.(*sqliteTx)
		if !ok {
			return errors.New("delivery requires the center SQLite transaction")
		}
		graph, err := taskGraphSnapshotTx(ctx, tx, c.Project)
		if err != nil {
			return err
		}
		if graph.Authority.AuthorityVersion != expectedAuthorityVersion {
			return nativeErr("stale_version", "native project authority changed during PR verification")
		}
		current, ok := graph.ByID[task.TaskID]
		if !ok {
			return nativeErr("task_not_found", "native task was not found")
		}
		var existingRun string
		var existingMerge string
		err = tx.tx.QueryRowContext(ctx, `SELECT selected_workflow_run_id,merge_commit FROM task_deliveries WHERE project_identity=? AND task_id=?`, c.Project, task.TaskID).Scan(&existingRun, &existingMerge)
		if err == nil {
			if current.Status != "closed" {
				return nativeErr("inconsistent_delivery", "task has a delivery without closed state")
			}
			if existingRun == workflow.WorkflowRunID && existingMerge == delivery.MergeCommit {
				var record TaskDeliveryRecord
				if e := scanTaskDelivery(tx.tx, c.Project, task.TaskID, &record); e != nil {
					return e
				}
				receipt = TaskDeliveryReceipt{TaskID: task.TaskID, TaskVersion: current.Version, WorkflowRunID: existingRun, GraphRevision: graph.Authority.GraphRevision, Status: "closed", Delivery: record}
				return taskDeliveryReplay{Receipt: receipt}
			}
			return nativeErr("inconsistent_delivery", "task already has a different authoritative delivery")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if current.Status == "closed" {
			return nativeErr("inconsistent_delivery", "closed task has no delivery record")
		}
		if current.Status == "discarded" {
			return nativeErr("task_discarded", "discarded task cannot be completed")
		}
		if current.Status != "in_progress" || current.Version != task.Version {
			return nativeErr("stale_version", "task state changed during PR verification")
		}
		var runBody []byte
		if err = tx.tx.QueryRowContext(ctx, `SELECT payload FROM workflow_runs WHERE project_identity=? AND workflow_run_id=? AND task_id=?`, c.Project, workflow.WorkflowRunID, task.TaskID).Scan(&runBody); err != nil {
			return nativeErr("immutable_task", "selected workflow run changed during PR verification")
		}
		var currentWorkflow WorkflowRunRecord
		if err = json.Unmarshal(runBody, &currentWorkflow); err != nil {
			return err
		}
		var execBody []byte
		if err = tx.tx.QueryRowContext(ctx, `SELECT payload FROM executions WHERE project_identity=? AND execution_id=? AND workflow_run_id=?`, c.Project, execution.ExecutionID, workflow.WorkflowRunID).Scan(&execBody); err != nil {
			return nativeErr("inconsistent_delivery", "selected execution changed during PR verification")
		}
		var currentExecution ExecutionRecord
		if err = json.Unmarshal(execBody, &currentExecution); err != nil {
			return err
		}
		currentAttempts, err := loadTaskExecutionAttempts(ctx, tx, c.Project, currentExecution.ExecutionID)
		if err != nil {
			return err
		}
		currentContract, err := nativeContractForTaskTx(ctx, tx, c.Project, current)
		if err != nil {
			return err
		}
		if currentContract.Digest != contract.Digest {
			return nativeErr("immutable_task", "fixed task contract changed during PR verification")
		}
		currentPublish, err := validateNativeDeliveryGates(current, currentContract, currentWorkflow, currentExecution, currentAttempts)
		if err != nil {
			return err
		}
		if currentWorkflow.WorkflowRunID != workflow.WorkflowRunID || currentWorkflow.SelectedResult != execution.ExecutionID || currentPublish.PRURL != publish.PRURL ||
			currentWorkflow.WorkflowVersion != workflow.WorkflowVersion || currentExecution.SessionID != execution.SessionID ||
			currentExecution.Workspace.Branch != execution.Workspace.Branch ||
			workflow.WorkflowVersion != WorkerFlowVersion && currentExecution.CandidateRevision != execution.CandidateRevision {
			return nativeErr("immutable_task", "selected result or PR binding changed during platform verification")
		}
		publication, _, err := taskDiscardPlanTx(ctx, tx, c.Project, task.TaskID)
		if err != nil {
			return err
		}
		if publication.UncertainPR || publication.PRURL != publish.PRURL {
			return nativeErr("merge_state_unknown", "task publication became unresolved during PR verification")
		}
		var reused int
		if err = tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM step_attempts a JOIN executions e ON e.project_identity=a.project_identity AND e.execution_id=a.execution_id JOIN workflow_runs w ON w.project_identity=e.project_identity AND w.workflow_run_id=e.workflow_run_id WHERE a.project_identity=? AND w.task_id<>? AND json_extract(a.payload,'$.result.pr_url')=?`, c.Project, task.TaskID, delivery.PRURL).Scan(&reused); err != nil {
			return err
		}
		if reused > 0 {
			return nativeErr("immutable_task", "PR is already bound to another native task")
		}
		if err = tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_runs WHERE project_identity=? AND task_id<>? AND json_extract(payload,'$.pr_url')=?`, c.Project, task.TaskID, delivery.PRURL).Scan(&reused); err != nil {
			return err
		}
		if reused > 0 {
			return nativeErr("immutable_task", "PR is already bound to another native task")
		}
		if err = tx.PutTaskDelivery(delivery); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: task_deliveries.pr_instance") {
				return nativeErr("immutable_task", "PR delivery identity is already bound to another task")
			}
			return err
		}
		next := current
		next.Status = "closed"
		next.Version++
		next.UpdatedAt = time.Now().UTC()
		if err = tx.UpdateNativeTaskCAS(next, current.Version); err != nil {
			return err
		}
		receipt = TaskDeliveryReceipt{TaskID: task.TaskID, TaskVersion: next.Version, WorkflowRunID: workflow.WorkflowRunID, GraphRevision: graph.Authority.GraphRevision + 1, Status: "closed", Delivery: delivery}
		body, _ := json.Marshal(receipt)
		_, err = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: task.TaskID, WorkflowRunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, ExternalSource: "native-task-delivery", ExternalID: delivery.PRURL + ":" + delivery.MergeCommit, Type: "task.delivered", ReceivedAt: delivery.RecordedAt, Payload: body})
		return err
	})
	var replay taskDeliveryReplay
	if errors.As(err, &replay) {
		return replay.Receipt, nil
	}
	return receipt, err
}

type taskDeliveryReplay struct{ Receipt TaskDeliveryReceipt }

func (e taskDeliveryReplay) Error() string { return "task delivery already committed" }

func loadTaskExecutionAttempts(ctx context.Context, tx *sqliteTx, project, executionID string) ([]StepAttemptRecord, error) {
	rows, err := tx.tx.QueryContext(ctx, `SELECT payload FROM step_attempts WHERE project_identity=? AND execution_id=? ORDER BY step_id,attempt_number`, project, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StepAttemptRecord{}
	for rows.Next() {
		var body []byte
		var record StepAttemptRecord
		if err = rows.Scan(&body); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(body, &record); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func nativeContractForTaskTx(ctx context.Context, tx *sqliteTx, project string, task TaskRecord) (TaskContract, error) {
	var body []byte
	var digest string
	err := tx.tx.QueryRowContext(ctx, `SELECT digest,payload FROM task_contracts WHERE project_identity=? AND contract_id=? AND task_id=?`, project, task.ContractID, task.TaskID).Scan(&digest, &body)
	if err != nil {
		return TaskContract{}, nativeErr("contract_mismatch", "native task has no matching fixed contract")
	}
	var contract TaskContract
	if err = json.Unmarshal(body, &contract); err != nil {
		return contract, err
	}
	if contract.Digest != digest {
		return contract, nativeErr("contract_mismatch", "fixed task contract digest differs from its stored row")
	}
	if err = ValidateTaskContract(contract); err != nil {
		return contract, err
	}
	return contract, nil
}

func scanTaskDelivery(db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, project, taskID string, delivery *TaskDeliveryRecord) error {
	var merged, recorded string
	err := db.QueryRowContext(context.Background(), `SELECT task_id,selected_workflow_run_id,pr_instance,pr_repository,pr_number,pr_url,merge_commit,merged_at,recorded_at,evidence FROM task_deliveries WHERE project_identity=? AND task_id=?`, project, taskID).Scan(&delivery.TaskID, &delivery.SelectedWorkflowRunID, &delivery.PRInstance, &delivery.PRRepository, &delivery.PRNumber, &delivery.PRURL, &delivery.MergeCommit, &merged, &recorded, &delivery.Evidence)
	if err != nil {
		return err
	}
	delivery.ProjectIdentity = project
	delivery.MergedAt, err = time.Parse(time.RFC3339Nano, merged)
	if err != nil {
		return err
	}
	delivery.RecordedAt, err = time.Parse(time.RFC3339Nano, recorded)
	return err
}
