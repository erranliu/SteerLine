package steerline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func waitWorkerOrcaMarker(ctx context.Context, c Config, identity ProcessIdentity, cursor int64, marker string) error {
	deadline, cancel := context.WithTimeout(ctx, positiveDuration(c.CommandTimeout, 30*time.Second))
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		batch, err := readWorkerOrcaOutput(deadline, c, identity, cursor)
		if err != nil {
			return err
		}
		for _, raw := range batch.Rows {
			if strings.TrimSpace(raw) == marker {
				return nil
			}
		}
		cursor = batch.NextCursor
		select {
		case <-deadline.Done():
			return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca did not confirm the fixed prompt chunk", Cause: deadline.Err()}
		case <-ticker.C:
		}
	}
}

func stageWorkerOrcaPrompt(ctx context.Context, c Config, identity ProcessIdentity, executionID, turnID, prompt string) error {
	for index, chunk := range orcaworker.PromptChunks(prompt) {
		command, marker, err := orcaworker.ChunkCommand(executionID, turnID, index, chunk)
		if err != nil {
			return err
		}
		cursor, err := currentWorkerOrcaCursor(ctx, c, identity)
		if err != nil {
			return err
		}
		_, sendErr := sendWorkerOrcaCommand(ctx, c, identity, command)
		// A transport error is not evidence that input was rejected. The exact
		// chunk marker is the only success proof; never send this chunk twice.
		if err := waitWorkerOrcaMarker(ctx, c, identity, cursor, marker); err != nil {
			return errors.Join(sendErr, err)
		}
	}
	return nil
}

func showWorkerOrcaTerminal(ctx context.Context, c Config, handle string) (orcaTerminal, error) {
	terminal, err := orcaworker.ShowTerminal(ctx, orcaWorkerMetaCaller(c), handle)
	return terminal, workerOrcaBackendError(err)
}

func verifyWorkerOrcaTerminal(term orcaTerminal, run *Run, expected *ProcessIdentity) error {
	if run == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || term.Handle == "" || term.RuntimeID == "" || term.IncarnationID == "" ||
		canonical(filepath.FromSlash(term.WorktreePath)) != canonical(*run.WorktreePath) ||
		strings.TrimPrefix(term.Branch, "refs/heads/") != *run.WorktreeBranch {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca terminal changed the fixed worker workspace or branch"}
	}
	if expected != nil && (expected.Kind != "orca" || expected.Terminal != term.Handle || expected.RuntimeID != term.RuntimeID || expected.Incarnation != term.IncarnationID) {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca terminal runtime, handle or incarnation changed"}
	}
	return nil
}

func createWorkerOrcaTerminal(ctx context.Context, c Config, run *Run) (orcaTerminal, error) {
	if run == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || !stableExecutionID(run.RunID) {
		return orcaTerminal{}, errors.New("worker terminal lacks a fixed run and workspace")
	}
	terminal, err := orcaworker.CreateTerminal(ctx, orcaWorkerMetaCaller(c), *run.WorktreePath, run.RunID)
	if err != nil {
		return orcaTerminal{}, workerOrcaBackendError(err)
	}
	if err := verifyWorkerOrcaTerminal(terminal, run, nil); err != nil {
		return orcaTerminal{}, err
	}
	return terminal, nil
}

func (s *Service) prepareWorkerOrcaTerminal(ctx context.Context, run *Run) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	workflow, execution := s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil || workflow.WorkflowVersion != WorkerFlowVersion || run.WorktreePath == nil || run.WorktreeBranch == nil {
		return errors.New("Orca terminal preparation lacks the fixed worker workspace")
	}
	if execution.Process != nil {
		term, err := showWorkerOrcaTerminal(ctx, fixedConfig, execution.Process.Terminal)
		if err != nil {
			return err
		}
		if err := verifyWorkerOrcaTerminal(term, run, execution.Process); err != nil {
			return err
		}
		s.refreshRun(run)
		return nil
	}
	opID, err := s.startOperation("prepare_worker_terminal", "worker-terminal:"+execution.ExecutionID, struct {
		ExecutionID string `json:"execution_id"`
		Workspace   string `json:"workspace"`
		Branch      string `json:"branch"`
	}{execution.ExecutionID, *run.WorktreePath, *run.WorktreeBranch})
	if err != nil {
		return err
	}
	_, execution = s.workflowRecords(run)
	if execution == nil {
		return errors.New("worker execution disappeared after Orca terminal intent was saved")
	}
	term, err := createWorkerOrcaTerminal(ctx, fixedConfig, run)
	if err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
		return err
	}
	identity := workerOrcaIdentity(term, fixedConfig)
	execution.Process = &identity
	s.refreshRun(run)
	if err := s.changeOperation(opID, "confirmed", "", mustJSON(identity), ""); err != nil {
		return fmt.Errorf("Orca terminal created but its identity could not be persisted: %w", err)
	}
	return nil
}

func workerOrcaIdentity(term orcaTerminal, c Config) ProcessIdentity {
	return ProcessIdentity{Kind: "orca", Terminal: term.Handle, RuntimeID: term.RuntimeID, Incarnation: term.IncarnationID, Executable: c.Orca.Executable}
}

type workerOrcaOutput = orcaworker.Output

