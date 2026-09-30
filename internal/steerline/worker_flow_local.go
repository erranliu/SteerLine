package steerline

import (
	"context"
	"errors"
	"os/exec"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

type workerTurnOutcome = codexworker.Outcome

func workerPRClues(text string) []string { return codexworker.PRClues(text) }
func readWorkerTurnOutcome(path string) (workerTurnOutcome, error) {
	return codexworker.ReadExecOutcome(path)
}

func workerFlowPrompt(task TaskRecord, contract TaskContract, workflowRunID string, localPublish bool) (string, error) {
	return workerflow.Prompt(task, contract, workflowRunID, localPublish)
}

func requireWorkerFlowCapabilities(ctx context.Context, c Config) error {
	if c.hostName() == "orca" {
		if c.Profile != "" || c.Sandbox != "workspace-write" || (c.Model == "") != (c.ReasoningEffort == "") {
			return &BackendError{Code: BackendUnsupported, Diagnostic: "Orca requires workspace-write, no Codex profile, and model with reasoning_effort together"}
		}
		if err := orcaworker.CheckStatus(ctx, orcaWorkerCaller(c)); err != nil {
			return err
		}
		_, err := nativeOrcaRepo(ctx, c)
		return err
	}
	if c.agentName() != "codex" {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "resumable worker flow requires Codex"}
	}
	if c.hostName() != "local" {
		return &BackendError{Code: BackendUnsupported, Diagnostic: "this worker-flow host is not supported"}
	}
	command := c.Codex
	if err := resolveCommand(&command); err != nil {
		return err
	}
	c.Codex = command
	runner := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	err := codexworker.ProbeCapabilities(ctx, codexworker.ProbeConfig{
		Executable: command.Executable, Args: command.Args, Repository: c.Repository, RunDir: c.RunDir,
	}, codexworker.ProbePort{
		Output: func(ctx context.Context, executable string, args []string) ([]byte, error) {
			return runner.Output(ctx, Command{Executable: executable, Args: args})
		},
		HistoryAvailable: func(ctx context.Context) bool { return nativeCodexHistoryAvailable(ctx, c) },
		StartDetached:    func(cmd *exec.Cmd) error { _, err := startDetachedProcess(cmd); return err },
	})
	var unsupported *codexworker.ProbeError
	if errors.As(err, &unsupported) {
		return &BackendError{Code: BackendUnsupported, Diagnostic: unsupported.Diagnostic, Cause: unsupported.Cause}
	}
	return err
}
