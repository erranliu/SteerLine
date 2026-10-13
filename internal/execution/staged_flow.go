package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// development/v3 no longer stores artifact snapshots. Retrying a submitted
	// report must reverify implementation in the original session; an unsent
	// first report can still summarize its successful original source.
	if (answer.OptionID == "continue" || answer.OptionID == "" && fixed.execution.RecoveryState != "") && w.StepID == executionmodel.StepReport {
		valid := false
		for i := len(w.StageInputs) - 1; i >= 0; i-- {
			previous := w.StageInputs[i]
			if previous.StepID != executionmodel.StepReport {
				continue
			}
			source := stageResult(w, previous.SourceCommandID)
			valid = source != nil && source.Succeeded && source.Failure == "" && source.TurnID == previous.SourceTurnID && s.workerInputNotSubmitted(run, previous.CommandID)
			break
		}
		if !valid {
			w.StepID, w.ReportRetry = executionmodel.StepImplement, 0
		}
	}
	if request.InteractionKind == "async" {
		step, err := asyncContinuationStep(w, request)
		if err != nil {
			return err
		}
		w.StepID = step
	}
	if e := fixed.execution; e.PendingCommandID != "" {
		pending := e.PendingCommandID
		if !s.workerInputNotSubmitted(run, pending) {
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
			w.StepID, w.ReportRetry = executionmodel.StepImplement, 0
		}
	}
	selected := ""
	for _, option := range request.Options {
		if option.ID == answer.OptionID {
			selected = option.Label
			break
		}
	}
	diagnostic := w.StageDiagnostic
	if diagnostic == request.Context {
		diagnostic = ""
	}
	questionID, sourceTurnID := "", ""
	if request.QuestionRoute != nil {
		questionID, sourceTurnID = request.QuestionRoute.QuestionID, request.QuestionRoute.TurnID
	}
	feedback, err := json.Marshal(struct {
		Question     string              `json:"question"`
		Context      string              `json:"context,omitempty"`
		Selected     string              `json:"selected_answer,omitempty"`
		Note         string              `json:"user_text,omitempty"`
		Answers      map[string][]string `json:"answers,omitempty"`
		Diagnostic   string              `json:"blocker,omitempty"`
		QuestionID   string              `json:"question_id,omitempty"`
		SourceTurnID string              `json:"source_turn_id,omitempty"`
	}{request.Question, request.Context, selected, answer.Note, answer.Answers, diagnostic, questionID, sourceTurnID})
	if err != nil {
		return err
	}
	input := executionmodel.StageInput{FeedbackDecisionID: answer.DecisionID, CommandID: commandID, AttemptID: fixed.attempt.AttemptID, StepID: w.StepID, TaskFacts: stageTaskFacts(w)}
	if executionmodel.ReportStep(w.StepID) {
		for i := len(w.StageInputs) - 1; i >= 0; i-- {
			if w.StageInputs[i].StepID == w.StepID {
				previous := w.StageInputs[i]
				input.SourceCommandID, input.SourceTurnID, input.ReportToolPolicy = previous.SourceCommandID, previous.SourceTurnID, previous.ReportToolPolicy
				break
			}
		}
	}
	prompt, err := renderStageInput(w, input)
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

func renderStageInput(w *executionmodel.WorkflowRunRecord, input executionmodel.StageInput) (string, error) {
	facts := map[string]any{}
	switch input.StepID {
	case executionmodel.StepImplement:
		task := input.TaskFacts
		if len(task) == 0 {
			task = stageTaskFacts(w)
		}
		facts["task"] = task
	case executionmodel.StepReport, executionmodel.StepPublishReport:
		source := stageResult(w, input.SourceCommandID)
		original := stageInput(w, input.SourceCommandID)
		sourceStep := executionmodel.StepImplement
		if input.StepID == executionmodel.StepPublishReport {
			sourceStep = executionmodel.StepPublish
		}
		if source == nil || original == nil || original.StepID != sourceStep || source.StepID != sourceStep || source.TurnID != input.SourceTurnID {
			return "", errors.New("report lacks its original execution facts")
		}
		// A retry's preceding reply is the failed report, not its source execution.
		for _, prior := range w.StageInputs {
			if prior.StepID == input.StepID && prior.SourceCommandID == input.SourceCommandID {
				if source.FinalVerified && source.Final != "" {
					facts["original_result"] = clip(source.Final, 16*1024)
					if len(source.Final) > 16*1024 {
						facts["evidence_gap"] = "Original result is truncated."
					}
				}
				break
			}
		}
		if !source.Succeeded {
			facts["execution_failed"] = true
		}
		if source.Failure != "" {
			facts["failure"] = source.Failure
		}
		if !source.FinalVerified || source.Final == "" {
			facts["evidence_gap"] = firstNonemptyStage(source.FinalDiagnostic, "Original final reply is unavailable or unverified.")
		}

	case executionmodel.StepPublish:
	default:
		return "", errors.New("no automatic prompt for this step")
	}
	prompt, err := renderWorkerPrompt(input.StepID, facts)
	if w.PRURL != "" && (input.StepID == executionmodel.StepImplement || input.StepID == executionmodel.StepPublish) {
		prompt += "\nExisting PR: " + w.PRURL + "\nContinue on the original branch and update this same PR; do not create another PR."
	}
	return prompt, err
}

