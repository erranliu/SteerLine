package steerline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
)

const existingAcceptanceVersion = "existing-acceptance/v1"

// ExistingAcceptanceInput is an operator closeout for the two Phase 1 Tasks
// whose implementation was merged before their native Task records existed.
// It cannot replace a running worker, an unknown step, or a normal PR delivery.
type ExistingAcceptanceInput struct {
	Version             string                  `json:"version"`
	TaskID              string                  `json:"task_id"`
	WorkflowRunID       string                  `json:"workflow_run_id"`
	ExpectedTaskVersion uint64                  `json:"expected_task_version"`
	RequestID           string                  `json:"request_id"`
	RequestedBy         string                  `json:"requested_by"`
	PrimaryPRURL        string                  `json:"primary_pr_url"`
	MergedPRs           []ExistingMergedPRInput `json:"merged_prs"`
	EvidenceRef         FileReference           `json:"evidence_ref"`
	StoppedUnknown      *ExistingStoppedUnknown `json:"stopped_unknown,omitempty"`
}

// ExistingStoppedUnknown names the one #4 verify worker whose exact Dispatch
// was abandoned and whose original terminal has since been observed closed.
type ExistingStoppedUnknown struct {
	AttemptID string `json:"attempt_id"`
	Reason    string `json:"reason"`
}

type ExistingMergedPRInput struct {
	URL          string `json:"url"`
	HeadRevision string `json:"head_revision"`
}

func DecodeExistingAcceptanceInput(data []byte) (ExistingAcceptanceInput, error) {
	var input ExistingAcceptanceInput
	if _, err := CanonicalJSON(data); err != nil {
		return input, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, err
	}
	if input.Version != existingAcceptanceVersion || ValidateNativeTaskID(input.TaskID) != nil || input.WorkflowRunID == "" || input.ExpectedTaskVersion == 0 || !controlIDPattern.MatchString(input.RequestID) || strings.TrimSpace(input.RequestedBy) == "" || input.PrimaryPRURL == "" || len(input.MergedPRs) == 0 || len(input.MergedPRs) > 8 {
		return input, nativeErr("contract_mismatch", "existing acceptance requires exact task, run, version, request, operator, primary PR and bounded merged PRs")
	}
	if err := validateFileReferenceShape(input.EvidenceRef); err != nil || !strings.HasPrefix(input.EvidenceRef.Path, "docs/validation/") {
		return input, nativeErr("contract_mismatch", "existing acceptance requires one pinned validation report")
	}
	seen, primary := map[string]bool{}, false
	for _, pr := range input.MergedPRs {
		if pr.URL == "" || !validCommit(pr.HeadRevision) || seen[pr.URL] {
			return input, nativeErr("contract_mismatch", "existing acceptance PRs need distinct exact URLs and heads")
		}
		seen[pr.URL] = true
		primary = primary || pr.URL == input.PrimaryPRURL
	}
	if !primary {
		return input, nativeErr("contract_mismatch", "primary PR is missing from the merged PR list")
	}
	if input.StoppedUnknown != nil {
		if input.TaskID != "4" || !stableExecutionID(input.StoppedUnknown.AttemptID) {
			return input, nativeErr("contract_mismatch", "stopped unknown reconciliation is limited to #4 and one exact Attempt")
		}
		if _, err := validateConfirmationBasis(input.StoppedUnknown.Reason); err != nil {
			return input, err
		}
	}
	return input, nil
}

func existingAcceptanceGatePrefix(taskID string, contract TaskContract) (string, error) {
	switch {
	case taskID == "3" && contract.ContractID == "steerline-roadmap-3-49ac56e45454" && contract.SpecRef.Path == "docs/spec/pr-05-intervention.spec.md":
		return "H-A", nil
	case taskID == "4" && contract.ContractID == "steerline-roadmap-4-49ac56e45454" && contract.SpecRef.Path == "docs/spec/01-orca-remote-protocol.md":
		return "P", nil
	default:
		return "", nativeErr("immutable_task", "existing acceptance is limited to the original Phase 1 acceptance Tasks #3 and #4")
	}
}

