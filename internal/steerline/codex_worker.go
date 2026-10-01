package steerline

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func appServerAskDecision(run *Run, questions []executionapi.Question, route executionapi.QuestionRoute) (*DecisionState, error) {
	return WriteDecisionRequest(run, decisionRequestFromEventFact(executionapi.EventFact{Questions: questions, QuestionRoute: &route}))
}

func appServerDirectory(c Config, run *Run) string {
	if run.DomainAttemptNumber > 1 {
		return filepath.Join(c.RunDir, "runs", run.RunID, "attempts", run.AttemptID, "app-server")
	}
	return filepath.Join(c.RunDir, "runs", run.RunID, "app-server")
}

func codexBackend(c Config, run *Run) codexworker.Backend {
	settings := codexProfileSettings(c)
	permissions, _ := codexworker.CompilePolicy(c.ExecutionPolicy, runtime.GOOS, c.hostName(), c.agentName(), settings.WindowsSandbox, settings.Profile, settings.Args)
	return codexworker.Backend{Config: codexworker.Config{Policy: c.ExecutionPolicy, Executable: settings.Executable, Args: settings.Args, Profile: settings.Profile, Sandbox: permissions.Sandbox, WindowsSandbox: settings.WindowsSandbox, Model: settings.Model, ReasoningEffort: settings.ReasoningEffort, Workspace: deref(run.WorktreePath), RunDir: c.RunDir, RunID: run.RunID, ServerDir: appServerDirectory(c, run), Environment: func(base []string) []string {
		return workerProcessEnvironment(c, run, base)
	}}, Process: codexworker.ProcessPort{Start: codexProcessStart, Inspect: inspectProcess, NewGeneration: newID, WriteAtomic: AtomicWrite}}
}
func workerProcessEnvironment(c Config, run *Run, base []string) []string {
	blocked := append(c.publicationSettings().AuthenticationRefs, "BEADS_*", "BD_*", "GH_TOKEN", "GITHUB_TOKEN", "GITEA_TOKEN", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_*", "GIT_CONFIG_VALUE_*")
	return unsetEnv(overrideEnv(base, map[string]string{"STEERLINE_WORKFLOW_RUN_ID": run.RunID, "STEERLINE_EXECUTION_ID": run.ExecutionID}), blocked)
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
