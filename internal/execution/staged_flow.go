package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func stageInput(w *executionmodel.WorkflowRunRecord, command string) *executionmodel.StageInput {
	for i := range w.StageInputs {
		if w.StageInputs[i].CommandID == command {
			return &w.StageInputs[i]
		}
	}
	return nil
}

func firstNonemptyStage(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (s *Service) startStageFeedback(ctx context.Context, run *reconstructiontypes.RunView, answer executionmodel.DecisionAnswer, commandID string, requests ...executionmodel.DecisionRequest) error {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	request := executionmodel.DecisionRequest{}
	if len(requests) > 0 {
		request = requests[0]
	} else if run.Decision != nil {
		request = run.Decision.Request
	} else {
		return errors.New("feedback lost its original request")
	}
	w := fixed.workflow
	if w.Status == "reconciling" || s.hasUnknownWorkerOperation(run) {
		return errors.New("unknown original outcomes require read-only reconciliation")
	}
	if e := fixed.execution; e.PendingCommandID != "" {
		pending := e.PendingCommandID
		if s.operationByKey("worker-turn:"+e.ExecutionID+":"+pending) != nil || s.operationByKey("worker-session:"+e.ExecutionID+":"+pending) != nil {
			return errors.New("original native input must be reconciled before feedback")
		}
		// Explicit new feedback supersedes an input proven never submitted.
		// Its immutable history remains; it cannot be replayed by a later tick.
		e.PendingCommandID, e.PendingTurnID = "", ""
	}
	if w.StepID == executionmodel.StepAwaitMerge || answer.OptionID == "implement" {
		if answer.OptionID == "publish" {
			w.StepID = executionmodel.StepPublish
		} else {
			w.StepID, w.CandidateSHA, w.ReportRetry = executionmodel.StepImplement, "", 0
		}
	}
	feedback, err := json.Marshal(struct {
		Type       string `json:"type"`
		Answer     string `json:"answer"`
		Diagnostic string `json:"blocker,omitempty"`
	}{"feedback", workerFeedbackText(answer, request), w.StageDiagnostic})
	if err != nil {
		return err
	}
	if w.StepID == executionmodel.StepImplement {
		return s.startWorkerFlowTurn(ctx, run, "Current step: implement.execute. Apply the accepted feedback to the original fixed Spec/Plan. Complete implementation, required checks, self-review and a local candidate commit; do not publish. End normally, without JSON.\n"+string(feedback), commandID)
	}
	input := executionmodel.StageInput{CommandID: commandID, AttemptID: fixed.attempt.AttemptID, StepID: w.StepID, CandidateSHA: w.CandidateSHA}
	if w.StepID == executionmodel.StepReport {
		for i := len(w.StageInputs) - 1; i >= 0; i-- {
			if w.StageInputs[i].StepID == executionmodel.StepReport {
				previous := w.StageInputs[i]
				input.SourceCommandID, input.SourceTurnID, input.Snapshot, input.ReportToolPolicy = previous.SourceCommandID, previous.SourceTurnID, previous.Snapshot, previous.ReportToolPolicy
				break
			}
		}
	} else if w.StepID == executionmodel.StepPublish {
		lifecycle, _, err := s.lifecycleForRun(run)
		if err != nil {
			return err
		}
		facts, err := s.inspectStage(ctx, run, lifecycle, fixed)
		if err != nil {
			return err
		}
		if facts.Workspace.Head != w.CandidateSHA || facts.Workspace.Dirty {
			return errors.New("publish continuation candidate changed")
		}
		input.Snapshot = facts
	}
	prompt, err := renderStageInput(w, input)
	if err != nil {
		return err
	}
	w.InputVersion++
	input.Version = w.InputVersion
	workspaceInstruction, err := workerWorkspaceInstruction(fixed.execution.Workspace)
	if err != nil {
		return err
	}
	input.Prompt = prompt + "\n" + string(feedback) + workspaceInstruction + publicationTargetInstruction(fixed.config)
	w.StageInputs = append(w.StageInputs, input)
	return s.startWorkerFlowTurn(ctx, run, input.Prompt, commandID)
}

func stageResult(w *executionmodel.WorkflowRunRecord, command string) *executionmodel.StageResult {
	for i := range w.StageResults {
		if w.StageResults[i].CommandID == command {
			return &w.StageResults[i]
		}
	}
	return nil
}

func sameStageSnapshot(a, b *executionmodel.StageSnapshot) bool {
	if a == nil || b == nil {
		return false
	}
	x, y := *a, *b
	x.Workspace.CheckedAt, y.Workspace.CheckedAt = time.Time{}, time.Time{}
	return x.Workspace.ContentDigest != "" && reflect.DeepEqual(x, y)
}

func (s *Service) inspectStage(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort, fixed workerFlowContext) (*executionmodel.StageSnapshot, error) {
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return nil, err
	}
	workspace, err := lifecycle.InspectLifecycleWorkspace(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	if workspace.Head == "" || workspace.Branch != fixed.execution.Workspace.Branch || workspace.ContentDigest == "" {
		return nil, errors.New("workspace facts are incomplete or changed binding")
	}
	value := &executionmodel.StageSnapshot{Workspace: workspace}
	if fixed.workflow.PRURL != "" {
		info, err := publicationClient(fixed.config, s.runtime.ProcessHost).Query(ctx, fixed.workflow.PRURL)
		if err != nil {
			return nil, err
		}
		value.PR = &info
	}
	return value, nil
}

func renderStageInput(w *executionmodel.WorkflowRunRecord, input executionmodel.StageInput) (string, error) {
	var text string
	switch input.StepID {
	case executionmodel.StepReport:
		source := stageResult(w, input.SourceCommandID)
		original := stageInput(w, input.SourceCommandID)
		if source == nil || original == nil || original.StepID != executionmodel.StepImplement || source.StepID != executionmodel.StepImplement || source.TurnID != input.SourceTurnID || source.Snapshot == nil {
			return "", errors.New("report lacks its original execution facts")
		}
		// Bound evidence is serialized as data, never promoted to instructions.
		sourceValue := *source
		truncated := len(sourceValue.Final) > 16*1024
		if truncated {
			sourceValue.Final = clip(sourceValue.Final, 16*1024)
		}
		goal := original.Prompt
		goalTruncated := len(goal) > 8*1024
		if goalTruncated {
			goal = clip(goal, 8*1024)
		}
		facts, err := json.Marshal(struct {
			OriginalGoal   string                        `json:"original_goal"`
			GoalTruncated  bool                          `json:"goal_truncated"`
			Source         executionmodel.StageResult    `json:"source_execution"`
			Snapshot       *executionmodel.StageSnapshot `json:"snapshot"`
			ToolPolicy     string                        `json:"tool_policy"`
			FinalTruncated bool                          `json:"final_truncated"`
		}{goal, goalTruncated, sourceValue, input.Snapshot, input.ReportToolPolicy, truncated})
		if err != nil {
			return "", err
		}
		text = "Current step: implement.report. Only summarize the supplied original execution, existing outputs and failures. Do not change code, retry commands or perform additional checks. Unknown evidence must remain unknown. Report generation does not make the source execution successful.\nFacts:\n" + string(facts)
	case executionmodel.StepPublish:
		facts, err := json.Marshal(struct {
			CandidateSHA string                        `json:"candidate_sha"`
			Snapshot     *executionmodel.StageSnapshot `json:"snapshot"`
		}{input.CandidateSHA, input.Snapshot})
		if err != nil || !stageSHA.MatchString(input.CandidateSHA) {
			return "", errors.New("publish lacks its frozen candidate")
		}
		text = "Current step: publish. Publish the specified candidate SHA to the fixed target, create or reuse its matching PR. Do not modify the candidate code or create another commit. First query existing branch/PR results before completing unfinished actions. Return the actual PR URL or partial results, blockers and unresolved actions in this same turn.\nFacts:\n" + string(facts)
	default:
		return "", errors.New("no automatic prompt for this step")
	}
	return text + "\n" + stageResultContract, nil
}

// queueStage changes the one business authority and freezes the next input
// intent inside the same worker commit as the accepted source result.
func (s *Service) queueStage(run *reconstructiontypes.RunView, w *executionmodel.WorkflowRunRecord, e *executionmodel.ExecutionRecord, input executionmodel.StageInput, fixed workerFlowContext, manualReason ...string) error {
	input.CommandID, input.AttemptID = newID(), fixed.attempt.AttemptID
	w.InputVersion++
	input.Version = w.InputVersion
	prompt, err := renderStageInput(w, input)
	if err != nil {
		return err
	}
	workspaceInstruction, err := workerWorkspaceInstruction(fixed.execution.Workspace)
	if err != nil {
		return err
	}
	input.Prompt = prompt + workspaceInstruction + publicationTargetInstruction(fixed.config)
	w.StepID, w.Status, w.UpdatedAt = input.StepID, "queued", now()
	w.StageDiagnostic = ""
	w.StageInputs = append(w.StageInputs, input)
	e.PendingCommandID, e.PendingTurnID = input.CommandID, ""
	if len(manualReason) > 0 && manualReason[0] != "" {
		w.Status, w.StageDiagnostic = "waiting_input", manualReason[0]
		if run.Decision == nil || run.Decision.Status != "answer_saved" {
			decision, err := WriteDecisionRequest(run, executionmodel.DecisionRequest{Question: "Confirm continuation of the original step", Context: manualReason[0], Options: []executionbackend.DecisionOption{{ID: "continue", Label: "Continue the original step"}}, AllowFreeText: true})
			if err != nil {
				return err
			}
			s.setRunDecision(run, decision)
			if record := s.decisionRecord(run); record != nil {
				record.RecoveryVersion = e.RecoveryVersion
				record.RecoveryState = e.RecoveryState
			}
		}
	}
	s.refreshRun(run)
	return s.commitWorker(run)
}

func (s *Service) stageWait(run *reconstructiontypes.RunView, w *executionmodel.WorkflowRunRecord, e *executionmodel.ExecutionRecord, status, reason string, report *stageReport) error {
	w.Status, w.StageDiagnostic, w.UpdatedAt = status, reason, now()
	e.Error = ptr(clip(reason, 2000))
	if report == nil && status == "blocked" {
		report = &stageReport{Summary: reason}
	}
	if report != nil {
		e.Summary = ptr(report.Summary)
		if (status == "waiting_input" || status == "blocked") && (run.Decision == nil || run.Decision.Status != "answer_saved") {
			decision, err := WriteDecisionRequest(run, executionmodel.DecisionRequest{Question: firstNonemptyStage(report.Question, "Resolve the current step blocker"), Context: report.Summary, Options: []executionbackend.DecisionOption{{ID: "continue", Label: "Continue this same step"}, {ID: "implement", Label: "Apply code feedback in implement"}}, AllowFreeText: true})
			if err != nil {
				return err
			}
			s.setRunDecision(run, decision)
		}
	}
	s.refreshRun(run)
	return s.commitWorker(run)
}

// acceptStageTurn consumes only adapter-normalized, stopped native evidence.
// The adapter never decodes business JSON or chooses a successor.
func (s *Service) acceptStageTurn(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort, fixed workerFlowContext, evidence executionbackend.TurnEvidence) error {
	w, e := fixed.workflow, fixed.execution
	if previous := stageResult(w, evidence.CommandID); previous != nil {
		if previous.TurnID != evidence.TurnID || previous.SessionID != evidence.SessionID || previous.Final != evidence.LastMessage || previous.Succeeded != evidence.Succeeded || previous.FinalSource != evidence.FinalSource || previous.FinalItemID != evidence.FinalItemID || previous.FinalVerified != evidence.FinalVerified || previous.FinalNativeSessionID != evidence.FinalNativeSessionID || previous.FinalNativeTurnID != evidence.FinalNativeTurnID || previous.Failure != evidence.Failure || previous.FinalDiagnostic != evidence.FinalDiagnostic {
			return errors.New("conflicting duplicate stage result")
		}
		return nil // first-wins, including a replay after the step advanced
	}
	if w.FlowRef != executionmodel.DevelopmentFlow || !executionmodel.ModelStep(w.StepID) {
		return errors.New("fixed flow/step has no rule")
	}
	input := stageInput(w, evidence.CommandID)
	if input == nil || input.AttemptID != fixed.attempt.AttemptID || input.Version != w.InputVersion || input.StepID != w.StepID ||
		e.CurrentCommandID != evidence.CommandID || e.CurrentTurnID != evidence.TurnID || e.SessionID != evidence.SessionID || !evidence.Started || !evidence.Completed {
		return executionmodel.ErrTurnStale
	}
	if err := s.recordDeclaredFinal(ctx, run, evidence); err != nil {
		return err
	}
	if err := s.stopVerifiedWorkerTurn(run, w, e, evidence.TurnID, map[bool]string{true: "succeeded", false: "failed"}[evidence.Succeeded], false); err != nil {
		return err
	}
	if (evidence.FinalOrigin == "worker_declared" || input.StepID != executionmodel.StepImplement || evidence.Succeeded) && (!evidence.FinalVerified || evidence.FinalSource == "" || evidence.FinalItemID == "") {
		return s.stageWait(run, w, e, "reconciling", "original stopped turn lacks verified final provenance: "+evidence.FinalDiagnostic, nil)
	}
	result := executionmodel.StageResult{CommandID: evidence.CommandID, AttemptID: input.AttemptID, StepID: input.StepID, InputVersion: input.Version, TurnID: evidence.TurnID, SessionID: evidence.SessionID, Succeeded: evidence.Succeeded, FinalVerified: evidence.FinalVerified, Final: evidence.LastMessage, FinalNativeSessionID: evidence.FinalNativeSessionID, FinalNativeTurnID: evidence.FinalNativeTurnID, FinalDiagnostic: evidence.FinalDiagnostic, FinalSource: evidence.FinalSource, FinalItemID: evidence.FinalItemID, Failure: evidence.Failure}
	facts, inspectErr := s.inspectStage(ctx, run, lifecycle, fixed)
	result.Snapshot = facts
	if facts != nil {
		workspace := facts.Workspace
		applyWorkflowWorkspaceEvidence(run, e, executionmodel.WorkspaceEvidence{Repository: workspace.Repository, Worktree: workspace.Worktree, Branch: workspace.Branch, Base: workspace.Base, Head: workspace.Head, Dirty: workspace.Dirty, VerifiedAt: workspace.CheckedAt})
	}
	if inspectErr != nil {
		return s.stageWait(run, w, e, "reconciling", "workspace/publication facts unknown: "+inspectErr.Error(), nil)
	}
	w.StageResults = append(w.StageResults, result)
	manualReason := ""
	if e.RecoveryRequired || e.OwnerServiceInstanceID != s.State.Service.InstanceID || len(e.PendingQuestions) != 0 || s.hasUnknownWorkerOperation(run) || e.Environment != nil && (!e.Environment.Ready || e.Environment.Blocker != nil) || run.Decision != nil && run.Decision.Status == "answer_saved" {
		manualReason = "original session or prior-service input requires explicit continuation; no automatic next turn"
	}
	if input.StepID == executionmodel.StepImplement {
		policy := lifecycle.PolicyCapabilities().ReportToolPolicy
		if policy == "" {
			policy = "frozen_native_permissions; fact_comparison_only; external_writes_not_strongly_isolated"
		}
		return s.queueStage(run, w, e, executionmodel.StageInput{StepID: executionmodel.StepReport, SourceCommandID: input.CommandID, SourceTurnID: evidence.TurnID, Snapshot: facts, ReportToolPolicy: policy}, fixed, manualReason)
	}
	if input.StepID == executionmodel.StepReport && !sameStageSnapshot(input.Snapshot, facts) {
		return s.stageWait(run, w, e, "blocked", "artifact_mismatch: report changed workspace or bound publication facts", nil)
	}
	if input.StepID == executionmodel.StepPublish && (facts.Workspace.Head != input.CandidateSHA || facts.Workspace.Dirty) {
		return s.stageWait(run, w, e, "blocked", "artifact_mismatch: publication changed the frozen candidate", nil)
	}
	report, err := decodeStageReport(evidence.LastMessage, input.StepID)
	if !evidence.FinalVerified || evidence.FinalSource == "" || evidence.FinalItemID == "" {
		err = errors.New("result lacks verified native final provenance: " + evidence.FinalDiagnostic)
	}
	if err == nil && input.StepID == executionmodel.StepPublish && report.Outputs.PRURL != "" {
		w.PRCandidateURL = report.Outputs.PRURL
	}
	if err == nil && report.Outcome == "blocked" && report.Blocker.Category == "unknown_external" {
		w.StageResults[len(w.StageResults)-1].Outcome = report.Outcome
		return s.stageWait(run, w, e, "reconciling", fmt.Sprintf("unknown_external: %s; %s; known=%s; unknown=%s", report.Blocker.Action, report.Blocker.Error, report.Blocker.Known, report.Blocker.Unknown), &report)
	}
	if err != nil || !evidence.Succeeded && report.Outcome == "done" {
		reason := "result_invalid: " + evidence.Failure
		if err != nil {
			reason += "; " + err.Error()
		}
		if input.StepID == executionmodel.StepReport && w.ReportRetry == 0 {
			w.ReportRetry = 1
			return s.queueStage(run, w, e, executionmodel.StageInput{StepID: input.StepID, SourceCommandID: input.SourceCommandID, SourceTurnID: input.SourceTurnID, Snapshot: input.Snapshot, ReportToolPolicy: input.ReportToolPolicy}, fixed, manualReason)
		}
		return s.stageWait(run, w, e, "blocked", reason, nil)
	}
	w.StageResults[len(w.StageResults)-1].Outcome = report.Outcome
	if input.StepID == executionmodel.StepPublish && report.Outputs.PRURL != "" {
		w.PRCandidateURL = report.Outputs.PRURL
	}
	if report.Outcome == "needs_input" {
		return s.stageWait(run, w, e, "waiting_input", report.Question, &report)
	}
	if report.Outcome == "blocked" {
		status := "blocked"
		if report.Blocker.Category == "unknown_external" {
			status = "reconciling"
		}
		return s.stageWait(run, w, e, status, fmt.Sprintf("%s: %s; %s; known=%s; unknown=%s", report.Blocker.Category, report.Blocker.Action, report.Blocker.Error, report.Blocker.Known, report.Blocker.Unknown), &report)
	}
	if input.StepID == executionmodel.StepReport {
		source := stageResult(w, input.SourceCommandID)
		if source == nil || source.TurnID != input.SourceTurnID || !source.Succeeded || source.Failure != "" {
			return s.stageWait(run, w, e, "blocked", "source execution failed; report generated, original failure retained", &report)
		}
		if !source.FinalVerified || source.FinalSource == "" || source.FinalItemID == "" {
			return s.stageWait(run, w, e, "reconciling", "source final provenance is unavailable; report generated, evidence gap retained", &report)
		}
		if source.Snapshot == nil || !sameStageSnapshot(source.Snapshot, facts) || report.Outputs.CandidateSHA != facts.Workspace.Head || facts.Workspace.Dirty {
			return s.stageWait(run, w, e, "blocked", "artifact_mismatch: candidate does not match source execution snapshot", &report)
		}
		w.CandidateSHA = report.Outputs.CandidateSHA
		e.Summary = ptr(report.Summary)
		return s.queueStage(run, w, e, executionmodel.StageInput{StepID: executionmodel.StepPublish, CandidateSHA: w.CandidateSHA, Snapshot: facts}, fixed, manualReason)
	}
	info, queryErr := publicationClient(fixed.config, s.runtime.ProcessHost).Query(ctx, report.Outputs.PRURL)
	if queryErr != nil {
		return s.stageWait(run, w, e, "reconciling", "publication result unknown: "+queryErr.Error(), &report)
	}
	return s.acceptStagePublication(run, fixed, input, facts, report, info)
}

func (s *Service) acceptStagePublication(run *reconstructiontypes.RunView, fixed workerFlowContext, input *executionmodel.StageInput, facts *executionmodel.StageSnapshot, report stageReport, info executionmodel.PRInfo) error {
	w, e := fixed.workflow, fixed.execution
	if facts.Workspace.Head != input.CandidateSHA || facts.Workspace.Dirty || s.hasUnknownWorkerOperation(run) {
		return s.stageWait(run, w, e, "reconciling", "original publication cannot settle while candidate or native operation facts differ", &report)
	}
	if err := publication.ValidateWorkerPRIdentity(info, report.Outputs.PRURL, fixed.config.PR.Repository, w.Branch, facts.Workspace.Branch); err != nil || info.HeadRevision != input.CandidateSHA || info.State != "open" && info.State != "merged" {
		return s.stageWait(run, w, e, "blocked", "artifact_mismatch: PR repository, branch, base or candidate differs", &report)
	}
	if err := BindWorkerPR(w, e, info, report.Outputs.PRURL, fixed.config.PR.Repository, false); err != nil {
		return s.stageWait(run, w, e, "reconciling", err.Error(), &report)
	}
	w.StepID, w.Status, w.StageDiagnostic, w.UpdatedAt = executionmodel.StepAwaitMerge, "waiting_external", "", now()
	e.Summary, e.Error = ptr(report.Summary), nil
	if err := s.offerWorkerPRFeedback(run, info.URL); err != nil {
		return err
	}
	s.refreshRun(run)
	return s.commitWorker(run)
}

func (s *Service) dispatchQueuedStage(ctx context.Context, run *reconstructiontypes.RunView) (bool, error) {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return false, err
	}
	w, e := fixed.workflow, fixed.execution
	if w.Status != "queued" || e.PendingCommandID == "" {
		return false, nil
	}
	input := stageInput(w, e.PendingCommandID)
	if input == nil {
		return false, nil
	}
	if e.RecoveryRequired || e.OwnerServiceInstanceID != s.State.Service.InstanceID {
		return true, nil
	}
	if s.workers[run.RunID] != nil || s.lifecycleEffectPending(run.RunID) {
		return true, nil
	}
	if input.Version != w.InputVersion || input.StepID != w.StepID || strings.TrimSpace(input.Prompt) == "" {
		return true, errors.New("queued input lost its frozen binding")
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return true, err
	}
	if s.operationByKey("worker-turn:"+e.ExecutionID+":"+input.CommandID) == nil && s.operationByKey("worker-session:"+e.ExecutionID+":"+input.CommandID) == nil {
		if err := verifyPublicationTargetForRun(ctx, fixed.config, s.runtime.Directory, s.runtime.ProcessHost, run); err != nil {
			return true, s.stageWait(run, w, e, "blocked", "fixed publication target changed: "+err.Error(), nil)
		}
		if input.StepID == executionmodel.StepReport || input.StepID == executionmodel.StepPublish {
			facts, err := s.inspectStage(ctx, run, lifecycle, fixed)
			if err != nil {
				return true, s.stageWait(run, w, e, "reconciling", "pre-dispatch facts unknown: "+err.Error(), nil)
			}
			if input.StepID == executionmodel.StepReport && !sameStageSnapshot(input.Snapshot, facts) || input.StepID == executionmodel.StepPublish && (facts.Workspace.Head != input.CandidateSHA || facts.Workspace.Dirty) {
				return true, s.stageWait(run, w, e, "blocked", "artifact_mismatch before input dispatch", nil)
			}
		}
	}
	return true, s.startWorkerLifecycle(ctx, run, lifecycle, fixed, input.CommandID, input.Prompt)
}

