// Package reconstruction rebuilds execution state from fixed records and checkpoint metadata.
// It performs no I/O and has no service, adapter or persistence dependency.
package reconstruction

import (
	"encoding/json"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func Run(task taskmodel.TaskRecord, workflow executionmodel.WorkflowRunRecord, execution executionmodel.ExecutionRecord, attempts []executionmodel.StepAttemptRecord, decisions []executionmodel.DecisionRecord, deliveries []executionmodel.TaskDeliveryRecord) (RunView, error) {
	if workflow.TaskID != task.TaskID || execution.WorkflowRunID != workflow.WorkflowRunID || workflow.SelectedResult != "" && workflow.SelectedResult != execution.ExecutionID {
		return RunView{}, errors.New("task, workflow and selected execution identity differ")
	}
	if workflow.WorkflowVersion == executionmodel.Version {
		count := 0
		for _, attempt := range attempts {
			if attempt.ExecutionID == execution.ExecutionID {
				if attempt.StepID != "worker" || execution.CurrentStep != "worker" {
					return RunView{}, errors.New("worker flow has an invalid step identity")
				}
				count++
			}
		}
		if count < 1 {
			return RunView{}, errors.New("worker flow has no worker attempt identity")
		}
	}
	run := RunView{FlowRef: workflow.FlowRef, StepID: workflow.StepID, BusinessStatus: workflow.Status, StageDiagnostic: workflow.StageDiagnostic, StageResults: executionmodel.CloneStageResults(workflow.StageResults), TaskID: task.TaskID, DomainTaskID: task.TaskID, Title: task.Title, TaskStatus: task.Status,
		CandidateTiming: "wall_since_reservation",
		ResourceStatus:  executionmodel.ExecutionResourceStatus(execution),
		SessionID:       execution.SessionID, SessionGeneration: execution.SessionGeneration, TurnNumber: execution.TurnNumber, CurrentTurnID: execution.CurrentTurnID,
		Abandonment: workflow.Abandonment, ExecutionProfile: workflow.ExecutionProfile, PRHistory: workflow.PRHistory, CandidateCreatedAt: workflow.CreatedAt,
		RecoveryRequired: execution.RecoveryRequired, RecoveryState: execution.RecoveryState, RecoveryVersion: execution.RecoveryVersion,
		RunID: workflow.WorkflowRunID, WorkflowVersion: workflow.WorkflowVersion, WorkflowConfigDigest: workflow.ConfigDigest,
		ControlRevision: workflow.ControlRevision, ExecutionID: execution.ExecutionID, Claimant: task.Claimant,
		Host: workflowExecutionHost(workflow), Backend: execution.Backend, Protocol: execution.Protocol,
		Agent: execution.Agent, Target: execution.Target, Process: execution.Process, WorkspaceIntent: execution.WorkspaceIntent,
		StartedAt: copyPtr(execution.TurnStartedAt), EndedAt: copyPtr(execution.TurnEndedAt), LogPath: copyPtr(execution.LogPath),
		LastEvent: copyPtr(execution.LastEvent), Summary: copyPtr(execution.Summary), Error: copyPtr(execution.Error), ManualStop: copyPtr(execution.ManualStop),
		FinishTaskStatus: finishStatus(task, workflow), ExecutionStatus: executionmodel.ExecutionViewStatus(execution),
		PR: executionmodel.PRState{State: nonEmpty(workflow.PRState, "unknown"), QueriedAt: copyPtr(workflow.PRQueriedAt),
			QueryError: copyPtr(workflow.PRQueryError), MergedAt: copyPtr(workflow.PRMergedAt), MergeCommit: copyPtr(workflow.PRMergeCommit)}}
	if attempt := CurrentAttempt(attempts, &execution); attempt != nil {
		if workflow.WorkflowVersion == executionmodel.Version && execution.CurrentAttemptID != "" && execution.CurrentAttemptID != attempt.AttemptID {
			return RunView{}, errors.New("worker flow current attempt pointer differs from latest worker generation")
		}
		run.AttemptID, run.DomainAttemptNumber = attempt.AttemptID, attempt.AttemptNumber
	}
	if workflow.Abandonment != nil {
		run.CandidateEndedAt = ptr(workflow.Abandonment.At)
	}
	if execution.Environment != nil && execution.Environment.Blocker != nil {
		blocker := *execution.Environment.Blocker
		run.EnvironmentBlocker = &blocker
	}

	for _, input := range workflow.StageInputs {
		if executionmodel.ReportStep(input.StepID) {
			run.ReportToolPolicy = input.ReportToolPolicy
		}
	}
	run = Execution(run, execution)
	if workflow.PRURL != "" {
		run.PR.URL = ptr(workflow.PRURL)
	}
	if execution.DecisionID != "" {
		for _, record := range decisions {
			if record.DecisionID == execution.DecisionID {
				if record.WorkflowRunID != workflow.WorkflowRunID || record.ExecutionID != execution.ExecutionID || record.AttemptID != run.AttemptID || record.Version == 0 {
					return RunView{}, errors.New("current decision differs from its execution or attempt")
				}
				var err error
				run.Decision, err = decisionView(record)
				if err != nil {
					return RunView{}, err
				}
				break
			}
		}
		if run.Decision == nil {
			return RunView{}, errors.New("execution refers to a missing decision")
		}
	}
	if task.Status == "closed" {
		for _, delivery := range deliveries {
			if delivery.TaskID == task.TaskID && delivery.SelectedWorkflowRunID == workflow.WorkflowRunID {
				run.ExecutionStatus = "succeeded"
				if !delivery.RecordedAt.IsZero() {
					run.CandidateEndedAt = ptr(delivery.RecordedAt)
				}
				run.PR = executionmodel.PRState{URL: ptr(delivery.PRURL), State: "merged", MergedAt: ptr(delivery.MergedAt), MergeCommit: ptr(delivery.MergeCommit)}
			}
		}
	}
	return run, nil
}

func workflowExecutionHost(workflow executionmodel.WorkflowRunRecord) string {
	if workflow.ExecutionProfile == nil {
		return ""
	}
	return workflow.ExecutionProfile.Host
}

func Execution(run RunView, e executionmodel.ExecutionRecord) RunView {
	run.Process, run.Target = copyPtr(e.Process), e.Target
	run.WorkspaceIntent = copyPtr(e.WorkspaceIntent)
	path, branch, base := e.Workspace.Worktree, e.Workspace.Branch, e.Workspace.Base
	if e.WorkspaceIntent != nil {
		if e.Workspace.Worktree == "" {
			branch, base = e.WorkspaceIntent.Branch, e.WorkspaceIntent.BaseCommit
		}
		if path == "" && e.Target.ConnectionID == "" && e.Target.ExternalRuntime == "" {
			path = e.WorkspaceIntent.Path
		}
		if branch == "" {
			branch = e.WorkspaceIntent.Branch
		}
		if base == "" {
			base = e.WorkspaceIntent.BaseCommit
		}
	}
	run.WorktreePath, run.WorktreeBranch, run.BaseCommit = nil, nil, nil
	if path != "" {
		run.WorktreePath = ptr(path)
	}
	if branch != "" {
		run.WorktreeBranch = ptr(branch)
	}
	if base != "" {
		run.BaseCommit = ptr(base)
	}
	return run
}

func decisionView(record executionmodel.DecisionRecord) (*executionmodel.DecisionState, error) {
	var request executionmodel.DecisionRequest
	if err := decodeStrictJSON(record.Request, &request); err != nil || request.FormatVersion != executionmodel.DecisionRequestFormatVersion {
		return nil, errors.New("persisted decision request has an unsupported format; preserve it for manual recovery")
	}
	if err := executionmodel.ValidateDecisionRequest(&request); err != nil {
		return nil, err
	}
	decision := &executionmodel.DecisionState{ID: record.DecisionID, AttemptID: record.AttemptID, Version: record.Version, Request: request, Status: record.Status}
	expectedQuestionID := record.DecisionID
	if request.QuestionRoute != nil {
		if err := request.QuestionRoute.ValidateIdentity(executionbackend.QuestionRouteIdentity{
			ProjectID: record.ProjectIdentity, WorkflowRunID: record.WorkflowRunID, ExecutionID: record.ExecutionID,
			AttemptID: record.AttemptID, Generation: request.QuestionRoute.Generation,
			TurnID: request.QuestionRoute.TurnID, QuestionID: request.QuestionRoute.QuestionID,
		}); err != nil {
			return nil, err
		}
		expectedQuestionID = request.QuestionRoute.QuestionID
	}
	if expectedQuestionID != record.QuestionID {
		return nil, errors.New("persisted question route differs from its decision binding")
	}
	if len(record.Answer) > 0 {
		var answer executionmodel.DecisionAnswer
		if err := json.Unmarshal(record.Answer, &answer); err != nil {
			return nil, err
		}
		if answer.DecisionID != record.DecisionID || answer.RunID != record.WorkflowRunID || executionmodel.ValidateDecisionAnswer(&request, &answer) != nil {
			return nil, errors.New("saved answer differs from its fixed question or identity")
		}
		decision.OptionID, decision.Note, decision.Answers, decision.AnsweredBy = answer.OptionID, answer.Note, answer.Answers, answer.AnsweredBy
		decision.SourceAttemptID, decision.RecoveryVersion = answer.SourceAttemptID, answer.ExpectedRecoveryVersion
		decision.AnswerDecisionVersion, decision.AnswerRunRevision = answer.ExpectedDecisionVersion, answer.ExpectedRunRevision
		if !answer.AnsweredAt.IsZero() {
			decision.AnsweredAt = &answer.AnsweredAt
		}
	}
	return decision, nil
}

func copyPtr[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return ptr(*value)
}

func ptr[T any](value T) *T { return &value }

func nonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
