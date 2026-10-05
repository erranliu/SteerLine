package execution

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
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// start_task applied receipt remains gated on a persisted worker identity.

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
func (c CommandService) StartNativeCandidate(ctx context.Context, config Config, options TaskClaimOptions) (executionmodel.TaskClaimReceipt, error) {
	options.startCandidate, options.Explicit = true, true
	return c.ClaimNativeTask(ctx, config, options)
}

func (c CommandService) ClaimNativeTask(ctx context.Context, config Config, options TaskClaimOptions) (executionmodel.TaskClaimReceipt, error) {
	config = copyWorkerFlowConfig(config)
	sqlite := c.Store
	if sqlite == nil || c.Project == "" || config.Identity() != c.Project {
		return executionmodel.TaskClaimReceipt{}, nativeErr("authority_mismatch", "claim requires the configured native project")
	}
	if config.WorkflowProtocol != executionmodel.Version {
		return executionmodel.TaskClaimReceipt{}, nativeErr("unsupported_schema", "only worker-flow-v1 may claim a task")
	}
	if err := validateWorkflowConfig(&config, nil); err != nil {
		return executionmodel.TaskClaimReceipt{}, err
	}
	entry, err := registrationFor(config)
	if err != nil {
		return executionmodel.TaskClaimReceipt{}, err
	}
	if err := config.PrepareWorktreeRoot(); err != nil {
		return executionmodel.TaskClaimReceipt{}, err
	}
	if err := taskmodel.ValidateID(options.TaskID); err != nil {
		return executionmodel.TaskClaimReceipt{}, err
	}
	if !controlIDPattern.MatchString(options.RequestID) || options.ExpectedTaskVersion == 0 || options.ExpectedAuthorityVersion == 0 || options.WorkflowRunID != "" && !executionbackend.StableExecutionID(options.WorkflowRunID) {
		return executionmodel.TaskClaimReceipt{}, nativeErr("invalid_batch", "claim requires request ID and current task/project versions")
	}
	if options.ServiceInstanceID != "" && !executionbackend.StableExecutionID(options.ServiceInstanceID) {
		return executionmodel.TaskClaimReceipt{}, nativeErr("invalid_batch", "claim service instance identity is invalid")
	}
	if options.ExecutionProfile != nil && !options.ExecutionProfile.MatchesConfig(config) {
		return executionmodel.TaskClaimReceipt{}, nativeErr("contract_mismatch", "selected execution profile differs from the claim configuration")
	}
	if strings.TrimSpace(options.RequestedBy) == "" {
		options.RequestedBy = defaultRequestedBy()
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
		return executionmodel.TaskClaimReceipt{}, err
	}
	if old, found, e := sqlite.LookupCommand(ctx, c.Project, options.RequestID); e != nil {
		return executionmodel.TaskClaimReceipt{}, e
	} else if found {
		return replayTaskStart(old, requestPayload, options.TaskID, options.RequestedBy, kind)
	}

	if !config.ExecutionRequirements.Empty() {
		evaluation, err := evaluateExecutionRequirements(ctx, config)
		if err != nil {
			return executionmodel.TaskClaimReceipt{}, err
		}
		config.requirementsEvaluation = &evaluation
	}
	claimPreparation, err := entry.PrepareClaim(ctx, config)
	if err != nil {
		return executionmodel.TaskClaimReceipt{}, err
	}
	if err := requireWorkerFlowCapabilities(ctx, config); err != nil {
		return executionmodel.TaskClaimReceipt{}, err
	}
	if err := config.BindPublicationTarget(ctx); err != nil {
		return executionmodel.TaskClaimReceipt{}, err
	}
	if options.ExecutionProfile != nil {
		p := config.bindProjectPolicy(*options.ExecutionProfile)
		options.ExecutionProfile = &p
	}
	graph, err := sqlite.TaskGraphSnapshot(ctx, c.Project)
	if err != nil {
		return executionmodel.TaskClaimReceipt{}, err
	}
	if graph.Authority.AuthorityVersion != options.ExpectedAuthorityVersion {
		return executionmodel.TaskClaimReceipt{}, nativeErr("stale_version", fmt.Sprintf("expected authority version %d, found %d", options.ExpectedAuthorityVersion, graph.Authority.AuthorityVersion))
	}
	if options.startCandidate && (options.ExpectedGraphRevision == 0 || graph.Authority.GraphRevision != options.ExpectedGraphRevision) {
		return executionmodel.TaskClaimReceipt{}, nativeErr("stale_version", "candidate start requires current graph revision")
	}
	if err = validateNativeConfiguredTarget(ctx, config, graph.Authority); err != nil {
		return executionmodel.TaskClaimReceipt{}, err
	}
	task, exists := graph.ByID[options.TaskID]
	if !exists {
		return executionmodel.TaskClaimReceipt{}, nativeErr("task_not_found", "native task was not found")
	}
	if task.Status == "discarded" {
		return executionmodel.TaskClaimReceipt{}, nativeErr("task_discarded", "discarded task cannot be claimed")
	}
	if task.Status != expectedStatus || task.Version != options.ExpectedTaskVersion {
		return executionmodel.TaskClaimReceipt{}, nativeErr("stale_version", fmt.Sprintf("task is %s at version %d", task.Status, task.Version))
	}
	_, delivered := graph.Deliveries[task.TaskID]
	if taskmodel.DeliveryInconsistent(task.Status, delivered) {
		return executionmodel.TaskClaimReceipt{}, nativeErr("inconsistent_delivery", "task state conflicts with its delivery record")
	}
	if graph.Authority.DispatchPaused {
		return executionmodel.TaskClaimReceipt{}, nativeErr("dispatch_paused", "dispatch is paused")
	}
	if !options.startCandidate && (options.Explicit && graph.Authority.DispatchMode != "manual" || !options.Explicit && graph.Authority.DispatchMode != "auto") {
		return executionmodel.TaskClaimReceipt{}, nativeErr("wrong_mode", "claim mode does not match project dispatch mode")
	}
	if c.Work == nil {
		return executionmodel.TaskClaimReceipt{}, nativeErr("authority_mismatch", "task work port is required")
	}
	work, err := c.Work.Fixed(ctx, c.Project, options.TaskID, options.ExpectedTaskVersion, options.startCandidate)
	if err != nil {
		return executionmodel.TaskClaimReceipt{}, err
	}
	if work.Authority.AuthorityVersion != graph.Authority.AuthorityVersion || work.Authority.GraphRevision != graph.Authority.GraphRevision {
		return executionmodel.TaskClaimReceipt{}, nativeErr("stale_version", "fixed work graph changed")
	}
	task = work.Record
	allowed := map[string]bool{}
	types := config.ExecutableTypes
	if len(types) == 0 {
		types = []string{"task", "bug", "feature", "chore"}
	}
	for _, typ := range types {
		allowed[typ] = true
	}
	if !allowed[task.Type] {
		return executionmodel.TaskClaimReceipt{}, nativeErr("task_not_ready", "task type is not executable in this project")
	}
	contract := work.Contract
	if c.Work == nil {
		return executionmodel.TaskClaimReceipt{}, errors.New("task contract verification port is required")
	}
	if err = c.Work.VerifyContractFiles(ctx, contract); err != nil {
		return executionmodel.TaskClaimReceipt{}, err
	}
	var candidateSnapshot reconstruction.Snapshot
	var commonBase string
	if options.startCandidate {
		candidateSnapshot, err = readCommandSnapshot(ctx, sqlite, c.Project)
		if err != nil {
			return executionmodel.TaskClaimReceipt{}, err
		}
		if participatingCandidates(candidateSnapshot, task.TaskID) >= 5 {
			return executionmodel.TaskClaimReceipt{}, nativeErr("candidate_limit_reached", "five candidates already participate in this task experiment")
		}
		commonBase, err = executionmodel.CandidateCommonBase(candidateSnapshot.WorkflowRuns, candidateSnapshot.Executions, task.TaskID)
		if err != nil {
			return executionmodel.TaskClaimReceipt{}, err
		}
		if _, err = verifyKnownCandidatePRs(ctx, config, candidateSnapshot, task.TaskID, nil, ""); err != nil {
			return executionmodel.TaskClaimReceipt{}, err
		}
	}
	base, err := acquireNativeBase(ctx, config)
	if err != nil {
		return executionmodel.TaskClaimReceipt{}, nativeErr("task_not_ready", "fixed target branch base could not be acquired: "+err.Error())
	}
	if options.startCandidate {
		base = commonBase
		if _, err := (Runner{Dir: config.Repository, Timeout: duration(config.CommandTimeout), ProcessHost: config.ProcessHost}).Output(ctx, config.Git, "cat-file", "-e", base+"^{commit}"); err != nil {
			return executionmodel.TaskClaimReceipt{}, nativeErr("base_missing_delivery", "selected execution cannot acquire the experiment's common base")
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
			return executionmodel.TaskClaimReceipt{}, nativeErr("task_not_ready", "predecessor delivery changed during claim preflight")
		}
		predecessorMerges[edge.PredecessorID] = delivery.MergeCommit
		r := Runner{Dir: config.Repository, Timeout: duration(config.CommandTimeout), ProcessHost: config.ProcessHost}
		if _, e := r.Output(ctx, config.Git, "merge-base", "--is-ancestor", delivery.MergeCommit, base); e != nil {
			return executionmodel.TaskClaimReceipt{}, nativeErr("base_missing_delivery", "acquired branch base does not contain a verified predecessor merge commit")
		}
	}
	backend, protocol := entry.Key.Backend, entry.Key.Protocol
	target := TargetFor(config)
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
		return executionmodel.TaskClaimReceipt{}, err
	}
	attempt := executionmodel.StepAttemptRecord{ProjectIdentity: c.Project, AttemptID: attemptID, ExecutionID: executionID, StepID: "worker", AttemptNumber: 1, Status: "planned", InputRevision: base, ContractDigest: contract.Digest, ExternalBinding: target, CreatedAt: created, UpdatedAt: created}
	nextTask := task
	if !options.startCandidate {
		nextTask, err = c.Work.Claim(work, claimant, created)
		if err != nil {
			return executionmodel.TaskClaimReceipt{}, err
		}
	}
	operationRequest, _ := json.Marshal(map[string]any{"task_id": task.TaskID, "task_version": options.ExpectedTaskVersion, "authority_version": options.ExpectedAuthorityVersion, "workflow_run_id": workflowRunID, "execution_id": executionID, "attempt_id": attemptID, "base_commit": base, "predecessor_merge_commits": predecessorMerges, "requested_by": options.RequestedBy})
	if err := verifyClaimConfigFiles(config); err != nil {
		return executionmodel.TaskClaimReceipt{}, err
	}
	if !VerifyFixedExecutionProfile(*wfr.ExecutionProfile, config.ExecutionDirectory) {
		return executionmodel.TaskClaimReceipt{}, nativeErr("stale_profile", "claim snapshot changed after evaluation")
	}
	command := storageports.CommandRecord{ProjectIdentity: c.Project, CommandID: options.RequestID, Kind: kind, TargetID: task.TaskID, ExpectedVersion: options.ExpectedTaskVersion, IdempotencyKey: options.RequestID, RequestedBy: options.RequestedBy, Payload: requestPayload, Status: "in_flight", CreatedAt: created}
	operation := executionmodel.OperationRecord{ProjectIdentity: c.Project, OperationID: options.RequestID, IdempotencyKey: options.RequestID, Kind: kind, Status: "pending", PayloadDigest: operationDigest(operationRequest), Request: operationRequest, CreatedAt: created, UpdatedAt: created}
	return sqlite.CommitTaskClaim(ctx, storageports.ClaimCommit{Before: task, After: nextTask, Contract: contract, AuthorityVersion: options.ExpectedAuthorityVersion, GraphRevision: graph.Authority.GraphRevision, Candidate: options.startCandidate, Explicit: options.Explicit, ExpectedPRBindings: executionmodel.CandidatePRBindings(candidateSnapshot.WorkflowRuns, task.TaskID), PredecessorMerges: predecessorMerges, Workflow: wfr, Execution: execution, Attempt: attempt, Command: command, Operation: operation})
}

