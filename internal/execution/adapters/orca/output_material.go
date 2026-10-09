package orcaworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

const outputDirectory = ".steerline-output"

// OutputMaterial is an explicit worker-declared response, not an inferred
// provider message phase. Only its exact original Dispatch can settle it.
type OutputMaterial struct {
	Outcome       string `json:"-"`
	WorkflowRunID string `json:"workflow_run_id"`
	TurnID        string `json:"turn_id"`
	TaskID        string `json:"orca_task_id"`
	DispatchID    string `json:"dispatch_id"`
	Body          string `json:"body"`
}

type outputBinding struct {
	Version     int                              `json:"v"`
	ProjectID   string                           `json:"project_id"`
	WorkflowID  string                           `json:"workflow_id"`
	ExecutionID string                           `json:"execution_id"`
	AttemptID   string                           `json:"attempt_id"`
	TurnID      string                           `json:"turn_id"`
	Workspace   string                           `json:"workspace"`
	Command     executionbackend.Command         `json:"command"`
	Process     executionbackend.ProcessIdentity `json:"process"`
}

func outputRelativePath(workflow, turn string) (string, error) {
	if !executionbackend.StableExecutionID(workflow) || !executionbackend.StableExecutionID(turn) {
		return "", errors.New("output material lacks its original workflow/turn identity")
	}
	return filepath.Join(outputDirectory, workflow, turn+".json"), nil
}

func OutputInstruction(workflow, turn string) string {
	path, err := outputRelativePath(workflow, turn)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("\nBefore sending native worker_done, use your own file tool to atomically write one UTF-8 JSON file at %s in this worktree. This is the complete response material for this turn, not a completion receipt. Fields: workflow_run_id=%q, turn_id=%q, orca_task_id and dispatch_id from the original Orca preamble, and body containing your complete final response exactly (including a structured result if requested). Do not commit this generated file. Then send native worker_done through Orca as instructed by the original preamble. A file alone does not prove completion.\n", filepath.ToSlash(path), workflow, turn)
}

func (l *ExecutionLifecycle) saveOutputBinding(snapshot executionbackend.LifecycleSnapshot, process executionbackend.ProcessIdentity, turn NativeTurnState) error {
	binding := outputBinding{Version: 1, ProjectID: snapshot.Session.ProjectID, WorkflowID: snapshot.Session.WorkflowRunID, ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID, TurnID: turn.TurnID, Workspace: snapshot.Workspace.Path, Command: executionbackend.Command{Executable: l.orca.Executable, Args: append([]string(nil), l.orca.Args...)}, Process: process}
	raw, err := json.Marshal(binding)
	if err != nil {
		return err
	}
	dir := filepath.Join(l.setup.RunDirectory, "runs", binding.WorkflowID, "turns", binding.ExecutionID)
	path := filepath.Join(dir, "orca-output."+binding.TurnID+".json")
	if old, readErr := os.ReadFile(path); readErr == nil {
		if !bytes.Equal(old, raw) {
			return errors.New("original Orca output source binding changed")
		}
		return nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	return l.setup.ProcessHost.WriteExecutionFile(path, raw)
}

func readOutputMaterial(ctx context.Context, call MetaCall, binding outputBinding) (*OutputMaterial, error) {
	relative, err := outputRelativePath(binding.WorkflowID, binding.TurnID)
	if err != nil {
		return nil, err
	}
	if err = noLinks(binding.Workspace, relative); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(binding.Workspace)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(relative)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 2*executionmodel.MaxConversationBodyBytes+65537))
	if err != nil {
		return nil, err
	}
	if len(raw) > 2*executionmodel.MaxConversationBodyBytes+65536 {
		return nil, errors.New("worker output material exceeds its read budget")
	}
	var material OutputMaterial
	if err = executionbackend.DecodeStrictJSON(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf}), &material, "output material has trailing JSON"); err != nil {
		return nil, err
	}
	if material.WorkflowRunID != binding.WorkflowID || material.TurnID != binding.TurnID || material.TaskID != binding.Process.Task || material.DispatchID != binding.Process.Dispatch || strings.TrimSpace(material.Body) == "" || len(material.Body) > executionmodel.MaxConversationBodyBytes {
		return nil, errors.New("worker output material differs from its original Dispatch or is incomplete")
	}
	p := binding.Process
	status, err := VerifySettledDispatch(ctx, call, DispatchIdentity{RunID: p.OrchestrationRun, TaskID: p.Task, DispatchID: p.Dispatch, RuntimeID: p.RuntimeID, Terminal: p.Terminal, Incarnation: p.Incarnation, WorktreeID: p.WorktreeID})
	if err != nil {
		return nil, err
	}
	if status != "completed" && status != "failed" {
		return nil, nil
	}
	material.Outcome = status
	return &material, nil
}

func noLinks(root, relative string) error {
	path := filepath.Clean(root)
	for _, part := range append([]string{""}, strings.Split(relative, string(filepath.Separator))...) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !samePath(resolved, path) {
			return errors.New("worker output path contains a link or reparse point")
		}
	}
	return nil
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
		return false, errors.Join(err, errors.New("native Orca output exclude is unexpectedly large"))
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
			return errors.New("native Orca output exclude is unexpectedly large")
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
