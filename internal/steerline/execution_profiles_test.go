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
	"time"
)

func testProfileSteps(site string) *ProjectSteps {
	step := StepContract{Provider: "command", Entry: "tools/mock.py", Version: "minimal-v1", InputSchema: NativeTaskInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: site, Timeout: time.Minute, Command: []string{"python", "tools/mock.py"}}
	steps := &ProjectSteps{Implement: step, Verify: step, PublishPR: step}
	steps.Verify.RequiredChecks = []string{"mock"}
	return steps
}

func TestConfigLoadsLocalAndRemoteOrcaProfilesWithoutProbingTargets(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("local Orca terminal target uses Windows cmd.exe")
	}
	base, _ := nativeTaskFixture(t)
	if err := os.MkdirAll(filepath.Join(base.Repository, "tools"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base.Repository, "tools", "mock.py"), []byte("print('mock')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	definition := DefaultMinimalWorkflowDefinition(false)
	localSteps := testProfileSteps("local")
	localOrca := ExecutionProfile{ID: "orca-local", Name: "Orca 本机", Host: "orca", Agent: "codex", Model: "chosen-model", ReasoningEffort: "high", Sandbox: "workspace-write", Codex: Command{Executable: "codex"}, Orca: OrcaConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "15m"}, ProjectSteps: testProfileSteps("target")}
	remoteOrca := ExecutionProfile{ID: "orca-remote", Name: "Orca 远程", Host: "orca_supervised", Agent: "codex", Model: "chosen-model", ReasoningEffort: "xhigh", Sandbox: "workspace-write", Codex: Command{Executable: "codex"}, Orca: OrcaConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "15m", Supervised: &SupervisedOrcaConfig{ConnectionID: "connection", EnvironmentID: "environment", RepositorySelector: "repo", CoordinatorTerminal: "terminal", PollInterval: "5s", CapabilityReportVersion: 1}}, ProjectSteps: testProfileSteps("target")}
	input := map[string]any{"repository": base.Repository, "branch": "main", "task_authority": "native", "host": "local", "agent": "codex", "sandbox": "workspace-write", "workflow_definition": definition, "project_steps": localSteps, "execution_profiles": []ExecutionProfile{localOrca, remoteOrca}, "default_execution_profile": ProjectExecutionProfileID}
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
	if err != nil || len(loaded.AvailableExecutionProfiles()) != 3 || loaded.DefaultProfileID() != ProjectExecutionProfileID {
		t.Fatalf("configured profiles were not loaded: %+v / %v", loaded.ExecutionProfiles, err)
	}
	for _, profile := range loaded.ExecutionProfiles {
		selected, chosen, err := loaded.ResolveExecutionProfile(profile.ID, profile.Digest())
		if err != nil || selected.Host != profile.Host || chosen.ProjectSteps.Implement.ExecutionSite != "target" {
			t.Fatalf("Orca profile changed route: %+v / %v", chosen, err)
		}
	}
	localOrca.ProjectSteps = testProfileSteps("local")
	input["execution_profiles"] = []ExecutionProfile{localOrca, remoteOrca}
	write()
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "execution_site=target") {
		t.Fatalf("Orca profile with local steps was accepted: %v", err)
	}
}

func TestNativeClaimPersistsChosenExecutionProfile(t *testing.T) {
	ctx := context.Background()
	base, commit := nativeTaskFixture(t)
	if out, err := exec.Command("git", "-C", base.Repository, "push", "origin", "main").CombinedOutput(); err != nil {
		t.Fatalf("push fixture base: %v: %s", err, out)
	}
	base.WorkflowDefinition = ptr(DefaultMinimalWorkflowDefinition(false))
	step := StepContract{Provider: "command", Entry: "builtin:verify", Version: MinimalWorkflowVersion, InputSchema: NativeTaskInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "local", Timeout: time.Minute, Command: []string{"git", "--version"}}
	base.ProjectSteps = &ProjectSteps{Implement: step, Verify: step, PublishPR: step}
	base.ProjectSteps.Verify.RequiredChecks = []string{"unit"}
	base.WorkspaceDirtyIgnore = WorkspaceDirtyIgnore{Extensions: []string{".meta"}}
	profile := ExecutionProfile{ID: "local-strong", Name: "本地高强度", Host: "local", Agent: "codex", Model: "chosen-model", ReasoningEffort: "xhigh", Sandbox: "danger-full-access", Codex: Command{Executable: "git"}, ProjectSteps: base.ProjectSteps}
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
	if wf.ExecutionProfile == nil || wf.ExecutionProfile.ID != profile.ID || !wf.ExecutionProfile.Verify(wf.ProjectSteps) || wf.Runner.Model != "chosen-model" || wf.Runner.ReasoningEffort != "xhigh" || execution.Backend != string(BackendLocal) || len(wf.WorkspaceDirtyIgnore.Extensions) != 1 || wf.WorkspaceDirtyIgnore.Extensions[0] != ".meta" {
		t.Fatalf("chosen execution route was not fixed in the claim: %+v / %+v", wf, execution)
	}
	base.ExecutionProfiles[0].Model = "later-edit"
	base.WorkspaceDirtyIgnore = WorkspaceDirtyIgnore{}
	run := &Run{RunID: wf.WorkflowRunID, Host: "local", Agent: "codex", Backend: execution.Backend, Protocol: execution.Protocol, Target: execution.Target}
	service := &Service{Config: base, baseConfig: base, State: Runtime{Runs: []*Run{run}, WorkflowRuns: snapshot.WorkflowRuns}}
	if err := service.useFixedRunProfile(run); err != nil || service.Config.Model != "chosen-model" || len(service.Config.WorkspaceDirtyIgnore.Extensions) != 1 {
		t.Fatalf("recovery adopted the edited preset: %+v / %v", service.Config, err)
	}
}

