package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func nativeOrcaRelayPath(worktree, runID, turnID string) (string, error) {
	if !filepath.IsAbs(worktree) || !stableExecutionID(runID) || !stableExecutionID(turnID) {
		return "", errors.New("native Orca relay has no fixed worktree and turn")
	}
	return filepath.Join(worktree, ".steerline-relay", runID, turnID+".json"), nil
}

func removeNativeOrcaRelayRequest(worktree, runID, turnID string) {
	if path, err := nativeOrcaRelayPath(worktree, runID, turnID); err == nil && nativeOrcaRelayWithinWorktree(path, worktree) {
		_ = os.Remove(path)
	}
}

func nativeOrcaRelayWithinWorktree(path, worktree string) bool {
	root, rootErr := filepath.EvalSymlinks(worktree)
	resolved, pathErr := filepath.EvalSymlinks(path)
	if rootErr != nil || pathErr != nil {
		return false
	}
	rel, err := filepath.Rel(root, resolved)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func readNativeOrcaDoneRequest(path, worktree string, run *Run, turn *OrcaNativeTurn) (*nativeOrcaDoneRequest, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 32<<10 {
		return nil, errors.New("native Orca relay request is not a bounded regular file")
	}
	if !nativeOrcaRelayWithinWorktree(path, worktree) {
		return nil, errors.New("native Orca relay request resolves outside the fixed worktree")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
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
	request, err := readNativeOrcaDoneRequest(path, execution.Workspace.Worktree, run, turn)
	if err != nil || request == nil {
		return err
	}
	key := "orca-native-relay-done:" + execution.ExecutionID + ":" + turn.TurnID
	if s.nativeOrcaOperation(key) != nil {
		// A lost Orca receipt is reconciled from the original Run inbox.
		return nil
	}
	if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
		return err
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	intent := *request
	intent.DispatchCapability = ""
	opID, err := s.startOperation("orca_native_relay_done", key, intent)
	if err != nil {
		return err
	}
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
	_ = os.Remove(path)
	return nil
}
