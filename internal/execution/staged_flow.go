package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/execution/reliable"
	workflow "github.com/erranliu/SteerLine/internal/execution/workflow"
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
	fixed, err := s.workerRecords(run)
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
	intent := workflow.FeedbackIntent{Option: answer.OptionID, Continue: answer.OptionID == "continue" || answer.OptionID == "" && fixed.execution.RecoveryState != "", Async: request.InteractionKind == "async"}
	if request.QuestionRoute != nil {
		intent.SourceTurn, intent.SourceAttempt = request.QuestionRoute.TurnID, request.QuestionRoute.AttemptID
	}
	// Convert the middle layer's unsent proof; Workflow never reads Operations.
	for i := len(w.StageInputs) - 1; i >= 0; i-- {
		previous := w.StageInputs[i]
		if previous.StepID == executionmodel.StepReport {
			if s.workerInputNotSubmitted(run, previous.CommandID) {
				intent.UnsentReportCommand = previous.CommandID
			}
			break
		}
	}
	step, retry, err := workflow.FeedbackStep(workflowSnapshot(w), intent)
	if err != nil {
		return err
	}
	if e := fixed.execution; e.PendingCommandID != "" {
		if !s.workerInputNotSubmitted(run, e.PendingCommandID) {
			return errors.New("original native input must be reconciled before feedback")
		}
		e.PendingCommandID, e.PendingTurnID = "", ""
	}
	w.StepID, w.ReportRetry = step, uint8(retry)
	selected := ""
	for _, option := range request.Options {
		if option.ID == answer.OptionID {
			selected = option.Label
			break
		}
	}
	diagnostic := w.StageDiagnostic
	questionID, sourceTurnID := "", ""
	if request.QuestionRoute != nil {
		questionID, sourceTurnID = request.QuestionRoute.QuestionID, request.QuestionRoute.TurnID
	}
	feedback, err := workflow.RenderFeedbackData(workflow.FeedbackData{Question: request.Question, Context: request.Context, Selected: selected, Note: answer.Note, Answers: answer.Answers, Diagnostic: diagnostic, QuestionID: questionID, SourceTurnID: sourceTurnID})
	if err != nil {
		return err
	}
	input := executionmodel.StageInput(workflow.FeedbackInput(workflowSnapshot(w)))
	input.FeedbackDecisionID, input.CommandID, input.AttemptID = answer.DecisionID, commandID, fixed.attempt.AttemptID
	prompt, err := workflow.RenderInput(workflowSnapshot(w), workflowInput(input))
	if err != nil {
		return err
	}
	w.InputVersion++
	input.Version = w.InputVersion
	if err := validateWorkerWorkspace(fixed.execution.Workspace); err != nil {
		return err
	}
	input.Prompt = prompt + "\nAdditional data:\n" + string(feedback) + stagePublicationInstruction(input.StepID, fixed.config)
	w.StageInputs = append(w.StageInputs, input)
	return s.startWorkerFlowTurn(ctx, run, input.Prompt, commandID)
}

func stageTaskFacts(w *executionmodel.WorkflowRunRecord) json.RawMessage {
	for _, input := range w.StageInputs {
		if len(input.TaskFacts) > 0 {
			return append(json.RawMessage(nil), input.TaskFacts...)
		}
	}
	return nil
}

func stageResult(w *executionmodel.WorkflowRunRecord, command string) *executionmodel.StageResult {
	for i := range w.StageResults {
		if w.StageResults[i].CommandID == command {
			return &w.StageResults[i]
		}
	}
	return nil
}

