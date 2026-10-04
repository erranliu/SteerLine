package steerline

import (
	"context"
	"errors"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/projection"
)

func (s *Service) refreshNativeSnapshot(ctx context.Context) error {
	if s.domainStore == nil {
		return errors.New("native service store is required")
	}
	snapshot, err := s.domainStore.Query(ctx, s.Config.identity)
	if err != nil {
		return err
	}
	for _, task := range snapshot.Tasks {
		if task.Source != "native" || task.ExternalProvider != "" || task.ExternalProject != "" || task.ExternalID != "" || task.IdentityMapDigest != "" {
			return nativeErr("authority_mismatch", "non-native task identity")
		}
		if task.Status == "in_progress" && latestNativeWorkflow(snapshot.WorkflowRuns, task.TaskID) == nil {
			return nativeErr("inconsistent_delivery", "claimed task has no workflow")
		}
	}
	for _, workflow := range snapshot.WorkflowRuns {
		if workflow.WorkflowVersion != executionmodel.Version {
			return nativeErr("unsupported_schema", "unsupported workflow version")
		}
	}
	authority, err := s.domainStore.NativeAuthority(ctx, s.Config.identity)
	if err != nil {
		return err
	}
	s.domain = snapshot
	s.workerDecisionWrites = nil
	s.pendingOperations = nil
	var copyErr error
	s.committed, copyErr = copyDomainSnapshot(snapshot)
	if copyErr != nil {
		return copyErr
	}
	s.State.Control.DispatchMode, s.State.Control.DispatchPaused, s.State.Control.Revision = authority.DispatchMode, authority.DispatchPaused, authority.AuthorityVersion
	s.State.SchemaVersion = WorkerFlowRuntimeSchema
	return s.projectState()
}

func latestNativeWorkflow(records []executionmodel.WorkflowRunRecord, taskID string) *executionmodel.WorkflowRunRecord {
	return projection.LatestWorkflow(records, taskID)
}
