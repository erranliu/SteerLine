package steerline

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfigLoadsLocalAndRemoteOrcaProfilesWithoutProbingTargets(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("local Orca terminal target uses Windows cmd.exe")
	}
	base, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_HOME", t.TempDir())
	localOrca := ExecutionProfile{ID: "orca-local", Name: "Orca 本机", Host: "orca", Agent: "codex", Sandbox: "workspace-write", Codex: Command{Executable: "codex"}, Orca: OrcaConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "15m"}}
	remoteOrca := ExecutionProfile{ID: "orca-remote", Name: "Orca 远程", Host: "orca_supervised", Agent: "codex", Model: "chosen-model", ReasoningEffort: "xhigh", Sandbox: "workspace-write", Codex: Command{Executable: "codex"}, Orca: OrcaConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "15m", Supervised: &SupervisedOrcaConfig{ConnectionID: "connection", EnvironmentID: "environment", RepositorySelector: "repo", CoordinatorTerminal: "terminal", PollInterval: "5s", CapabilityReportVersion: 1}}}
	local := ExecutionProfile{ID: "local", Name: "本地", Host: "local", Agent: "codex", Sandbox: "workspace-write", Codex: Command{Executable: "codex"}}
	writeGlobalExecutionFixture(t, []ExecutionProfile{local, localOrca, remoteOrca}, "local")
	input := map[string]any{"repository": base.Repository, "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion}
	path := filepath.Join(t.TempDir(), "config.json")
	write := func() {
		t.Helper()
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	loaded, err := LoadConfig(path)
	if err != nil || len(loaded.AvailableExecutionProfiles()) != 3 || loaded.DefaultProfileID() != "local" {
		t.Fatalf("configured profiles were not loaded: %+v / %v", loaded.ExecutionProfiles, err)
	}
	for _, profile := range loaded.AvailableExecutionProfiles() {
		selected, chosen, err := loaded.ResolveExecutionProfile(profile.ID, profile.Digest())
		if err != nil || selected.Host != profile.Host {
			t.Fatalf("Orca profile changed route: %+v / %v", chosen, err)
		}
		if (profile.Host == "orca_supervised") != (profile.StaticUnavailableReason() != "") {
			t.Fatalf("Orca profile availability is wrong: %+v", profile)
		}
	}
}

func TestNativeClaimPersistsChosenExecutionProfile(t *testing.T) {
	ctx := context.Background()
	base, commit := nativeTaskFixture(t)
	if out, err := exec.Command("git", "-C", base.Repository, "push", "origin", "main").CombinedOutput(); err != nil {
		t.Fatalf("push fixture base: %v: %s", err, out)
	}
	profile := ExecutionProfile{ID: "local-strong", Name: "本地高强度", Host: "local", Agent: "codex", Model: "chosen-model", ReasoningEffort: "xhigh", Sandbox: "danger-full-access", WindowsSandbox: "elevated", Codex: Command{Executable: "git"}}
	base.ExecutionProfiles = []ExecutionProfile{profile}
	reader, err := OpenNativeTaskProject(ctx, &base)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteStore(filepath.Join(base.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	commands := CommandService{Store: store, Project: base.Identity()}
	if _, err = commands.CreateTaskBatch(ctx, base, taskBatchFixture(t, base, commit, batchTasks(501), nil), strings.Repeat("1", 32), 1, "planner"); err != nil {
		t.Fatal(err)
	}
	authority, err := store.nativeAuthority(ctx, base.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err = store.Transact(ctx, func(tx DomainTx) error {
		return tx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	selected, fixedChoice, err := base.ResolveExecutionProfile(profile.ID, profile.Digest())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = commands.ClaimNativeTask(ctx, selected, TaskClaimOptions{TaskID: "501", RequestID: strings.Repeat("2", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 2, RequestedBy: "operator", Explicit: true, ExecutionProfile: &fixedChoice}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Query(ctx, base.Identity())
	if err != nil || len(snapshot.WorkflowRuns) != 1 || len(snapshot.Executions) != 1 {
		t.Fatalf("selected claim did not persist a workflow: %+v / %v", snapshot, err)
	}
	wf, execution := snapshot.WorkflowRuns[0], snapshot.Executions[0]
	if wf.WorkflowVersion != WorkerFlowVersion || wf.ExecutionProfile == nil || wf.ExecutionProfile.ID != profile.ID || !wf.ExecutionProfile.VerifyWorkerFlow() || wf.Runner.Model != "chosen-model" || wf.Runner.ReasoningEffort != "xhigh" || wf.Runner.WindowsSandbox != "elevated" || wf.ExecutionProfile.WindowsSandbox != "elevated" || execution.Backend != string(BackendLocal) || execution.CurrentStep != "worker" {
		t.Fatalf("chosen execution route was not fixed in the claim: %+v / %+v", wf, execution)
	}
	base.ExecutionProfiles[0].Model = "later-edit"
	run := &Run{RunID: wf.WorkflowRunID, Host: "local", Agent: "codex", Backend: execution.Backend, Protocol: execution.Protocol, Target: execution.Target}
	service := &Service{Config: base, baseConfig: base, State: Runtime{Runs: []*Run{run}, WorkflowRuns: snapshot.WorkflowRuns}}
	fixed, err := service.executionConfigForRun(run)
	if err != nil || fixed.Model != "chosen-model" || fixed.Sandbox != "danger-full-access" {
		t.Fatalf("recovery adopted the edited preset: %+v / %v", fixed, err)
	}
}

func TestAutomaticDispatchRequiresAnExplicitDefaultWithMultipleProfiles(t *testing.T) {
	ctx := context.Background()
	base, _ := nativeTaskFixture(t)
	base.ExecutionProfiles = []ExecutionProfile{{ID: "other", Name: "另一个本地配置", Host: "local", Agent: "codex", Sandbox: "workspace-write", Codex: Command{Executable: "codex"}}}
	reader, err := OpenNativeTaskProject(ctx, &base)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteStore(filepath.Join(base.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	commands := CommandService{Store: store, Project: base.Identity()}
	mode := "auto"
	if _, err := commands.SetNativeDispatchState(ctx, NativeDispatchUpdate{RequestID: strings.Repeat("e", 32), ExpectedAuthorityVersion: 1, Mode: &mode, RequestedBy: "operator"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if service, err := NewService(base); err == nil {
		service.Close()
		t.Fatal("automatic dispatch started without a configured profile")
	}
	base.DefaultExecutionProfile = ProjectExecutionProfileID
	service, err := NewService(base)
	if err != nil {
		t.Fatalf("explicit default did not restore auto mode: %v", err)
	}
	service.Close()
}

func TestWorkerFlowClaimsFreezeIndependentLocalProfiles(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	c.WorkflowProtocol, c.Claimant = WorkerFlowVersion, "operator"
	c.ExecutionProfiles = []ExecutionProfile{
		{ID: "local-first", Name: "First local", Host: "local", Agent: "codex", Model: "first-model", ReasoningEffort: "high", Sandbox: "workspace-write", Codex: Command{Executable: "codex"}},
		{ID: "local-second", Name: "Second local", Host: "local", Agent: "codex", Model: "second-model", ReasoningEffort: "xhigh", Sandbox: "workspace-write", Codex: Command{Executable: "codex"}},
	}
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("push isolated fixture: %v: %s", err, out)
	}
	commands := CommandService{Store: store, Project: c.Identity()}
	input := taskBatchFixture(t, c, commit, batchTasks(611, 612), nil)
	validation, err := commands.ValidateTaskBatch(ctx, c, input)
	if err != nil || !validation.Valid {
		t.Fatalf("validate fixed tasks: %+v %v", validation, err)
	}
	if _, err := commands.CreateTaskBatch(ctx, c, input, strings.Repeat("d", 32), validation.AuthorityVersion, "operator"); err != nil {
		t.Fatal(err)
	}
	authority, err := store.nativeAuthority(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err := store.Transact(ctx, func(tx DomainTx) error {
		return tx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	for index, taskID := range []string{"611", "612"} {
		profile := c.ExecutionProfiles[index]
		selected := c.ApplyExecutionProfile(profile)
		receipt, err := commands.ClaimNativeTask(ctx, selected, TaskClaimOptions{TaskID: taskID, RequestID: strings.Repeat(string(rune('e'+index)), 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true, ExecutionProfile: &profile})
		if err != nil || receipt.Status != "claim_reserved" {
			t.Fatalf("claim task %s with local profile: receipt=%+v err=%v", taskID, receipt, err)
		}
	}
	service, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	edited := c
	edited.ExecutionProfiles = append([]ExecutionProfile(nil), c.ExecutionProfiles...)
	edited.ExecutionProfiles[0].Model = "later-project-edit"
	reopened, err := NewService(edited)
	if err != nil {
		t.Fatalf("current-format restart changed the fixed local profiles: %v", err)
	}
	defer reopened.Close()
	for index, taskID := range []string{"611", "612"} {
		run := reopened.find(taskID)
		if run == nil {
			t.Fatalf("task %s lost its claimed worker flow", taskID)
		}
		workflow, execution := reopened.workflowRecords(run)
		fixed, err := reopened.executionConfigForRun(run)
		if err != nil || workflow == nil || workflow.WorkflowVersion != WorkerFlowVersion || workflow.ExecutionProfile == nil || workflow.ExecutionProfile.ID != c.ExecutionProfiles[index].ID || execution == nil || execution.ExecutionID != workflow.SelectedResult || fixed.Model != c.ExecutionProfiles[index].Model || len(reopened.State.Attempts) != 2 {
			t.Fatalf("task %s adopted another local profile or step chain: workflow=%+v execution=%+v config=%+v err=%v", taskID, workflow, execution, fixed, err)
		}
	}
}

func TestStartPayloadRequiresCompleteExecutionProfileSelection(t *testing.T) {
	valid := ControlRequest{Action: "start_task", Payload: []byte(`{"task_id":"101","execution_profile_id":"local-fast","execution_profile_digest":"` + strings.Repeat("a", 64) + `"}`)}
	parsed, err := decodeControlPayload(valid)
	if err != nil || parsed.TaskID != "101" || parsed.ExecutionProfileID != "local-fast" {
		t.Fatalf("profile selection was not decoded: %+v / %v", parsed, err)
	}
	for _, payload := range []string{`{"task_id":"101","execution_profile_id":"local-fast"}`, `{"task_id":"101","execution_profile_digest":"digest"}`, `{"task_id":"101","execution_profile_id":"local-fast","execution_profile_digest":"digest","mode":"auto"}`} {
		if _, err := decodeControlPayload(ControlRequest{Action: "start_task", Payload: json.RawMessage(payload)}); err == nil {
			t.Fatalf("incomplete or mixed start payload was accepted: %s", payload)
		}
	}
}

func TestUnsupportedLocalAgentProfileIsMarkedUnavailable(t *testing.T) {
	profile := ExecutionProfile{ID: "local-pi", Name: "Local Pi", Host: "local", Agent: "pi"}
	if reason := profile.StaticUnavailableReason(); !strings.Contains(reason, "codex") {
		t.Fatalf("unsupported native agent was shown as selectable: %q", reason)
	}
}