// queueStage changes the one business authority and freezes the next input
// intent inside the same worker commit as the accepted source result.
func (s *Service) queueStage(run *reconstructiontypes.RunView, w *executionmodel.WorkflowRunRecord, e *executionmodel.ExecutionRecord, input executionmodel.StageInput, fixed workerFlowContext, manualReason ...string) error {
	input.CommandID, input.AttemptID = newID(), fixed.attempt.AttemptID
	input.TaskFacts = stageTaskFacts(w)
	w.InputVersion++
	input.Version = w.InputVersion
	prompt, err := renderStageInput(w, input)
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

func (s *Service) stageWait(run *reconstructiontypes.RunView, w *executionmodel.WorkflowRunRecord, e *executionmodel.ExecutionRecord, status, reason string, report *stageReport) error {
	w.Status, w.StageDiagnostic, w.UpdatedAt = status, reason, now()
	e.Error = ptr(clip(reason, 2000))
	if report == nil && status == "blocked" {
		report = &stageReport{Summary: reason}
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
func (s *Service) acceptStageTurn(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort, fixed workerFlowContext, evidence executionbackend.TurnEvidence) error {
	w, e := fixed.workflow, fixed.execution
	if previous := stageResult(w, evidence.CommandID); previous != nil {
		if previous.TurnID != evidence.TurnID || previous.SessionID != evidence.SessionID || previous.Final != evidence.LastMessage || previous.Succeeded != evidence.Succeeded || previous.FinalSource != evidence.FinalSource || previous.FinalItemID != evidence.FinalItemID || previous.FinalVerified != evidence.FinalVerified || previous.FinalNativeSessionID != evidence.FinalNativeSessionID || previous.FinalNativeTurnID != evidence.FinalNativeTurnID || previous.Failure != evidence.Failure || previous.FinalDiagnostic != evidence.FinalDiagnostic || previous.NativeOutcome != evidence.NativeOutcome {
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
	nativeOutcome := map[bool]string{true: "succeeded", false: "failed"}[evidence.Succeeded]
	if evidence.NativeOutcome != "" {
		nativeOutcome = evidence.NativeOutcome
		if nativeOutcome != "succeeded" && nativeOutcome != "failed" && nativeOutcome != "cancelled" && nativeOutcome != "unknown" {
			return errors.New("invalid normalized native outcome")
		}
	}
	if err := s.stopVerifiedWorkerTurn(run, w, e, evidence.TurnID, nativeOutcome, false); err != nil {
		return err
	}
	failedCore := !evidence.Succeeded && (input.StepID == executionmodel.StepImplement || input.StepID == executionmodel.StepReport)
	if !failedCore && input.StepID != executionmodel.StepImplement && (!evidence.FinalVerified || evidence.FinalSource == "" || evidence.FinalItemID == "") {
		return s.stageWait(run, w, e, "reconciling", "original stopped turn lacks verified final provenance: "+evidence.FinalDiagnostic, nil)
	}
	result := executionmodel.StageResult{NativeOutcome: evidence.NativeOutcome, CommandID: evidence.CommandID, AttemptID: input.AttemptID, StepID: input.StepID, InputVersion: input.Version, TurnID: evidence.TurnID, SessionID: evidence.SessionID, Succeeded: evidence.Succeeded, FinalVerified: evidence.FinalVerified, Final: evidence.LastMessage, FinalNativeSessionID: evidence.FinalNativeSessionID, FinalNativeTurnID: evidence.FinalNativeTurnID, FinalDiagnostic: evidence.FinalDiagnostic, FinalSource: evidence.FinalSource, FinalItemID: evidence.FinalItemID, Failure: evidence.Failure}
	w.StageResults = append(w.StageResults, result)
	if failedCore {
		s.markWorkerRecoveryRequired(run)
		w.StageResults[len(w.StageResults)-1].Outcome = "blocked"
		if s.hasUnknownWorkerOperation(run) {
			return s.stageWait(run, w, e, "reconciling", "original failed turn has unknown external operations", nil)
		}
		return s.waitWorkerFlow(run, "Original turn failed or was interrupted; continue the original session to verify current work: "+evidence.Failure)
	}
	manualReason := ""
	if e.RecoveryRequired || e.OwnerServiceInstanceID != s.State.Service.InstanceID || hasBlockingPendingQuestions(e) || s.hasUnknownWorkerOperation(run) || e.Environment != nil && (!e.Environment.Ready || e.Environment.Blocker != nil) || run.Decision != nil && run.Decision.Status == "answer_saved" && run.Decision.Request.InteractionKind != "async" {
		manualReason = "original session or prior-service input requires explicit continuation; no automatic next turn"
	}
	if input.StepID == executionmodel.StepImplement {
		policy := lifecycle.PolicyCapabilities().ReportToolPolicy
		if policy == "" {
			policy = "frozen_native_permissions; business_read_only_is_prompt_policy"
		}
		return s.queueStage(run, w, e, executionmodel.StageInput{StepID: executionmodel.StepReport, SourceCommandID: input.CommandID, SourceTurnID: evidence.TurnID, ReportToolPolicy: policy}, fixed, manualReason)
	}
	report, err := decodeStageReport(evidence.LastMessage, input.StepID)
	if !evidence.FinalVerified || evidence.FinalSource == "" || evidence.FinalItemID == "" {
		err = errors.New("result lacks verified native final provenance: " + evidence.FinalDiagnostic)
	}
	if err == nil && executionmodel.PublicationStep(input.StepID) && report.Outputs.PRURL != "" {
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
		if err != nil && input.StepID == executionmodel.StepPublish {
			policy := lifecycle.PolicyCapabilities().ReportToolPolicy
			if policy == "" {
				policy = "frozen_native_permissions; business_read_only_is_prompt_policy"
			}
			return s.queueStage(run, w, e, executionmodel.StageInput{StepID: executionmodel.StepPublishReport, SourceCommandID: input.CommandID, SourceTurnID: evidence.TurnID, ReportToolPolicy: policy}, fixed, manualReason)
		}
		if input.StepID == executionmodel.StepReport && w.ReportRetry == 0 {
			w.ReportRetry = 1
			return s.queueStage(run, w, e, executionmodel.StageInput{StepID: input.StepID, SourceCommandID: input.SourceCommandID, SourceTurnID: input.SourceTurnID, ReportToolPolicy: input.ReportToolPolicy}, fixed, manualReason)
		}
		return s.stageWait(run, w, e, "blocked", reason, nil)
	}
	w.StageResults[len(w.StageResults)-1].Outcome = report.Outcome
	if executionmodel.PublicationStep(input.StepID) && report.Outputs.PRURL != "" {
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
		s.expireAsyncQuestions(run, e)
		e.Summary = ptr(report.Summary)
		return s.queueStage(run, w, e, executionmodel.StageInput{StepID: executionmodel.StepPublish}, fixed, manualReason)
	}
	if input.StepID == executionmodel.StepPublishReport && !publicationReportSourceSucceeded(w, input) {
		return s.stageWait(run, w, e, "reconciling", "source publication failed; report generated, original failure retained", &report)
	}
	info, queryErr := publicationClient(fixed.config, s.runtime.ProcessHost).Query(ctx, report.Outputs.PRURL)
	if queryErr != nil {
		return s.stageWait(run, w, e, "reconciling", "publication result unknown: "+queryErr.Error(), &report)
	}
	return s.acceptStagePublication(run, fixed, report, info)
}

// A report cannot upgrade a failed or differently bound publication. Its URL
// may still be observed through read-only reconciliation and merged delivery.
func publicationReportSourceSucceeded(w *executionmodel.WorkflowRunRecord, input *executionmodel.StageInput) bool {
	if input.StepID != executionmodel.StepPublishReport {
		return input.StepID == executionmodel.StepPublish
	}
	source, original := stageResult(w, input.SourceCommandID), stageInput(w, input.SourceCommandID)
	return source != nil && original != nil && source.StepID == executionmodel.StepPublish && original.StepID == executionmodel.StepPublish && source.TurnID == input.SourceTurnID && source.Succeeded && source.Failure == "" && source.FinalVerified && source.FinalSource != "" && source.FinalItemID != ""
}

func (s *Service) acceptStagePublication(run *reconstructiontypes.RunView, fixed workerFlowContext, report stageReport, info executionmodel.PRInfo) error {
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
		if result != nil && input != nil && executionmodel.PublicationStep(result.StepID) && result.Succeeded && result.FinalVerified && result.Outcome == "done" && publicationReportSourceSucceeded(w, input) {
			report, decodeErr := decodeStageReport(result.Final, input.StepID)
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