func existingAcceptanceGateIDs(spec, report []byte, prefix string) ([]string, error) {
	pattern := regexp.MustCompile(`(?m)^\|[ \t]*(` + regexp.QuoteMeta(prefix) + `[0-9]+)[ \t]*\|`)
	set := map[string]bool{}
	for _, match := range pattern.FindAllSubmatch(spec, -1) {
		set[string(match[1])] = true
	}
	if len(set) == 0 {
		return nil, nativeErr("contract_mismatch", "pinned Spec has no numbered acceptance gates")
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		if !bytes.Contains(report, []byte("| "+id+" |")) {
			return nil, nativeErr("contract_mismatch", "validation report does not account for acceptance gate "+id)
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func existingAcceptanceRunState(snapshot DomainSnapshot, runtimeState Runtime, taskID, workflowRunID string) (WorkflowRunRecord, ExecutionRecord, []StepAttemptRecord, error) {
	var workflow *WorkflowRunRecord
	for i := range snapshot.WorkflowRuns {
		candidate := &snapshot.WorkflowRuns[i]
		if candidate.TaskID == taskID && candidate.WorkflowRunID == workflowRunID {
			workflow = candidate
		}
	}
	if workflow == nil || workflow.WorkflowVersion != MinimalWorkflowVersion || workflow.Phase != "blocked" || workflow.SelectedResult != "" || workflow.PendingDecisions != 0 {
		return WorkflowRunRecord{}, ExecutionRecord{}, nil, nativeErr("task_not_ready", "acceptance closeout requires the original unpublished blocked workflow")
	}
	var execution *ExecutionRecord
	for i := range snapshot.Executions {
		candidate := &snapshot.Executions[i]
		if candidate.WorkflowRunID != workflowRunID {
			continue
		}
		if execution != nil {
			return WorkflowRunRecord{}, ExecutionRecord{}, nil, nativeErr("immutable_task", "acceptance closeout found more than one execution")
		}
		execution = candidate
	}
	if execution == nil || execution.Status != "failed" || execution.Process != nil {
		return WorkflowRunRecord{}, ExecutionRecord{}, nil, nativeErr("outcome_unknown", "acceptance closeout requires a settled failed execution with no process")
	}
	var run *Run
	for _, candidate := range runtimeState.Runs {
		if candidate == nil || candidate.TaskID != taskID {
			continue
		}
		if candidate.RunID == workflowRunID {
			run = candidate
		} else if candidate.ExecutionStatus == "running" || candidate.ExecutionStatus == "pending" || candidate.ExecutionStatus == "unknown" || candidate.FinishTaskStatus == "waiting_pr_merge" {
			return WorkflowRunRecord{}, ExecutionRecord{}, nil, nativeErr("slot_busy", "another workflow for this task is not settled")
		}
	}
	if run == nil || run.ExecutionID != execution.ExecutionID || run.Process != nil || run.ExecutionStatus != "failed" || run.FinishTaskStatus != "blocked" || deref(run.PR.URL) != "" {
		return WorkflowRunRecord{}, ExecutionRecord{}, nil, nativeErr("outcome_unknown", "original run has unresolved execution or PR state")
	}
	attempts := []StepAttemptRecord{}
	for _, attempt := range snapshot.Attempts {
		if attempt.ExecutionID != execution.ExecutionID {
			continue
		}
		switch attempt.Status {
		case "succeeded", "failed", "stopped":
		default:
			return WorkflowRunRecord{}, ExecutionRecord{}, nil, nativeErr("outcome_unknown", "original step attempt is not settled: "+attempt.AttemptID)
		}
		if attempt.StepID == "publish_pr" {
			return WorkflowRunRecord{}, ExecutionRecord{}, nil, nativeErr("merge_state_unknown", "acceptance closeout cannot replace an attempted PR publication")
		}
		attempts = append(attempts, attempt)
	}
	if len(attempts) == 0 {
		return WorkflowRunRecord{}, ExecutionRecord{}, nil, nativeErr("task_not_ready", "acceptance closeout has no original step history")
	}
	return *workflow, *execution, attempts, nil
}

func existingAcceptanceUnknownState(snapshot DomainSnapshot, runtimeState Runtime, input ExistingAcceptanceInput) (WorkflowRunRecord, ExecutionRecord, StepAttemptRecord, *Run, error) {
	if input.StoppedUnknown == nil || input.TaskID != "4" {
		return WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, nil, nativeErr("contract_mismatch", "no fixed #4 unknown Attempt was supplied")
	}
	var workflow *WorkflowRunRecord
	for i := range snapshot.WorkflowRuns {
		w := &snapshot.WorkflowRuns[i]
		if w.TaskID == input.TaskID && w.WorkflowRunID == input.WorkflowRunID {
			workflow = w
		} else if w.TaskID == input.TaskID && w.Phase != "abandoned" && w.Phase != "completed" {
			return WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, nil, nativeErr("slot_busy", "another #4 workflow is active")
		}
	}
	if workflow == nil || workflow.WorkflowVersion != MinimalWorkflowVersion || workflow.Phase != "blocked" || workflow.SelectedResult != "" || workflow.PendingDecisions != 0 || workflow.ExecutionProfile == nil {
		return WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, nil, nativeErr("task_not_ready", "#4 requires its exact blocked unpublished workflow and pinned profile")
	}
	var execution *ExecutionRecord
	for i := range snapshot.Executions {
		e := &snapshot.Executions[i]
		if e.WorkflowRunID == input.WorkflowRunID {
			if execution != nil {
				return WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, nil, nativeErr("immutable_task", "#4 has multiple executions")
			}
			execution = e
		}
	}
	if execution == nil || execution.Status != "blocked" || execution.CurrentStep != "verify" || execution.Process == nil || execution.Process.Kind != "orca_supervised" || execution.Process.Dispatch == "" || execution.Process.Terminal == "" || execution.Process.Incarnation == "" || execution.Process.RuntimeID == "" || execution.Process.ExecutionHost == "" {
		return WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, nil, nativeErr("outcome_unknown", "#4 has no exact unresolved supervised verify worker")
	}
	var attempt *StepAttemptRecord
	for i := range snapshot.Attempts {
		a := &snapshot.Attempts[i]
		if a.ExecutionID != execution.ExecutionID {
			continue
		}
		if a.StepID == "publish_pr" {
			return WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, nil, nativeErr("merge_state_unknown", "#4 has attempted publication")
		}
		if a.AttemptID == input.StoppedUnknown.AttemptID {
			attempt = a
		} else if a.Status != "succeeded" && a.Status != "failed" && a.Status != "stopped" {
			return WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, nil, nativeErr("outcome_unknown", "another #4 Attempt is unresolved")
		}
	}
	if attempt == nil || attempt.StepID != "verify" || attempt.Status != "outcome_unknown" || len(attempt.Result) != 0 || attempt.ExternalBinding.Dispatch != execution.Process.Dispatch {
		return WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, nil, nativeErr("outcome_unknown", "the named #4 verify Attempt is not the fixed unknown worker")
	}
	var run *Run
	for _, candidate := range runtimeState.Runs {
		if candidate == nil {
			continue
		}
		if candidate.RunID == input.WorkflowRunID {
			run = candidate
		} else if candidate.TaskID == input.TaskID && (candidate.ExecutionStatus == "unknown" || candidate.ExecutionStatus == "running" || candidate.ExecutionStatus == "pending") {
			return WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, nil, nativeErr("slot_busy", "another #4 Run is unresolved")
		}
	}
	if run == nil || run.ExecutionID != execution.ExecutionID || run.AttemptID != attempt.AttemptID || run.ExecutionStatus != "unknown" || run.FinishTaskStatus != "blocked" || run.Process == nil || *run.Process != *execution.Process || deref(run.PR.URL) != "" || runtimeState.CurrentTask != nil && *runtimeState.CurrentTask != "4" || !runtimeState.SlotOccupied || runtimeState.Control.Operation != nil {
		return WorkflowRunRecord{}, ExecutionRecord{}, StepAttemptRecord{}, nil, nativeErr("outcome_unknown", "#4 runtime does not match its reserved unknown Attempt")
	}
	return *workflow, *execution, *attempt, run, nil
}

func verifyExistingStoppedUnknown(ctx context.Context, config Config, workflow WorkflowRunRecord, execution ExecutionRecord) error {
	p := execution.Process
	if p == nil || workflow.ExecutionProfile == nil || !workflow.ExecutionProfile.Verify(workflow.ProjectSteps) {
		return nativeErr("contract_mismatch", "#4 pinned execution profile is unavailable")
	}
	fixed := config.ApplyFixedExecutionProfile(*workflow.ExecutionProfile, workflow.ProjectSteps)
	if fixed.hostName() != "orca_supervised" || fixed.Orca.Supervised == nil || fixed.Orca.Supervised.EnvironmentID != p.ExecutionHost {
		return nativeErr("authority_mismatch", "#4 exact remote host differs from its pinned profile")
	}
	client := orcaClient{cfg: fixed}
	out, err := client.call(ctx, positiveDuration(fixed.CommandTimeout, 30*time.Second), "orchestration", "worker-show", "--dispatch", p.Dispatch, "--json")
	if err != nil {
		return nativeErr("outcome_unknown", "original Dispatch cannot be inspected")
	}
	env, err := parseEnvelope(out)
	if err != nil {
		return nativeErr("outcome_unknown", "original Dispatch has no valid observation")
	}
	var worker struct {
		Dispatch struct {
			ID     string `json:"id"`
			RunID  string `json:"runId"`
			TaskID string `json:"taskId"`
			Status string `json:"status"`
		} `json:"dispatch"`
		Worker struct {
			DispatchID          string            `json:"dispatchId"`
			State               string            `json:"state"`
			WorktreeID          string            `json:"worktreeId"`
			AgentTerminalHandle string            `json:"agentTerminalHandle"`
			ResidualResources   []json.RawMessage `json:"residualResources"`
		} `json:"worker"`
		Server struct {
			EnvironmentID string `json:"environmentId"`
		} `json:"server"`
	}
	if json.Unmarshal(env.Result, &worker) != nil || env.Meta.RuntimeID != p.CoordinatorRuntimeID || worker.Dispatch.ID != p.Dispatch || worker.Dispatch.RunID != p.OrchestrationRun || worker.Dispatch.TaskID != p.Task || worker.Dispatch.Status != "failed" || worker.Worker.DispatchID != p.Dispatch || worker.Worker.State != "abandoned" || worker.Worker.WorktreeID != p.WorktreeID || worker.Worker.AgentTerminalHandle != p.Terminal || len(worker.Worker.ResidualResources) != 0 || worker.Server.EnvironmentID != p.ExecutionHost {
		return nativeErr("outcome_unknown", "original Dispatch is not exact, abandoned and resource-free")
	}
	out, err = client.call(ctx, positiveDuration(fixed.CommandTimeout, 30*time.Second), "terminal", "show", "--environment", p.ExecutionHost, "--terminal", p.Terminal, "--json")
	if err != nil {
		return nativeErr("outcome_unknown", "original remote terminal cannot be inspected")
	}
	env, err = parseEnvelope(out)
	if err != nil {
		return nativeErr("outcome_unknown", "original remote terminal has no valid observation")
	}
	var remote struct {
		Terminal orcaTerminal `json:"terminal"`
	}
	if json.Unmarshal(env.Result, &remote) != nil || env.Meta.RuntimeID != p.RuntimeID || remote.Terminal.Handle != p.Terminal || remote.Terminal.IncarnationID != p.Incarnation || remote.Terminal.WorktreeID != p.WorktreeID || remote.Terminal.Connected || remote.Terminal.ExitCause == nil || remote.Terminal.ExitCause.Kind != "operator_close" {
		return nativeErr("outcome_unknown", "original remote terminal is not the exact operator-closed incarnation")
	}
	return nil
}

// CompleteExistingAcceptance uses previously merged PRs only for the fixed
// Phase 1 acceptance Tasks. It leaves their failed worker history unchanged.
func (c CommandService) CompleteExistingAcceptance(ctx context.Context, config Config, input ExistingAcceptanceInput, verifier TaskDeliveryVerifier) (TaskDeliveryReceipt, error) {
	var empty TaskDeliveryReceipt
	encoded, err := json.Marshal(input)
	if err != nil {
		return empty, err
	}
	input, err = DecodeExistingAcceptanceInput(encoded)
	if err != nil {
		return empty, err
	}
	sqlite, ok := c.Store.(*SQLiteStore)
	if !ok || c.Project == "" || config.Identity() != c.Project || verifier == nil {
		return empty, nativeErr("authority_mismatch", "existing acceptance requires the configured SQLite authority and PR verifier")
	}
	inputDigest := digestValue(input)
	graph, err := sqlite.taskGraphSnapshot(ctx, c.Project)
	if err != nil {
		return empty, err
	}
	task, exists := graph.ByID[input.TaskID]
	if !exists {
		return empty, nativeErr("task_not_found", "acceptance task does not exist")
	}
	if prior, delivered := graph.Deliveries[input.TaskID]; delivered {
		var evidence TaskDeliveryEvidence
		if task.Status == "closed" && json.Unmarshal(prior.Evidence, &evidence) == nil && evidence.ExistingAcceptance != nil && evidence.ExistingAcceptance.RequestID == input.RequestID && evidence.ExistingAcceptance.InputDigest == inputDigest {
			return TaskDeliveryReceipt{TaskID: input.TaskID, TaskVersion: task.Version, WorkflowRunID: input.WorkflowRunID, GraphRevision: graph.Authority.GraphRevision, Status: "closed", Delivery: prior}, nil
		}
		return empty, nativeErr("inconsistent_delivery", "task already has another delivery")
	}
	if task.Status != "in_progress" || task.Version != input.ExpectedTaskVersion {
		return empty, nativeErr("stale_version", "acceptance task claim or version changed")
	}
	if err = validateNativeConfiguredTarget(ctx, config, graph.Authority); err != nil {
		return empty, err
	}
	contract, err := nativeContractForTask(ctx, sqlite, c.Project, task)
	if err != nil {
		return empty, err
	}
	prefix, err := existingAcceptanceGatePrefix(task.TaskID, contract)
	if err != nil {
		return empty, err
	}
	if err = VerifyTaskContractFiles(ctx, config, contract); err != nil {
		return empty, err
	}
	runner := Runner{Dir: config.Repository, Timeout: duration(config.CommandTimeout)}
	spec, err := readCommittedRegularFile(ctx, runner, config.Git, contract.SpecRef)
	if err != nil {
		return empty, err
	}
	report, err := readCommittedRegularFile(ctx, runner, config.Git, input.EvidenceRef)
	if err != nil {
		return empty, err
	}
	hash := sha256.Sum256(report)
	if !strings.EqualFold(hex.EncodeToString(hash[:]), input.EvidenceRef.SHA256) {
		return empty, nativeErr("contract_mismatch", "acceptance report hash differs from the fixed Git blob")
	}
	gateIDs, err := existingAcceptanceGateIDs(spec, report, prefix)
	if err != nil {
		return empty, err
	}
	if _, err = runner.Output(ctx, config.Git, "merge-base", "--is-ancestor", input.EvidenceRef.Commit, "HEAD"); err != nil {
		return empty, nativeErr("contract_mismatch", "acceptance report commit is not in the current target branch")
	}
	projection, projectionRevision, err := sqlite.LoadProjection(ctx, c.Project)
	if err != nil {
		return empty, err
	}
	var runtimeState Runtime
	if err = json.Unmarshal(projection, &runtimeState); err != nil || runtimeState.SchemaVersion != 5 {
		return empty, nativeErr("unsupported_schema", "current native runtime projection is unavailable")
	}
	snapshot, err := sqlite.Query(ctx, c.Project)
	if err != nil {
		return empty, err
	}
	var workflow WorkflowRunRecord
	var execution ExecutionRecord
	if input.StoppedUnknown == nil {
		workflow, execution, _, err = existingAcceptanceRunState(snapshot, runtimeState, input.TaskID, input.WorkflowRunID)
	} else {
		workflow, execution, _, _, err = existingAcceptanceUnknownState(snapshot, runtimeState, input)
		if err == nil {
			err = verifyExistingStoppedUnknown(ctx, config, workflow, execution)
		}
	}
	if err != nil {
		return empty, err
	}
	proofs := make([]ExistingMergedPRProof, 0, len(input.MergedPRs))
	var primary PRInfo
	for _, candidate := range input.MergedPRs {
		info, queryErr := verifier.Query(ctx, candidate.URL)
		if queryErr != nil || !info.Merged || info.State != "merged" || info.MergedAt == nil || info.MergedAt.IsZero() || info.MergeCommit == nil || !validCommit(*info.MergeCommit) || info.URL != candidate.URL || info.Repository != config.PR.Repository || info.BaseBranch != graph.Authority.Branch || info.HeadRevision != candidate.HeadRevision {
			return empty, nativeErr("merge_state_unknown", "acceptance PR is not the exact merged repository/base/head: "+candidate.URL)
		}
		if _, mergeErr := runner.Output(ctx, config.Git, "merge-base", "--is-ancestor", *info.MergeCommit, "HEAD"); mergeErr != nil {
			return empty, nativeErr("merge_state_unknown", "acceptance PR merge commit is not in the current target branch")
		}
		proofs = append(proofs, ExistingMergedPRProof{URL: info.URL, HeadRevision: info.HeadRevision, MergeCommit: *info.MergeCommit, MergedAt: *info.MergedAt})
		if info.URL == input.PrimaryPRURL {
			primary = info
		}
	}
	if primary.URL == "" || !primary.MergedAt.Before(task.CreatedAt) {
		return empty, nativeErr("immutable_task", "primary implementation PR must have merged before this acceptance Task was created")
	}
	number, err := config.PR.PRNumber(primary.URL)
	if err != nil {
		return empty, err
	}
	acceptance := &ExistingAcceptanceEvidence{RequestID: input.RequestID, RequestedBy: input.RequestedBy, InputDigest: inputDigest, ContractDigest: contract.Digest, GateIDs: gateIDs, EvidenceRef: input.EvidenceRef, MergedPRs: proofs, StoppedUnknown: input.StoppedUnknown}
	evidence := TaskDeliveryEvidence{PRInstance: strings.TrimSuffix(config.PR.BaseURL, "/"), URL: primary.URL, Repository: primary.Repository, PRNumber: int64(number), BaseBranch: primary.BaseBranch, State: primary.State, Merged: true, MergedAt: *primary.MergedAt, MergeCommit: *primary.MergeCommit, ExistingAcceptance: acceptance}
	evidenceBytes, _ := json.Marshal(evidence)
	delivery := TaskDeliveryRecord{ProjectIdentity: c.Project, TaskID: input.TaskID, SelectedWorkflowRunID: input.WorkflowRunID, PRInstance: evidence.PRInstance, PRRepository: evidence.Repository, PRNumber: evidence.PRNumber, PRURL: evidence.URL, MergeCommit: evidence.MergeCommit, MergedAt: evidence.MergedAt, RecordedAt: time.Now().UTC(), Evidence: evidenceBytes}
	var receipt TaskDeliveryReceipt
	err = sqlite.Transact(ctx, func(domainTx DomainTx) error {
		tx := domainTx.(*sqliteTx)
		currentGraph, graphErr := taskGraphSnapshotTx(ctx, tx, c.Project)
		if graphErr != nil {
			return graphErr
		}
		current, found := currentGraph.ByID[input.TaskID]
		if !found || currentGraph.Authority.AuthorityVersion != graph.Authority.AuthorityVersion || current.Status != "in_progress" || current.Version != input.ExpectedTaskVersion || current.ContractID != contract.ContractID {
			return nativeErr("stale_version", "acceptance task or authority changed during external verification")
		}
		if _, delivered := currentGraph.Deliveries[input.TaskID]; delivered {
			return nativeErr("inconsistent_delivery", "acceptance task gained another delivery")
		}
		var storedProjection []byte
		var storedRevision uint64
		if e := tx.tx.QueryRowContext(ctx, `SELECT payload,revision FROM projection WHERE project_identity=?`, c.Project).Scan(&storedProjection, &storedRevision); e != nil || storedRevision != projectionRevision {
			return nativeErr("stale_version", "runtime projection changed during acceptance verification")
		}
		var currentRuntime Runtime
		if e := json.Unmarshal(storedProjection, &currentRuntime); e != nil {
			return e
		}
		currentSnapshot, e := querySnapshot(ctx, tx.tx, c.Project)
		if e != nil {
			return e
		}
		if input.StoppedUnknown != nil {
			_, currentExecution, unknownAttempt, currentRun, unknownErr := existingAcceptanceUnknownState(currentSnapshot, currentRuntime, input)
			if unknownErr != nil || currentExecution.ExecutionID != execution.ExecutionID {
				return nativeErr("stale_version", "#4 exact unknown worker changed during external verification")
			}
			stamp := time.Now().UTC()
			if e = TransitionStepAttempt(&unknownAttempt, "stopped"); e != nil {
				return e
			}
			if e = tx.PutStepAttempt(unknownAttempt); e != nil {
				return e
			}
			currentExecution.Status, currentExecution.Process, currentExecution.UpdatedAt = "failed", nil, stamp
			if e = tx.PutExecution(currentExecution); e != nil {
				return e
			}
			currentRun.ExecutionStatus, currentRun.Process, currentRun.EndedAt = "failed", nil, &stamp
			currentRun.ManualStop = &ManualStopConfirmation{AttemptID: unknownAttempt.AttemptID, ConfirmedAt: stamp, ConfirmedBy: input.RequestedBy, Basis: input.StoppedUnknown.Reason}
			currentRuntime.CurrentTask = nil
			currentRuntime.SlotOccupied = false
			for _, other := range currentRuntime.Runs {
				if other != nil && other.RunID != currentRun.RunID && (other.ExecutionStatus == "unknown" || other.ExecutionStatus == "pending" || other.ExecutionStatus == "running" || other.FinishTaskStatus == "waiting_decision") {
					return nativeErr("slot_busy", "another worker still occupies the project")
				}
			}
			currentRuntime.Revision++
			projectionBody, marshalErr := json.Marshal(currentRuntime)
			if marshalErr != nil {
				return marshalErr
			}
			if e = tx.PutProjection(c.Project, currentRuntime.Revision, projectionBody); e != nil {
				return e
			}
			body, _ := json.Marshal(map[string]any{"attempt_id": unknownAttempt.AttemptID, "dispatch": execution.Process.Dispatch, "terminal": execution.Process.Terminal, "reason": input.StoppedUnknown.Reason, "requested_by": input.RequestedBy})
			if _, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: input.TaskID, WorkflowRunID: input.WorkflowRunID, ExecutionID: execution.ExecutionID, AttemptID: unknownAttempt.AttemptID, ExternalSource: "native-task-acceptance", ExternalID: input.RequestID + ":reconcile", Type: "task.unknown_reconciled", ReceivedAt: stamp, Payload: body}); e != nil {
				return e
			}
			currentSnapshot, e = querySnapshot(ctx, tx.tx, c.Project)
			if e != nil {
				return e
			}
		}
		currentWorkflow, currentExecution, _, e := existingAcceptanceRunState(currentSnapshot, currentRuntime, input.TaskID, input.WorkflowRunID)
		if e != nil || currentWorkflow.WorkflowRunID != workflow.WorkflowRunID || currentExecution.ExecutionID != execution.ExecutionID {
			return nativeErr("stale_version", "original failed workflow changed during acceptance verification")
		}
		currentContract, e := nativeContractForTaskTx(ctx, tx, c.Project, current)
		if e != nil || currentContract.Digest != contract.Digest {
			return nativeErr("contract_mismatch", "fixed acceptance Task contract changed")
		}
		if e = tx.PutTaskDelivery(delivery); e != nil {
			return e
		}
		next := current
		next.Status, next.Version, next.UpdatedAt = "closed", current.Version+1, time.Now().UTC()
		if e = tx.UpdateNativeTaskCAS(next, current.Version); e != nil {
			return e
		}
		receipt = TaskDeliveryReceipt{TaskID: input.TaskID, TaskVersion: next.Version, WorkflowRunID: input.WorkflowRunID, GraphRevision: currentGraph.Authority.GraphRevision + 1, Status: "closed", Delivery: delivery}
		body, _ := json.Marshal(map[string]any{"receipt": receipt, "existing_acceptance": acceptance})
		_, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: input.TaskID, WorkflowRunID: input.WorkflowRunID, ExecutionID: execution.ExecutionID, ExternalSource: "native-task-acceptance", ExternalID: input.RequestID, Type: "task.accepted_existing", ReceivedAt: delivery.RecordedAt, Payload: body})
		return e
	})
	if err != nil {
		return empty, err
	}
	return receipt, nil
}