func replayTaskClaim(old storageports.CommandRecord, payload []byte, taskID string, requestedBy string) (executionmodel.TaskClaimReceipt, error) {
	return replayTaskStart(old, payload, taskID, requestedBy, "claim_task")
}

func replayTaskStart(old storageports.CommandRecord, payload []byte, taskID string, requestedBy, kind string) (executionmodel.TaskClaimReceipt, error) {
	if old.Kind != kind || old.TargetID != taskID || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, payload) {
		return executionmodel.TaskClaimReceipt{}, nativeErr("idempotency_conflict", "request ID was reused with different claim content")
	}
	if old.Status != "accepted" || len(old.Result) == 0 {
		return executionmodel.TaskClaimReceipt{}, nativeErr("idempotency_conflict", "claim request has no committed result")
	}
	var receipt executionmodel.TaskClaimReceipt
	if err := json.Unmarshal(old.Result, &receipt); err != nil {
		return receipt, nativeErr("unsupported_schema", "saved claim receipt is unreadable")
	}
	return receipt, nil
}

func acquireNativeBase(ctx context.Context, c Config) (string, error) {
	ref := fmt.Sprintf("refs/remotes/%s/%s", c.Remote, c.Branch)
	spec := fmt.Sprintf("+refs/heads/%s:%s", c.Branch, ref)
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout), ProcessHost: c.ProcessHost}
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
