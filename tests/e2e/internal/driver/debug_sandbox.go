package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstruction "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/evidence"
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
)

// RunDebugSandbox verifies two original native tasks in a fresh generated package.
// Both the standalone script and L3 call this driver; it never answers a review.
func RunDebugSandbox(ctx context.Context, root, authHome string) (result map[string]any, err error) {
	result = map[string]any{"passed": false, "cleanup_complete": false, "usage_verified": false, "manual_confirmation": false, "root": root}
	d, err := openDebugRelease(root)
	if err != nil {
		return result, err
	}
	if _, err := os.Stat(filepath.Join(root, "scheme.json")); !os.IsNotExist(err) {
		return result, errors.New("requires a fresh unselected package")
	}
	var service *testprocess.Process
	var log *os.File
	var branches, urls []string
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cleanupErr := debugSandboxCleanup(cleanupCtx, d, service, branches)
		if log != nil {
			cleanupErr = errors.Join(cleanupErr, log.Close())
		}
		result["cleanup_complete"], result["cleanup_error"] = cleanupErr == nil, fmt.Sprint(cleanupErr)
		err = errors.Join(err, cleanupErr)
		result["passed"] = result["passed"] == true && err == nil
		result["error"] = fmt.Sprint(err)
		if writeErr := controlled.Write(filepath.Join(root, "evidence", "sandbox-live.json"), result); writeErr != nil {
			err = errors.Join(err, writeErr)
			result["passed"] = false
		}
	}()
	outputFor := func(cmd *exec.Cmd, timeout time.Duration) ([]byte, error) {
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		commandCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		err := testprocess.Run(commandCtx, cmd)
		return out.Bytes(), err
	}
	output := func(cmd *exec.Cmd) ([]byte, error) { return outputFor(cmd, 30*time.Second) }
	prepare := d.command(d.helper, "debug", "prepare")
	// Only provisioning reads the caller's existing login. Subsequent commands
	// use the package's isolated homes and privately copied authentication.
	var prepareEnv []string
	for _, entry := range prepare.Env {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "CODEX_HOME") && !strings.EqualFold(key, "GH_CONFIG_DIR") {
			prepareEnv = append(prepareEnv, entry)
		}
	}
	if authHome != "" {
		prepareEnv = append(prepareEnv, "CODEX_HOME="+authHome)
	}
	for _, key := range []string{"CODEX_HOME", "GH_CONFIG_DIR"} {
		if key == "CODEX_HOME" && authHome != "" {
			continue
		}
		if value := os.Getenv(key); value != "" {
			prepareEnv = append(prepareEnv, key+"="+value)
		}
	}
	prepare.Env = prepareEnv
	if out, err := output(prepare); err != nil {
		return result, fmt.Errorf("live preparation: %v %s", err, out)
	}
	common, err := d.git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return result, err
	}
	project := strings.ToLower(filepath.Clean(common))
	db := filepath.Join(d.stateDir(), "steerline.db")
	sibling := filepath.Join(root, "sandbox-negative", "sentinel.txt")
	if err := os.MkdirAll(filepath.Dir(sibling), 0700); err != nil {
		return result, err
	}
	if err := os.WriteFile(sibling, []byte("unchanged"), 0600); err != nil {
		return result, err
	}
	serviceCmd := d.command(d.helper, "debug", "run")
	log, err = os.Create(filepath.Join(root, "evidence", "sandbox-service.log"))
	if err != nil {
		return result, err
	}
	serviceCmd.Stdout, serviceCmd.Stderr = log, log
	service, err = testprocess.Start(serviceCmd)
	if err != nil {
		return result, err
	}
	result["service_pid"] = service.Cmd.Process.Pid
	gh := func(args ...string) ([]byte, error) { return output(d.command("gh", args...)) }
	wait := func(label string, predicate func() (bool, error)) error {
		for ctx.Err() == nil {
			ok, err := predicate()
			if err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			if ok {
				return nil
			}
			select {
			case <-service.Done:
				return fmt.Errorf("%s: service exited; see %s", label, log.Name())
			default:
			}
			time.Sleep(time.Second)
		}
		return fmt.Errorf("%s: %w; inspect package evidence", label, ctx.Err())
	}
	if err := wait("service ready", func() (bool, error) {
		snap, err := ReadApplicationProjectSnapshot(ctx, db, project, "", 100)
		if err != nil || snap.ExecutionState.Checkpoint == nil {
			return false, nil
		}
		runtime, err := reconstruction.BuildFromStorage(snap.ExecutionState, *snap.ExecutionState.Checkpoint)
		return err == nil && runtime.Service.Ready, nil
	}); err != nil {
		return result, err
	}
	control := func(action, task string) error {
		args := []string{"debug", "control", "-config", d.config, "-action", action, "-profile", "live"}
		if task != "" {
			args = append(args, "-task-id", task)
		}
		// Match Client.ControlExpectedStatus: admitted native preparation waits
		// for the original receipt; it is not a 30s command or a new request.
		out, err := outputFor(d.command(d.helper, args...), 390*time.Second)
		if err != nil {
			return fmt.Errorf("%s: %v %s", action, err, out)
		}
		return nil
	}
	if err := control("resume_dispatch", ""); err != nil {
		return result, err
	}
	var proofs []map[string]any
	for _, taskID := range []string{"1", "2"} {
		if err := control("start_task", taskID); err != nil {
			return result, err
		}
		var run executionmodel.WorkflowRunRecord
		if err := wait("task "+taskID+" native publication", func() (bool, error) {
			snap, err := ReadWorkflow(ctx, db, project)
			if err != nil {
				return false, nil
			}
			for _, candidate := range snap.WorkflowRuns {
				if candidate.TaskID != taskID {
					continue
				}
				run = candidate
				if candidate.StepID == "await_merge" && candidate.PRURL != "" {
					return true, nil
				}
				for _, e := range snap.Executions {
					if e.WorkflowRunID == candidate.WorkflowRunID && e.Workspace.Branch != "" {
						found := false
						for _, branch := range branches {
							found = found || branch == e.Workspace.Branch
						}
						if !found {
							branches = append(branches, e.Workspace.Branch)
						}
					}
					if e.WorkflowRunID == candidate.WorkflowRunID && e.DecisionID != "" {
						return false, fmt.Errorf("worker needs input at %s; inspect its original journal", candidate.StepID)
					}
				}
				if candidate.PendingDecisions > 0 {
					return false, fmt.Errorf("worker needs input at %s; inspect its original journal", candidate.StepID)
				}
				return false, nil
			}
			return false, nil
		}); err != nil {
			return result, err
		}
		snap, err := ReadWorkflow(ctx, db, project)
		if err != nil {
			return result, err
		}
		var execution executionmodel.ExecutionRecord
		for _, e := range snap.Executions {
			if e.WorkflowRunID == run.WorkflowRunID {
				execution = e
			}
		}
		if execution.SessionID == "" || len(run.StageResults) != 3 {
			return result, errors.New("missing three original native stage results")
		}
		for _, stage := range run.StageResults {
			if !stage.Succeeded || !stage.FinalVerified || stage.SessionID != execution.SessionID {
				return result, errors.New("unverified native stage result")
			}
		}
		out, err := gh("pr", "view", run.PRURL, "--repo", debugRepository, "--json", "url,state,headRefOid,headRefName,baseRefName")
		if err != nil {
			return result, fmt.Errorf("PR query: %v %s", err, out)
		}
		var pr struct{ URL, State, HeadRefOid, HeadRefName, BaseRefName string }
		if err := json.Unmarshal(out, &pr); err != nil {
			return result, err
		}
		if pr.State != "OPEN" || pr.HeadRefOid != run.CandidateSHA || pr.HeadRefName != execution.Workspace.Branch || pr.BaseRefName != d.base {
			return result, errors.New("PR differs from fixed candidate/base")
		}
		urls = append(urls, pr.URL)
		foundBranch := false
		for _, branch := range branches {
			foundBranch = foundBranch || branch == pr.HeadRefName
		}
		if !foundBranch {
			branches = append(branches, pr.HeadRefName)
		}
		result["pr_urls"] = urls
		files, err := d.git("-C", execution.Workspace.Worktree, "diff", "--name-only", execution.Workspace.Base+".."+run.CandidateSHA)
		if err != nil || files != "result.txt" {
			return result, fmt.Errorf("candidate files: %q %v", files, err)
		}
		candidateContent, err := output(d.command("git", "-C", d.repo, "show", run.CandidateSHA+":result.txt"))
		if err != nil || strings.ReplaceAll(string(candidateContent), "\r\n", "\n") != "SteerlineTest debug result\n"+execution.Workspace.Branch+"\n" {
			return result, fmt.Errorf("candidate result does not identify its own branch: %q %v", candidateContent, err)
		}
		for _, stage := range run.StageResults {
			journal := filepath.Join(d.stateDir(), "runs", run.WorkflowRunID, "turns", stage.CommandID, "appserver.jsonl")
			if err := debugSandboxStageCommands(journal, stage.StepID); err != nil {
				return result, err
			}
		}
		probe, escalations, err := debugSandboxNativeEvidence(d, run.WorkflowRunID, execution.SessionID)
		if err != nil {
			return result, err
		}
		bound, err := evidence.Candidate(snap, run.WorkflowRunID, execution.ExecutionID)
		if err != nil {
			return result, err
		}
		settings, err := evidence.ModelSelection(bound)
		if err != nil || settings.Model != native.Model || settings.Effort != native.Effort {
			return result, errors.New("sandbox frozen model/effort differs from live registry")
		}
		if err := evidence.FixedNativeLaunch(bound); err != nil {
			return result, err
		}
		usage, err := debugSandboxUsage(d, run, execution.Workspace.Worktree)
		if err != nil {
			return result, err
		}
		proofs = append(proofs, map[string]any{"task_id": taskID, "run_id": run.WorkflowRunID, "worktree": execution.Workspace.Worktree, "session_id": execution.SessionID, "candidate_sha": run.CandidateSHA, "pr": pr, "native_stages": 3, "sandbox_probe": probe, "escalation_requests": escalations, "usage": usage})
		result["candidates"] = proofs
		if err := controlled.Write(filepath.Join(root, "evidence", "sandbox-live.json"), result); err != nil {
			return result, err
		}
		if out, err := gh("pr", "ready", pr.URL, "--repo", debugRepository); err != nil {
			return result, fmt.Errorf("ready: %v %s", err, out)
		}
		if out, err := gh("pr", "merge", pr.URL, "--repo", debugRepository, "--merge", "--match-head-commit", run.CandidateSHA); err != nil {
			return result, fmt.Errorf("merge isolated base: %v %s", err, out)
		}
		if err := wait("task "+taskID+" delivery", func() (bool, error) {
			snap, err := ReadWorkflow(ctx, db, project)
			if err != nil {
				return false, nil
			}
			for _, delivery := range snap.Deliveries {
				if delivery.TaskID == taskID {
					return true, nil
				}
			}
			return false, nil
		}); err != nil {
			return result, err
		}
		merged, err := gh("pr", "view", pr.URL, "--repo", debugRepository, "--json", "url,state,mergedAt,mergeCommit,headRefOid,baseRefName")
		if err != nil {
			return result, fmt.Errorf("merged PR query: %v", err)
		}
		var mergedProof map[string]any
		if err := json.Unmarshal(merged, &mergedProof); err != nil || mergedProof["state"] != "MERGED" || mergedProof["headRefOid"] != run.CandidateSHA || mergedProof["baseRefName"] != d.base {
			return result, errors.New("merged PR identity differs from the verified candidate")
		}
		proofs[len(proofs)-1]["merged_pr"] = mergedProof
	}
	if proofs[0]["worktree"] == proofs[1]["worktree"] || proofs[0]["session_id"] == proofs[1]["session_id"] {
		return result, errors.New("separate tasks reused a workspace or session")
	}
	if b, err := os.ReadFile(sibling); err != nil || string(b) != "unchanged" {
		return result, errors.New("worker wrote to the sibling")
	}
	result["candidates"], result["deliveries"], result["sibling_unchanged"], result["usage_verified"] = proofs, 2, true, true
	result["passed"] = true
	return result, nil
}

