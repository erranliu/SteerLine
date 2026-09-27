package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSupervisedOrcaRealNativeTaskLifecycle is an opt-in end-to-end probe for
// native-task authority on the supervised remote backend. It only targets the
// isolated SteerlineTest repository and records each platform identity.
func TestSupervisedOrcaRealNativeTaskLifecycle(t *testing.T) {
	if os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_PROBE") != "1" {
		t.Skip("set STEERLINE_REAL_SUPERVISED_NATIVE_PROBE=1 for the authorized native remote acceptance run")
	}
	need := func(name string) string {
		t.Helper()
		v := strings.TrimSpace(os.Getenv(name))
		if v == "" {
			t.Fatalf("%s is required for the native supervised probe", name)
		}
		return v
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root, err := os.MkdirTemp("", "s3-")
	if err != nil {
		t.Fatalf("create short isolated native acceptance root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	repositoryURL := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_REPOSITORY_URL"))
	if repositoryURL == "" {
		repositoryURL = "https://github.com/erranliu/SteerlineTest.git"
	}
	nativeBaseBranch := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_BASE_BRANCH"))
	if nativeBaseBranch == "" {
		nativeBaseBranch = "codex/spec03-native-acceptance-base"
	}
	testGitHubRepository := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_GITHUB_REPOSITORY"))
	if testGitHubRepository == "" {
		testGitHubRepository = "erranliu/SteerlineTest"
	}
	repo := filepath.Join(root, "native-project")
	if output, err := exec.Command("git", "clone", "--quiet", "--branch", nativeBaseBranch, repositoryURL, repo).CombinedOutput(); err != nil {
		t.Fatalf("clone isolated test repository: %v: %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(repo, ".beads")); err == nil {
		t.Skip("the selected acceptance repository already contains Beads authority; use a dedicated native-task repository instead of overwriting it")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	baseBytes, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSpace(string(baseBytes))
	if !commitHash.MatchString(base) {
		t.Fatalf("invalid native test repository base %q", base)
	}
	t.Setenv("STEERLINE_HOME", filepath.Join(root, "steerline-home"))
	c := Config{
		TaskAuthority: "native", Repository: repo, Remote: "origin", Branch: nativeBaseBranch, RuntimeFile: "runtime.json",
		Claimant: "native-supervised-acceptance", Git: Command{Executable: "git"}, Codex: Command{Executable: "git"},
		TaskPoll: "250ms", PRPoll: "1s", Heartbeat: "1s", CommandTimeout: "30s", ShutdownGrace: "5s",
		Sandbox: "workspace-write", Agent: "codex", Host: "orca_supervised", Model: "gpt-6-luna", ReasoningEffort: "high",
		Orca: OrcaConfig{Executable: "orca", Shell: "powershell", WaitTimeout: "15m", Supervised: &SupervisedOrcaConfig{
			ConnectionID: need("STEERLINE_REAL_SUPERVISED_CONNECTION_ID"), EnvironmentID: need("STEERLINE_REAL_SUPERVISED_ENVIRONMENT_ID"),
			RepositorySelector: need("STEERLINE_REAL_SUPERVISED_REPOSITORY_SELECTOR"), CoordinatorTerminal: need("STEERLINE_REAL_SUPERVISED_COORDINATOR_TERMINAL"),
			ExpectedCodexVersion: "", PollInterval: "2s", CapabilityReportVersion: 1,
		}},
		PR: PRConfig{Kind: "command", BaseURL: "https://github.com", Repository: testGitHubRepository, Query: Command{Executable: "gh", Args: []string{
			"api", "repos/{repository}/pulls/{number}", "--jq", `{url:.html_url,repository:.base.repo.full_name,base_branch:.base.ref,head_revision:.head.sha,state:(if .merged then "merged" elif .state=="closed" then "closed_unmerged" else .state end),merged:.merged,merged_at:.merged_at,merge_commit:.merge_commit_sha}`,
		}}, Close: Command{Executable: "gh", Args: []string{
			"api", "repos/{repository}/pulls/{number}", "-X", "PATCH", "-f", "state=closed", "--jq", `{url:.html_url,repository:.base.repo.full_name,base_branch:.base.ref,head_revision:.head.sha,state:(if .merged then "merged" elif .state=="closed" then "closed_unmerged" else .state end),merged:.merged,merged_at:.merged_at,merge_commit:.merge_commit_sha}`,
		}}},
	}
	definition := DefaultMinimalWorkflowDefinition(true)
	step := func(provider, entry string, command []string) StepContract {
		return StepContract{Provider: provider, Entry: entry, Version: MinimalWorkflowVersion, InputSchema: NativeTaskInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "target", Timeout: 10 * time.Minute, Command: command}
	}
	stageFile := "steerline-spec03-native-stage.txt"
	nativeAcceptanceTitle := "Native supervised acceptance artifact"
	verifyCommand := fmt.Sprintf("$h=(git rev-parse HEAD:%s).Trim(); if($LASTEXITCODE -ne 0 -or $h -cne 'a7890c716cc3ad0cca79f8d7b68cffe125e6b106'){exit 7}; exit 0", stageFile)
	publishCommand := strings.ReplaceAll(strings.ReplaceAll("$ErrorActionPreference='Stop'; $branch=git branch --show-current; $head=(git rev-parse HEAD).Trim(); git push -u origin HEAD; if($LASTEXITCODE -ne 0){exit $LASTEXITCODE}; $matches=@(gh pr list --repo erranliu/SteerlineTest --head $branch --state all --json url,headRefOid,baseRefName | ConvertFrom-Json | Where-Object {$_.headRefOid -eq $head -and $_.baseRefName -eq 'main'}); if($matches.Count -gt 1){exit 17}; if($matches.Count -eq 1){$url=$matches[0].url}else{$url=gh pr create --repo erranliu/SteerlineTest --draft --base main --head $branch --title ('Native supervised acceptance '+$branch) --body 'Native task authority supervised remote acceptance; one verified file.'; if($LASTEXITCODE -ne 0){exit $LASTEXITCODE}}; Write-Output ('PR_URL='+($url|Select-Object -Last 1)); exit 0", "erranliu/SteerlineTest", testGitHubRepository), "main", nativeBaseBranch)
	c.WorkflowDefinition = &definition
	c.ProjectSteps = &ProjectSteps{
		Implement: step("agent_skill", "builtin:implement", nil),
		Verify:    step("command", "README.md", []string{"pwsh.exe", "-NoProfile", "-Command", verifyCommand}),
		Review:    step("agent_skill", "builtin:review", nil),
		PublishPR: step("command", "README.md", []string{"pwsh.exe", "-NoProfile", "-Command", publishCommand}),
	}
	c.ProjectSteps.Verify.RequiredChecks = []string{"content-smoke"}
	if err = c.ResolveIdentity(ctx); err != nil {
		t.Fatal(err)
	}
	stateKey, err := repoStateKey(c)
	if err != nil {
		t.Fatal(err)
	}
	c.RunDir = filepath.Join(SteerlineStateRoot(), stateKey)
	c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	configBytes, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	configHash := sha256.Sum256(configBytes)
	c.configDigest = hex.EncodeToString(configHash[:])
	if err = validateSupervisedOrcaConfig(&c); err != nil {
		t.Fatal(err)
	}
	if _, err = c.validateNativeWorkflow(); err != nil {
		t.Fatal(err)
	}
	minimalConfig, err := c.minimalConfig()
	if err != nil || minimalConfig == nil {
		t.Fatalf("native minimal workflow configuration: %v", err)
	}
	if err = c.validateMinimalPreflight(*minimalConfig); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(c.RunDir, 0700); err != nil {
		t.Fatal(err)
	}
	project, err := OpenNativeTaskProject(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = project.Close(); err != nil {
		t.Fatal(err)
	}
	// This per-test capability manifest is derived from previously accepted
	// real-machine receipts and never touches a production project state root.
	backend := orcaSupervisedBackend{config: c}
	report, err := backend.Probe(ctx, targetFor(c), agentFor(c))
	if err != nil {
		t.Fatal(err)
	}
	evidenceSources := map[string][]string{
		"start":                {"spec03-supervised-r19-retry2-publish-2026-09-26.json"},
		"observe":              {"spec03-supervised-r08-runtime-replacement-2026-09-26.json"},
		"workspace_inspection": {"spec03-supervised-r19-retry2-staged-2026-09-26.json"},
		"output_reading":       {"spec03-supervised-r19-retry2-publish-2026-09-26.json"},
		"message_inbox":        {"spec03-supervised-r19-retry2-complete-2026-09-26.json", "spec03-supervised-viewer-decision-live-2026-09-26.json"},
		"resource_settlement":  {"spec03-supervised-r19-retry2-complete-2026-09-26.json"},
	}
	acceptanceRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	acceptanceRoot = filepath.Join(acceptanceRoot, "..", "..", "docs", "spec", "orca-probe", "samples")
	manifest := map[string]any{"kind": "isolated-native-task-test-capabilities", "captured_at": now(), "capabilities": map[string]any{}}
	manifestCaps := map[string]any{}
	for name, files := range evidenceSources {
		refs := []map[string]string{}
		for _, file := range files {
			path := filepath.Join(acceptanceRoot, file)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("read real acceptance source %s: %v", file, readErr)
			}
			sum := sha256.Sum256(data)
			refs = append(refs, map[string]string{"path": file, "sha256": hex.EncodeToString(sum[:])})
		}
		manifestCaps[name] = refs
	}
	manifest["capabilities"] = manifestCaps
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "native-supervised-capability-evidence.json")
	if err = AtomicWrite(manifestPath, append(manifestBytes, '\n')); err != nil {
		t.Fatal(err)
	}
	manifestHash := sha256.Sum256(append(manifestBytes, '\n'))
	for i := range report.Capabilities {
		if evidenceSources[report.Capabilities[i].Name] != nil {
			report.Capabilities[i].Status = CapabilityVerified
			report.Capabilities[i].Evidence = manifestPath + "#sha256=" + hex.EncodeToString(manifestHash[:])
		}
	}
	if err = (executionRecords{root: c.RunDir}).saveCapabilityReport(report); err != nil {
		t.Fatal(err)
	}
	lock, err := LockRepository(c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenNativeTaskCommandStore(ctx, &c)
	if err != nil {
		lock.Close()
		t.Fatal(err)
	}
	target, err := configuredRepositoryTarget(ctx, c)
	if err != nil {
		store.Close()
		lock.Close()
		t.Fatal(err)
	}
	specRef := nativeRef(t, c, base, "docs/spec/native-acceptance.spec.md")
	planRef := nativeRef(t, c, base, "docs/spec/native-acceptance.plan.md")
	contract := TaskContract{ContractID: "native-supervised-acceptance-" + newID()[:8], Repository: target, Remote: c.Remote, Branch: c.Branch, SpecRef: specRef, PlanRef: planRef, AcceptanceRefs: []FileReference{specRef}}
	contract.Digest, err = TaskContractDigest(contract)
	if err != nil {
		store.Close()
		lock.Close()
		t.Fatal(err)
	}
	authority, err := store.nativeAuthority(ctx, c.Identity())
	if err != nil {
		store.Close()
		lock.Close()
		t.Fatal(err)
	}
	batch := TaskBatchInput{SchemaVersion: "task-batch/v1", ProjectID: c.Identity(), Tasks: []TaskBatchTask{{LocalKey: "native-r19", TaskID: 91024, Title: nativeAcceptanceTitle, Type: "task", Contract: contract}}, Dependencies: []TaskBatchEdge{}}
	batchBytes, err := json.Marshal(batch)
	if err != nil {
		store.Close()
		lock.Close()
		t.Fatal(err)
	}
	commands := CommandService{Store: store, Project: c.Identity()}
	if _, err = commands.CreateTaskBatch(ctx, c, batchBytes, newID()[:32], authority.AuthorityVersion, c.Claimant); err != nil {
		store.Close()
		lock.Close()
		t.Fatal(err)
	}
	authority, err = store.nativeAuthority(ctx, c.Identity())
	if err != nil {
		store.Close()
		lock.Close()
		t.Fatal(err)
	}
	auto, unpaused := "auto", false
	if _, err = commands.SetNativeDispatchState(ctx, NativeDispatchUpdate{RequestID: newID()[:32], ExpectedAuthorityVersion: authority.AuthorityVersion, Mode: &auto, Paused: &unpaused, RequestedBy: c.Claimant}); err != nil {
		store.Close()
		lock.Close()
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		lock.Close()
		t.Fatal(err)
	}
	lock.Close()
	service, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	serviceCtx, serviceCancel := context.WithCancel(context.Background())
	serviceDone := make(chan error, 1)
	go func() { serviceDone <- service.RunWithDispatch(serviceCtx, "auto") }()
	defer func() {
		serviceCancel()
		select {
		case <-serviceDone:
		case <-time.After(20 * time.Second):
		}
		_ = service.Close()
	}()
	reader, err := OpenNativeTaskReader(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	query := QueryService{Store: reader, Project: c.Identity()}
	taskID := "91024"
	deadline := time.Now().Add(45 * time.Minute)
	mergedURL := ""
	mergedHead := ""
	mergeRequested := map[string]bool{}
	for time.Now().Before(deadline) {
		if err = ctx.Err(); err != nil {
			t.Fatal(err)
		}
		select {
		case serviceErr := <-serviceDone:
			serviceDone = nil
			if serviceErr != nil {
				var runTarget, executionTarget ExecutionTarget
				var workflowStep string
				for _, active := range service.State.Runs {
					if active == nil || active.DomainTaskID != taskID {
						continue
					}
					runTarget = active.Target
					if execution := service.executionRecord(active); execution != nil {
						executionTarget, workflowStep = execution.Target, execution.CurrentStep
					}
					break
				}
				t.Fatalf("native supervised center exited before task delivery: %v (run_target=%+v execution_target=%+v current_step=%s)", serviceErr, runTarget, executionTarget, workflowStep)
			}
			t.Fatal("native supervised center exited before task delivery")
		default:
		}
		page, queryErr := query.ShowNativeTask(ctx, taskID)
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		snapshot, snapshotErr := reader.Query(ctx, c.Identity())
		if snapshotErr != nil {
			t.Fatal(snapshotErr)
		}
		prURL, candidateHead, foundPR := nativePublishedCandidate(snapshot, taskID)
		if foundPR {
			mergedURL, mergedHead = prURL, candidateHead
			if !mergeRequested[mergedURL] {
				view, viewErr := exec.Command("gh", "pr", "view", mergedURL, "--repo", c.PR.Repository, "--json", "state,headRefOid,baseRefName,isDraft").Output()
				if viewErr != nil {
					t.Fatal(viewErr)
				}
				var pr struct {
					State string `json:"state"`
					Head  string `json:"headRefOid"`
					Base  string `json:"baseRefName"`
					Draft bool   `json:"isDraft"`
				}
				if json.Unmarshal(view, &pr) != nil || pr.Head != mergedHead || pr.Base != nativeBaseBranch || pr.State != "OPEN" && pr.State != "MERGED" {
					t.Fatalf("native test PR identity differs from the fixed head/base: %s", view)
				}
				if pr.State == "OPEN" {
					if pr.Draft {
						if output, readyErr := exec.Command("gh", "pr", "ready", mergedURL, "--repo", c.PR.Repository).CombinedOutput(); readyErr != nil {
							t.Fatalf("mark the exact native acceptance PR ready: %v: %s", readyErr, output)
						}
					}
					if output, mergeErr := exec.Command("gh", "pr", "merge", mergedURL, "--repo", c.PR.Repository, "--merge", "--match-head-commit", mergedHead).CombinedOutput(); mergeErr != nil {
						view, viewErr = exec.Command("gh", "pr", "view", mergedURL, "--repo", c.PR.Repository, "--json", "state,headRefOid,baseRefName,isDraft").Output()
						if viewErr != nil {
							t.Fatalf("merge the exact native acceptance PR: %v: %s; inspect PR: %v", mergeErr, output, viewErr)
						}
						if json.Unmarshal(view, &pr) != nil || pr.State != "MERGED" || pr.Head != mergedHead || pr.Base != nativeBaseBranch {
							t.Fatalf("merge the exact native acceptance PR: %v: %s; current PR: %s", mergeErr, output, view)
						}
					}
				}
				mergeRequested[mergedURL] = true
			}
		}
		if len(page.Tasks) == 1 && page.Tasks[0].Status == "closed" && page.Tasks[0].Delivery != nil {
			if evidencePath := os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_EVIDENCE_PATH"); evidencePath != "" {
				executionID := nativeTaskExecutionID(snapshot, taskID)
				if executionID == "" {
					t.Fatal("completed native acceptance has no execution identity")
				}
				events, eventsErr := service.domainStore.Events(ctx, c.Identity(), executionID, "", 0, 1000)
				if eventsErr != nil {
					t.Fatalf("read native acceptance status evidence: %v", eventsErr)
				}
				remoteCodexVersion, versionErr := nativeObservedCodexVersion(events)
				if versionErr != nil {
					t.Fatalf("read observed remote Codex CLI version: %v", versionErr)
				}
				evidence := map[string]any{"captured_at": now(), "project_id": c.Identity(), "native_task_id": taskID, "task_status": page.Tasks[0].Status, "delivery": page.Tasks[0].Delivery, "test_pr_url": mergedURL, "candidate_head": mergedHead, "base_commit": base, "remote_codex_cli_version": remoteCodexVersion, "configured_codex_cli_version_pin": c.Orca.Supervised.ExpectedCodexVersion, "capability_report_confined_to_temp_state": c.RunDir}
				encoded, marshalErr := json.MarshalIndent(evidence, "", "  ")
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				if err = os.MkdirAll(filepath.Dir(evidencePath), 0700); err != nil {
					t.Fatal(err)
				}
				if err = AtomicWrite(evidencePath, append(encoded, '\n')); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("native task %s closed after supervised PR %s merged at %s", taskID, mergedURL, page.Tasks[0].Delivery.MergeCommit)
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal(errors.New("native task did not complete remote publish, merge and delivery before timeout"))
}

func nativeTaskExecutionID(snapshot DomainSnapshot, taskID string) string {
	workflowRunID := ""
	var latestWorkflow time.Time
	for _, workflow := range snapshot.WorkflowRuns {
		if workflow.TaskID == taskID && (workflowRunID == "" || workflow.UpdatedAt.After(latestWorkflow)) {
			workflowRunID, latestWorkflow = workflow.WorkflowRunID, workflow.UpdatedAt
		}
	}
	for _, execution := range snapshot.Executions {
		if execution.WorkflowRunID == workflowRunID {
			return execution.ExecutionID
		}
	}
	return ""
}

func nativeObservedCodexVersion(events []DomainEvent) (string, error) {
	observed := ""
	for _, event := range events {
		if event.Type != "orca.status" || len(event.Payload) == 0 {
			continue
		}
		var message OrcaInboxMessage
		if err := json.Unmarshal(event.Payload, &message); err != nil {
			return "", fmt.Errorf("decode persisted Orca status message: %w", err)
		}
		if message.FromHandle == "" || !strings.HasPrefix(message.FromHandle, "dispatch:") {
			continue
		}
		var status struct {
			TaskID             string `json:"taskId"`
			DispatchID         string `json:"dispatchId"`
			RemoteCodexVersion string `json:"remote_codex_version"`
		}
		if err := json.Unmarshal(message.Payload, &status); err != nil {
			return "", fmt.Errorf("decode persisted remote Codex version: %w", err)
		}
		version := strings.TrimSpace(status.RemoteCodexVersion)
		if version == "" {
			continue
		}
		if event.AttemptID == "" || status.TaskID == "" || status.DispatchID == "" || message.FromHandle != "dispatch:"+status.DispatchID {
			return "", errors.New("remote Codex version evidence is not bound to its exact Dispatch and Attempt")
		}
		if observed != "" && observed != version {
			return "", fmt.Errorf("native task reported inconsistent Codex CLI versions %q and %q", observed, version)
		}
		observed = version
	}
	if observed == "" {
		return "", errors.New("native task status messages contain no remote_codex_version")
	}
	return observed, nil
}

func TestNativeObservedCodexVersionReadsPersistedStatus(t *testing.T) {
	events := []DomainEvent{{AttemptID: "attempt-test", Type: "orca.status", Payload: mustJSON(OrcaInboxMessage{FromHandle: "dispatch:ctx-test", Payload: mustJSON(map[string]any{"taskId": "task-test", "dispatchId": "ctx-test", "remote_codex_version": "codex-cli 0.157.1"})})}}
	if got, err := nativeObservedCodexVersion(events); err != nil || got != "codex-cli 0.157.1" {
		t.Fatalf("observed Codex CLI version = %q, %v", got, err)
	}
	events = append(events, DomainEvent{AttemptID: "attempt-next", Type: "orca.status", Payload: mustJSON(OrcaInboxMessage{FromHandle: "dispatch:ctx-next", Payload: mustJSON(map[string]any{"taskId": "task-test", "dispatchId": "ctx-next", "remote_codex_version": "codex-cli 0.157.2"})})})
	if _, err := nativeObservedCodexVersion(events); err == nil || !strings.Contains(err.Error(), "inconsistent") {
		t.Fatalf("inconsistent worker versions were accepted: %v", err)
	}
	if _, err := nativeObservedCodexVersion(nil); err == nil {
		t.Fatal("missing remote Codex CLI version evidence was accepted")
	}
}

func nativePublishedCandidate(snapshot DomainSnapshot, taskID string) (string, string, bool) {
	workflowRunID := ""
	var latestWorkflow time.Time
	for _, workflow := range snapshot.WorkflowRuns {
		if workflow.TaskID == taskID && (workflowRunID == "" || workflow.UpdatedAt.After(latestWorkflow)) {
			workflowRunID, latestWorkflow = workflow.WorkflowRunID, workflow.UpdatedAt
		}
	}
	if workflowRunID == "" {
		return "", "", false
	}
	var candidate string
	var executionID string
	for _, execution := range snapshot.Executions {
		if execution.WorkflowRunID == workflowRunID {
			candidate, executionID = execution.CandidateRevision, execution.ExecutionID
			break
		}
	}
	if candidate == "" || executionID == "" {
		return "", "", false
	}
	var latestAttempt *StepAttemptRecord
	for i := range snapshot.Attempts {
		attempt := &snapshot.Attempts[i]
		if attempt.ExecutionID == executionID && attempt.StepID == "publish_pr" && attempt.Status == "succeeded" && (latestAttempt == nil || attempt.UpdatedAt.After(latestAttempt.UpdatedAt)) {
			latestAttempt = attempt
		}
	}
	if latestAttempt == nil || len(latestAttempt.Result) == 0 {
		return "", "", false
	}
	var result StepGateResult
	if json.Unmarshal(latestAttempt.Result, &result) != nil || result.PRURL == "" || result.PublishedHead != candidate {
		return "", "", false
	}
	return result.PRURL, candidate, true
}
