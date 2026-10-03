package steerline

import (
	"context"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
)

// A verified open PR does not require input, but the original session remains
// available if the user wants to request more work before merging it.
func (s *Service) offerWorkerPRFeedback(run *Run, url string) error {
	if run == nil || run.WorkflowVersion != WorkerFlowVersion || url == "" {
		return errors.New("optional PR feedback requires a fixed worker and PR")
	}
	if run.Decision != nil && run.Decision.Status == "pending" {
		return nil
	}
	decision, err := WriteDecisionRequest(run, DecisionRequest{
		Question: "The worker stopped. What should it do next?",
		Context:  "PR 已核实并等待合并。本轮已结束，提交反馈后将在原 worker 会话开始下一轮。",
		Options:  []executionapi.DecisionOption{{ID: "continue", Label: "Continue current work"}}, AllowFreeText: true,
	})
	if err != nil {
		return err
	}
	s.setRunDecision(run, decision)
	return nil
}

func (s *Service) pollWorkerFlowPR(ctx context.Context, run *Run, task *TaskRecord, workflow *WorkflowRunRecord) error {
	fixed, contextErr := s.workerFlowContext(run)
	if contextErr != nil {
		return contextErr
	}
	c := fixed.config
	if run == nil || task == nil || workflow == nil || workflow.WorkflowVersion != WorkerFlowVersion {
		return nil
	}
	if task.Status == "closed" {
		return nil
	}
	if task.Status == "discarded" {
		if workflow.PRURL == "" {
			return nil
		}
		_, err := (CommandService{Store: s.commandStore, Project: c.identity}).CompleteNativeTask(ctx, c, task.TaskID, task.Version, workflow.WorkflowRunID, publicationClient(c))
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
	info, err := publicationClient(c).Query(ctx, url)
	workflow.PRQueriedAt = &queried
	s.refreshRun(run)
	if err != nil {
		workflow.PRQueryError = ptr(clip(err.Error(), 1000))
		s.refreshRun(run)
		if workflow.PRURL == "" && workflow.Phase != "executing" {
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
		if workflow.Phase != "executing" {
			return s.waitWorkerFlow(run, "PR link is for a different repository, base or worker branch")
		}
		return s.commitWorker(run)
	}
	pendingQuestion := run.Decision != nil && (run.Decision.Status == "answer_saved" || run.Decision.Status == "pending" && run.Decision.Request.Question != "The worker stopped. What should it do next?")
	previousPRURL := workflow.PRURL
	if err := BindWorkerPR(workflow, execution, info, url, c.PR.Repository, pendingQuestion); err != nil {
		return s.waitWorkerFlow(run, "Several PRs or conflicting PR identities need a user decision: "+err.Error())
	}
	if run.Decision != nil && run.Decision.Status == "pending" && !pendingQuestion && previousPRURL != info.URL && workflow.Phase != "waiting_decision" {
		s.setDecisionStatus(run, "superseded")
	}
	if workflow.Phase == "waiting_pr_merge" && execution.Status != "active" {
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
	if info.State == "closed_unmerged" && execution.Status != "active" {
		return s.waitWorkerFlow(run, "PR closed without merge. Send feedback to the original worker session.")
	}
	return nil
}
