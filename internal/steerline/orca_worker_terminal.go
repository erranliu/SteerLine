package steerline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func stageWorkerOrcaPrompt(ctx context.Context, c Config, identity executionapi.ProcessIdentity, executionID, turnID, prompt string) error {
	return orcaworker.StagePrompt(ctx, orcaWorkerMetaCaller(c), workerOrcaTerminalIdentity(identity), positiveDuration(c.CommandTimeout, 30*time.Second), executionID, turnID, prompt, workerOrcaBackendError)
}

func showWorkerOrcaTerminal(ctx context.Context, c Config, handle string) (orcaTerminal, error) {
	terminal, err := orcaworker.ShowTerminal(ctx, orcaWorkerMetaCaller(c), handle)
	return terminal, workerOrcaBackendError(err)
}

func verifyWorkerOrcaTerminal(term orcaTerminal, run *Run, expected *executionapi.ProcessIdentity) error {
	if run == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || term.Handle == "" || term.RuntimeID == "" || term.IncarnationID == "" ||
		canonical(filepath.FromSlash(term.WorktreePath)) != canonical(*run.WorktreePath) ||
		strings.TrimPrefix(term.Branch, "refs/heads/") != *run.WorktreeBranch {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca terminal changed the fixed worker workspace or branch"}
	}
	if expected != nil && (expected.Kind != "orca" || expected.Terminal != term.Handle || expected.RuntimeID != term.RuntimeID || expected.Incarnation != term.IncarnationID) {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca terminal runtime, handle or incarnation changed"}
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
	opID, err := s.startWorkerOperation(run, "prepare_worker_terminal", "worker-terminal:"+execution.ExecutionID, struct {
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
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
		return err
	}
	identity := workerOrcaIdentity(term, fixedConfig)
	execution.Process = &identity
	s.refreshRun(run)
	if err := s.changeWorkerOperation(run, opID, "confirmed", "", mustJSON(identity), ""); err != nil {
		return fmt.Errorf("Orca terminal created but its identity could not be persisted: %w", err)
	}
	return nil
}

func workerOrcaIdentity(term orcaTerminal, c Config) executionapi.ProcessIdentity {
	return executionapi.ProcessIdentity{Kind: "orca", Terminal: term.Handle, RuntimeID: term.RuntimeID, Incarnation: term.IncarnationID, Executable: c.Orca.Executable}
}

type workerOrcaOutput = orcaworker.Output

func workerOrcaTerminalIdentity(identity executionapi.ProcessIdentity) orcaworker.TerminalIdentity {
	return orcaworker.TerminalIdentity{Kind: identity.Kind, Handle: identity.Terminal, RuntimeID: identity.RuntimeID, Incarnation: identity.Incarnation}
}

func readWorkerOrcaOutput(ctx context.Context, c Config, identity executionapi.ProcessIdentity, cursor int64) (workerOrcaOutput, error) {
	output, err := orcaworker.ReadOutput(ctx, orcaWorkerMetaCaller(c), workerOrcaTerminalIdentity(identity), cursor)
	return output, workerOrcaBackendError(err)
}

func sendWorkerOrcaCommand(ctx context.Context, c Config, identity executionapi.ProcessIdentity, command string) (string, error) {
	requestID, err := orcaworker.SendCommand(ctx, orcaWorkerMetaCaller(c), workerOrcaTerminalIdentity(identity), command)
	return requestID, workerOrcaBackendError(err)
}

type workerOrcaProcess struct {
	config   Config
	identity executionapi.ProcessIdentity
}

func (p *workerOrcaProcess) Identity() executionapi.ProcessIdentity { return p.identity }
func (p *workerOrcaProcess) Close()                                 {} // Only the center observer stops.
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

func currentWorkerOrcaCursor(ctx context.Context, c Config, identity executionapi.ProcessIdentity) (int64, error) {
	cursor, err := orcaworker.CurrentCursor(ctx, orcaWorkerMetaCaller(c), identity.Terminal, identity.RuntimeID)
	return cursor, workerOrcaBackendError(err)
}

func orcaTurnPath(c Config, runID, turnID string) string {
	return filepath.Join(c.RunDir, "runs", runID, "turns", turnID)
}

func prepareOrcaTurnJournal(c Config, run *Run, turnID string, cursor int64) (string, string, error) {
	if run == nil || !stableExecutionID(turnID) || !stableExecutionID(run.ExecutionID) || !stableExecutionID(run.AttemptID) {
		return "", "", errors.New("Orca worker journal lacks fixed identities")
	}
	dir := orcaTurnPath(c, run.RunID, turnID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", err
	}
	events := filepath.Join(dir, "events.jsonl")
	file, err := os.OpenFile(events, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", "", err
	}
	if err := file.Close(); err != nil {
		return "", "", err
	}
	f, identity, err := openJournal(events)
	if err != nil {
		return "", "", err
	}
	_ = f.Close()
	segment := "orca-jsonl-v1:" + identity
	marker := ConversationSourceMarker{Version: 1, ProjectID: c.identity, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, Backend: executionapi.BackendOrca, Protocol: executionapi.ExecutionProtocol(run.Protocol), Agent: "codex", TargetID: run.Target.ID, Source: "codex-jsonl", SourceSegment: segment, SourceIdentity: identity, RecordName: "events.jsonl", CreatedAt: now(), ReaderCommand: codexReaderCommand(c)}
	if err := marker.validate(); err != nil {
		return "", "", err
	}
	if err := AtomicWrite(filepath.Join(dir, ConversationSourceMarkerName), append(mustJSON(marker), '\n')); err != nil {
		return "", "", err
	}
	if err := AtomicWrite(filepath.Join(dir, "orca-cursor.json"), append(mustJSON(orcaworker.TurnCursor{Version: 1, Cursor: cursor}), '\n')); err != nil {
		return "", "", err
	}
	return events, identity, nil
}

func observeWorkerOrcaTurn(c Config, run *Run, turnID string, identity executionapi.ProcessIdentity, events, sourceIdentity string) *Worker {
	ctx, cancel := context.WithCancel(context.Background())
	worker := &Worker{proc: &workerOrcaProcess{config: c, identity: identity}, Events: make(chan WorkerEvent, 64), Done: make(chan error, 1), cancelObservation: cancel, observationDone: make(chan struct{})}
	waited := make(chan error, 1)
	go orcaworker.PullTurn(ctx, orcaWorkerMetaCaller(c), workerOrcaTerminalIdentity(identity), turnID, events, waited, AtomicWrite, workerOrcaBackendError)
	go observeDurableWorkerSegment(ctx, worker, events, sourceIdentity, "orca-jsonl-v1:"+sourceIdentity, run.RunID, run.AttemptID, waited)
	return worker
}

func (s *Service) closeNativeOrcaTerminal(ctx context.Context, run *Run, execution *ExecutionRecord, identity executionapi.ProcessIdentity, role string) error {
	if execution == nil {
		return errors.New("Orca terminal has no fixed execution")
	}
	config, configErr := s.executionConfigForWorkflowID(execution.WorkflowRunID)
	if configErr != nil {
		return configErr
	}
	fixedConfig := copyWorkerFlowConfig(config)
	if identity.Terminal == "" || identity.Incarnation == "" || identity.RuntimeID == "" || identity.RuntimeID != execution.Process.RuntimeID {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca terminal cleanup lacks its original identity"}
	}
	key := "orca-native-close:" + execution.ExecutionID + ":" + role
	if op := s.operationByKey(key); op != nil && op.Status == "confirmed" {
		return nil
	}
	term, err := showWorkerOrcaTerminal(ctx, fixedConfig, identity.Terminal)
	if err != nil {
		if op := s.operationByKey(key); op != nil {
			selector := "path:" + filepath.ToSlash(fixedConfig.Repository)
			if role == "agent" {
				selector = "id:" + identity.WorktreeID
			}
			terms, runtimeID, listErr := nativeOrcaTerminals(ctx, fixedConfig, selector)
			if listErr == nil && runtimeID == identity.RuntimeID {
				for _, candidate := range terms {
					if candidate.Handle == identity.Terminal {
						return err
					}
				}
				return s.changeWorkerOperation(run, op.OperationID, "confirmed", op.PlatformRequestID, mustJSON(map[string]string{"closed_handle": identity.Terminal, "proof": "absent_on_original_runtime"}), "")
			}
		}
		return err
	}
	if term.RuntimeID != identity.RuntimeID || term.IncarnationID != identity.Incarnation || role == "agent" && term.WorktreeID != identity.WorktreeID || role == "coordinator" && canonical(filepath.FromSlash(term.WorktreePath)) != canonical(fixedConfig.Repository) {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca cleanup terminal was replaced or moved"}
	}
	op := s.operationByKey(key)
	opID := ""
	if op == nil {
		opID, err = s.startWorkerOperation(run, "orca_native_terminal_close", key, map[string]string{"role": role, "handle": identity.Terminal, "incarnation": identity.Incarnation, "runtime": identity.RuntimeID})
		if err != nil {
			return err
		}
	} else {
		opID = op.OperationID
	}
	receipt, err := orcaworker.CloseTerminal(ctx, orcaWorkerMetaCaller(fixedConfig), identity.Terminal, identity.RuntimeID)
	if err != nil {
		var callErr *orcaworker.CallError
		if errors.As(err, &callErr) {
			_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
		}
		return workerOrcaBackendError(err)
	}
	return s.changeWorkerOperation(run, opID, "confirmed", "", receipt, "")
}

func (s *Service) finishNativeOrcaCleanup(ctx context.Context, run *Run) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	workflow, execution := s.workflowRecords(run)
	if run == nil || workflow == nil || execution == nil || execution.OrcaNative == nil || execution.Process == nil || execution.Process.Kind != "orca-native" || workflow.Phase != "completed" {
		return nil
	}
	state := execution.OrcaNative
	if execution.PendingTurnID != "" || execution.CurrentTurnID == "" || execution.CurrentTurnID != execution.LastStoppedTurnID || state.DeliveryID != "" {
		return nil // Delivery succeeded, but the external worker or inbox is not yet safe to close.
	}
	if err := s.closeNativeOrcaTerminal(ctx, run, execution, *execution.Process, "agent"); err != nil {
		return err
	}
	if state.Coordinator.Terminal == "" || execution.Target.OrchestrationRun == "" {
		return nil
	}
	delivery, err := orcaworker.CheckInbox(ctx, orcaWorkerMetaCaller(fixedConfig), state.Coordinator.Terminal, execution.Target.OrchestrationRun)
	if err != nil {
		return err
	}
	if delivery.RuntimeID != state.Coordinator.RuntimeID || delivery.RunID != execution.Target.OrchestrationRun || delivery.DeliveryID != "" || len(delivery.Messages) != 0 {
		return &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "Orca coordinator inbox is not drained after delivery"}
	}
	return s.closeNativeOrcaTerminal(ctx, run, execution, state.Coordinator, "coordinator")
}
