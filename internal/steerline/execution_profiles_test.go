package steerline

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

func TestNativeClaimPersistsChosenExecutionProfile(t *testing.T) {
	ctx := context.Background()
	base, commit := nativeTaskFixture(t)
	if out, err := exec.Command("git", "-C", base.Repository, "push", "origin", "main").CombinedOutput(); err != nil {
		t.Fatalf("push fixture base: %v: %s", err, out)
	}
	profile := neutralProfile(t, "local-strong", "本地高强度", neutralSettings{Executable: os.Args[0], Model: "chosen-model", ReasoningEffort: "xhigh"})
	base.ExecutionProfiles = []ExecutionProfile{profile}
	reader, err := OpenNativeTaskProject(ctx, &base)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := openSQLiteFixture(filepath.Join(base.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	commands := newTestCommandService(store, base.Identity())
	if _, err = commands.CreateTaskBatch(ctx, base, taskBatchFixture(t, base, commit, batchTasks(501), nil), strings.Repeat("1", 32), 1, "planner"); err != nil {
		t.Fatal(err)
	}
	authority, err := store.NativeAuthority(ctx, base.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err = store.transactRecords(ctx, func(tx *fixtureTransaction) error {
		return tx.UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	selected, fixedChoice, err := base.ResolveExecutionProfile(profile.ID, base.bindProjectPolicy(profile).Digest())
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
	if wf.WorkflowVersion != executionmodel.Version || wf.ExecutionProfile == nil || wf.ExecutionProfile.ID != profile.ID || !neutralVerifyFixedExecutionProfile(*wf.ExecutionProfile) || !slices.Contains(wf.ExecutionProfile.ImplementationDisplay, "Model：chosen-model") || !slices.Contains(wf.ExecutionProfile.ImplementationDisplay, "Reasoning effort：xhigh") || execution.Backend != string(executionbackend.BackendLocal) || execution.CurrentStep != "worker" {
		t.Fatalf("chosen execution route was not fixed in the claim: %+v / %+v", wf, execution)
	}
	base.ExecutionProfiles[0].ImplementationConfigs[0].Payload = mustJSON(neutralSettings{Executable: os.Args[0], Model: "later-edit", ReasoningEffort: "xhigh"})
	run := &Run{RunID: wf.WorkflowRunID, Host: "local", Agent: "fixture", Backend: execution.Backend, Protocol: execution.Protocol, Target: execution.Target}
	service := &Service{Config: base, baseConfig: base, State: Runtime{}, domain: DomainSnapshot{WorkflowRuns: snapshot.WorkflowRuns}, runs: []*Run{run}}
	fixed, err := service.executionConfigForRun(run)
	if err != nil || neutralProfileSettings(fixed).Model != "chosen-model" || fixed.ExecutionPolicy.Filesystem.WriteScope != "workspace" {
		t.Fatalf("recovery adopted the edited preset: %+v / %v", fixed, err)
	}
}

func TestAutomaticDispatchRequiresAnExplicitDefaultWithMultipleProfiles(t *testing.T) {
	ctx := context.Background()
	base, _ := nativeTaskFixture(t)
	base.ExecutionProfiles = []ExecutionProfile{neutralProfile(t, "other", "另一个本地配置", neutralSettings{Executable: "fixture"})}
	reader, err := OpenNativeTaskProject(ctx, &base)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := openSQLiteFixture(filepath.Join(base.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	commands := newTestCommandService(store, base.Identity())
	mode := "auto"
	if _, err := commands.SetNativeDispatchState(ctx, NativeDispatchUpdate{RequestID: strings.Repeat("e", 32), ExpectedAuthorityVersion: 1, Mode: &mode, RequestedBy: "operator"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if service, err := newNeutralService(base); err == nil {
		service.Close()
		t.Fatal("automatic dispatch started without a configured profile")
	}
	base.DefaultExecutionProfile = ProjectExecutionProfileID
	service, err := newNeutralService(base)
	if err != nil {
		t.Fatalf("explicit default did not restore auto mode: %v", err)
	}
	service.Close()
}

func TestWorkerFlowClaimsFreezeIndependentLocalProfiles(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	c.WorkflowProtocol, c.Claimant = executionmodel.Version, "operator"
	c.ExecutionProfiles = []ExecutionProfile{
		neutralProfile(t, "local-first", "First local", neutralSettings{Executable: "fixture", Model: "first-model", ReasoningEffort: "high"}),
		neutralProfile(t, "local-second", "Second local", neutralSettings{Executable: "fixture", Model: "second-model", ReasoningEffort: "xhigh"}),
	}
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("push isolated fixture: %v: %s", err, out)
	}
	commands := newTestCommandService(store, c.Identity())
	input := taskBatchFixture(t, c, commit, batchTasks(611, 612), nil)
	validation, err := commands.ValidateTaskBatch(ctx, c, input)
	if err != nil || !validation.Valid {
		t.Fatalf("validate fixed tasks: %+v %v", validation, err)
	}
	if _, err := commands.CreateTaskBatch(ctx, c, input, strings.Repeat("d", 32), validation.AuthorityVersion, "operator"); err != nil {
		t.Fatal(err)
	}
	authority, err := store.NativeAuthority(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err := store.transactRecords(ctx, func(tx *fixtureTransaction) error {
		return tx.UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	for index, taskID := range []string{"611", "612"} {
		profile := c.bindProjectPolicy(c.ExecutionProfiles[index])
		selected := c.ApplyExecutionProfile(profile)
		receipt, err := commands.ClaimNativeTask(ctx, selected, TaskClaimOptions{TaskID: taskID, RequestID: strings.Repeat(string(rune('e'+index)), 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true, ExecutionProfile: &profile})
		if err != nil || receipt.Status != "claim_reserved" {
			t.Fatalf("claim task %s with local profile: receipt=%+v err=%v", taskID, receipt, err)
		}
	}
	service, err := newNeutralService(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	edited := c
	edited.ExecutionProfiles = append([]ExecutionProfile(nil), c.ExecutionProfiles...)
	edited.ExecutionProfiles[0].ImplementationConfigs = cloneImplementationInputs(c.ExecutionProfiles[0].ImplementationConfigs)
	edited.ExecutionProfiles[0].ImplementationConfigs[0].Payload = mustJSON(neutralSettings{Executable: "fixture", Model: "later-project-edit", ReasoningEffort: "high"})
	reopened, err := newNeutralService(edited)
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
		if err != nil || workflow == nil || workflow.WorkflowVersion != executionmodel.Version || workflow.ExecutionProfile == nil || workflow.ExecutionProfile.ID != c.ExecutionProfiles[index].ID || execution == nil || execution.ExecutionID != workflow.SelectedResult || neutralProfileSettings(fixed).Model != neutralProfileFromInputs(c.ExecutionProfiles[index].ImplementationConfigs).Model || len(reopened.domain.Attempts) != 2 {
			t.Fatalf("task %s adopted another local profile or step chain: workflow=%+v execution=%+v config=%+v err=%v", taskID, workflow, execution, fixed, err)
		}
	}
}

func TestStartPayloadRequiresCompleteExecutionProfileSelection(t *testing.T) {
	valid := productapi.ControlRequest{Action: "start_task", Payload: []byte(`{"task_id":"101","execution_profile_id":"local-fast","execution_profile_digest":"` + strings.Repeat("a", 64) + `"}`)}
	parsed, err := decodeControlPayload(valid)
	if err != nil || parsed.TaskID != "101" || parsed.ExecutionProfileID != "local-fast" {
		t.Fatalf("profile selection was not decoded: %+v / %v", parsed, err)
	}
	for _, payload := range []string{`{"task_id":"101","execution_profile_id":"local-fast"}`, `{"task_id":"101","execution_profile_digest":"digest"}`, `{"task_id":"101","execution_profile_id":"local-fast","execution_profile_digest":"digest","mode":"auto"}`} {
		if _, err := decodeControlPayload(productapi.ControlRequest{Action: "start_task", Payload: json.RawMessage(payload)}); err == nil {
			t.Fatalf("incomplete or mixed start payload was accepted: %s", payload)
		}
	}
}
