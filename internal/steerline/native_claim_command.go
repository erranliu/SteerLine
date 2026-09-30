package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	taskdomain "github.com/erranliu/SteerLine/internal/steerline/task"
)

// TaskClaimReceipt records reservation, not worker start. The existing

// start_task applied receipt remains gated on a persisted worker identity.
type TaskClaimReceipt = taskdomain.TaskClaimReceipt

type TaskClaimOptions struct {
	TaskID                   string
	RequestID                string
	ExpectedTaskVersion      uint64
	ExpectedAuthorityVersion uint64
	RequestedBy              string
	Explicit                 bool
	WorkflowRunID            string
	ExecutionProfile         *ExecutionProfile
	ServiceInstanceID        string
}

func (c CommandService) ClaimNativeTask(ctx context.Context, config Config, options TaskClaimOptions) (TaskClaimReceipt, error) {
	sqlite := c.Store
	if sqlite == nil || c.Project == "" || config.Identity() != c.Project {
		return TaskClaimReceipt{}, nativeErr("authority_mismatch", "claim requires the configured native project")
	}
	if config.WorkflowProtocol != WorkerFlowVersion {
		return TaskClaimReceipt{}, nativeErr("unsupported_schema", "only worker-flow-v1 may claim a task")
	}
	if err := validateWorkflowConfig(&config, nil); err != nil {
		return TaskClaimReceipt{}, err
	}
	if config.managedWindowsOrca() {
		if _, err := nativeOrcaFixedAgentCommand(config); err != nil {
			return TaskClaimReceipt{}, err
		}
	}
	if err := ValidateNativeTaskID(options.TaskID); err != nil {
		return TaskClaimReceipt{}, err
	}
	if !controlIDPattern.MatchString(options.RequestID) || options.ExpectedTaskVersion == 0 || options.ExpectedAuthorityVersion == 0 || options.WorkflowRunID != "" && !stableExecutionID(options.WorkflowRunID) {
		return TaskClaimReceipt{}, nativeErr("invalid_batch", "claim requires request ID and current task/project versions")
	}
	if options.ServiceInstanceID != "" && !stableExecutionID(options.ServiceInstanceID) {
		return TaskClaimReceipt{}, nativeErr("invalid_batch", "claim service instance identity is invalid")
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
	if old, found, e := sqlite.lookupNativeCommand(ctx, c.Project, options.RequestID); e != nil {
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
	base, err := acquireNativeBase(ctx, config)
	if err != nil {
		return TaskClaimReceipt{}, nativeErr("task_not_ready", "fixed target branch base could not be acquired: "+err.Error())
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
	wfr := WorkflowRunRecord{ProjectIdentity: c.Project, WorkflowRunID: workflowRunID, TaskID: task.TaskID, WorkflowVersion: WorkerFlowVersion, ConfigDigest: config.configDigest, Runner: runner, Phase: "queued", Repository: config.Repository, Branch: config.Branch, SelectedResult: executionID, CreatedAt: created, UpdatedAt: created}
	if options.ExecutionProfile != nil {
		fixed := options.ExecutionProfile.Freeze()
		wfr.ExecutionProfile = &fixed
	}
	if config.managedWindowsOrca() && wfr.ExecutionProfile == nil {
		fixed := config.projectExecutionProfile().Freeze()
		wfr.ExecutionProfile = &fixed
	}
	execution := ExecutionRecord{ProjectIdentity: c.Project, ExecutionID: executionID, CurrentAttemptID: attemptID, OwnerServiceInstanceID: options.ServiceInstanceID, WorkflowRunID: workflowRunID, Agent: config.agentName(), Backend: string(backend), Protocol: string(protocol), Target: target, Workspace: WorkspaceEvidence{Repository: config.identity, Branch: config.Branch, Base: base}, Status: "pending", CurrentStep: "worker", CreatedAt: created, UpdatedAt: created}
	if config.managedWindowsOrca() {
		if _, err := nativeOrcaFixedAgentCommand(config); err != nil {
			return TaskClaimReceipt{}, err
		}
		execution.Environment = &WorkerEnvironment{ProfileDigest: wfr.ExecutionProfile.Digest, Preparation: &WorkerPreparation{TurnID: newID(), Nonce: newID(), StartedAt: created, Deadline: created.Add(positiveDuration(config.Orca.WaitTimeout, 2*time.Minute))}}
	}
	execution.WorkspaceIntent, err = planWorkspace(config, task.TaskID, workflowRunID, base)
	if err != nil {
		return TaskClaimReceipt{}, err
	}
	attempt := StepAttemptRecord{ProjectIdentity: c.Project, AttemptID: attemptID, ExecutionID: executionID, StepID: "worker", AttemptNumber: 1, Status: "planned", InputRevision: base, ContractDigest: contract.Digest, ExternalBinding: target, CreatedAt: created, UpdatedAt: created}
	nextTask, err := taskdomain.Claim(task, claimant, created)
	if err != nil {
		return TaskClaimReceipt{}, err
	}
	operationRequest, _ := json.Marshal(map[string]any{"task_id": task.TaskID, "task_version": options.ExpectedTaskVersion, "authority_version": options.ExpectedAuthorityVersion, "workflow_run_id": workflowRunID, "execution_id": executionID, "attempt_id": attemptID, "base_commit": base, "predecessor_merge_commits": predecessorMerges, "requested_by": options.RequestedBy})
	var receipt TaskClaimReceipt
	err = sqlite.transactSQLite(ctx, func(tx *sqliteTx) error {
		if old, found, e := tx.lookupNativeCommand(ctx, c.Project, options.RequestID); e != nil {
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
		if options.ExecutionProfile != nil && (!options.ExecutionProfile.MatchesConfig(config) || wfr.ExecutionProfile.Digest != options.ExecutionProfile.Digest()) {
			return nativeErr("stale_profile", "selected Profile digest changed before atomic claim")
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
	return taskdomain.DependencyClaimError(blockers)
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