func TestAutomaticDispatchRequiresAnExplicitDefaultWithMultipleProfiles(t *testing.T) {
	ctx := context.Background()
	base, _ := nativeTaskFixture(t)
	base.WorkflowDefinition = ptr(DefaultMinimalWorkflowDefinition(false))
	base.ProjectSteps = testProfileSteps("local")
	base.ExecutionProfiles = []ExecutionProfile{{ID: "other", Name: "另一个本地配置", Host: "local", Agent: "codex", Sandbox: "workspace-write", Codex: Command{Executable: "codex"}, ProjectSteps: testProfileSteps("local")}}
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

func TestServiceStartControlDispatchesChosenProfile(t *testing.T) {
	t.Setenv("STEERLINE_FAKE_NATIVE_STEP_DECISION", "implement")
	f := newFixture(t)
	f.c.configDigest = strings.Repeat("f", 64)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	step := func(name string) StepContract {
		return StepContract{Provider: "command", Entry: "builtin:" + name, Version: "selected-profile-v1", InputSchema: NativeTaskInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "local", Timeout: 10 * time.Second, Command: []string{executable, "--fake", "native-step"}}
	}
	f.c.ProjectSteps = &ProjectSteps{Implement: step("implement"), Verify: step("verify"), PublishPR: step("publish_pr")}
	f.c.ProjectSteps.Verify.RequiredChecks = []string{"unit"}
	profile := ExecutionProfile{ID: "local-selected", Name: "已选本地方案", Host: "local", Agent: "codex", Model: "selected-model", ReasoningEffort: "xhigh", Sandbox: "workspace-write", Codex: f.c.Codex, ProjectSteps: f.c.ProjectSteps}
	otherProfile := profile
	otherProfile.ID, otherProfile.Name, otherProfile.Model = "local-other", "另一套本地方案", "other-model"
	f.c.ExecutionProfiles = []ExecutionProfile{profile, otherProfile}
	service := f.service(t)
	defer service.Close()
	observerCtx, stopObservers := context.WithCancel(context.Background())
	defer stopObservers()
	service.runContext = observerCtx
	service.workerMessages = make(chan workerMessage, 128)
	service.State.Service.Mode, service.State.Service.Ready = "scheduler", true
	if err = service.save(); err != nil {
		t.Fatal(err)
	}
	contract := nativeContract(t, service.Config, f.commit, "selected-profile-contract")
	otherContract := nativeContract(t, service.Config, f.commit, "other-profile-contract")
	batch, err := json.Marshal(TaskBatchInput{SchemaVersion: taskBatchSchemaVersion, ProjectID: service.Config.Identity(), Tasks: []TaskBatchTask{{LocalKey: "selected", TaskID: 611, Title: "Selected profile", Type: "task", Contract: contract}, {LocalKey: "other", TaskID: 612, Title: "Other profile", Type: "task", Contract: otherContract}}, Dependencies: []TaskBatchEdge{}})
	if err != nil {
		t.Fatal(err)
	}
	commands := CommandService{Store: service.domainStore, Project: service.Config.Identity()}
	if _, err = seedExecutionProfileTasks(t, service.Config, service.domainStore, batch); err != nil {
		t.Fatal(err)
	}
	authority, err := service.domainStore.nativeAuthority(context.Background(), service.Config.Identity())
	if err != nil {
		t.Fatal(err)
	}
	manual, unpaused := "manual", false
	if _, err = commands.SetNativeDispatchState(context.Background(), NativeDispatchUpdate{RequestID: newID(), ExpectedAuthorityVersion: authority.AuthorityVersion, Mode: &manual, Paused: &unpaused, RequestedBy: "operator"}); err != nil {
		t.Fatal(err)
	}
	seedFakeCapabilities(t, service.Config)
	requestID := newID()
	req := ControlRequest{Action: "start_task", RequestID: requestID}
	payload := ControlPayload{TaskID: "611", ExecutionProfileID: profile.ID, ExecutionProfileDigest: profile.Digest()}
	op := &ControlOperation{RequestID: requestID, Action: "start_task", TaskID: payload.TaskID, RunID: newID(), Phase: "applying"}
	service.State.Control.Operation = op
	if err = service.save(); err != nil {
		t.Fatal(err)
	}
	if err = service.applyControlAction(context.Background(), req, payload, op, &ControlReceipt{}); err != nil {
		t.Fatal(err)
	}
	if service.worker == nil || service.Config.Model != "selected-model" {
		t.Fatal("start control did not launch the selected execution route")
	}
	run := service.find("611")
	workflow, execution := service.minimalRecords(run)
	if workflow == nil || workflow.ExecutionProfile == nil || workflow.ExecutionProfile.ID != profile.ID || execution == nil || execution.Backend != string(BackendLocal) {
		t.Fatalf("service did not persist selected route: %+v / %+v", workflow, execution)
	}
	firstWorker := service.worker
	service.State.Control.Operation = nil
	secondRequestID := newID()
	secondPayload := ControlPayload{TaskID: "612", ExecutionProfileID: otherProfile.ID, ExecutionProfileDigest: otherProfile.Digest()}
	secondOperation := &ControlOperation{RequestID: secondRequestID, Action: "start_task", TaskID: secondPayload.TaskID, RunID: newID(), Phase: "applying"}
	service.State.Control.Operation = secondOperation
	if err = service.save(); err != nil {
		t.Fatal(err)
	}
	if err = service.applyControlAction(context.Background(), ControlRequest{Action: "start_task", RequestID: secondRequestID}, secondPayload, secondOperation, &ControlReceipt{}); err != nil {
		t.Fatal(err)
	}
	secondRun := service.find("612")
	if secondRun == nil || service.workers[run.RunID] != firstWorker || service.workers[secondRun.RunID] == nil || service.workers[secondRun.RunID] == firstWorker || service.Config.Model != "other-model" {
		t.Fatal("independent manual tasks did not retain distinct simultaneous workers and profiles")
	}
	settled := map[string]bool{}
	for len(settled) != 2 {
		select {
		case message := <-service.workerMessages:
			if err = service.handleWorkerMessage(message); err != nil {
				t.Fatal(err)
			}
			if message.done {
				settled[message.runID] = true
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("parallel observers did not settle: %+v", settled)
		}
	}
	service.State.Control.Operation = nil
	if err = service.save(); err != nil {
		t.Fatal(err)
	}
	if err = service.Close(); err != nil {
		t.Fatal(err)
	}
	edited := f.c
	edited.ExecutionProfiles = append([]ExecutionProfile(nil), f.c.ExecutionProfiles...)
	edited.ExecutionProfiles[0].Model = "later-project-edit"
	edited.configDigest = strings.Repeat("d", 64)
	restarted, err := NewService(edited)
	if err != nil {
		t.Fatalf("restart failed for fixed profile: %v", err)
	}
	defer restarted.Close()
	fixedFirst, err := restarted.executionConfigForRun(restarted.find("611"))
	if err != nil || fixedFirst.Model != "selected-model" || fixedFirst.ReasoningEffort != "xhigh" {
		t.Fatalf("restart adopted edited settings for first run: model=%q effort=%q err=%v", fixedFirst.Model, fixedFirst.ReasoningEffort, err)
	}
	fixedSecond, err := restarted.executionConfigForRun(restarted.find("612"))
	if err != nil || fixedSecond.Model != "other-model" {
		t.Fatalf("restart adopted another run's settings: model=%q err=%v", fixedSecond.Model, err)
	}
}

func TestExecutionProfilesSelectAndFreezeOneTaskRoute(t *testing.T) {
	base := Config{Repository: "C:/repo", Host: "local", Agent: "codex", Model: "base-model", Sandbox: "workspace-write", Codex: Command{Executable: "codex"}, WorkflowDefinition: ptr(DefaultMinimalWorkflowDefinition(false)), ProjectSteps: testProfileSteps("local")}
	profile := ExecutionProfile{ID: "review-heavy", Name: "高强度本地", Host: "local", Agent: "codex", Model: "chosen-model", ReasoningEffort: "xhigh", Sandbox: "danger-full-access", Codex: Command{Executable: "codex"}, ProjectSteps: testProfileSteps("local")}
	base.ExecutionProfiles = []ExecutionProfile{profile}
	if base.DefaultProfileID() != "" {
		t.Fatal("multiple profiles gained an implicit auto-dispatch choice")
	}
	if _, _, err := base.ResolveExecutionProfile(profile.ID, strings.Repeat("0", 64)); err == nil {
		t.Fatal("stale profile digest was accepted")
	}
	selected, chosen, err := base.ResolveExecutionProfile(profile.ID, profile.Digest())
	if err != nil || selected.Model != "chosen-model" || selected.ReasoningEffort != "xhigh" || !chosen.MatchesConfig(selected) {
		t.Fatalf("wrong selected profile: %+v / %v", selected, err)
	}
	fixed := chosen.Freeze()
	if !fixed.Verify(*chosen.ProjectSteps) {
		t.Fatal("fixed profile digest did not include the fixed steps")
	}
	run := &Run{RunID: strings.Repeat("a", 32), Host: "local", Agent: "codex", Backend: string(BackendLocal), Protocol: string(ProtocolLocalProcess), Target: targetFor(selected)}
	s := &Service{Config: base, baseConfig: base, State: Runtime{Runs: []*Run{run}, WorkflowRuns: []WorkflowRunRecord{{WorkflowRunID: run.RunID, ProjectSteps: *chosen.ProjectSteps, ExecutionProfile: &fixed}}}}
	s.baseConfig.ExecutionProfiles = nil
	s.baseConfig.Model = "new-default"
	if err := s.useFixedRunProfile(run); err != nil || s.Config.Model != "chosen-model" || s.Config.Sandbox != "danger-full-access" {
		t.Fatalf("running task adopted edited config: %+v / %v", s.Config, err)
	}
	s.State.WorkflowRuns[0].ExecutionProfile.Model = "tampered"
	if err := s.useFixedRunProfile(run); err == nil {
		t.Fatal("tampered fixed route was accepted on recovery")
	}
}

func TestMixedTaskHistoriesKeepTheirOwnBackendRoutes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Orca terminal target uses Windows cmd.exe")
	}
	base := Config{Repository: "C:/repo", Host: "local", Agent: "codex", Sandbox: "workspace-write", WorkflowDefinition: ptr(DefaultMinimalWorkflowDefinition(false)), ProjectSteps: testProfileSteps("local")}
	orca := ExecutionProfile{ID: "orca-local", Name: "Orca 本机", Host: "orca", Agent: "codex", Model: "orca-model", Sandbox: "workspace-write", Orca: OrcaConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "15m"}, ProjectSteps: testProfileSteps("target")}
	base.ExecutionProfiles = []ExecutionProfile{orca}
	localFixed, orcaFixed := base.projectExecutionProfile().Freeze(), orca.Freeze()
	localRun := &Run{RunID: strings.Repeat("a", 32), Host: "local", Agent: "codex", Backend: string(BackendLocal), Protocol: string(ProtocolLocalProcess), Target: targetFor(base)}
	orcaConfig := base.ApplyExecutionProfile(orca)
	orcaRun := &Run{RunID: strings.Repeat("b", 32), Host: "orca", Agent: "codex", Backend: string(BackendOrca), Protocol: string(ProtocolOrcaTerminalV2), Target: targetFor(orcaConfig)}
	s := &Service{Config: orcaConfig, baseConfig: base, State: Runtime{Runs: []*Run{localRun, orcaRun}, WorkflowRuns: []WorkflowRunRecord{{WorkflowRunID: localRun.RunID, ProjectSteps: *base.ProjectSteps, ExecutionProfile: &localFixed}, {WorkflowRunID: orcaRun.RunID, ProjectSteps: *orca.ProjectSteps, ExecutionProfile: &orcaFixed}}}}
	for _, sample := range []struct {
		run  *Run
		host string
	}{{localRun, "local"}, {orcaRun, "orca"}} {
		config, err := s.executionConfigForRun(sample.run)
		if err != nil || config.Host != sample.host || hostForExecution(sample.run.Backend, sample.run.Protocol) != sample.host {
			t.Fatalf("historical run used another task's route: run=%s host=%q err=%v", sample.run.RunID, config.Host, err)
		}
	}
	localRun.ExecutionStatus, orcaRun.ExecutionStatus = "running", "running"
	if err := s.restoreActiveExecutionProfile(); err != nil || s.Config.Host != base.Host {
		t.Fatalf("multiple active routes changed the project control configuration: host=%q err=%v", s.Config.Host, err)
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

func TestUnsupportedAgentSkillProfileIsMarkedUnavailable(t *testing.T) {
	profile := ExecutionProfile{ID: "orca-pi", Name: "Orca Pi", Host: "orca", Agent: "pi", ProjectSteps: &ProjectSteps{Implement: StepContract{Provider: "agent_skill"}}}
	if reason := profile.StaticUnavailableReason(); !strings.Contains(reason, "codex") {
		t.Fatalf("unsupported native agent was shown as selectable: %q", reason)
	}
}
