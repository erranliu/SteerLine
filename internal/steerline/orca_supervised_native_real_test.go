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
	dagMode := os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_DAG") == "1"
	resumeMode := os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_RESUME") == "1"
	root := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_ROOT"))
	if root == "" {
		var err error
		root, err = os.MkdirTemp("", "s3-")
		if err != nil {
			t.Fatalf("create short isolated native acceptance root: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(root) })
	} else if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatalf("create persistent native acceptance root: %v", err)
	}
	repositoryURL := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_REPOSITORY_URL"))
	if repositoryURL == "" {
		repositoryURL = "https://github.com/erranliu/SteerlineTest.git"
	}
	nativeBaseBranch := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_BASE_BRANCH"))
	if nativeBaseBranch == "" {
		nativeBaseBranch = "codex/spec03-native-acceptance-base"
		if dagMode {
			nativeBaseBranch = "main"
		}
	}
	testGitHubRepository := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_GITHUB_REPOSITORY"))
	if testGitHubRepository == "" {
		testGitHubRepository = "erranliu/SteerlineTest"
	}
	repo := filepath.Join(root, "native-project")
	if !resumeMode {
		if output, err := exec.Command("git", "clone", "--quiet", "--branch", nativeBaseBranch, repositoryURL, repo).CombinedOutput(); err != nil {
			t.Fatalf("clone isolated test repository: %v: %s", err, output)
		}
	} else if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		t.Fatalf("resume requires the original isolated repository: %v", err)
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
		ExecutableTypes: []string{"task"},
		TaskPoll:        "250ms", PRPoll: "1s", Heartbeat: "1s", CommandTimeout: "30s", ShutdownGrace: "5s",
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
	if dagMode {
		c.ProjectSteps.Verify = step("command", "tools/verify-native-evidence.py", []string{"python.exe", "tools/verify-native-evidence.py"})
		c.ProjectSteps.Verify.RequiredChecks = []string{"identity", "evidence", "changed_paths"}
		c.ProjectSteps.PublishPR = step("agent_skill", "docs/steps/publish-pr.md", nil)
	}
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
	if !resumeMode {
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
		if currentTargetEvidence := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_CAPABILITY_SAMPLES")); currentTargetEvidence != "" {
			acceptanceRoot = currentTargetEvidence
			if err := verifySupervisedReadOnlyCapabilitySample(acceptanceRoot, c.Orca.Supervised.EnvironmentID, c.Orca.Supervised.RepositorySelector); err != nil {
				t.Fatalf("current supervised target bootstrap evidence: %v", err)
			}
			evidenceSources = map[string][]string{
				"start":                {"post-sac/start.json"},
				"observe":              {"post-sac/final-worker.json", "post-sac/final-fleet.json"},
				"workspace_inspection": {"remote-setup.json", "post-sac/output.json"},
				"output_reading":       {"post-sac/output.json"},
				"message_inbox":        {"post-sac/status-delivery.json", "post-sac/done-delivery.json", "post-sac/final-task.json"},
				"resource_settlement":  {"post-sac/release.json", "post-sac/final-terminals.json"},
			}
		}
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
	}
	dispatchMode := "auto"
	if dagMode {
		dispatchMode = "manual"
	}
	if !resumeMode {
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
		contractFor := func(id string) TaskContract {
			t.Helper()
			name := "native-acceptance"
			if dagMode {
				name = "native-" + id
			}
			specRef := nativeRef(t, c, base, "docs/spec/"+name+".spec.md")
			planRef := nativeRef(t, c, base, "docs/spec/"+name+".plan.md")
			contract := TaskContract{ContractID: "native-supervised-" + id + "-" + newID()[:8], Repository: target, Remote: c.Remote, Branch: c.Branch, SpecRef: specRef, PlanRef: planRef, AcceptanceRefs: []FileReference{specRef}}
			var digestErr error
			contract.Digest, digestErr = TaskContractDigest(contract)
			if digestErr != nil {
				t.Fatal(digestErr)
			}
			return contract
		}
		authority, err := store.nativeAuthority(ctx, c.Identity())
		if err != nil {
			store.Close()
			lock.Close()
			t.Fatal(err)
		}
		batch := TaskBatchInput{SchemaVersion: "task-batch/v1", ProjectID: c.Identity(), Dependencies: []TaskBatchEdge{}}
		if dagMode {
			batch.Tasks = []TaskBatchTask{{LocalKey: "native-103", TaskID: 103, Title: "Native 分支 B 的依赖释放", Type: "task", Contract: contractFor("103")}, {LocalKey: "native-104", TaskID: 104, Title: "Native 双前置汇合与交付", Type: "task", Contract: contractFor("104")}}
			batch.Dependencies = []TaskBatchEdge{{Predecessor: TaskBatchReference{LocalKey: "native-103"}, Successor: TaskBatchReference{LocalKey: "native-104"}}}
		} else {
			batch.Tasks = []TaskBatchTask{{LocalKey: "native-r19", TaskID: 91024, Title: nativeAcceptanceTitle, Type: "task", Contract: contractFor("91024")}}
		}
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
		unpaused := false
		if _, err = commands.SetNativeDispatchState(ctx, NativeDispatchUpdate{RequestID: newID()[:32], ExpectedAuthorityVersion: authority.AuthorityVersion, Mode: &dispatchMode, Paused: &unpaused, RequestedBy: c.Claimant}); err != nil {
			store.Close()
			lock.Close()
			t.Fatal(err)
		}
		if err = store.Close(); err != nil {
			lock.Close()
			t.Fatal(err)
		}
		lock.Close()
	}
	setupOnly := os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_SETUP_ONLY") == "1"
	configPath := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_CONFIG_PATH"))
	if setupOnly && configPath == "" {
		configPath = filepath.Join(root, "acceptance-config.json")
	}
	if configPath != "" {
		if err = AtomicWrite(configPath, configBytes); err != nil {
			t.Fatal(err)
		}
		t.Logf("isolated native acceptance config ready: %s", configPath)
	}
	if setupOnly {
		return
	}
	service, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	serviceCtx, serviceCancel := context.WithCancel(context.Background())
	serviceDone := make(chan error, 1)
	go func() { serviceDone <- service.RunWithDispatch(serviceCtx, dispatchMode) }()
	serviceStopped := false
	defer func() {
		if serviceStopped {
			return
		}
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
	if dagMode {
		taskID = "103"
		if !resumeMode {
			successor, successorErr := query.ShowNativeTask(ctx, "104")
			if successorErr != nil || len(successor.Tasks) != 1 || successor.Tasks[0].DependencyReady {
				t.Fatalf("native DAG successor was ready before its predecessor merged: %+v %v", successor, successorErr)
			}
			request, submitErr := SubmitControl(c, "start_task", map[string]string{"task_id": taskID}, c.Claimant)
			if submitErr != nil {
				t.Fatal(submitErr)
			}
			receiptCtx, receiptCancel := context.WithTimeout(ctx, 5*time.Minute)
			defer receiptCancel()
			receipt, receiptErr := WaitControlReceipt(receiptCtx, c, request.RequestID, 100*time.Millisecond)
			if receiptErr != nil || receipt.Status != "applied" {
				t.Fatalf("native DAG predecessor was not started: %+v %v", receipt, receiptErr)
			}
		}
	}
	deadline := time.Now().Add(45 * time.Minute)
	mergedURL := ""
	mergedHead := ""
	mergeRequested := map[string]bool{}
	answerCommands := map[string]string{}
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
		if os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_AUTO_ANSWER") == "1" {
			executionID := nativeTaskExecutionID(snapshot, taskID)
			for _, decision := range snapshot.Decisions {
				if decision.ExecutionID != executionID || decision.Status != "pending" || answerCommands[decision.DecisionID] != "" {
					continue
				}
				command, _, answerErr := SubmitAnswerDecision(ctx, c, decision.WorkflowRunID, decision.DecisionID, "yes", "", c.Claimant, 0, 0, "", "")
				if answerErr != nil {
					t.Fatalf("submit original native business answer %s: %v", decision.DecisionID, answerErr)
				}
				answerCommands[decision.DecisionID] = command.CommandID
				t.Logf("native business question %s answered through CommandService command %s", decision.DecisionID, command.CommandID)
			}
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
			var successorStatus string
			if dagMode {
				successor, successorErr := query.ShowNativeTask(ctx, "104")
				if successorErr != nil || len(successor.Tasks) != 1 || !successor.Tasks[0].DependencyReady {
					t.Fatalf("merged native predecessor did not unlock DAG successor: %+v %v", successor, successorErr)
				}
				successorStatus = successor.Tasks[0].Status
			}
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
				capabilityEvidence := ""
				if capabilityPath := os.Getenv("STEERLINE_REAL_SUPERVISED_NATIVE_CAPABILITY_EVIDENCE_PATH"); capabilityPath != "" {
					serviceCancel()
					select {
					case <-serviceDone:
					case <-time.After(20 * time.Second):
						t.Fatal("native center did not stop before capability certification")
					}
					if err = service.Close(); err != nil {
						t.Fatal(err)
					}
					serviceStopped = true
					capabilityEvidence, err = certifyCurrentNativeSupervisedRun(ctx, c, snapshot, executionID, events, capabilityPath)
					if err != nil {
						t.Fatalf("certify current supervised target: %v", err)
					}
				}
				evidence := map[string]any{"captured_at": now(), "project_id": c.Identity(), "native_task_id": taskID, "task_status": page.Tasks[0].Status, "delivery": page.Tasks[0].Delivery, "test_pr_url": mergedURL, "candidate_head": mergedHead, "base_commit": base, "remote_codex_cli_version": remoteCodexVersion, "configured_codex_cli_version_pin": c.Orca.Supervised.ExpectedCodexVersion, "capability_report_confined_to_temp_state": c.RunDir}
				if len(answerCommands) > 0 {
					evidence["business_answer_commands"] = answerCommands
				}
				if capabilityEvidence != "" {
					evidence["current_target_capability_evidence"] = capabilityEvidence
				}
				if dagMode {
					evidence["dag_successor_task_id"] = "104"
					evidence["dag_successor_dependency_ready"] = true
					evidence["dag_successor_status"] = successorStatus
				}
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

// verifySupervisedReadOnlyCapabilitySample binds the opt-in bootstrap receipt
// set to one current remote host, Task and Dispatch before it can certify the
// native canary's isolated state. The previous host's receipts are not reused.
func verifySupervisedReadOnlyCapabilitySample(root, environmentID, repositorySelector string) error {
	read := func(name string, target any) error {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		var envelope struct {
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			return err
		}
		if !envelope.OK {
			return fmt.Errorf("%s is not a successful Orca receipt", name)
		}
		return json.Unmarshal(envelope.Result, target)
	}
	var startReceipt struct {
		State  string `json:"state"`
		Stage  string `json:"stage"`
		RunID  string `json:"runId"`
		TaskID string `json:"taskId"`
		ID     string `json:"dispatchId"`
		Server struct {
			EnvironmentID string `json:"environmentId"`
		} `json:"server"`
	}
	var startEnvelope struct {
		State   string          `json:"state"`
		Method  string          `json:"method"`
		Receipt json.RawMessage `json:"receipt"`
	}
	if err := read("post-sac/start.json", &startEnvelope); err != nil {
		return err
	}
	if err := json.Unmarshal(startEnvelope.Receipt, &startReceipt); err != nil {
		return err
	}
	if startEnvelope.State != "completed" || startEnvelope.Method != "orchestration.workerStart" || startReceipt.State != "ready" || startReceipt.Stage != "input_accepted" || startReceipt.Server.EnvironmentID != environmentID || startReceipt.RunID == "" || startReceipt.TaskID == "" || startReceipt.ID == "" {
		return errors.New("bootstrap start receipt lacks the fixed remote Run, Task or Dispatch")
	}
	var worker struct {
		Dispatch struct {
			ID     string `json:"id"`
			RunID  string `json:"runId"`
			TaskID string `json:"taskId"`
			Status string `json:"status"`
		} `json:"dispatch"`
		Worker struct {
			State      string `json:"state"`
			Stage      string `json:"stage"`
			WorktreeID string `json:"worktreeId"`
		} `json:"worker"`
		Projection struct {
			Liveness struct {
				Verdict string `json:"verdict"`
			} `json:"liveness"`
		} `json:"projection"`
	}
	if err := read("post-sac/final-worker.json", &worker); err != nil {
		return err
	}
	if worker.Dispatch.ID != startReceipt.ID || worker.Dispatch.RunID != startReceipt.RunID || worker.Dispatch.TaskID != startReceipt.TaskID || worker.Dispatch.Status != "completed" || worker.Worker.State != "succeeded" || worker.Worker.Stage != "released" || worker.Projection.Liveness.Verdict != "exited" || !strings.HasPrefix(worker.Worker.WorktreeID, strings.TrimPrefix(repositorySelector, "id:")+"::") {
		return errors.New("bootstrap final worker does not match the successful original Dispatch")
	}
	var setup struct {
		Setups []struct {
			RepoID string `json:"repoId"`
		} `json:"setups"`
	}
	if err := read("remote-setup.json", &setup); err != nil {
		return err
	}
	if len(setup.Setups) != 1 || setup.Setups[0].RepoID != strings.TrimPrefix(repositorySelector, "id:") {
		return errors.New("bootstrap remote project setup differs from the configured repository")
	}
	checkMessage := func(name, kind string) (string, error) {
		var delivery struct {
			State   string `json:"state"`
			Method  string `json:"method"`
			Receipt struct {
				RunID      string `json:"runId"`
				DeliveryID string `json:"deliveryId"`
				Messages   []struct {
					Type       string `json:"type"`
					FromHandle string `json:"from_handle"`
					Payload    string `json:"payload"`
				} `json:"messages"`
			} `json:"receipt"`
		}
		if err := read(name, &delivery); err != nil {
			return "", err
		}
		if delivery.State != "completed" || delivery.Receipt.RunID != startReceipt.RunID || delivery.Receipt.DeliveryID == "" || len(delivery.Receipt.Messages) != 1 || delivery.Receipt.Messages[0].Type != kind || delivery.Receipt.Messages[0].FromHandle != "dispatch:"+startReceipt.ID {
			return "", fmt.Errorf("%s does not contain one exact %s Delivery", name, kind)
		}
		var payload struct {
			TaskID     string `json:"taskId"`
			DispatchID string `json:"dispatchId"`
			Outcome    string `json:"outcome"`
		}
		if err := json.Unmarshal([]byte(delivery.Receipt.Messages[0].Payload), &payload); err != nil {
			return "", err
		}
		if payload.TaskID != startReceipt.TaskID || payload.DispatchID != startReceipt.ID || kind == "worker_done" && payload.Outcome != "succeeded" {
			return "", fmt.Errorf("%s payload does not bind the original Task and Dispatch", name)
		}
		return delivery.Receipt.DeliveryID, nil
	}
	statusDeliveryID, err := checkMessage("post-sac/status-delivery.json", "status")
	if err != nil {
		return err
	}
	doneDeliveryID, err := checkMessage("post-sac/done-delivery.json", "worker_done")
	if err != nil {
		return err
	}
	for name, deliveryID := range map[string]string{"post-sac/status-ack.json": statusDeliveryID, "post-sac/done-ack.json": doneDeliveryID} {
		var ack struct {
			State   string `json:"state"`
			Receipt struct {
				RunID        string `json:"runId"`
				Acknowledged string `json:"acknowledged"`
			} `json:"receipt"`
		}
		if err := read(name, &ack); err != nil {
			return err
		}
		if ack.State != "completed" || ack.Receipt.RunID != startReceipt.RunID || ack.Receipt.Acknowledged != deliveryID {
			return fmt.Errorf("%s does not confirm the original Run Delivery", name)
		}
	}
	var task struct {
		RunID string `json:"runId"`
		Tasks []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"tasks"`
	}
	if err := read("post-sac/final-task.json", &task); err != nil {
		return err
	}
	if task.RunID != startReceipt.RunID || len(task.Tasks) != 1 || task.Tasks[0].ID != startReceipt.TaskID || task.Tasks[0].Status != "completed" {
		return errors.New("bootstrap Task did not complete under the original Run")
	}
	var release struct {
		State   string `json:"state"`
		Method  string `json:"method"`
		Receipt struct {
			DispatchID string `json:"dispatchId"`
			State      string `json:"state"`
			Archive    struct {
				Status string `json:"status"`
			} `json:"archive"`
		} `json:"receipt"`
	}
	if err := read("post-sac/release.json", &release); err != nil {
		return err
	}
	if release.State != "completed" || release.Receipt.DispatchID != startReceipt.ID || release.Receipt.State != "released" || release.Receipt.Archive.Status != "captured" {
		return errors.New("bootstrap original Dispatch was not released with archived output")
	}
	var output struct {
		DispatchID string `json:"dispatchId"`
		Source     string `json:"source"`
		Exact      bool   `json:"sourceExact"`
		Archived   bool   `json:"archived"`
		Transcript struct {
			ReturnedMessageCount int `json:"returnedMessageCount"`
		} `json:"transcript"`
	}
	if err := read("post-sac/output.json", &output); err != nil {
		return err
	}
	if output.DispatchID != startReceipt.ID || output.Source != "transcript" || !output.Exact || !output.Archived || output.Transcript.ReturnedMessageCount == 0 {
		return errors.New("bootstrap output lacks exact archived transcript evidence")
	}
	var terminals struct {
		Total int `json:"totalCount"`
	}
	if err := read("post-sac/final-terminals.json", &terminals); err != nil {
		return err
	}
	if terminals.Total != 0 {
		return errors.New("bootstrap worktree still has a live terminal")
	}
	return nil
}

func TestDesktop64gSupervisedCapabilityReceiptsBindOneDispatch(t *testing.T) {
	root := filepath.Join("..", "..", "docs", "spec", "orca-probe", "samples", "spec03-desktop64g-2026-09-28")
	if err := verifySupervisedReadOnlyCapabilitySample(root, "e68f4edf-8a83-42c2-a9da-6d404d3aa3d8", "id:30162573-cc04-416e-bef4-226c418dde1f"); err != nil {
		t.Fatal(err)
	}
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
			DecisionAppliedID  string `json:"decision_applied"`
			StepResult         struct {
				AttemptID string `json:"attempt_id"`
				Result    struct {
					StepID string `json:"step_id"`
				} `json:"result"`
			} `json:"step_result"`
		}
		payload := message.Payload
		if len(payload) > 0 && payload[0] == '"' {
			var encoded string
			if err := json.Unmarshal(payload, &encoded); err != nil {
				return "", fmt.Errorf("decode persisted remote status string: %w", err)
			}
			payload = []byte(encoded)
		}
		if err := json.Unmarshal(payload, &status); err != nil {
			return "", fmt.Errorf("decode persisted remote Codex version: %w", err)
		}
		version := strings.TrimSpace(status.RemoteCodexVersion)
		if version == "" {
			continue
		}
		if status.DecisionAppliedID != "" && status.StepResult.Result.StepID == "" {
			// The separately accepted answer-application proof carries a Codex
			// version but no StepResult. The five step receipts below provide the
			// fixed Attempt-bound version evidence for this report.
			continue
		}
		if status.TaskID == "" || status.DispatchID == "" || message.FromHandle != "dispatch:"+status.DispatchID || !stableExecutionID(status.StepResult.AttemptID) || event.AttemptID != "" && event.AttemptID != status.StepResult.AttemptID || event.AttemptID == "" && status.StepResult.Result.StepID != "prepare" {
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

func certifyCurrentNativeSupervisedRun(ctx context.Context, c Config, snapshot DomainSnapshot, executionID string, events []DomainEvent, evidencePath string) (string, error) {
	probe, err := (orcaSupervisedBackend{config: c}).Probe(ctx, targetFor(c), agentFor(c))
	if err != nil {
		return "", err
	}
	client := orcaClient{cfg: c}
	readResult := func(args ...string) (json.RawMessage, error) {
		out, callErr := client.call(ctx, 30*time.Second, args...)
		envelope, parseErr := parseEnvelope(out)
		if callErr != nil || parseErr != nil {
			return nil, errors.Join(callErr, parseErr)
		}
		return envelope.Result, nil
	}
	messageCounts := map[string]map[string]int{}
	for _, event := range events {
		if event.Type != "orca.status" && event.Type != "orca.worker_done" {
			continue
		}
		var message OrcaInboxMessage
		if err := json.Unmarshal(event.Payload, &message); err != nil {
			return "", err
		}
		dispatch := strings.TrimPrefix(message.FromHandle, "dispatch:")
		if dispatch == message.FromHandle || dispatch == "" {
			return "", errors.New("capability evidence lacks the exact Dispatch sender")
		}
		if messageCounts[dispatch] == nil {
			messageCounts[dispatch] = map[string]int{}
		}
		messageCounts[dispatch][event.Type]++
	}
	requiredSteps := map[string]bool{"prepare": true, "implement": true, "verify": true, "review": true, "publish_pr": true}
	observedSteps := map[string]bool{}
	var dispatches []map[string]any
	for _, attempt := range snapshot.Attempts {
		if attempt.ExecutionID != executionID {
			continue
		}
		if !requiredSteps[attempt.StepID] || observedSteps[attempt.StepID] || attempt.Status != "succeeded" || attempt.ExternalBinding.Dispatch == "" {
			return "", fmt.Errorf("current native run has an invalid %s attempt for capability evidence", attempt.StepID)
		}
		observedSteps[attempt.StepID] = true
		dispatch := attempt.ExternalBinding.Dispatch
		showBytes, err := readResult("orchestration", "worker-show", "--dispatch", dispatch, "--json")
		if err != nil {
			return "", err
		}
		var show struct {
			Dispatch struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"dispatch"`
			Worker struct {
				State string `json:"state"`
				Stage string `json:"stage"`
			} `json:"worker"`
			Projection struct {
				Liveness struct {
					Verdict string `json:"verdict"`
				} `json:"liveness"`
			} `json:"projection"`
		}
		if err := json.Unmarshal(showBytes, &show); err != nil {
			return "", err
		}
		if show.Dispatch.ID != dispatch || show.Dispatch.Status != "completed" || show.Worker.State != "succeeded" || show.Worker.Stage != "released" || show.Projection.Liveness.Verdict != "exited" {
			return "", fmt.Errorf("Dispatch %s lacks exact completed and released worker evidence", dispatch)
		}
		readBytes, err := readResult("orchestration", "worker-read", "--dispatch", dispatch, "--source", "transcript", "--limit", "1", "--json")
		if err != nil {
			return "", err
		}
		var read struct {
			Source      string `json:"source"`
			SourceExact bool   `json:"sourceExact"`
			Transcript  struct {
				ReturnedMessageCount int `json:"returnedMessageCount"`
			} `json:"transcript"`
		}
		if err := json.Unmarshal(readBytes, &read); err != nil {
			return "", err
		}
		if read.Source != "transcript" || !read.SourceExact || read.Transcript.ReturnedMessageCount == 0 || messageCounts[dispatch]["orca.status"] == 0 || messageCounts[dispatch]["orca.worker_done"] == 0 {
			return "", fmt.Errorf("Dispatch %s lacks archived output or center inbox evidence", dispatch)
		}
		dispatches = append(dispatches, map[string]any{"step": attempt.StepID, "attempt_id": attempt.AttemptID, "dispatch_id": dispatch, "worker_state": show.Worker.State, "dispatch_status": show.Dispatch.Status, "resource_stage": show.Worker.Stage, "liveness": show.Projection.Liveness.Verdict, "output_source": read.Source, "output_exact": read.SourceExact, "status_messages": messageCounts[dispatch]["orca.status"], "worker_done_messages": messageCounts[dispatch]["orca.worker_done"]})
	}
	if len(observedSteps) != len(requiredSteps) {
		return "", fmt.Errorf("capability evidence covers %d of %d required native steps", len(observedSteps), len(requiredSteps))
	}
	manifest := map[string]any{"kind": "current-supervised-native-target-capability", "captured_at": now(), "project_identity": c.Identity(), "execution_id": executionID, "target": probe.Target, "center_orca_version": probe.CenterVersion, "remote_orca_version": probe.PlatformVersion, "dispatches": dispatches, "verified_capabilities": []string{"start", "observe", "workspace_inspection", "output_reading", "message_inbox", "resource_settlement"}, "remote_cancellation": "unsupported"}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := os.MkdirAll(filepath.Dir(evidencePath), 0700); err != nil {
		return "", err
	}
	if err := AtomicWrite(evidencePath, manifestBytes); err != nil {
		return "", err
	}
	sum := sha256.Sum256(manifestBytes)
	copyPath := filepath.Join(c.RunDir, "execution", "v1", "evidence", hex.EncodeToString(sum[:])+".json")
	if err := os.MkdirAll(filepath.Dir(copyPath), 0700); err != nil {
		return "", err
	}
	if err := AtomicWrite(copyPath, manifestBytes); err != nil {
		return "", err
	}
	evidenceRef := copyPath + "#sha256=" + hex.EncodeToString(sum[:])
	for i := range probe.Capabilities {
		switch probe.Capabilities[i].Name {
		case "start", "observe", "workspace_inspection", "output_reading", "message_inbox", "resource_settlement":
			probe.Capabilities[i].Status = CapabilityVerified
			probe.Capabilities[i].Evidence = evidenceRef
		}
	}
	repoLock, err := LockRepository(c.Identity())
	if err != nil {
		return "", err
	}
	defer repoLock.Close()
	runLock, err := LockRepository(c.RunDir)
	if err != nil {
		return "", err
	}
	defer runLock.Close()
	records := executionRecords{root: c.RunDir}
	if err := records.saveCapabilityReport(probe); err != nil {
		return "", err
	}
	if _, err := records.loadCapabilityReport(probe); err != nil {
		return "", err
	}
	return evidenceRef, nil
}

