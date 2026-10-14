package execution

import (
	"context"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

const workerPRFeedbackQuestion = "追加修改（可选）"

func workerContinuationPrompt(request executionmodel.DecisionRequest) bool {
	return request.QuestionRoute == nil && (request.Question == "The worker stopped. What should it do next?" || request.Question == workerPRFeedbackQuestion)
}

func (s *Service) offerWorkerPRFeedback(run *reconstructiontypes.RunView, url string) error {
	if s.candidateBusinessClosed(run) {
		return nil
	}
	if run == nil || run.WorkflowVersion != executionmodel.Version || url == "" {
		return errors.New("optional PR feedback requires a fixed worker and PR")
	}
	if run.Decision != nil && run.Decision.Status == "pending" {
		return nil
	}
	decision, err := WriteDecisionRequest(run, executionmodel.DecisionRequest{
		Question:      workerPRFeedbackQuestion,
		Options:       []executionbackend.DecisionOption{{ID: "implement", Label: "修改候选代码"}, {ID: "publish", Label: "修复发布或 PR 元数据"}},
		Context:       "执行已结束，PR 已核实并等待人工合并。当前无需继续执行；仅在需要追加修改时填写反馈，提交后在原 worker 会话开始下一轮。",
		AllowFreeText: true,
	})
	if err != nil {
		return err
	}
	s.setRunDecision(run, decision)
	return nil
}

func (s *Service) pollWorkerFlowPR(ctx context.Context, run *reconstructiontypes.RunView, task *taskmodel.TaskRecord, workflow *executionmodel.WorkflowRunRecord) error {
	if run == nil || task == nil || workflow == nil || task.Status == "closed" || !executionmodel.CandidateParticipates(workflow.Status) {
		return nil
	}
	fixed, contextErr := s.workerFlowContext(run)
	if contextErr != nil {
		return contextErr
	}
	// Only a PR already bound to this candidate can close a task during a
	// feedback turn. Unbound publication clues still require their own stage.
	awaitingMerge := workflow.StepID == executionmodel.StepAwaitMerge
	if !awaitingMerge && workflow.PRURL == "" {
		return nil
	}
	c := fixed.config
	if run == nil || task == nil || workflow == nil || workflow.WorkflowVersion != executionmodel.Version {
		return nil
	}
	if task.Status == "discarded" {
		if workflow.PRURL == "" {
			return nil
		}
		_, err := (s.commandService(c.identity)).CompleteNativeTask(ctx, c, task.TaskID, task.Version, workflow.WorkflowRunID, publicationClient(c, s.runtime.ProcessHost))
		if isNativeErrorCode(err, "task_discarded") {
			return nil
		}
		return err
	}
	url := workflow.PRURL
	if url == "" {
		url = workflow.PRCandidateURL
	}
	if url == "" {
		return nil
	}
	queried := now()
	info, err := publicationClient(c, s.runtime.ProcessHost).Query(ctx, url)
	workflow.PRQueriedAt = &queried
	s.refreshRun(run)
	if err != nil {
		workflow.PRQueryError = ptr(clip(err.Error(), 1000))
		s.refreshRun(run)
		if awaitingMerge && workflow.PRURL == "" && workflow.Status != "running" {
			return s.waitWorkerFlow(run, "PR link could not be verified; the center will retry: "+err.Error())
		}
		return s.commitWorker(run)
	}
	execution := s.executionRecord(run)
	if execution == nil {
		return nativeErr("inconsistent_delivery", "linked PR has no selected worker execution")
	}
	if err := publication.ValidateWorkerPRIdentity(info, url, c.PR.Repository, workflow.Branch, execution.Workspace.Branch); err != nil {
		workflow.PRQueryError = ptr("PR does not belong to the fixed task execution branch")
		s.refreshRun(run)
		if awaitingMerge && workflow.Status != "running" {
			return s.waitWorkerFlow(run, "PR link is for a different repository, base or worker branch")
		}
		return s.commitWorker(run)
	}
	pendingQuestion := execution.PendingCommandID != "" || execution.PendingTurnID != "" || run.Decision != nil && (run.Decision.Status == "answer_saved" || run.Decision.Status == "pending" && !workerContinuationPrompt(run.Decision.Request))
	previousPRURL := workflow.PRURL
	if awaitingMerge {
		if err := BindWorkerPR(workflow, execution, info, url, c.PR.Repository, pendingQuestion); err != nil {
			return s.waitWorkerFlow(run, "Several PRs or conflicting PR identities need a user decision: "+err.Error())
		}
	}
	if awaitingMerge && run.Decision != nil && run.Decision.Status == "pending" && !pendingQuestion && previousPRURL != info.URL && workflow.Status != "waiting_input" {
		s.setDecisionStatus(run, "superseded")
	}
	if awaitingMerge && info.State == "open" && workflow.Status == "waiting_external" && execution.Status != "active" {
		if err := s.offerWorkerPRFeedback(run, info.URL); err != nil {
			return err
		}
	}
	workflow.PRURL, workflow.PRState, workflow.PRMergedAt, workflow.PRMergeCommit, workflow.PRQueryError = info.URL, info.State, info.MergedAt, info.MergeCommit, nil
	s.refreshRun(run)
	if err := s.commitWorker(run); err != nil {
		return err
	}
	if info.State == "merged" {
		return s.nativeCompleteMergedTask(ctx, run)
	}
	if awaitingMerge && info.State == "closed_unmerged" && execution.Status != "active" {
		return s.waitWorkerFlow(run, "PR closed without merge. Send feedback to the original worker session.")
	}
	return nil
}
