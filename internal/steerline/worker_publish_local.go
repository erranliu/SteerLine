package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
)

func (s *Service) localWorkerRun(runID string) *Run {
	for _, run := range s.runs {
		if run != nil && run.RunID == runID {
			return run
		}
	}
	return nil
}

// publishLocalWorkerRun executes Git and GitHub operations in the service's
// host identity. The worker supplies only staged content and an exact run-ID
// handoff; it never receives the host's credentials.
func (s *Service) publishLocalWorkerRun(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, turnID string) (PRInfo, error) {
	var empty PRInfo
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return empty, configErr
	}
	c := fixed.config
	if run == nil || workflow == nil || execution == nil || run.WorkspaceIntent == nil || run.WorktreePath == nil || workflow.WorkflowRunID != run.RunID || execution.WorkflowRunID != run.RunID || run.Protocol != string(executionapi.ProtocolLocalCodexAppServer) || !codexworker.ValidNativeID(turnID) || execution.CurrentTurnID != turnID || execution.LastStoppedTurnID != turnID || c.hostName() != "local" {
		return empty, errors.New("local host publication requires a fixed GitHub command provider and worker execution")
	}
	if canonical(*run.WorktreePath) != canonical(run.WorkspaceIntent.Path) || run.WorktreeBranch == nil || *run.WorktreeBranch != run.WorkspaceIntent.Branch || execution.Workspace.Branch != *run.WorktreeBranch || workflow.Branch == "" {
		return empty, errors.New("local host publication workspace or target branch differs from the fixed execution")
	}

	client := publication.NewClient(c.PR, Runner{Dir: *run.WorktreePath, Timeout: duration(c.CommandTimeout), Env: map[string]string{"GIT_TERMINAL_PROMPT": "0"}}, duration(c.CommandTimeout))
	publisher := publication.HostPublisher{Git: Runner{Dir: *run.WorktreePath, Timeout: duration(c.CommandTimeout), Env: map[string]string{"GIT_TERMINAL_PROMPT": "0"}}, GitCommand: c.Git, PR: client}
	request := publication.Request{Version: 1, RunID: run.RunID, ExecutionID: execution.ExecutionID, TurnID: turnID, TaskID: run.TaskID, Workspace: *run.WorkspaceIntent, GitCommonDir: c.identity, Remote: c.Remote, BaseBranch: workflow.Branch, Provider: publication.ProviderIdentity(c.PR)}
	request, err := publisher.Preflight(ctx, request)

	if err != nil {
		return empty, err
	}
	opID, err := s.startWorkerOperation(run, "publish_local_worker", "local-publish:"+run.RunID+":"+turnID, request)
	if err != nil {
		return empty, err
	}
	run = s.localWorkerRun(run.RunID)
	workflow, execution = s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil {
		return empty, errors.New("fixed worker execution disappeared after publication intent was saved")
	}
	saveStage := func(result publication.Result) error {
		op := snapshotOperation(&s.domain, opID)
		if op == nil || op.Status != "in_flight" {
			return errors.New("publication stage has no in-flight intent")
		}
		op.PlatformRequestID, op.Receipt, op.UnknownReason, op.UpdatedAt = result.PRURL, mustJSON(result), "", now()
		return s.commitWorker(run, opID)
	}
	result, publishErr := publisher.Publish(ctx, request, saveStage)
	if publishErr != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", result.PRURL, mustJSON(result), "local GitHub publication requires reconciliation")
		return empty, publishErr
	}
	if !result.Complete() {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", result.PRURL, mustJSON(result), "publication receipt is incomplete")
		return empty, errors.New("host publication did not confirm all stages")
	}
	info := *result.PR
	if err = BindWorkerPR(workflow, execution, info, result.PRURL, c.PR.Repository, false); err != nil {
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", result.PRURL, mustJSON(result), "provider identity could not be bound")
		return empty, err
	}
	s.refreshRun(run)
	if err = s.changeWorkerOperation(run, opID, "confirmed", result.PRURL, mustJSON(result), ""); err != nil {
		return empty, err
	}
	return info, nil
}

// A service exit can happen after the host has pushed or created the PR but
// before it has saved the receipt. Reconcile only observed, exact identities;
// never issue another create from an uncertain publication operation.
func (s *Service) reconcileLocalWorkerPublication(ctx context.Context, run *Run) (bool, error) {
	if run == nil {
		return false, nil
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return false, err
	}
	c := fixed.config
	if c.hostName() != "local" {
		return false, nil
	}
	var op *OperationRecord
	for i := range s.domain.Operations {
		candidate := &s.domain.Operations[i]
		if candidate.Kind == "publish_local_worker" && strings.HasPrefix(candidate.IdempotencyKey, "local-publish:"+run.RunID+":") && (candidate.Status == "in_flight" || candidate.Status == "outcome_unknown") {
			if op != nil {
				return false, errors.New("several local publication operations need manual reconciliation")
			}
			op = candidate
		}
	}
	if op == nil {
		return false, nil
	}
	var request publication.Request
	if err = json.Unmarshal(op.Request, &request); err != nil || request.ValidateCandidate() != nil || request.RunID != run.RunID || run.Protocol != string(executionapi.ProtocolLocalCodexAppServer) || run.WorktreePath == nil || run.WorktreeBranch == nil || request.Workspace.Path != *run.WorktreePath || request.Workspace.Branch != *run.WorktreeBranch {
		return false, errors.Join(err, errors.New("saved local publication intent is invalid"))
	}
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || execution.LastStoppedTurnID != request.TurnID || request.ExecutionID != execution.ExecutionID || request.TaskID != run.TaskID || request.BaseBranch != workflow.Branch || request.Remote != c.Remote || request.GitCommonDir != c.identity || !request.Provider.Matches(c.PR) || workflow.Phase != "waiting_decision" || run.FinishTaskStatus != "waiting_decision" || op.IdempotencyKey != "local-publish:"+run.RunID+":"+request.TurnID {
		return false, errors.New("saved local publication no longer matches the stopped worker turn")
	}
	git := Runner{Dir: *run.WorktreePath, Timeout: duration(c.CommandTimeout), Env: map[string]string{"GIT_TERMINAL_PROMPT": "0"}}
	client := publication.NewClient(c.PR, git, duration(c.CommandTimeout))
	publisher := publication.HostPublisher{Git: git, GitCommand: c.Git, PR: client}
	receipt, e := publisher.Reconcile(ctx, request)
	if e != nil {
		return false, e
	}
	info, url := *receipt.PR, receipt.PRURL
	if e = BindWorkerPR(workflow, execution, info, url, c.PR.Repository, false); e != nil {
		return false, e
	}
	s.refreshRun(run)
	if e = s.offerWorkerPRFeedback(run, url); e != nil {
		return false, e
	}
	if e = s.setOperationState(op.OperationID, "confirmed", url, mustJSON(receipt), ""); e != nil {
		return false, e
	}
	if e = s.commitWorker(run, op.OperationID); e != nil {
		return false, e
	}
	return true, nil
}