func TestNativeObservedCodexVersionReadsPersistedStatus(t *testing.T) {
	firstAttempt, nextAttempt := strings.Repeat("a", 32), strings.Repeat("b", 32)
	firstStatus := map[string]any{"taskId": "task-test", "dispatchId": "ctx-test", "remote_codex_version": "codex-cli 0.157.1", "step_result": map[string]any{"attempt_id": firstAttempt, "result": map[string]any{"step_id": "implement"}}}
	events := []DomainEvent{{AttemptID: firstAttempt, Type: "orca.status", Payload: mustJSON(OrcaInboxMessage{FromHandle: "dispatch:ctx-test", Payload: mustJSON(firstStatus)})}}
	if got, err := nativeObservedCodexVersion(events); err != nil || got != "codex-cli 0.157.1" {
		t.Fatalf("observed Codex CLI version = %q, %v", got, err)
	}
	encodedStatus := string(mustJSON(firstStatus))
	events[0].Payload = mustJSON(OrcaInboxMessage{FromHandle: "dispatch:ctx-test", Payload: mustJSON(encodedStatus)})
	if got, err := nativeObservedCodexVersion(events); err != nil || got != "codex-cli 0.157.1" {
		t.Fatalf("string-encoded Orca status version = %q, %v", got, err)
	}
	prepareStatus := map[string]any{"taskId": "task-test", "dispatchId": "ctx-test", "remote_codex_version": "codex-cli 0.157.1", "step_result": map[string]any{"attempt_id": firstAttempt, "result": map[string]any{"step_id": "prepare"}}}
	events[0].AttemptID = ""
	events[0].Payload = mustJSON(OrcaInboxMessage{FromHandle: "dispatch:ctx-test", Payload: mustJSON(string(mustJSON(prepareStatus)))})
	if got, err := nativeObservedCodexVersion(events); err != nil || got != "codex-cli 0.157.1" {
		t.Fatalf("prepare status without projected attempt = %q, %v", got, err)
	}
	proofStatus := map[string]any{"taskId": "task-test", "dispatchId": "ctx-next", "remote_codex_version": "codex-cli 0.157.1", "decision_applied": "question-test", "applied_decision_attempt_id": nextAttempt, "applied_decision_execution_id": "execution-test", "applied_answer_body": "yes"}
	events = append(events, DomainEvent{AttemptID: nextAttempt, Type: "orca.status", Payload: mustJSON(OrcaInboxMessage{FromHandle: "dispatch:ctx-next", Payload: mustJSON(proofStatus)})})
	if got, err := nativeObservedCodexVersion(events); err != nil || got != "codex-cli 0.157.1" {
		t.Fatalf("separate decision proof was mistaken for a step result: %q, %v", got, err)
	}
	events = append(events, DomainEvent{AttemptID: nextAttempt, Type: "orca.status", Payload: mustJSON(OrcaInboxMessage{FromHandle: "dispatch:ctx-next", Payload: mustJSON(map[string]any{"taskId": "task-test", "dispatchId": "ctx-next", "remote_codex_version": "codex-cli 0.157.2", "step_result": map[string]any{"attempt_id": nextAttempt, "result": map[string]any{"step_id": "verify"}}})})})
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