func validateExistingAcceptanceProof(evidence TaskDeliveryEvidence) error {
	a := evidence.ExistingAcceptance
	if a == nil {
		return nil
	}
	if !controlIDPattern.MatchString(a.RequestID) || strings.TrimSpace(a.RequestedBy) == "" || !validSHA256(a.InputDigest) || !validSHA256(a.ContractDigest) || len(a.GateIDs) == 0 || len(a.MergedPRs) == 0 || len(a.MergedPRs) > 8 || validateFileReferenceShape(a.EvidenceRef) != nil {
		return nativeErr("inconsistent_delivery", "existing acceptance evidence is incomplete")
	}
	primary, seen := false, map[string]bool{}
	for _, pr := range a.MergedPRs {
		if pr.URL == "" || !validCommit(pr.HeadRevision) || !validCommit(pr.MergeCommit) || pr.MergedAt.IsZero() || seen[pr.URL] {
			return nativeErr("inconsistent_delivery", "existing acceptance PR proof is incomplete or duplicated")
		}
		seen[pr.URL] = true
		if pr.URL == evidence.URL {
			primary = pr.MergeCommit == evidence.MergeCommit && pr.MergedAt.Equal(evidence.MergedAt)
		}
	}
	if !primary {
		return nativeErr("inconsistent_delivery", "existing acceptance primary PR differs from delivery")
	}
	if a.StoppedUnknown != nil {
		if !stableExecutionID(a.StoppedUnknown.AttemptID) {
			return nativeErr("inconsistent_delivery", "settled unknown Attempt identity is incomplete")
		}
		if _, err := validateConfirmationBasis(a.StoppedUnknown.Reason); err != nil {
			return nativeErr("inconsistent_delivery", "settled unknown evidence basis is incomplete")
		}
	}
	return nil
}
