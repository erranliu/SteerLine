package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type nativeStepDispatchOperation struct {
	Input NativeTaskWorkerContext `json:"input"`
}

func (s *Service) reconcileExitedNativeStep(ctx context.Context) error {
	if s.worker != nil {
		return nil
	}
	for _, run := range s.State.Runs {
		if run == nil || run.WorkflowVersion != MinimalWorkflowVersion {
			continue
		}
		task := nativeTaskRecord(s.State.Tasks, run.DomainTaskID)
		workflow, execution := s.minimalRecords(run)
		attempt := s.minimalAttempt(run, run.AttemptID)
		if task == nil || task.Status != "in_progress" || workflow == nil || execution == nil || attempt == nil || execution.CurrentStep != attempt.StepID || execution.Process == nil || !isReconcileableStepStatus(attempt.Status) {
			continue
		}
		config, configErr := s.executionConfigForRun(run)
		if configErr != nil {
			return configErr
		}
		ref, err := (executionRecords{root: config.RunDir}).loadBinding(run.RunID, attempt.AttemptID)
		if err != nil || ref.RunID != run.RunID || ref.AttemptID != attempt.AttemptID || ref.External != *execution.Process || ref.Backend != BackendKind(execution.Backend) || ref.Protocol != ExecutionProtocol(execution.Protocol) || ref.Target.ID != targetFor(config).ID {
			continue
		}
		backend := backendFor(config)
		observation, err := backend.Observe(ctx, ref)
		if err != nil || observation.Liveness != "exited" {
			continue
		}
		resultPath := attempt.ResultPath
		if resultPath == "" {
			continue
		}
		_, _, _, fixedResultPath := minimalAttemptPaths(config, run, attempt)
		if canonical(resultPath) != canonical(fixedResultPath) {
			return nativeErr("authority_mismatch", "native step result path differs from the fixed attempt")
		}
		resultInfo, err := os.Stat(resultPath)
		if err != nil || !resultInfo.Mode().IsRegular() || resultInfo.Size() <= 0 {
			continue
		}
		s.Config = config // This exact run is now the one execution being reconciled.
		if ref.Backend == BackendOrca {
			if err = s.reconcileExitedOrcaStepLauncher(ctx, run, workflow, execution, attempt); err != nil {
				run.Error = ptr("native Orca result is preserved but launcher cleanup requires reconciliation: " + clip(err.Error(), 1000))
				return errors.Join(err, s.save())
			}
		}
		if attempt.Status != "running" {
			if err = TransitionStepAttempt(attempt, "running"); err != nil {
				return err
			}
		}
		execution.Status, execution.UpdatedAt = "active", now()
		workflow.Phase, workflow.UpdatedAt = "executing", now()
		run.ExecutionStatus, run.FinishTaskStatus = "running", "pending"
		run.StepResultPath, run.EndedAt, run.Error = ptr(resultPath), nil, nil
		run.Process = execution.Process
		s.State.SlotOccupied, s.State.CurrentTask = true, ptr(run.TaskID)
		if err = s.save(); err != nil {
			return err
		}
		run = s.find(task.TaskID)
		if run == nil {
			return nativeErr("inconsistent_delivery", "reconciled native step disappeared from the runtime projection")
		}
		s.active = run
		s.worker = nil
		s.note(run, "steer", "step_result", "reconciled", "accepted the saved result after observing the original worker exit", attempt.AttemptID, map[string]any{"backend": ref.Backend, "protocol": ref.Protocol, "result_path": resultPath})
		return s.finishMinimalStep(nil)
	}
	return nil
}

func isReconcileableStepStatus(status string) bool {
	switch status {
	case "dispatching", "dispatched", "running", "outcome_unknown":
		return true
	default:
		return false
	}
}