func debugSandboxCleanup(ctx context.Context, d *debugRelease, service *testprocess.Process, branches []string) error {
	var failures []error
	run := func(cmd *exec.Cmd) ([]byte, error) {
		var output bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &output
		err := testprocess.Run(ctx, cmd)
		return output.Bytes(), err
	}
	if service != nil {
		_, stopErr := run(d.command(d.binary, "stop-service", "-config", d.config))
		failures = append(failures, stopErr, service.Stop(ctx))
	}
	for _, branch := range branches {
		if branch == "main" || branch == d.base || !strings.HasPrefix(branch, "codex/") {
			failures = append(failures, errors.New("refuse unowned debug head cleanup"))
			continue
		}
		raw, queryErr := run(d.command("gh", "pr", "list", "--repo", debugRepository, "--base", d.base, "--head", branch, "--state", "open", "--json", "url,baseRefName,headRefName"))
		var prs []struct{ URL, BaseRefName, HeadRefName string }
		if queryErr != nil {
			failures = append(failures, queryErr)
			continue
		}
		if decodeErr := json.Unmarshal(raw, &prs); decodeErr != nil {
			failures = append(failures, decodeErr)
			continue
		}
		for _, pr := range prs {
			if pr.BaseRefName != d.base || pr.HeadRefName != branch || !strings.HasPrefix(pr.URL, "https://github.com/"+debugRepository+"/pull/") {
				failures = append(failures, errors.New("refuse PR cleanup with different base/head"))
				continue
			}
			_, closeErr := run(d.command("gh", "pr", "close", pr.URL, "--repo", debugRepository))
			failures = append(failures, closeErr)
		}
	}
	// Check each ref independently: a missing base must not hide a surviving head.
	for _, branch := range append(append([]string{}, branches...), d.base) {
		if branch != d.base && (branch == "main" || !strings.HasPrefix(branch, "codex/")) {
			continue // Rejected above; never delete a ref outside the owned heads.
		}
		raw, queryErr := run(d.command("git", "-C", d.repo, "ls-remote", "origin", "refs/heads/"+branch))
		if queryErr != nil {
			failures = append(failures, queryErr)
			continue
		}
		if strings.TrimSpace(string(raw)) == "" {
			continue
		}
		_, deleteErr := run(d.command("git", "-C", d.repo, "-c", "credential.helper=", "push", "origin", "--delete", branch))
		failures = append(failures, deleteErr)
		if deleteErr == nil {
			raw, queryErr = run(d.command("git", "-C", d.repo, "ls-remote", "origin", "refs/heads/"+branch))
			if queryErr != nil {
				failures = append(failures, queryErr)
			} else if strings.TrimSpace(string(raw)) != "" {
				failures = append(failures, fmt.Errorf("owned ref survives cleanup: %s", branch))
			}
		}
	}
	return errors.Join(failures...)
}

