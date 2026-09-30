package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

func (s *Service) processPendingCommands(ctx context.Context) error {
	return s.processPendingCommandsTracked(ctx, nil)
}

// processPendingCommandsTracked reports whether a service-owned command was
// processed. The scheduler uses this to avoid publishing an unchanged runtime
// on every control poll.
func (s *Service) processPendingCommandsTracked(ctx context.Context, processed *bool) error {
	if s.domainStore == nil {
		return nil
	}
	service := CommandService{Store: s.domainStore, Project: s.Config.identity}
	// Process saved answers before new work; reject retired recovery commands.
	for _, status := range []string{"in_flight", "pending"} {
		commands, err := s.domainStore.serviceCommands(ctx, s.Config.identity, status, 200)
		if err != nil {
			return err
		}
		for _, command := range commands {
			if err := ctx.Err(); err != nil {
				return err
			}
			if command.Kind != "answer_decision" {
				if validServiceCommandKind(command.Kind) {
					return nativeErr("unsupported_schema", "saved command uses a retired recovery action; preserve its state for manual recovery")
				}
				continue
			}
			if processed != nil {
				*processed = true
			}
			var answer DecisionAnswer
			if err = decode(command.Payload, &answer); err != nil {
				result, _ := json.Marshal(map[string]string{"error": "invalid decision answer payload"})
				if e := service.SetStatus(ctx, command, "rejected", result); e != nil {
					return e
				}
				continue
			}
			if command.Status == "pending" {
				if err = service.SetStatus(ctx, command, "in_flight", nil); err != nil {
					return err
				}
				command.Status = "in_flight"
			}
			if run := s.findDecision(command.TargetID); run != nil && run.Decision != nil && run.Decision.Status == "pending" {
				if err = s.requireDecisionReplyCapability(ctx, run); err != nil {
					code := "rejected"
					var backendErr *BackendError
					if errors.As(err, &backendErr) {
						code = string(backendErr.Code)
					}
					result, _ := json.Marshal(map[string]string{"code": code, "error": err.Error()})
					if e := service.SetStatus(ctx, command, "rejected", result); e != nil {
						return e
					}
					continue
				}
			}
			record, _, acceptErr := service.AcceptAnswer(ctx, command, answer)
			if acceptErr != nil {
				code := "rejected"
				if errors.Is(acceptErr, ErrAnswerConflict) {
					code = "answer_conflict"
				}
				result, _ := json.Marshal(map[string]string{"code": code, "error": acceptErr.Error()})
				if err = service.SetStatus(ctx, command, "rejected", result); err != nil {
					return err
				}
				continue
			}
			run := s.findDecision(command.TargetID)
			if run != nil && run.Decision != nil && run.Decision.ID == command.TargetID {
				if record.WorkflowRunID != run.RunID || record.ExecutionID != run.ExecutionID || record.AttemptID != run.Decision.AttemptID {
					return fmt.Errorf("accepted decision %s no longer matches the runtime identity", command.TargetID)
				}
				*s.decisionRecord(run) = record
				s.markWorkerDecisionWrite(record.DecisionID)
				revision, revisionErr := s.domainStore.workerWorkflowRevision(ctx, s.Config.identity, run.RunID)
				if revisionErr != nil {
					return revisionErr
				}
				if workflow := s.workflowRecord(run); workflow != nil {
					workflow.ControlRevision = revision
				}
				if s.store.baseline == nil {
					s.store.baseline = domainFingerprints(s.State)
				}
				s.store.baseline["decision:"+record.DecisionID] = recordFingerprint(record)
				s.refreshRun(run)
				s.note(run, "steer", "decision", "answer_saved", "decision answer saved", decisionAnswerSummary(run), decisionAnswerMeta(run))
				if err = s.commitWorker(run); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *Service) findDecision(id string) *Run {
	for _, run := range s.State.Runs {
		if run != nil && run.Decision != nil && run.Decision.ID == id {
			return run
		}
	}
	return nil
}

func (s *Service) processCommandsAndDecisions(ctx context.Context) error {
	if err := s.processPendingCommands(ctx); err != nil {
		return err
	}
	return s.applyDecisions(ctx)
}
