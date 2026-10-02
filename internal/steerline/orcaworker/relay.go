package orcaworker

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

type DoneRequest struct {
	WorkflowRunID string `json:"workflow_run_id"`
	TurnID        string `json:"turn_id"`
	TaskID        string `json:"orca_task_id"`
	DispatchID    string `json:"dispatch_id"`
	// Used only for the Orca call; never persist it in a SteerLine operation.
	DispatchCapability string `json:"dispatch_capability,omitempty"`
	Outcome            string `json:"outcome"`
	Subject            string `json:"subject"`
	Body               string `json:"body"`
}

var ErrActiveDispatchIdentity = errors.New("Orca relay cannot verify the original active Dispatch")

func RelayInstruction(runID, turnID string) string {
	return fmt.Sprintf("\nWindows workspace-write delivery: Orca CLI cannot connect to its runtime pipe from this sandbox. For your final worker_done, write one UTF-8 JSON file atomically at .steerline-relay/%s/%s.json in this worktree. Use the Orca Task and Dispatch IDs and the --dispatch-capability value from the original injected Orca preamble. Fields: workflow_run_id=%q, turn_id=%q, orca_task_id, dispatch_id, dispatch_capability (the preamble's dcap_ value; never print it), outcome (succeeded or failed), subject, body. The center will forward this exact result through Orca from your bound terminal and verify settlement. Do not commit this relay file. Do not claim that delivery succeeded merely because the file was written.\n", runID, turnID, runID, turnID)
}

// EnsureRelayIgnored updates only the verified repository's local exclude file.
func EnsureRelayIgnored(repositoryGitDir string) error {
	return ensureIgnored(repositoryGitDir, "/.steerline-relay/")
}

func EnsureReadinessIgnored(repositoryGitDir string) error {
	return ensureIgnored(repositoryGitDir, "/.steerline-readiness/")
}

func RelayIgnored(repositoryGitDir string) (bool, error) {
	return ignoredPattern(repositoryGitDir, "/.steerline-relay/")
}

func ReadinessIgnored(repositoryGitDir string) (bool, error) {
	return ignoredPattern(repositoryGitDir, "/.steerline-readiness/")
}

func ignoredPattern(repositoryGitDir, pattern string) (bool, error) {
	root, err := os.OpenRoot(repositoryGitDir)
	if err != nil {
		return false, err
	}
	defer root.Close()
	reader, err := root.Open("info/exclude")
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return false, errors.Join(err, errors.New("native Orca relay exclude is unexpectedly large"))
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == pattern {
			return true, nil
		}
	}
	return false, nil
}

func ensureIgnored(repositoryGitDir, pattern string) error {
	root, err := os.OpenRoot(repositoryGitDir)
	if err != nil {
		return err
	}
	defer root.Close()
	const path = "info/exclude"
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

func RelayPath(worktree, runID, turnID string) (string, error) {
	if !filepath.IsAbs(worktree) || !stableID(runID) || !stableID(turnID) {
		return "", errors.New("native Orca relay has no fixed worktree and turn")
	}
	return filepath.Join(worktree, ".steerline-relay", runID, turnID+".json"), nil
}

func RemoveRelayRequest(worktree, runID, turnID string) error {
	if _, err := RelayPath(worktree, runID, turnID); err != nil {
		return err
	}
	root, err := os.OpenRoot(worktree)
	if err != nil {
		return err
	}
	defer root.Close()
	removeErr := root.Remove(filepath.Join(".steerline-relay", runID, turnID+".json"))
	if errors.Is(removeErr, os.ErrNotExist) {
		return nil
	}
	return removeErr
}

func ReadDoneRequest(path, worktree, runID, turnID, taskID, dispatchID string) (*DoneRequest, error) {
	expected, err := RelayPath(worktree, runID, turnID)
	if err != nil || !samePath(path, expected) {
		return nil, errors.New("native Orca relay request path is not fixed")
	}
	root, err := os.OpenRoot(worktree)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(filepath.Join(".steerline-relay", runID, turnID+".json"))
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
	var request DoneRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, err
	}
	if request.WorkflowRunID != runID || request.TurnID != turnID || request.TaskID != taskID || request.DispatchID != dispatchID || request.Outcome != "succeeded" && request.Outcome != "failed" ||
		strings.TrimSpace(request.Subject) == "" || strings.TrimSpace(request.Body) == "" || len(request.Subject) > 200 || len(request.Body) > 16000 || !strings.HasPrefix(request.DispatchCapability, "dcap_") || len(request.DispatchCapability) > 128 {
		return nil, errors.New("native Orca relay request does not match the fixed Run/Task/Dispatch")
	}
	return &request, nil
}

func VerifyActiveDispatch(ctx context.Context, call MetaCall, expected DispatchIdentity) (string, error) {
	envelope, err := call(ctx, "orchestration", "worker-show", "--dispatch", expected.DispatchID, "--json")
	if err != nil {
		return "", err
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
	if err := json.Unmarshal(envelope.Payload, &current); err != nil || envelope.RuntimeID != expected.RuntimeID || current.Dispatch.ID != expected.DispatchID || current.Dispatch.RunID != expected.RunID || current.Dispatch.TaskID != expected.TaskID || current.Worker.AgentTerminalHandle != expected.Terminal {
		return "", ErrActiveDispatchIdentity
	}
	return current.Dispatch.Status, nil
}

func SendDone(ctx context.Context, call MetaCall, request DoneRequest, runID, from, runtimeID string) (MutationReceipt, error) {
	envelope, err := call(ctx, "orchestration", "send", "--run", runID, "--to", "run:"+runID,
		"--from", from, "--type", "worker_done", "--subject", request.Subject,
		"--body", request.Body, "--task-id", request.TaskID, "--dispatch-id", request.DispatchID,
		"--outcome", request.Outcome, "--json", "--dispatch-capability", request.DispatchCapability)
	if err != nil {
		return MutationReceipt{}, &CallError{Cause: err, Result: envelope}
	}
	var receipt struct {
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(envelope.Payload, &receipt); err != nil || envelope.RuntimeID != runtimeID || receipt.Message.ID == "" || receipt.Mutation.RequestID == "" {
		return MutationReceipt{}, errors.New("Orca relay receipt lacks fixed message/request/runtime identity")
	}
	return MutationReceipt{ID: receipt.Message.ID, RequestID: receipt.Mutation.RequestID, Payload: envelope.Payload}, nil
}
