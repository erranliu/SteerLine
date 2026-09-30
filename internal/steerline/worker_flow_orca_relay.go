package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Orca's Windows named pipe is inaccessible to commands in Codex's
// workspace-write sandbox. The worker can write this one bounded request in
// its own checkout; the center forwards it only for the bound Dispatch.
type nativeOrcaDoneRequest struct {
	WorkflowRunID string `json:"workflow_run_id"`
	TurnID        string `json:"turn_id"`
	TaskID        string `json:"orca_task_id"`
	DispatchID    string `json:"dispatch_id"`
	// Used only for the Orca call. Never persist it in a SteerLine operation.
	DispatchCapability string `json:"dispatch_capability,omitempty"`
	Outcome            string `json:"outcome"`
	Subject            string `json:"subject"`
	Body               string `json:"body"`
}

func nativeOrcaRelayInstruction(runID, turnID string) string {
	return fmt.Sprintf("\nWindows workspace-write delivery: Orca CLI cannot connect to its runtime pipe from this sandbox. For your final worker_done, write one UTF-8 JSON file atomically at .steerline-relay/%s/%s.json in this worktree. Use the Orca Task and Dispatch IDs and the --dispatch-capability value from the original injected Orca preamble. Fields: workflow_run_id=%q, turn_id=%q, orca_task_id, dispatch_id, dispatch_capability (the preamble's dcap_ value; never print it), outcome (succeeded or failed), subject, body. The center will forward this exact result through Orca from your bound terminal and verify settlement. Do not commit this relay file. Do not claim that delivery succeeded merely because the file was written.\n", runID, turnID, runID, turnID)
}

