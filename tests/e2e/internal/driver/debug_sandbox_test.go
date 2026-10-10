package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstruction "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

// Opt-in only: real Codex and real GitHub in a freshly generated debug package.
func TestDebugReleaseCodexSandbox(t *testing.T) {
	root := os.Getenv("STEERLINE_DEBUG_SANDBOX_ROOT")
	if root == "" {
		t.Skip("run scripts/test-codex-sandbox.ps1")
	}
	d, err := openDebugRelease(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "scheme.json")); !os.IsNotExist(err) {
		t.Fatal("requires a fresh unselected package")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	output := func(cmd *exec.Cmd) ([]byte, error) {
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		err := testprocess.Run(ctx, cmd)
		return out.Bytes(), err
	}
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
	for _, key := range []string{"CODEX_HOME", "GH_CONFIG_DIR"} {
		if value := os.Getenv(key); value != "" {
			prepareEnv = append(prepareEnv, key+"="+value)
		}
	}
	prepare.Env = prepareEnv
	if out, err := output(prepare); err != nil {
		t.Fatalf("live preparation: %v %s", err, out)
	}
	common, err := d.git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		t.Fatal(err)
	}
	project := strings.ToLower(filepath.Clean(common))
	db := filepath.Join(d.stateDir(), "steerline.db")
	sibling := filepath.Join(root, "sandbox-negative", "sentinel.txt")
	if err := os.MkdirAll(filepath.Dir(sibling), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sibling, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	serviceCmd := d.command(d.helper, "debug", "run")
	log, err := os.Create(filepath.Join(root, "evidence", "sandbox-service.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	serviceCmd.Stdout, serviceCmd.Stderr = log, log
	service, err := testprocess.Start(serviceCmd)
	if err != nil {
		t.Fatal(err)
	}
	var branches, urls []string
	gh := func(args ...string) ([]byte, error) { return output(d.command("gh", args...)) }
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = testprocess.Run(stopCtx, d.command(d.binary, "stop-service", "-config", d.config))
		_ = service.Stop(stopCtx)
		// Only exact candidate PRs verified below are owned by this test.
		for _, url := range urls {
			out, err := gh("pr", "view", url, "--repo", debugRepository, "--json", "state")
			var state struct{ State string }
			if err != nil || json.Unmarshal(out, &state) != nil {
				t.Errorf("owned PR cleanup state unavailable: %s", url)
				continue
			}
			if state.State == "OPEN" {
				if _, err := gh("pr", "close", url, "--repo", debugRepository); err != nil {
					t.Errorf("owned PR cleanup: %s: %v", url, err)
				}
			}
		}
		if remote, err := d.git("ls-remote", "origin", "refs/heads/"+d.base); err == nil && remote != "" {
			args := append([]string{"-c", "credential.helper=", "push", "origin", "--delete"}, append(branches, d.base)...)
			if _, err := d.git(args...); err != nil {
				t.Errorf("owned debug branch cleanup: %v", err)
			}
		}
	}()
	wait := func(label string, predicate func() (bool, error)) {
		t.Helper()
		for ctx.Err() == nil {
			ok, err := predicate()
			if err != nil {
				t.Fatal(label, ": ", err)
			}
			if ok {
				return
			}
			select {
			case <-service.Done:
				t.Fatalf("%s: service exited; see %s", label, log.Name())
			default:
			}
			time.Sleep(time.Second)
		}
		t.Fatal(label, ": timeout; inspect package evidence")
	}
	wait("service ready", func() (bool, error) {
		snap, err := ReadApplicationProjectSnapshot(ctx, db, project, "", 100)
		if err != nil || snap.ExecutionState.Checkpoint == nil {
			return false, nil
		}
		runtime, err := reconstruction.BuildFromStorage(snap.ExecutionState, *snap.ExecutionState.Checkpoint)
		return err == nil && runtime.Service.Ready, nil
	})
	control := func(action, task string) {
		t.Helper()
		args := []string{"debug", "control", "-config", d.config, "-action", action, "-profile", "live"}
		if task != "" {
			args = append(args, "-task-id", task)
		}
		out, err := output(d.command(d.helper, args...))
		if err != nil {
			t.Fatalf("%s: %v %s", action, err, out)
		}
	}
	control("resume_dispatch", "")
	var proofs []map[string]any
	for _, taskID := range []string{"1", "2"} {
		control("start_task", taskID)
		var run executionmodel.WorkflowRunRecord
		wait("task "+taskID+" native publication", func() (bool, error) {
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
		})
		snap, err := ReadWorkflow(ctx, db, project)
		if err != nil {
			t.Fatal(err)
		}
		var execution executionmodel.ExecutionRecord
		for _, e := range snap.Executions {
			if e.WorkflowRunID == run.WorkflowRunID {
				execution = e
			}
		}
		if execution.SessionID == "" || len(run.StageResults) != 3 {
			t.Fatal("missing three original native stage results")
		}
		for _, stage := range run.StageResults {
			if !stage.Succeeded || !stage.FinalVerified || stage.SessionID != execution.SessionID {
				t.Fatal("unverified native stage result")
			}
		}
		out, err := gh("pr", "view", run.PRURL, "--repo", debugRepository, "--json", "url,state,headRefOid,headRefName,baseRefName")
		if err != nil {
			t.Fatalf("PR query: %v %s", err, out)
		}
		var pr struct{ URL, State, HeadRefOid, HeadRefName, BaseRefName string }
		if err := json.Unmarshal(out, &pr); err != nil {
			t.Fatal(err)
		}
		if pr.State != "OPEN" || pr.HeadRefOid != run.CandidateSHA || pr.HeadRefName != execution.Workspace.Branch || pr.BaseRefName != d.base {
			t.Fatal("PR differs from fixed candidate/base")
		}
		branches, urls = append(branches, pr.HeadRefName), append(urls, pr.URL)
		files, err := d.git("-C", execution.Workspace.Worktree, "diff", "--name-only", execution.Workspace.Base+".."+run.CandidateSHA)
		if err != nil || files != "result.txt" {
			t.Fatalf("candidate files: %q %v", files, err)
		}
		probe, escalations, err := debugSandboxNativeEvidence(d, run.WorkflowRunID, execution.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		proofs = append(proofs, map[string]any{"task_id": taskID, "run_id": run.WorkflowRunID, "worktree": execution.Workspace.Worktree, "session_id": execution.SessionID, "candidate_sha": run.CandidateSHA, "pr": pr, "native_stages": 3, "sandbox_probe": probe, "escalation_requests": escalations})
		if err := controlled.Write(filepath.Join(root, "evidence", "sandbox-live.json"), map[string]any{"passed": false, "candidates": proofs}); err != nil {
			t.Fatal(err)
		}
		if out, err := gh("pr", "ready", pr.URL, "--repo", debugRepository); err != nil {
			t.Fatalf("ready: %v %s", err, out)
		}
		if out, err := gh("pr", "merge", pr.URL, "--repo", debugRepository, "--merge", "--match-head-commit", run.CandidateSHA); err != nil {
			t.Fatalf("merge isolated base: %v %s", err, out)
		}
		wait("task "+taskID+" delivery", func() (bool, error) {
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
		})
		merged, err := gh("pr", "view", pr.URL, "--repo", debugRepository, "--json", "url,state,mergedAt,mergeCommit,headRefOid,baseRefName")
		if err != nil {
			t.Fatalf("merged PR query: %v", err)
		}
		var mergedProof map[string]any
		if err := json.Unmarshal(merged, &mergedProof); err != nil || mergedProof["state"] != "MERGED" || mergedProof["headRefOid"] != run.CandidateSHA || mergedProof["baseRefName"] != d.base {
			t.Fatal("merged PR identity differs from the verified candidate")
		}
		proofs[len(proofs)-1]["merged_pr"] = mergedProof
	}
	if proofs[0]["worktree"] == proofs[1]["worktree"] || proofs[0]["session_id"] == proofs[1]["session_id"] {
		t.Fatal("separate tasks reused a workspace or session")
	}
	if b, err := os.ReadFile(sibling); err != nil || string(b) != "unchanged" {
		t.Fatal("worker wrote to the sibling")
	}
	if err := controlled.Write(filepath.Join(root, "evidence", "sandbox-live.json"), map[string]any{"passed": true, "candidates": proofs, "deliveries": 2, "sibling_unchanged": true, "manual_confirmation": false}); err != nil {
		t.Fatal(err)
	}
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
