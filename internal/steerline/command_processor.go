package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

func (s *Service) processPendingCommands(ctx context.Context) error {
	if s.domainStore == nil {
		return nil
	}
	service := CommandService{Store: s.domainStore, Project: s.Config.identity}
	for _, status := range []string{"pending", "in_flight"} {
		commands, err := s.domainStore.Commands(ctx, s.Config.identity, status, 200)
		if err != nil {
			return err
		}
		for _, command := range commands {
			if err := ctx.Err(); err != nil {
				return err
			}
			if command.Kind != "answer_decision" {
				payload, _ := json.Marshal(map[string]string{"error": "unsupported command kind"})
				if err = service.SetStatus(ctx, command, "rejected", payload); err != nil {
					return err
				}
				continue
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
			run := s.findDecision(command.TargetID)
			if run == nil || run.Decision == nil || run.FinishTaskStatus != "waiting_decision" || run.RunID != answer.RunID || run.Decision.ID != command.TargetID {
				result, _ := json.Marshal(map[string]string{"error": "decision target no longer exists"})
				if err = service.SetStatus(ctx, command, "rejected", result); err != nil {
					return err
				}
				continue
			}
			switch {
			case run.Decision.Status == "pending" && run.Decision.Version == command.ExpectedVersion:
				answer.DecisionID = run.Decision.ID
				answer.RunID = run.RunID
				answer.AnsweredBy = clip(answer.AnsweredBy, 200)
				answer.Note = clip(answer.Note, 4000)
				if answer.AnsweredAt.IsZero() {
					answer.AnsweredAt = now()
				}
				if err = validateDecisionAnswer(&run.Decision.Request, &answer); err != nil {
					result, _ := json.Marshal(map[string]string{"error": err.Error()})
					if e := service.SetStatus(ctx, command, "rejected", result); e != nil {
						return e
					}
					continue
				}
				run.Decision.Status = "answer_saved"
				run.Decision.Version++
				run.Decision.OptionID = answer.OptionID
				run.Decision.Note = answer.Note
				run.Decision.AnsweredBy = answer.AnsweredBy
				run.Decision.AnsweredAt = &answer.AnsweredAt
				s.note(run, "steer", "decision", "answer_saved", "decision answer saved", decisionAnswerSummary(run), decisionAnswerMeta(run))
				if err = s.save(); err != nil {
					return err
				}
			case run.Decision.Status == "answer_saved" && run.Decision.Version == command.ExpectedVersion+1:
				saved, e := ReadDecisionAnswer(run)
				if e != nil {
					return fmt.Errorf("recover decision answer command: %w", e)
				}
				if saved.OptionID != answer.OptionID || saved.Note != answer.Note || saved.AnsweredBy != answer.AnsweredBy {
					return errors.New("persisted decision answer does not match its command")
				}
			default:
				result, _ := json.Marshal(map[string]string{"error": "decision version is stale or already applied"})
				if err = service.SetStatus(ctx, command, "rejected", result); err != nil {
					return err
				}
				continue
			}
			result, _ := json.Marshal(map[string]any{"decision_id": command.TargetID, "version": run.Decision.Version, "status": run.Decision.Status})
			if err = service.SetStatus(ctx, command, "accepted", result); err != nil {
				return err
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