// Keep the short-lived capability file out of git add -A without changing the
// project's tracked .gitignore. Git's shared exclude file covers every linked
// worktree of this one repository.
func ensureNativeOrcaRelayIgnored(ctx context.Context, c Config, worktree string) error {
	if c.identity == "" || !filepath.IsAbs(worktree) {
		return errors.New("native Orca relay ignore has no fixed repository/worktree")
	}
	out, err := gitRunner(c, worktree).Output(ctx, c.Git, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil {
		return err
	}
	if canonical(strings.TrimSpace(string(out))) != canonical(filepath.Join(c.identity, "info", "exclude")) {
		return errors.New("native Orca relay exclude is outside the fixed repository")
	}
	root, err := os.OpenRoot(c.identity)
	if err != nil {
		return err
	}
	defer root.Close()
	const path, pattern = "info/exclude", "/.steerline-relay/"
	var current []byte
	reader, err := root.Open(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		var readErr error
		current, readErr = io.ReadAll(io.LimitReader(reader, (1<<20)+1))
		if err := errors.Join(readErr, reader.Close()); err != nil {
			return err
		}
		if len(current) > 1<<20 {
			return errors.New("native Orca relay exclude is unexpectedly large")
		}
	}
	for _, line := range strings.Split(string(current), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}
	addition := pattern + "\n"
	if len(current) > 0 && current[len(current)-1] != '\n' {
		addition = "\n" + addition
	}
	f, err := root.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.WriteString(addition)
	return errors.Join(writeErr, f.Sync(), f.Close())
}

func nativeOrcaRelayPath(worktree, runID, turnID string) (string, error) {
	if !filepath.IsAbs(worktree) || !stableExecutionID(runID) || !stableExecutionID(turnID) {
		return "", errors.New("native Orca relay has no fixed worktree and turn")
	}
	return filepath.Join(worktree, ".steerline-relay", runID, turnID+".json"), nil
}

func removeNativeOrcaRelayRequest(worktree, runID, turnID string) {
	if _, err := nativeOrcaRelayPath(worktree, runID, turnID); err != nil {
		return
	}
	root, err := os.OpenRoot(worktree)
	if err == nil {
		defer root.Close()
		_ = root.Remove(filepath.Join(".steerline-relay", runID, turnID+".json"))
	}
}

func readNativeOrcaDoneRequest(path, worktree string, run *Run, turn *OrcaNativeTurn) (*nativeOrcaDoneRequest, error) {
	expected, err := nativeOrcaRelayPath(worktree, run.RunID, turn.TurnID)
	if err != nil || canonical(path) != canonical(expected) {
		return nil, errors.New("native Orca relay request path is not fixed")
	}
	root, err := os.OpenRoot(worktree)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(filepath.Join(".steerline-relay", run.RunID, turn.TurnID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 32<<10 {
		return nil, errors.New("native Orca relay request is not a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, (32<<10)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 32<<10 {
		return nil, errors.New("native Orca relay request grew beyond the bound")
	}
	var request nativeOrcaDoneRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, err
	}
	if request.WorkflowRunID != run.RunID || request.TurnID != turn.TurnID || request.TaskID != turn.TaskID || request.DispatchID != turn.DispatchID || request.Outcome != "succeeded" && request.Outcome != "failed" ||
		strings.TrimSpace(request.Subject) == "" || strings.TrimSpace(request.Body) == "" || len(request.Subject) > 200 || len(request.Body) > 16000 || !strings.HasPrefix(request.DispatchCapability, "dcap_") || len(request.DispatchCapability) > 128 {
		return nil, errors.New("native Orca relay request does not match the fixed Run/Task/Dispatch")
	}
	return &request, nil
}

func (s *Service) relayNativeOrcaDone(ctx context.Context, run *Run, execution *ExecutionRecord, turn *OrcaNativeTurn) error {
	path, err := nativeOrcaRelayPath(execution.Workspace.Worktree, run.RunID, turn.TurnID)
	if err != nil {
		return err
	}
	key := "orca-native-relay-done:" + execution.ExecutionID + ":" + turn.TurnID
	if s.nativeOrcaOperation(key) != nil {
		removeNativeOrcaRelayRequest(execution.Workspace.Worktree, run.RunID, turn.TurnID)
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca result relay was already attempted, but the original Run inbox has no worker_done; preserve the Dispatch for reconciliation"}
	}
	request, err := readNativeOrcaDoneRequest(path, execution.Workspace.Worktree, run, turn)
	if err != nil || request == nil {
		return err
	}
	if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
		return err
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	shown, err := workerOrcaCall(ctx, fixed.config, "orchestration", "worker-show", "--dispatch", turn.DispatchID, "--json")
	if err != nil {
		return err
	}
	var current struct {
		Dispatch struct {
			ID     string `json:"id"`
			RunID  string `json:"runId"`
			TaskID string `json:"taskId"`
			Status string `json:"status"`
		} `json:"dispatch"`
		Worker struct {
			AgentTerminalHandle string `json:"agentTerminalHandle"`
		} `json:"worker"`
	}
	if err := json.Unmarshal(shown.Result, &current); err != nil || shown.Meta.RuntimeID != execution.Process.RuntimeID || current.Dispatch.ID != turn.DispatchID || current.Dispatch.RunID != execution.Target.OrchestrationRun || current.Dispatch.TaskID != turn.TaskID || current.Worker.AgentTerminalHandle != execution.Process.Terminal {
		return errors.New("Orca relay cannot verify the original active Dispatch")
	}
	if current.Dispatch.Status != "dispatched" {
		return nil // The original worker_done may still be in transit to the inbox.
	}
	intent := *request
	intent.DispatchCapability = ""
	opID, err := s.startOperation("orca_native_relay_done", key, intent)
	if err != nil {
		return err
	}
	defer removeNativeOrcaRelayRequest(execution.Workspace.Worktree, run.RunID, turn.TurnID)
	args := []string{"orchestration", "send", "--run", execution.Target.OrchestrationRun, "--to", "run:" + execution.Target.OrchestrationRun,
		"--from", execution.Process.Terminal, "--type", "worker_done", "--subject", request.Subject,
		"--body", request.Body, "--task-id", request.TaskID, "--dispatch-id", request.DispatchID,
		"--outcome", request.Outcome, "--json"}
	args = append(args, "--dispatch-capability", request.DispatchCapability)
	env, err := workerOrcaCall(ctx, fixed.config, args...)
	if err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, err.Error())
		return err
	}
	var receipt struct {
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(env.Result, &receipt); err != nil || env.Meta.RuntimeID != execution.Process.RuntimeID || receipt.Message.ID == "" || receipt.Mutation.RequestID == "" {
		_ = s.changeOperation(opID, "outcome_unknown", "", nil, "Orca relay receipt lacks fixed message/request/runtime identity")
		return errors.New("Orca relay receipt lacks fixed message/request/runtime identity")
	}
	if err := s.changeOperation(opID, "accepted", receipt.Mutation.RequestID, env.Result, ""); err != nil {
		return err
	}
	s.note(run, "worker", "orca_relay", "accepted", "Orca accepted the worker's relay request", receipt.Message.ID, map[string]any{"dispatch_id": turn.DispatchID})
	if err := s.save(); err != nil {
		return err
	}
	return nil
}