// Business-side unknowns are observed without sending a new model input. The
// original result remains first-wins. Only actual merged delivery proof can
// settle an otherwise failed/unknown publisher through the existing gate.
func (s *Service) reconcileStageFacts(ctx context.Context, run *reconstructiontypes.RunView) error {
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	w, e := fixed.workflow, fixed.execution
	if w.Status != "reconciling" || e.CurrentTurnID != e.LastStoppedTurnID || e.PendingCommandID != "" {
		return nil
	}
	if w.PRQueriedAt != nil && now().Sub(*w.PRQueriedAt) < 30*time.Second {
		return nil
	}
	w.PRQueriedAt = ptr(now())
	// An unavailable observation is not an accepted result. Re-read only the
	// original stopped native turn; no new input or changed source is allowed.
	if stageResult(w, e.CurrentCommandID) == nil && stageInput(w, e.CurrentCommandID) != nil {
		snapshot, err := s.lifecycleSnapshot(run, w, e, fixed.attempt, nil)
		if err != nil {
			return err
		}
		receipt, observer, readErr := lifecycle.ObserveLifecycle(ctx, snapshot)
		if observer != nil {
			_ = observer.Close(ctx)
		}
		if readErr == nil && receipt.TurnEvidence != nil && receipt.TurnEvidence.Completed {
			return s.acceptStageTurn(ctx, run, lifecycle, fixed, *receipt.TurnEvidence)
		}
		w.StageDiagnostic = "original stopped turn remains unavailable: " + firstNonemptyStage(receipt.Diagnostic, fmt.Sprint(readErr))
		s.refreshRun(run)
		return s.commitWorker(run)
	}
	facts, err := s.inspectStage(ctx, run, lifecycle, fixed)
	if err != nil {
		w.StageDiagnostic = "read-only reconciliation: " + err.Error()
		s.refreshRun(run)
		return s.commitWorker(run)
	}
	if w.StepID == executionmodel.StepPublish && w.PRCandidateURL != "" {
		info, err := publicationClient(fixed.config, s.runtime.ProcessHost).Query(ctx, w.PRCandidateURL)
		if err != nil {
			w.PRQueryError = ptr(err.Error())
			s.refreshRun(run)
			return s.commitWorker(run)
		}
		if err := publication.ValidateWorkerPRIdentity(info, w.PRCandidateURL, fixed.config.PR.Repository, w.Branch, facts.Workspace.Branch); err != nil || info.HeadRevision != w.CandidateSHA {
			w.StageDiagnostic = "artifact_mismatch in read-only publication reconciliation"
			s.refreshRun(run)
			return s.commitWorker(run)
		}
		w.PRState, w.PRQueryError = info.State, nil
		w.StageDiagnostic = "original publication observed by read-only reconciliation; original outcome retained"
		result, input := stageResult(w, e.CurrentCommandID), stageInput(w, e.CurrentCommandID)
		if result != nil && input != nil && result.StepID == executionmodel.StepPublish && result.Succeeded && result.FinalVerified && result.Outcome == "done" {
			report, decodeErr := decodeStageReport(result.Final, executionmodel.StepPublish)
			if decodeErr == nil && report.Outputs.PRURL == w.PRCandidateURL {
				return s.acceptStagePublication(run, fixed, input, facts, report, info)
			}
		}
		if info.Merged && info.State == "merged" && info.MergedAt != nil && info.MergeCommit != nil {
			if err := BindWorkerPR(w, e, info, w.PRCandidateURL, fixed.config.PR.Repository, false); err != nil {
				return err
			}
			w.StepID, w.Status = executionmodel.StepAwaitMerge, "waiting_external"
			s.refreshRun(run)
			if err := s.commitWorker(run); err != nil {
				return err
			}
			return s.nativeCompleteMergedTask(ctx, run)
		}
	}
	s.refreshRun(run)
	return s.commitWorker(run)
}
