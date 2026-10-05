package execution

import (
	"context"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func (s *Service) nativeCompleteMergedTask(ctx context.Context, run *reconstructiontypes.RunView) error {
	if run == nil || run.PR.URL == nil {
		return nativeErr("merge_state_unknown", "native task has no fixed PR URL")
	}
	task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
	workflow, _ := s.workflowRecords(run)
	if task == nil || workflow == nil {
		return nativeErr("inconsistent_delivery", "native delivery task or workflow record is missing")
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	receipt, err := (s.commandService(fixed.config.identity)).CompleteNativeTask(ctx, fixed.config, task.TaskID, task.Version, workflow.WorkflowRunID, publicationClient(fixed.config))
	if err != nil {
		return err
	}
	if receipt.Status != "closed" || receipt.Delivery.TaskID != run.DomainTaskID {
		return nativeErr("inconsistent_delivery", "native delivery transaction returned an invalid task receipt")
	}
	if err = s.refreshNativeSnapshot(ctx); err != nil {
		return err
	}
	run = s.find(receipt.WorkflowRunID)
	if run == nil {
		return nativeErr("inconsistent_delivery", "closed native task disappeared from the execution projection")
	}
	s.executionRecord(run).Error = nil
	s.refreshRun(run)
	s.note(run, "steer", "delivery", "completed", "native task delivered after verified PR merge", receipt.Delivery.PRURL, map[string]any{"merge_commit": receipt.Delivery.MergeCommit, "graph_revision": receipt.GraphRevision})
	if err := s.commitWorker(run); err != nil {
		return err
	}
	var cleanupErr error
	if workflow.WorkflowVersion == executionmodel.Version {
		lifecycle, _, lifecycleErr := s.lifecycleForRun(run)
		if lifecycleErr != nil {
			cleanupErr = lifecycleErr
		} else {
			cleanupErr = s.cleanupLifecycleResources(ctx, run, lifecycle)
		}
	}
	if cleanupErr != nil {
		s.executionRecord(run).LastEvent = ptr("Task delivery completed; owned execution cleanup needs reconciliation: " + cleanupErr.Error())
		s.refreshRun(run)
		return s.commitWorker(run)
	}
	return nil
}

func (s *Service) pollNativePRs(ctx context.Context, onlyRunID string) error {
	var operationErr error
	for _, run := range s.runs {
		if run == nil || onlyRunID != "" && run.RunID != onlyRunID {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
		if task == nil {
			continue
		}
		workflow, _ := s.workflowRecords(run)
		if workflow == nil {
			continue
		}
		if workflow.WorkflowVersion != executionmodel.Version {
			return nativeErr("unsupported_schema", "PR polling found an unsupported workflow")
		}
		if err := s.pollWorkerFlowPR(ctx, run, task, workflow); err != nil {
			operationErr = errors.Join(operationErr, err)
		}
	}
	return operationErr
}