func (s *Service) queueStage(run *reconstructiontypes.RunView, w *executionmodel.WorkflowRunRecord, e *executionmodel.ExecutionRecord, input executionmodel.StageInput, fixed workerRecords, manualReason ...string) error {
	input.CommandID, input.AttemptID = newID(), fixed.attempt.AttemptID
	input.TaskFacts = stageTaskFacts(w)
	w.InputVersion++
	input.Version = w.InputVersion
	prompt, err := workflow.RenderInput(workflowSnapshot(w), workflowInput(input))
	if err != nil {
		return err
	}
	if err := validateWorkerWorkspace(fixed.execution.Workspace); err != nil {
		return err
	}
	if executionmodel.ReportStep(input.StepID) && unresolvedAsyncDecision(run) {
		questions, _ := json.Marshal(run.Decision.Request.Questions)
		prompt += "\nOriginal unresolved asynchronous questions: " + string(questions) + "\nAssess whether these answers are still required. If required return needs_input; do not issue duplicate questions. Return done only when the work is complete without these answers."
	}
	input.Prompt = prompt + stagePublicationInstruction(input.StepID, fixed.config)
	w.StepID, w.Status, w.UpdatedAt = input.StepID, "queued", now()
	w.StageDiagnostic = ""
	w.StageInputs = append(w.StageInputs, input)
	e.PendingCommandID, e.PendingTurnID = input.CommandID, ""
	if len(manualReason) > 0 && manualReason[0] != "" {
		w.Status, w.StageDiagnostic = "waiting_input", manualReason[0]
		if !unresolvedAsyncDecision(run) && (run.Decision == nil || run.Decision.Status != "answer_saved") {
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

func (s *Service) stageWait(run *reconstructiontypes.RunView, w *executionmodel.WorkflowRunRecord, e *executionmodel.ExecutionRecord, status, reason string, report *workflow.Report) error {
	w.Status, w.StageDiagnostic, w.UpdatedAt = status, reason, now()
	e.Error = ptr(clip(reason, 2000))
	if report == nil && status == "blocked" {
		report = &workflow.Report{Summary: reason}
	}
	if report != nil {
		e.Summary = ptr(report.Summary)
		// Keep unresolved async questions immutable; workflow status supplies their current blocking fact.
		if (status == "waiting_input" || status == "blocked") && !unresolvedAsyncDecision(run) && (run.Decision == nil || run.Decision.Status != "answer_saved") {
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
func (s *Service) acceptStageTurn(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort, fixed workerRecords, evidence executionbackend.TurnEvidence) error {
	w, e := fixed.workflow, fixed.execution
	var previous *executionbackend.TurnEvidence
	if stored := stageResult(w, evidence.CommandID); stored != nil {
		value := storedTurnEvidence(*stored)
		previous = &value
	}
	if accepted, err := reliable.AcceptedResult(previous, evidence); err != nil || accepted {
		return err
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
	normalized, err := reliable.NormalizeResult(executionbackend.SessionContext{CurrentCommandID: e.CurrentCommandID, CurrentTurnID: e.CurrentTurnID, SessionID: e.SessionID}, evidence)
	if err != nil {
		return err
	}
	if err := s.stopVerifiedWorkerTurn(run, w, e, evidence.TurnID, normalized.NativeOutcome, false); err != nil {
		return err
	}
	transition := workflow.Route(workflowSnapshot(w), workflowInput(*input), normalized, lifecycle.PolicyCapabilities().ReportToolPolicy)
	if !transition.AcceptResult {
		return s.stageWait(run, w, e, transition.Status, transition.Reason, transition.Report)
	}
	result := executionmodel.StageResult{NativeOutcome: evidence.NativeOutcome, CommandID: evidence.CommandID, AttemptID: input.AttemptID, StepID: input.StepID, InputVersion: input.Version, TurnID: evidence.TurnID, SessionID: evidence.SessionID, Succeeded: evidence.Succeeded, FinalVerified: evidence.FinalVerified, Final: evidence.LastMessage, FinalNativeSessionID: evidence.FinalNativeSessionID, FinalNativeTurnID: evidence.FinalNativeTurnID, FinalDiagnostic: evidence.FinalDiagnostic, FinalSource: evidence.FinalSource, FinalItemID: evidence.FinalItemID, Failure: evidence.Failure, Outcome: transition.Outcome}
	w.StageResults = append(w.StageResults, result)
	w.ReportRetry = uint8(transition.ReportRetry)
	if transition.PRCandidateURL != "" {
		w.PRCandidateURL = transition.PRCandidateURL
	}
	if transition.RecoverOriginal {
		s.markWorkerRecoveryRequired(run)
		if s.hasUnknownWorkerOperation(run) {
			return s.stageWait(run, w, e, "reconciling", "original failed turn has unknown external operations", nil)
		}
		return s.waitWorkerFlow(run, transition.Reason)
	}
	if transition.ExpireQuestions {
		s.expireAsyncQuestions(run, e)
		e.Summary = ptr(transition.Report.Summary)
	}
	if transition.Next != nil {
		manualReason := ""
		if s.workerContinuationConstraints(run).RequiresAuthorization() {
			manualReason = "original session or prior-service input requires explicit continuation; no automatic next turn"
		}
		return s.queueStage(run, w, e, executionmodel.StageInput(*transition.Next), fixed, manualReason)
	}
	if transition.Status != "" {
		return s.stageWait(run, w, e, transition.Status, transition.Reason, transition.Report)
	}
	if !transition.Publish {
		return errors.New("workflow result has no successor")
	}
	report := *transition.Report
	info, queryErr := publicationClient(fixed.config, s.runtime.ProcessHost).Query(ctx, report.Outputs.PRURL)
	if queryErr != nil {
		return s.stageWait(run, w, e, "reconciling", "publication result unknown: "+queryErr.Error(), &report)
	}
	return s.acceptStagePublication(run, fixed, report, info)
}

func (s *Service) acceptStagePublication(run *reconstructiontypes.RunView, fixed workerRecords, report workflow.Report, info executionmodel.PRInfo) error {
	w, e := fixed.workflow, fixed.execution
	if s.hasUnknownWorkerOperation(run) {
		return s.stageWait(run, w, e, "reconciling", "original publication has an unresolved native operation", &report)
	}
	if err := publication.ValidateWorkerPRIdentity(info, report.Outputs.PRURL, fixed.config.PR.Repository, w.Branch, e.Workspace.Branch); err != nil {
		return s.stageWait(run, w, e, "blocked", "pr_identity_mismatch: "+err.Error(), &report)
	}
	if w.PRURL != "" && w.PRURL != report.Outputs.PRURL {
		return s.stageWait(run, w, e, "blocked", "pr_identity_mismatch: publication must reuse the original PR", &report)
	}
	if err := BindWorkerPR(w, e, info, report.Outputs.PRURL, fixed.config.PR.Repository, false); err != nil {
		return s.stageWait(run, w, e, "reconciling", err.Error(), &report)
	}
	s.expireAsyncQuestions(run, e)
	w.StepID, w.Status, w.StageDiagnostic, w.UpdatedAt = executionmodel.StepAwaitMerge, "waiting_external", "", now()
	e.Summary, e.Error = ptr(report.Summary), nil
	if info.State == "closed_unmerged" {
		return s.waitWorkerFlow(run, "PR closed without merge; explicitly reopen the original PR or handle this execution.")
	}
	if info.State == "open" {
		if err := s.offerWorkerPRFeedback(run, info.URL); err != nil {
			return err
		}
	}
	s.refreshRun(run)
	return s.commitWorker(run)
}

func (s *Service) dispatchQueuedStage(ctx context.Context, run *reconstructiontypes.RunView) (bool, error) {
	fixed, err := s.workerRecords(run)
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
		if readErr == nil {
			if err := mergeLifecycleExecutionReceipt(e, receipt); err != nil {
				return err
			}
		}
		if readErr == nil && receipt.TurnEvidence != nil && receipt.TurnEvidence.Completed {
			return s.acceptStageTurn(ctx, run, lifecycle, fixed, *receipt.TurnEvidence)
		}
		w.StageDiagnostic = "original stopped turn remains unavailable: " + firstNonemptyStage(receipt.Diagnostic, fmt.Sprint(readErr))
		s.refreshRun(run)
		return s.commitWorker(run)
	}
	if executionmodel.PublicationStep(w.StepID) && w.PRCandidateURL != "" {
		if w.PRURL != "" && w.PRURL != w.PRCandidateURL {
			return s.stageWait(run, w, e, "blocked", "pr_identity_mismatch: publication must reuse the original PR", nil)
		}
		info, err := publicationClient(fixed.config, s.runtime.ProcessHost).Query(ctx, w.PRCandidateURL)
		if err != nil {
			w.PRQueryError = ptr(err.Error())
			s.refreshRun(run)
			return s.commitWorker(run)
		}
		identityErr := publication.ValidateWorkerPRIdentity(info, w.PRCandidateURL, fixed.config.PR.Repository, w.Branch, e.Workspace.Branch)
		if identityErr != nil {
			w.StageDiagnostic = "pr_identity_mismatch in read-only publication reconciliation: " + identityErr.Error()
			s.refreshRun(run)
			return s.commitWorker(run)
		}
		w.PRState, w.PRQueryError = info.State, nil
		w.StageDiagnostic = "original publication observed by read-only reconciliation; original outcome retained"
		result, input := stageResult(w, e.CurrentCommandID), stageInput(w, e.CurrentCommandID)
		if result != nil && input != nil && executionmodel.PublicationStep(result.StepID) && result.Succeeded && result.FinalVerified && result.Outcome == "done" && workflow.PublicationSourceSucceeded(workflowSnapshot(w), workflowInput(*input)) {
			report, decodeErr := workflow.DecodeReport(result.Final, input.StepID)
			if decodeErr == nil && report.Outputs.PRURL == w.PRCandidateURL {
				return s.acceptStagePublication(run, fixed, report, info)
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