func (s *Service) reconcileExitedOrcaStepLauncher(ctx context.Context, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, attempt *StepAttemptRecord) error {
	if run == nil || workflow == nil || execution == nil || attempt == nil || run.WorktreePath == nil || run.WorktreeBranch == nil {
		return nativeErr("authority_mismatch", "native Orca recovery lacks the fixed workspace and attempt")
	}
	dispatch, found := nativeStepDispatchRequest(s.State.Operations, attempt.OperationID)
	if !found {
		return nativeErr("inconsistent_delivery", "native Orca step recovery lacks its durable dispatch input")
	}
	task := nativeTaskRecord(s.State.Tasks, run.DomainTaskID)
	if task == nil || validateNativeTaskWorkerContext(dispatch.Input, s.Config.identity, task.TaskID, run, workflow, execution, attempt) != nil {
		return nativeErr("contract_mismatch", "native Orca recovery input differs from the fixed attempt")
	}
	contract := minimalStepContract(workflow.ProjectSteps, attempt.StepID)
	contractBytes, err := json.Marshal(contract)
	if err != nil || operationDigest(contractBytes) != attempt.ContractDigest {
		return nativeErr("contract_mismatch", "native Orca recovery step contract differs from its fixed attempt")
	}
	dir, _, _, resultPath := minimalAttemptPaths(s.Config, run, attempt)
	if canonical(resultPath) != canonical(attempt.ResultPath) {
		return nativeErr("authority_mismatch", "native Orca recovery result path differs from its attempt")
	}
	inputPath := filepath.Join(dir, "native-task-input.json")
	promptPath := filepath.Join(dir, "prompt.txt")
	schemaPath := filepath.Join(dir, "step-result.schema.json")
	stepExecutable, stepArgs, stdinPath, outputPath, captureStdout, err := s.orcaNativeStepCommand(s.Config, run, attempt, dispatch.Input.StepInput, contract, schemaPath, inputPath, promptPath, resultPath)
	if err != nil {
		return err
	}
	env := mergeStepEnv(minimalStepEnv(s.Config, run, attempt), contract.Environment)
	launcherPath := filepath.Join(*run.WorktreePath, ".steerline-launch-"+attempt.AttemptID+".cmd")
	expected := renderNativeOrcaBatchRunner(stepExecutable, stepArgs, stdinPath, outputPath, captureStdout, env)
	expectedDigest := operationDigest(expected)
	if attempt.NativeLauncherPath != "" && canonical(attempt.NativeLauncherPath) != canonical(launcherPath) {
		return nativeErr("authority_mismatch", "native Orca launcher path differs from its fixed step attempt")
	}
	if attempt.NativeLauncherDigest != "" && attempt.NativeLauncherDigest != expectedDigest {
		return nativeErr("contract_mismatch", "native Orca launcher digest differs from its fixed worker command")
	}
	if data, readErr := os.ReadFile(launcherPath); readErr == nil {
		if !bytes.Equal(data, expected) {
			return nativeErr("contract_mismatch", "native Orca launcher content differs from its fixed worker command")
		}
	} else if !os.IsNotExist(readErr) {
		return readErr
	}
	if attempt.NativeLauncherPath == "" {
		attempt.NativeLauncherPath = launcherPath
	}
	if attempt.NativeLauncherDigest == "" {
		attempt.NativeLauncherDigest = expectedDigest
	}
	return cleanupNativeOrcaStepLauncher(ctx, s.Config, run, attempt, launcherPath, expected)
}

func nativeStepDispatchRequest(operations []OperationRecord, operationID string) (nativeStepDispatchOperation, bool) {
	for _, operation := range operations {
		if operation.OperationID != operationID || operation.Kind != "dispatch_step" || len(operation.Request) == 0 {
			continue
		}
		var request nativeStepDispatchOperation
		if json.Unmarshal(operation.Request, &request) == nil && request.Input.SchemaVersion == NativeTaskInputSchema {
			return request, true
		}
	}
	return nativeStepDispatchOperation{}, false
}

func cleanupNativeOrcaStepLauncher(ctx context.Context, c Config, run *Run, attempt *StepAttemptRecord, path string, expected []byte) error {
	if run == nil || run.WorktreePath == nil || attempt == nil || canonical(filepath.Dir(path)) != canonical(*run.WorktreePath) || filepath.Base(path) != ".steerline-launch-"+attempt.AttemptID+".cmd" {
		return nativeErr("authority_mismatch", "native Orca recovery launcher path escaped its exact worktree attempt")
	}
	if data, err := os.ReadFile(path); err == nil {
		if !bytes.Equal(data, expected) {
			return nativeErr("contract_mismatch", "native Orca launcher changed before exit reconciliation")
		}
		if err = os.Remove(path); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	gitDir, err := gitRunner(c, *run.WorktreePath).Output(ctx, c.Git, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil {
		return fmt.Errorf("resolve the exact native Orca Git exclude file: %w", err)
	}
	excludePath := strings.TrimSpace(string(gitDir))
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(*run.WorktreePath, excludePath)
	}
	exclude, err := os.ReadFile(excludePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	pattern := "/" + filepath.Base(path)
	lines := strings.SplitAfter(string(exclude), "\n")
	filtered := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		if strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")) == pattern {
			removed = true
			continue
		}
		filtered = append(filtered, line)
	}
	if removed {
		return AtomicWrite(excludePath, []byte(strings.Join(filtered, "")))
	}
	return nil
}
