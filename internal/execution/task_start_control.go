package execution

import (
	"context"
	"errors"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

type taskStartCheckResult struct {
	request  productapi.ControlRequest
	digest   string
	prepared preparedTaskClaim
	err      error
}

// Only external preflight runs in the background. Task claims and receipts
// remain owned by the service actor, independently of other starting tasks.
func (s *Service) processTaskStartControl(ctx context.Context, req productapi.ControlRequest, payload productapi.ControlPayload, digest string) error {
	if s.State.Control.DispatchMode != "manual" {
		return s.rejectControl(req, digest, "wrong_mode", errors.New("start_task requires manual dispatch mode"))
	}
	if s.State.Control.DispatchPaused {
		return s.rejectControl(req, digest, "dispatch_paused", errors.New("dispatch is paused"))
	}
	if s.find(payload.TaskID) != nil {
		return s.rejectControl(req, digest, "already_managed", errAlreadyManaged)
	}
	return s.reserveControlStart(ctx, req, payload, false)
}

func (s *Service) applyTaskStartCheck(result taskStartCheckResult) error {
	req := result.request
	pending, found := s.startChecks[req.RequestID]
	if !found || RequestDigest(pending) != RequestDigest(req) {
		return errors.New("candidate preparation differs from its original request")
	}
	delete(s.startChecks, req.RequestID)
	run := s.find(req.RequestID)
	if run == nil {
		return errors.New("reserved candidate is missing; preserve the original start request")
	}
	w := s.workflowRecord(run)
	if w == nil {
		return nil
	}
	if s.candidateBusinessClosed(run) {
		facts := map[string]any{"request_id": req.RequestID}
		if result.err != nil {
			facts["error"] = clip(result.err.Error(), 2000)
		}
		if result.prepared.Commit != nil {
			facts["base_commit"] = result.prepared.Commit.Execution.Workspace.Base
		}
		s.note(run, "steer", "worker", "preparation_result", "original preparation result retained without activation", req.RequestID, facts)
		return s.commitWorker(run)
	}
	if !w.StartPending {
		return nil
	}
	ctx := s.runContext
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return nil
	}
	err := result.err
	if s.host.ControlRequestDigest(req.RequestID) != result.digest {
		err = errors.New("original start request changed during preparation")
	}
	if err == nil {
		if result.prepared.Commit == nil {
			err = errors.New("candidate preparation returned no declarations")
		} else {
			err = s.commandService(s.Config.identity).finishReservedStart(ctx, *result.prepared.Commit)
		}
	}
	if err != nil {
		if saveErr := s.failReservedStart(run, err.Error()); saveErr != nil {
			return saveErr
		}
		failed := s.baseReceipt(req, result.digest, "failed", controlFailureCode(err), err.Error())
		failed.RunID, failed.AttemptID, failed.CompletedAt = run.RunID, run.AttemptID, ptr(now())
		return s.writeReceipt(failed)
	}
	return s.refreshNativeSnapshot(ctx)
}
