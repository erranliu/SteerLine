package execution

import (
	"context"
	"errors"
	"fmt"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func (s *Service) dispatchNative(ctx context.Context) error {
	if !s.explicitDispatch && s.hasActiveWorkers() {
		return nil
	}
	if err := s.refreshNativeSnapshot(ctx); err != nil {
		return err
	}
	if !s.explicitDispatch && (s.State.Control.DispatchMode != "auto" || s.State.Control.DispatchPaused) {
		return nil
	}
	if !s.explicitDispatch && s.State.SlotOccupied && s.activeControlRequestID() == "" {
		return nil
	}
	if !s.explicitDispatch {
		if reason := s.slotReason(s.activeControlRequestID()); reason != "" {
			return errors.New(reason)
		}
	}
	if !s.explicitDispatch {
		if err := s.selectDefaultDispatchProfile(); err != nil {
			return err
		}
	} else if s.dispatchProfile == nil {
		if err := s.selectDispatchProfile(s.runtime.Profiles.DefaultProfileID(), ""); err != nil {
			return err
		}
	}
	if s.dispatchConfig == nil {
		return errors.New("native dispatch has no selected execution profile")
	}
	c := *s.dispatchConfig
	defer func() {
		s.dispatchConfig = nil
		s.dispatchProfile = nil
	}()
	ready, err := s.commandService(c.identity).Work.Ready(ctx, c.identity)
	if err != nil {
		return err
	}
	if len(ready) > 0 {
		if c.WorkflowProtocol == executionmodel.Version {
			if err = requireWorkerFlowCapabilitiesWithResolver(ctx, c, s.runtime.Directory, s.runtime.ProcessHost, s.lifecycleResolver); err != nil {
				return err
			}
		} else {
			return errors.New("unsupported workflow format; preserve original state")
		}
	}
	for _, task := range ready {
		if !s.taskWork.CanExecuteType(task.Type) {
			continue
		}
		if s.targetTask != "" && s.targetTask != task.TaskID {
			continue
		}
		authority, authErr := s.domainStore.NativeAuthority(ctx, c.identity)
		if authErr != nil {
			return authErr
		}
		requestID := newID()
		if s.explicitDispatch && s.activeControlRequestID() != "" {
			requestID = s.activeControlRequestID()
		}
		workflowRunID := ""
		if s.reservedRunID != "" {
			workflowRunID = s.reservedRunID
		}
		receipt, claimErr := (s.commandService(c.identity)).ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: task.TaskID, RequestID: requestID, ExpectedTaskVersion: task.Version, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: s.taskWork.DefaultRequestedBy(), Explicit: s.explicitDispatch, WorkflowRunID: workflowRunID, ExecutionProfile: s.dispatchProfile, ServiceInstanceID: s.State.Service.InstanceID})
		if claimErr != nil {
			if isNativeErrorCode(claimErr, "task_not_ready", "stale_version", "slot_busy", "dispatch_paused", "wrong_mode", "task_discarded") {
				if s.targetTask != "" {
					return claimErr
				}
				continue
			}
			return claimErr
		}
		if receipt.Status != "claim_reserved" || receipt.StartTaskApplied {
			return nativeErr("inconsistent_delivery", "native claim returned an invalid reservation status")
		}
		if err = s.refreshNativeSnapshot(ctx); err != nil {
			return err
		}
		run := s.find(receipt.WorkflowRunID)
		workflow, execution := s.workflowRecords(run)
		attempt := s.workflowAttempt(run, run.AttemptID)
		if run == nil || workflow == nil || execution == nil || attempt == nil {
			return nativeErr("inconsistent_delivery", "native claim reservation could not be reconstructed from center records")
		}
		intent := execution.WorkspaceIntent
		if intent == nil {
			return nativeErr("inconsistent_delivery", "claimed execution lacks its workspace intent")
		}
		if execution.Target.WorktreeID != "" || execution.Target.Branch != c.Branch {
			return nativeErr("authority_mismatch", "claimed execution already has a conflicting workspace identity")
		}
		// A workspace intent is a plan. Its resource identity is attached only
		// after the preparation receipt commits, through the shared write set.
		execution.UpdatedAt = now()
		execution.OwnerServiceInstanceID = s.State.Service.InstanceID
		workflow.UpdatedAt = now()
		attempt.ExternalBinding = execution.Target
		attempt.UpdatedAt = now()
		execution.WorkspaceIntent = intent
		s.refreshRun(run)
		s.executionRecord(run).Status = "pending"
		s.refreshRun(run)
		s.State.SlotOccupied = true
		s.State.CurrentTask = ptr(task.TaskID)
		if err = s.commitWorker(run); err != nil {
			return err
		}
		s.lastDispatched = task.TaskID
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return s.prepareWorkerFlow(ctx, task, run)
	}
	if s.targetTask != "" {
		return fmt.Errorf("%w: %s is not in the current ready native task set", errTaskNotReady, s.targetTask)
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