func workerOrcaTerminalIdentity(identity ProcessIdentity) orcaworker.TerminalIdentity {
	return orcaworker.TerminalIdentity{Kind: identity.Kind, Handle: identity.Terminal, RuntimeID: identity.RuntimeID, Incarnation: identity.Incarnation}
}

func readWorkerOrcaOutput(ctx context.Context, c Config, identity ProcessIdentity, cursor int64) (workerOrcaOutput, error) {
	output, err := orcaworker.ReadOutput(ctx, orcaWorkerMetaCaller(c), workerOrcaTerminalIdentity(identity), cursor)
	return output, workerOrcaBackendError(err)
}

func sendWorkerOrcaCommand(ctx context.Context, c Config, identity ProcessIdentity, command string) (string, error) {
	requestID, err := orcaworker.SendCommand(ctx, orcaWorkerMetaCaller(c), workerOrcaTerminalIdentity(identity), command)
	return requestID, workerOrcaBackendError(err)
}

type workerOrcaProcess struct {
	config   Config
	identity ProcessIdentity
}

func (p *workerOrcaProcess) Identity() ProcessIdentity { return p.identity }
func (p *workerOrcaProcess) Close()                    {} // Only the center observer stops.
func (p *workerOrcaProcess) Kill() {
	if p.config.hostName() != "orca" {
		return // Remote cancellation is not a certified capability.
	}
	ctx, cancel := context.WithTimeout(context.Background(), positiveDuration(p.config.CommandTimeout, 30*time.Second))
	defer cancel()
	_, _ = orcaworker.CloseTerminal(ctx, orcaWorkerMetaCaller(p.config), p.identity.Terminal, p.identity.RuntimeID)
}

func workerOrcaRemoteRepoPath(ctx context.Context, c Config, worktreeID string) (string, error) {
	path, err := orcaworker.RemoteRepoPath(ctx, orcaWorkerCaller(c), worktreeID)
	return path, workerOrcaBackendError(err)
}

func workerOrcaWriteDirs(ctx context.Context, c Config, run *Run) ([]string, error) {
	if c.Sandbox != "workspace-write" {
		return nil, nil
	}
	if c.hostName() == "orca_supervised" {
		path, err := workerOrcaRemoteRepoPath(ctx, c, run.Target.WorktreeID)
		if err != nil {
			return nil, err
		}
		return []string{path}, nil
	}
	args, err := codexLinkedWorktreeWriteArgs(ctx, c, *run.WorktreePath)
	if err != nil {
		return nil, err
	}
	dirs := []string{}
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) || args[i] != "--add-dir" {
			return nil, errors.New("local Codex worktree write arguments are malformed")
		}
		dirs = append(dirs, args[i+1])
	}
	return dirs, nil
}

func startWorkerOrcaTurn(ctx context.Context, c Config, run *Run, execution *ExecutionRecord, turnID, prompt string) (*Worker, string, string, error) {
	if run == nil || execution == nil || run.Process == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || run.RunID != execution.WorkflowRunID || run.ExecutionID != execution.ExecutionID || !stableExecutionID(turnID) {
		return nil, "", "", errors.New("Orca turn has no fixed worker, workspace or process")
	}
	term, err := showWorkerOrcaTerminal(ctx, c, run.Process.Terminal)
	if err != nil {
		return nil, "", "", err
	}
	if err := verifyWorkerOrcaTerminal(term, run, run.Process); err != nil {
		return nil, "", "", err
	}
	cursor, err := currentWorkerOrcaCursor(ctx, c, *run.Process)
	if err != nil {
		return nil, "", "", err
	}
	events, sourceIdentity, err := prepareOrcaTurnJournal(c, run, turnID, cursor)
	if err != nil {
		return nil, "", "", err
	}
	if err := AtomicWrite(filepath.Join(filepath.Dir(events), "prompt.txt"), []byte(prompt)); err != nil {
		return nil, "", "", err
	}
	if err := stageWorkerOrcaPrompt(ctx, c, *run.Process, execution.ExecutionID, turnID, prompt); err != nil {
		return nil, events, "", err
	}
	dirs, err := workerOrcaWriteDirs(ctx, c, run)
	if err != nil {
		return nil, "", "", err
	}
	command, err := orcaworker.TurnCommand(orcaworker.TurnOptions{Executable: c.Codex.Executable, Args: c.Codex.Args, Sandbox: c.Sandbox, Model: c.Model, ReasoningEffort: c.ReasoningEffort, Profile: c.Profile}, execution.ExecutionID, *run.WorktreePath, execution.SessionID, turnID, prompt, dirs)
	if err != nil {
		return nil, "", "", err
	}
	requestID, err := sendWorkerOrcaCommand(ctx, c, *run.Process, command)
	if err != nil {
		return nil, events, requestID, err
	}
	return observeWorkerOrcaTurn(c, run, turnID, *run.Process, events, sourceIdentity), events, requestID, nil
}

func resumeWorkerOrcaTurn(c Config, run *Run, turnID string) (*Worker, error) {
	if run == nil || run.Process == nil || !stableExecutionID(turnID) {
		return nil, errors.New("saved Orca worker turn has no terminal or turn identity")
	}
	events := filepath.Join(orcaTurnPath(c, run.RunID, turnID), "events.jsonl")
	f, sourceIdentity, err := openJournal(events)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return observeWorkerOrcaTurn(c, run, turnID, *run.Process, events, sourceIdentity), nil
}
