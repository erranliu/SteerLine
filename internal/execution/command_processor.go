package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/activityview"
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (s *Service) processPendingCommands(ctx context.Context) error {
	if s.commandStore == nil {
		return nil
	}
	service := s.commandService(s.Config.identity)
	// Process saved answers before new work; reject retired recovery commands.
	for _, status := range []string{"in_flight", "pending"} {
		commands, err := s.commandStore.ServiceCommands(ctx, s.Config.identity, status, 200)
		if err != nil {
			return err
		}
		for _, command := range commands {
			if err := ctx.Err(); err != nil {
				return err
			}
			if run := s.findDecision(command.TargetID); run != nil {
				if execution := s.executionRecord(run); execution != nil && execution.RecoveryRequired && execution.RecoveryMarkedAt != nil && !command.CreatedAt.After(*execution.RecoveryMarkedAt) {
					result, _ := json.Marshal(map[string]string{"code": "stale_recovery", "error": "saved input predates service recovery; submit a new decision after reviewing the worker state"})
					if err := service.SetStatus(ctx, command, "rejected", result); err != nil {
						return err
					}
					continue
				}
			}
			if command.Kind != "answer_decision" {
				if validServiceCommandKind(command.Kind) {
					return nativeErr("unsupported_schema", "saved command uses a retired recovery action; preserve its state for manual recovery")
				}
				continue
			}
			var answer executionmodel.DecisionAnswer
			if err = decode(command.Payload, &answer); err != nil {
				result, _ := json.Marshal(map[string]string{"error": "invalid decision answer payload"})
				if e := service.SetStatus(ctx, command, "rejected", result); e != nil {
					return e
				}
				continue
			}
			if run := s.findDecision(command.TargetID); run != nil {
				if execution := s.executionRecord(run); execution != nil && execution.RecoveryRequired &&
					(answer.SourceAttemptID != run.AttemptID || answer.ExpectedRecoveryVersion != execution.RecoveryVersion) {
					result, _ := json.Marshal(map[string]string{"code": "stale_recovery", "error": "worker Attempt or recovery version changed; refresh before answering"})
					if err := service.SetStatus(ctx, command, "rejected", result); err != nil {
						return err
					}
					continue
				}
			}
			if command.Status == "pending" {
				if err = service.SetStatus(ctx, command, "in_flight", nil); err != nil {
					return err
				}
				command.Status = "in_flight"
			}
			if run := s.findDecision(command.TargetID); run != nil {
				if execution := s.executionRecord(run); execution != nil && execution.RecoveryRequired &&
					(execution.RecoveryState == "unverified" || execution.RecoveryState == "" ||
						execution.RecoveryState == "interrupted" && run.Decision.Request.QuestionRoute != nil) {
					result, _ := json.Marshal(map[string]string{"code": "recovery_unverified", "error": "the original worker or request is not verified for this answer; review the recovery decision"})
					if err := service.SetStatus(ctx, command, "rejected", result); err != nil {
						return err
					}
					continue
				}
			}
			if run := s.findDecision(command.TargetID); run != nil && run.Decision != nil && run.Decision.Status == "pending" {
				if err = s.requireDecisionReplyCapability(ctx, run); err != nil {
					code := "rejected"
					var backendErr *executionbackend.BackendError
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
			if run := s.findDecision(command.TargetID); run != nil {
				if e := s.executionRecord(run); e != nil && e.Environment != nil && e.Environment.Blocker != nil {
					result := mustJSON(map[string]string{"code": "environment_blocked", "error": "resolve the environment blocker before sending feedback"})
					if err := service.SetStatus(ctx, command, "rejected", result); err != nil {
						return err
					}
					continue
				}
			}
			record, _, acceptErr := service.AcceptAnswer(ctx, command, answer)
			if acceptErr != nil {
				code := "rejected"
				if errors.Is(acceptErr, storageports.ErrAnswerConflict) {
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
				if err := s.refreshNativeSnapshot(ctx); err != nil {
					return err
				}
				run = s.findDecision(command.TargetID)
				if run == nil {
					return errors.New("accepted decision disappeared from its committed execution")
				}
				delete(s.workerDecisionWrites, record.DecisionID)
				s.note(run, "steer", "decision", "answer_saved", "decision answer saved", activityview.DecisionAnswerSummary(run), decisionAnswerMeta(run))
				if err = s.commitWorker(run); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *Service) findDecision(id string) *contracts.Run {
	for _, run := range s.runs {
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
