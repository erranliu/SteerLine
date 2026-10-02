package steerline

import (
	"context"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func appServerAskDecision(run *Run, questions []executionapi.Question, route executionapi.QuestionRoute) (*DecisionState, error) {
	return WriteDecisionRequest(run, decisionRequestFromEventFact(executionapi.EventFact{Questions: questions, QuestionRoute: &route}))
}

func workerProcessEnvironment(c Config, run *Run, base []string) []string {
	blocked := append(c.publicationSettings().AuthenticationRefs, "BEADS_*", "BD_*", "GH_TOKEN", "GITHUB_TOKEN", "GITEA_TOKEN", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_*", "GIT_CONFIG_VALUE_*")
	return unsetEnv(overrideEnv(base, map[string]string{"STEERLINE_WORKFLOW_RUN_ID": run.RunID, "STEERLINE_EXECUTION_ID": run.ExecutionID}), blocked)
}

// WorkerProcessEnvironment applies the center's blocked-secret policy to a
// child process environment without exposing service or store state.
func WorkerProcessEnvironment(c Config, run *Run, inherited []string) []string {
	return workerProcessEnvironment(c, run, inherited)
}
func (s *Service) handleAppServerLaunchMessage(runID string, message *appServerLaunchMessage) (retErr error) {
	if message == nil || message.request.Operation.ID == "" {
		return errors.New("execution lifecycle message lacks its durable operation")
	}
	if message.phase == "turn_result" && s.appServerLaunching[runID] == message.request.Input.Turn.CommandID {
		delete(s.appServerLaunching, runID)
	}
	var run *Run
	for _, candidate := range s.runs {
		if candidate != nil && candidate.RunID == runID {
			run = candidate
			break
		}
	}
	if run == nil || run.TaskStatus == "closed" || run.TaskStatus == "discarded" || run.RunID != runID ||
		run.ExecutionID != message.request.Snapshot.Session.ExecutionID || run.AttemptID != message.request.Snapshot.Session.AttemptID ||
		run.AttemptID != message.request.Operation.AttemptID {
		if message.observer != nil {
			_ = message.observer.Close(context.Background())
		}
		if message.ack != nil {
			message.ack <- appServerLaunchAck{}
		}
		return ErrWorkerTurnStale
	}
	if message.request.Snapshot.Session.AttemptID != run.AttemptID {
		return ErrWorkerTurnStale
	}
	if message.phase == "session_prepared" {
		startRequest, proceed, err := s.applyAppServerSessionReceipt(run, message.request, message.receipt, message.err)
		if message.ack != nil {
			message.ack <- appServerLaunchAck{Proceed: proceed && err == nil, Request: startRequest}
		}
		return err
	}
	if message.phase != "turn_result" {
		if message.observer != nil {
			_ = message.observer.Close(context.Background())
		}
		return errors.New("unknown execution lifecycle result phase")
	}
	return s.applyAppServerTurnReceipt(run, message.request, message.receipt, message.observer, message.err)
}