func debugSandboxUsage(d *debugRelease, workflow executionmodel.WorkflowRunRecord, cwd string) ([]native.Usage, error) {
	var original []byte
	matches := 0
	session := workflow.StageResults[0].SessionID
	err := filepath.WalkDir(filepath.Join(d.home, "codex", "sessions"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.Contains(entry.Name(), session) || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		business, err := native.BusinessRolloutMatches(raw, cwd)
		if err != nil {
			return err
		}
		if business {
			original = raw
			matches++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if matches != 1 {
		return nil, fmt.Errorf("sandbox usage has %d original root sessions", matches)
	}
	modelUsage, modelProof, err := native.RolloutUsage(original, cwd)
	if err != nil {
		return nil, err
	}
	if modelUsage.Session != session || modelUsage.Turn != workflow.StageResults[len(workflow.StageResults)-1].TurnID {
		return nil, errors.New("sandbox model usage differs from original session/final turn")
	}
	if err := os.WriteFile(filepath.Join(d.root, "evidence", workflow.WorkflowRunID+"-model-usage.jsonl"), modelProof, 0600); err != nil {
		return nil, err
	}
	var usages []native.Usage
	for _, stage := range workflow.StageResults {
		raw, err := os.ReadFile(filepath.Join(d.stateDir(), "runs", workflow.WorkflowRunID, "turns", stage.CommandID, "appserver.jsonl"))
		if err != nil {
			return nil, err
		}
		usage, proof, err := native.AppServerTurnUsage(raw, session, stage.TurnID)
		if err != nil {
			return nil, err
		}
		usage.Cwd = cwd
		if err := os.WriteFile(filepath.Join(d.root, "evidence", stage.CommandID+"-usage.jsonl"), proof, 0600); err != nil {
			return nil, err
		}
		if err := controlled.Write(filepath.Join(d.root, "evidence", stage.CommandID+"-usage.json"), usage); err != nil {
			return nil, err
		}
		usages = append(usages, usage)
	}
	return usages, nil
}

// Check the original native tool events, rather than trusting a model's claim
// that it has advanced to publication while still handling implementation.
func debugSandboxStageCommands(journal, step string) error {
	raw, err := os.ReadFile(journal)
	if err != nil {
		return err
	}
	publication := false
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event struct {
			Method string
			Params struct {
				Item struct{ Type, Command string }
			}
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("invalid %s native journal: %w", step, err)
		}
		if event.Method != "item/started" && event.Method != "item/completed" || event.Params.Item.Type != "commandExecution" {
			continue
		}
		command := strings.ToLower(strings.ReplaceAll(event.Params.Item.Command, "\\", "/"))
		publishes := strings.Contains(command, "-file ./publish.ps1") ||
			(strings.Contains(command, "git ") && strings.Contains(command, " push ")) ||
			strings.Contains(command, "gh pr create ")
		if step == executionmodel.StepReport {
			return fmt.Errorf("report used a command instead of formatting existing evidence: %s", journal)
		}
		if publishes && step != executionmodel.StepPublish {
			return fmt.Errorf("publication attempted during %s: %s", step, journal)
		}
		publication = publication || publishes
	}
	if step == executionmodel.StepPublish && !publication {
		return fmt.Errorf("publication stage has no original publish command: %s", journal)
	}
	return nil
}

func debugSandboxNativeEvidence(d *debugRelease, runID, sessionID string) (map[string]any, int, error) {
	var probe map[string]any
	err := filepath.WalkDir(filepath.Join(d.stateDir(), "runs", runID), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "appserver.jsonl" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(raw), "\n") {
			var e struct {
				Method string
				Params struct {
					Item struct {
						Type, Command, AggregatedOutput string
						ExitCode                        int
					}
				}
			}
			if json.Unmarshal([]byte(line), &e) != nil || e.Method != "item/completed" || e.Params.Item.Type != "commandExecution" || e.Params.Item.ExitCode != 0 || !strings.Contains(e.Params.Item.Command, "sandbox-check.ps1") {
				continue
			}
			for _, output := range strings.Split(e.Params.Item.AggregatedOutput, "\n") {
				var value map[string]any
				if json.Unmarshal([]byte(strings.TrimSpace(output)), &value) == nil && value["passed"] == true && value["temp_in_admin"] == true && value["project_check"] == true && value["sibling_write_denied"] == true {
					probe = value
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	if probe == nil {
		return nil, 0, fmt.Errorf("no successful native sandbox probe command for %s", runID)
	}
	escalations := 0
	// Inspect only this worker's private native rollout, never user-wide history.
	err = filepath.WalkDir(filepath.Join(d.home, "codex", "sessions"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.Contains(entry.Name(), sessionID) || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(raw), "\n") {
			var e struct {
				Type    string
				Payload struct{ Type, Input, Arguments string }
			}
			if json.Unmarshal([]byte(line), &e) != nil || e.Type != "response_item" || (e.Payload.Type != "custom_tool_call" && e.Payload.Type != "function_call") {
				continue
			}
			input := e.Payload.Input + e.Payload.Arguments
			if strings.Contains(input, "require_escalated") {
				escalations++
				if strings.Contains(input, "sandbox-check.ps1") {
					return fmt.Errorf("ordinary sandbox probe requested escalation")
				}
			}
		}
		return nil
	})
	return probe, escalations, err
}
