package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	taskdomain "github.com/erranliu/SteerLine/internal/steerline/task"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// start_task applied receipt remains gated on a persisted worker identity.
type TaskClaimReceipt = taskdomain.TaskClaimReceipt

type TaskClaimOptions struct {
	TaskID                   string
	RequestID                string
	ExpectedTaskVersion      uint64
	ExpectedAuthorityVersion uint64
	ExpectedGraphRevision    uint64
	RequestedBy              string
	Explicit                 bool
	WorkflowRunID            string
	ExecutionProfile         *ExecutionProfile
	ServiceInstanceID        string
	startCandidate           bool
	controlRequestDigest     string
}

// StartNativeCandidate preserves the original claim and fixed experiment base.
func (c CommandService) StartNativeCandidate(ctx context.Context, config Config, options TaskClaimOptions) (TaskClaimReceipt, error) {
	options.startCandidate, options.Explicit = true, true
	return c.ClaimNativeTask(ctx, config, options)
}

func (c CommandService) ClaimNativeTask(ctx context.Context, config Config, options TaskClaimOptions) (TaskClaimReceipt, error) {
	config = copyWorkerFlowConfig(config)
	sqlite := c.Store
	if sqlite == nil || c.Project == "" || config.Identity() != c.Project {
		return TaskClaimReceipt{}, nativeErr("authority_mismatch", "claim requires the configured native project")
	}
	if config.WorkflowProtocol != executionmodel.Version {
		return TaskClaimReceipt{}, nativeErr("unsupported_schema", "only worker-flow-v1 may claim a task")
	}
	if err := validateWorkflowConfig(&config, nil); err != nil {
		return TaskClaimReceipt{}, err
	}
	entry, err := registrationFor(config)
	if err != nil {
		return TaskClaimReceipt{}, err
	}
	if err := config.prepareWorktreeRoot(); err != nil {
		return TaskClaimReceipt{}, err
	}
	if err := taskmodel.ValidateID(options.TaskID); err != nil {
		return TaskClaimReceipt{}, err
	}
	if !controlIDPattern.MatchString(options.RequestID) || options.ExpectedTaskVersion == 0 || options.ExpectedAuthorityVersion == 0 || options.WorkflowRunID != "" && !executionbackend.StableExecutionID(options.WorkflowRunID) {
		return TaskClaimReceipt{}, nativeErr("invalid_batch", "claim requires request ID and current task/project versions")
	}
	if options.ServiceInstanceID != "" && !executionbackend.StableExecutionID(options.ServiceInstanceID) {
		return TaskClaimReceipt{}, nativeErr("invalid_batch", "claim service instance identity is invalid")
	}
	if options.ExecutionProfile != nil && !options.ExecutionProfile.MatchesConfig(config) {
		return TaskClaimReceipt{}, nativeErr("contract_mismatch", "selected execution profile differs from the claim configuration")
	}
	if strings.TrimSpace(options.RequestedBy) == "" {
		options.RequestedBy = DefaultRequestedBy()
	}
	requestBody := struct {
		TaskID               string `json:"task_id"`
		RequestedBy          string `json:"requested_by"`
		Explicit             bool   `json:"explicit"`
		WorkflowRunID        string `json:"workflow_run_id,omitempty"`
		ProfileID            string `json:"execution_profile_id,omitempty"`
		ProfileDigest        string `json:"execution_profile_digest,omitempty"`
		TaskVersion          uint64 `json:"task_version,omitempty"`
		AuthorityVersion     uint64 `json:"authority_version,omitempty"`
		GraphRevision        uint64 `json:"graph_revision,omitempty"`
		ControlRequestDigest string `json:"control_request_digest,omitempty"`
	}{TaskID: options.TaskID, RequestedBy: options.RequestedBy, Explicit: options.Explicit, WorkflowRunID: options.WorkflowRunID}
	kind, expectedStatus := "claim_task", "open"
	if options.startCandidate {
		kind, expectedStatus = "start_candidate", "in_progress"
		requestBody.TaskVersion, requestBody.AuthorityVersion = options.ExpectedTaskVersion, options.ExpectedAuthorityVersion
		requestBody.GraphRevision = options.ExpectedGraphRevision
		requestBody.ControlRequestDigest = options.controlRequestDigest
	}
	if options.ExecutionProfile != nil {
		requestBody.ProfileID = options.ExecutionProfile.ID
		requestBody.ProfileDigest = options.ExecutionProfile.Digest()
	}
	requestRaw, _ := json.Marshal(requestBody)
	requestPayload, err := taskmodel.CanonicalJSON(requestRaw)
	if err != nil {
		return TaskClaimReceipt{}, err
	}
	if old, found, e := sqlite.LookupCommand(ctx, c.Project, options.RequestID); e != nil {
		return TaskClaimReceipt{}, e
	} else if found {
		return replayTaskStart(old, requestPayload, options.TaskID, options.RequestedBy, kind)
	}

	if !config.ExecutionRequirements.Empty() {
		evaluation, err := evaluateExecutionRequirements(ctx, config)
		if err != nil {
			return TaskClaimReceipt{}, err
		}
		config.requirementsEvaluation = &evaluation
	}
	claimPreparation, err := entry.PrepareClaim(ctx, config)
	if err != nil {
		return TaskClaimReceipt{}, err
	}
	if err := requireWorkerFlowCapabilities(ctx, config); err != nil {
		return TaskClaimReceipt{}, err
	}
	if err := config.bindPublicationTarget(ctx); err != nil {
		return TaskClaimReceipt{}, err
	}
	if options.ExecutionProfile != nil {
		p := config.bindProjectPolicy(*options.ExecutionProfile)
		options.ExecutionProfile = &p
	}
	graph, err := sqlite.TaskGraphSnapshot(ctx, c.Project)
	if err != nil {
		return TaskClaimReceipt{}, err
	}
	if graph.Authority.AuthorityVersion != options.ExpectedAuthorityVersion {
		return TaskClaimReceipt{}, nativeErr("stale_version", fmt.Sprintf("expected authority version %d, found %d", options.ExpectedAuthorityVersion, graph.Authority.AuthorityVersion))
	}
	if options.startCandidate && (options.ExpectedGraphRevision == 0 || graph.Authority.GraphRevision != options.ExpectedGraphRevision) {
		return TaskClaimReceipt{}, nativeErr("stale_version", "candidate start requires current graph revision")
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
	if task.Status != expectedStatus || task.Version != options.ExpectedTaskVersion {
		return TaskClaimReceipt{}, nativeErr("stale_version", fmt.Sprintf("task is %s at version %d", task.Status, task.Version))
	}
	if nativeDeliveryInconsistent(task, graph) {
		return TaskClaimReceipt{}, nativeErr("inconsistent_delivery", "task state conflicts with its delivery record")
	}
	if graph.Authority.DispatchPaused {
		return TaskClaimReceipt{}, nativeErr("dispatch_paused", "dispatch is paused")
	}
	if !options.startCandidate && (options.Explicit && graph.Authority.DispatchMode != "manual" || !options.Explicit && graph.Authority.DispatchMode != "auto") {
		return TaskClaimReceipt{}, nativeErr("wrong_mode", "claim mode does not match project dispatch mode")
	}
	eligibilityTask := task
	if options.startCandidate {
		eligibilityTask.Status = "open"
	}
	if ready, blockers := taskDependencyState(eligibilityTask, graph); !ready {
		return TaskClaimReceipt{}, taskmodel.DependencyClaimError(blockers)
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
	contract, err := sqlite.TaskContract(ctx, c.Project, task)
	if err != nil {
		return TaskClaimReceipt{}, err
	}
	if err = VerifyTaskContractFiles(ctx, config, contract); err != nil {
		return TaskClaimReceipt{}, err
	}
	var candidateSnapshot DomainSnapshot
	var commonBase string
	if options.startCandidate {
		candidateSnapshot, err = sqlite.Query(ctx, c.Project)
		if err != nil {
			return TaskClaimReceipt{}, err
		}
		if participatingCandidates(candidateSnapshot, task.TaskID) >= 5 {
			return TaskClaimReceipt{}, nativeErr("candidate_limit_reached", "five candidates already participate in this task experiment")
		}
		commonBase, err = candidateBase(candidateSnapshot, task.TaskID)
		if err != nil {
			return TaskClaimReceipt{}, err
		}
		if _, err = verifyKnownCandidatePRs(ctx, config, candidateSnapshot, task.TaskID, nil, ""); err != nil {
			return TaskClaimReceipt{}, err
		}
	}
	base, err := acquireNativeBase(ctx, config)
	if err != nil {
		return TaskClaimReceipt{}, nativeErr("task_not_ready", "fixed target branch base could not be acquired: "+err.Error())
	}
	if options.startCandidate {
		base = commonBase
		if _, err := (Runner{Dir: config.Repository, Timeout: duration(config.CommandTimeout)}).Output(ctx, config.Git, "cat-file", "-e", base+"^{commit}"); err != nil {
			return TaskClaimReceipt{}, nativeErr("base_missing_delivery", "selected execution cannot acquire the experiment's common base")
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
	backend, protocol := entry.Key.Backend, entry.Key.Protocol
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
	wfr := executionmodel.WorkflowRunRecord{ProjectIdentity: c.Project, WorkflowRunID: workflowRunID, TaskID: task.TaskID, WorkflowVersion: executionmodel.Version, ConfigDigest: config.acceptedConfigVersion(), Phase: "queued", Repository: config.Repository, Branch: config.Branch, SelectedResult: executionID, CreatedAt: created, UpdatedAt: created}
	if options.ExecutionProfile != nil {
		fixed := options.ExecutionProfile.Freeze(config.identity)
		wfr.ExecutionProfile = &fixed
	}
	if wfr.ExecutionProfile == nil {
		fixed := config.projectExecutionProfile().Freeze(config.identity)
		wfr.ExecutionProfile = &fixed
	}
	execution := executionmodel.ExecutionRecord{ProjectIdentity: c.Project, ExecutionID: executionID, CurrentAttemptID: attemptID, Generation: newID(), ConfigurationDigest: wfr.ExecutionProfile.Digest, OwnerServiceInstanceID: options.ServiceInstanceID, WorkflowRunID: workflowRunID, Agent: config.agentName(), Backend: string(backend), Protocol: string(protocol), Target: target, Workspace: executionmodel.WorkspaceEvidence{Repository: config.identity, Branch: config.Branch, Base: base}, Status: "pending", CurrentStep: "worker", CreatedAt: created, UpdatedAt: created}
	if claimPreparation.RequiresReadiness {
		timeout := claimPreparation.WaitTimeout
		if timeout <= 0 {
			timeout = 2 * time.Minute
		}
		execution.Environment = &executionmodel.WorkerEnvironment{ProfileDigest: wfr.ExecutionProfile.Digest, Preparation: &executionmodel.WorkerPreparation{TurnID: newID(), Nonce: newID(), StartedAt: created, Deadline: created.Add(timeout)}}
	}
	execution.WorkspaceIntent, err = planWorkspace(config, task.TaskID, workflowRunID, base)
	if err != nil {
		return TaskClaimReceipt{}, err
	}
	attempt := executionmodel.StepAttemptRecord{ProjectIdentity: c.Project, AttemptID: attemptID, ExecutionID: executionID, StepID: "worker", AttemptNumber: 1, Status: "planned", InputRevision: base, ContractDigest: contract.Digest, ExternalBinding: target, CreatedAt: created, UpdatedAt: created}
	nextTask := task
	if !options.startCandidate {
		nextTask, err = taskmodel.Claim(task, claimant, created)
		if err != nil {
			return TaskClaimReceipt{}, err
		}
	}
	operationRequest, _ := json.Marshal(map[string]any{"task_id": task.TaskID, "task_version": options.ExpectedTaskVersion, "authority_version": options.ExpectedAuthorityVersion, "workflow_run_id": workflowRunID, "execution_id": executionID, "attempt_id": attemptID, "base_commit": base, "predecessor_merge_commits": predecessorMerges, "requested_by": options.RequestedBy})
	var receipt TaskClaimReceipt
	err = sqlite.TransactClaim(ctx, func(tx persistence.ClaimTx) error {
		if old, found, e := tx.LookupCommand(ctx, c.Project, options.RequestID); e != nil {
			return e
		} else if found {
			r, e := replayTaskStart(old, requestPayload, options.TaskID, options.RequestedBy, kind)
			if e != nil {
				return e
			}
			receipt = r
			return taskBatchReplay{Receipt: taskmodel.TaskBatchReceipt{RequestID: options.RequestID, Status: "accepted"}}
		}
		if e := verifyClaimConfigFiles(config); e != nil {
			return e
		}
		if !verifyFixedExecutionProfile(*wfr.ExecutionProfile, config.ExecutionDirectory) {
			return nativeErr("stale_profile", "claim snapshot changed after evaluation")
		}
		current, e := tx.TaskGraphSnapshot(ctx, c.Project)
		if e != nil {
			return e
		}
		currentTask, ok := current.ByID[task.TaskID]
		if !ok {
			return nativeErr("task_not_found", "native task was not found")
		}
		if options.ExecutionProfile != nil {
			currentProfile := options.ExecutionProfile.Freeze(config.identity)
			if !options.ExecutionProfile.MatchesConfig(config) || currentProfile.Digest != wfr.ExecutionProfile.Digest {
				return nativeErr("stale_profile", "selected Profile changed before atomic claim")
			}
		}
		if current.Authority.AuthorityVersion != options.ExpectedAuthorityVersion || current.Authority.GraphRevision != graph.Authority.GraphRevision || currentTask.Version != options.ExpectedTaskVersion || currentTask.Status != expectedStatus {
			return nativeErr("stale_version", "task or project authority changed during claim preflight")
		}
		if nativeDeliveryInconsistent(currentTask, current) {
			return nativeErr("inconsistent_delivery", "task state conflicts with its delivery record")
		}
		if current.Authority.DispatchPaused {
			return nativeErr("dispatch_paused", "dispatch was paused before claim")
		}
		if !options.startCandidate && (options.Explicit && current.Authority.DispatchMode != "manual" || !options.Explicit && current.Authority.DispatchMode != "auto") {
			return nativeErr("wrong_mode", "project dispatch mode changed before claim")
		}
		if options.startCandidate {
			stored, e := tx.Query(ctx, c.Project)
			if e != nil {
				return e
			}
			if candidatePRBindings(stored, task.TaskID) != candidatePRBindings(candidateSnapshot, task.TaskID) {
				return nativeErr("stale_version", "known PR bindings changed during start verification")
			}
			storedBase, err := candidateBase(stored, task.TaskID)
			if err != nil {
				return err
			}
			if storedBase != commonBase {
				return nativeErr("contract_mismatch", "experiment common base changed during start verification")
			}
		}
		currentEligibility := currentTask
		if options.startCandidate {
			currentEligibility.Status = "open"
		}
		ready, blockers := taskDependencyState(currentEligibility, current)
		if !ready {
			return taskmodel.DependencyClaimError(blockers)
		}
		for pred, merge := range predecessorMerges {
			if current.Deliveries[pred].MergeCommit != merge {
				return nativeErr("base_missing_delivery", "predecessor delivery identity changed during claim preflight")
			}
		}
		occupied, e := tx.OccupiedExecutions(ctx, c.Project)
		if e != nil {
			return e
		}
		if occupied > 0 && !options.Explicit {
			return nativeErr("slot_busy", "native execution slot is occupied or uncertain")
		}
		command := storageports.CommandRecord{ProjectIdentity: c.Project, CommandID: options.RequestID, Kind: kind, TargetID: task.TaskID, ExpectedVersion: options.ExpectedTaskVersion, IdempotencyKey: options.RequestID, RequestedBy: options.RequestedBy, Payload: requestPayload, Status: "in_flight", CreatedAt: created}
		var made bool
		command, made, e = tx.EnqueueCommand(command)
		if e != nil {
			return nativeErr("idempotency_conflict", e.Error())
		}
		if !made {
			return nativeErr("idempotency_conflict", "claim request ID is already in use")
		}
		operation := executionmodel.OperationRecord{ProjectIdentity: c.Project, OperationID: options.RequestID, IdempotencyKey: options.RequestID, Kind: kind, Status: "pending", PayloadDigest: digestCanonical(operationRequest), Request: operationRequest, CreatedAt: created, UpdatedAt: created}
		if _, made, e = tx.PutOperation(operation); e != nil {
			return e
		} else if !made {
			return nativeErr("idempotency_conflict", "claim operation request ID is already in use")
		}
		if !options.startCandidate {
			if e = tx.UpdateNativeTaskCAS(nextTask, options.ExpectedTaskVersion); e != nil {
				return e
			}
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
		eventType := "task.claim.reserved"
		if options.startCandidate {
			eventType = "candidate.start.reserved"
		}
		_, e = tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: c.Project, TaskID: task.TaskID, WorkflowRunID: workflowRunID, ExecutionID: executionID, AttemptID: attemptID, ExternalSource: "native-task-command", ExternalID: options.RequestID, Type: eventType, ReceivedAt: created, Payload: body})
		return e
	})
	var replay taskBatchReplay
	if errors.As(err, &replay) {
		return receipt, nil
	}
	return receipt, err
}

func replayTaskClaim(old storageports.CommandRecord, payload []byte, taskID string, requestedBy string) (TaskClaimReceipt, error) {
	return replayTaskStart(old, payload, taskID, requestedBy, "claim_task")
}

func replayTaskStart(old storageports.CommandRecord, payload []byte, taskID string, requestedBy, kind string) (TaskClaimReceipt, error) {
	if old.Kind != kind || old.TargetID != taskID || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, payload) {
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
	if !taskmodel.ValidCommit(base) {
		return "", errors.New("remote branch did not resolve to a full commit")
	}
	return base, nil
}
