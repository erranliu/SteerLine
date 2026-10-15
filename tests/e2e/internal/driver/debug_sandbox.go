package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/internal/taskrunner"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
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
	if authHome == "" {
		authHome = os.Getenv("CODEX_HOME")
	}
	if authHome == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return result, homeErr
		}
		authHome = filepath.Join(home, ".codex")
	}
	var service *testprocess.Process
	var log *os.File
	var branches, urls []string
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
	// Native login remains an external dependency. Generated packages never
	// receive copied authentication or user configuration.
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
	// This acceptance owns only the project service; the package still includes Web.
	serviceCmd := d.command(d.helper, "debug", "run-project")
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
		if err != nil || snap.RunnerError != nil || snap.RunnerState.Checkpoint == nil {
			return false, nil
		}
		runtime := taskrunner.Project(snap.RunnerState, *snap.RunnerState.Checkpoint)
		return runtime.Service.Ready, nil
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
		var candidate runnerbridge.Candidate
		if err := wait("task "+taskID+" Runner publication", func() (bool, error) {
			snap, err := ReadWorkflow(ctx, db, project)
			if err != nil {
				return false, nil
			}
			for _, c := range snap.Candidates {
				if c.TaskID != taskID {
					continue
				}
				candidate = c
				if c.Projection != nil && c.Projection.PR != nil {
					return true, nil
				}
				if c.Projection != nil && (c.Projection.Status == "blocked" || c.Projection.Question != nil) {
					return false, fmt.Errorf("Runner requires input: %s", c.Projection.Diagnostic)
				}
			}
			return false, nil
		}); err != nil {
			return result, err
		}
		run := candidate.Projection
		execution := run.Execution
		finals, err := readDebugRunnerFinals(ctx, db, project, candidate.ID)
		if err != nil {
			return result, err
		}
		if execution.SessionID == "" || len(finals) != 3 {
			return result, errors.New("missing three original Runner finals")
		}
		for _, f := range finals {
			if !f.Verified || f.SessionID != execution.SessionID || f.NativeOutcome != "succeeded" {
				return result, errors.New("Runner final provenance differs")
			}
		}
		out, err := gh("pr", "view", run.PR.URL, "--repo", debugRepository, "--json", "url,state,headRefOid,headRefName,baseRefName")
		if err != nil {
			return result, fmt.Errorf("PR query: %v %s", err, out)
		}
		var pr struct{ URL, State, HeadRefOid, HeadRefName, BaseRefName string }
		if err := json.Unmarshal(out, &pr); err != nil {
			return result, err
		}
		if pr.State != "OPEN" || pr.HeadRefName != execution.Branch || pr.BaseRefName != d.base {
			return result, errors.New("PR differs from assigned branch/base")
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
		files, err := d.git("-C", execution.Workspace, "diff", "--name-only", candidate.BaseRevision+".."+pr.HeadRefOid)
		if err != nil || files != "result.txt" {
			return result, fmt.Errorf("candidate files: %q %v", files, err)
		}
		candidateContent, err := output(d.command("git", "-C", execution.Workspace, "show", pr.HeadRefOid+":result.txt"))
		if err != nil || strings.ReplaceAll(string(candidateContent), "\r\n", "\n") != "SteerlineTest debug result\n"+execution.Branch+"\n" {
			return result, fmt.Errorf("candidate result does not identify its own branch: %q %v", candidateContent, err)
		}
		rollout, err := native.ReadBusinessRollout(authHome, execution.SessionID, execution.Workspace)
		if err != nil {
			return result, err
		}
		var commands []native.RolloutCommand
		for n, f := range finals {
			stage, err := native.RolloutTurnCommands(rollout, f.SessionID, f.TurnID, execution.Workspace)
			if err != nil {
				return result, err
			}
			step := []string{executionmodel.StepImplement, executionmodel.StepReport, executionmodel.StepPublish}[n]
			if err := debugSandboxStageCommands(stage, step); err != nil {
				return result, err
			}
			commands = append(commands, stage...)
		}
		probe, err := debugSandboxNativeEvidence(commands)
		if err != nil {
			return result, err
		}
		usage, err := debugSandboxUsage(d, rollout, finals, execution.Workspace)
		if err != nil {
			return result, err
		}
		proofs = append(proofs, map[string]any{"task_id": taskID, "run_id": candidate.RunID, "worktree": execution.Workspace, "session_id": execution.SessionID, "pr": pr, "native_stages": 3, "sandbox_probe": probe, "evidence_source": "native external rollout", "usage": usage})
		result["candidates"] = proofs
		if err := controlled.Write(filepath.Join(root, "evidence", "sandbox-live.json"), result); err != nil {
			return result, err
		}
		if out, err := gh("pr", "ready", pr.URL, "--repo", debugRepository); err != nil {
			return result, fmt.Errorf("ready: %v %s", err, out)
		}
		if out, err := gh("pr", "merge", pr.URL, "--repo", debugRepository, "--merge", "--match-head-commit", pr.HeadRefOid); err != nil {
			return result, fmt.Errorf("merge isolated base: %v %s", err, out)
		}
		if err := wait("task "+taskID+" delivery", func() (bool, error) {
			snap, err := ReadWorkflow(ctx, db, project)
			if err != nil {
				return false, nil
			}
			for _, delivery := range snap.Graph.Deliveries {
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
		if err := json.Unmarshal(merged, &mergedProof); err != nil || mergedProof["state"] != "MERGED" || mergedProof["baseRefName"] != d.base {
			return result, errors.New("merged PR identity differs from assigned base")
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
		select {
		case <-service.Done:
			// The retained original process has already exited; a failed
			// startup is not a live service that needs another stop command.
		default:
			_, stopErr := run(d.command(d.binary, "stop-service", "-config", d.config))
			failures = append(failures, stopErr, service.Stop(ctx))
		}
	}
	// Run cleanup under the original package's environment, and confirm the
	// original native owner has stopped before touching its owned Git facts.
	if out, err := run(d.command(d.helper, "runner-cleanup", "-config", d.config)); err != nil {
		return errors.Join(errors.Join(failures...), fmt.Errorf("original Runner cleanup: %v %s", err, out))
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
		_, deleteErr := run(d.command("git", "-C", d.repo, "push", "origin", "--delete", branch))
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

func debugSandboxUsage(d *debugRelease, raw []byte, finals []runnerclient.Final, cwd string) ([]native.Usage, error) {
	var usages []native.Usage
	for _, f := range finals {
		usage, proof, err := native.RolloutTurnUsage(raw, cwd, f.TurnID)
		if err != nil {
			return nil, err
		}
		usage.Cwd = cwd
		if err := os.WriteFile(filepath.Join(d.root, "evidence", f.CommandID+"-usage.jsonl"), proof, 0600); err != nil {
			return nil, err
		}
		usages = append(usages, usage)
	}
	return usages, nil
}
func readDebugRunnerFinals(ctx context.Context, db, project, candidate string) ([]runnerclient.Final, error) {
	reader, err := sqlite.OpenSQLiteReader(db)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	events, err := reader.RunnerEvents(ctx, project, candidate)
	if err != nil {
		return nil, err
	}
	result := []runnerclient.Final{}
	seen := map[string]bool{}
	for _, e := range events {
		if f := e.Final; f != nil && !seen[f.CommandID] {
			seen[f.CommandID] = true
			result = append(result, *f)
		}
	}
	return result, nil
}

// Check the original native tool events, rather than trusting a model's claim
// that it has advanced to publication while still handling implementation.
func debugSandboxStageCommands(commands []native.RolloutCommand, step string) error {
	publication := false
	for _, item := range commands {
		command := strings.ToLower(strings.ReplaceAll(strings.Join(item.Command, " "), "\\", "/"))
		publishes := strings.Contains(command, "-file ./publish.ps1") ||
			(strings.Contains(command, "git ") && strings.Contains(command, " push ")) ||
			strings.Contains(command, "gh pr create ")
		if step == executionmodel.StepReport {
			return errors.New("report used a command instead of formatting existing evidence")
		}
		if publishes && step != executionmodel.StepPublish {
			return fmt.Errorf("publication attempted during %s", step)
		}
		publication = publication || publishes
	}
	if step == executionmodel.StepPublish && !publication {
		return errors.New("publication stage has no original publish command")
	}
	return nil
}

func debugSandboxNativeEvidence(commands []native.RolloutCommand) (map[string]any, error) {
	for _, item := range commands {
		if item.Status != "completed" || item.ExitCode == nil || *item.ExitCode != 0 || !strings.Contains(strings.Join(item.Command, " "), "sandbox-check.ps1") {
			continue
		}
		for _, line := range strings.Split(item.Output, "\n") {
			var value map[string]any
			if json.Unmarshal([]byte(strings.TrimSpace(line)), &value) == nil && value["passed"] == true && value["temp_in_kernel"] == true && value["app_container"] == true && value["project_check"] == true && value["sibling_write_denied"] == true {
				return value, nil
			}
		}
	}
	return nil, errors.New("no successful native sandbox probe command in original rollout")
}
