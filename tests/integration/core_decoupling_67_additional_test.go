package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func additionalDirectory(t *testing.T, ledger *additionalLedger) *execution.ExecutionDirectory {
	t.Helper()
	key := execution.ExecutionRegistrationKey{Backend: "fixture", Protocol: additionalProtocol, Agent: "fixture"}
	codec := execution.ImplementationProfileCodec{Key: additionalKey, Version: 1, Normalize: func(data []byte, host, agent string) ([]byte, []string, error) {
		var value struct {
			Target string `json:"target"`
		}
		if err := json.Unmarshal(data, &value); err != nil {
			return nil, nil, err
		}
		if value.Target != "fixture:fixed" || host != "fixture" || agent != "fixture" {
			return nil, nil, errors.New("fixture supports only its fixed offline, read-only target")
		}
		return bytes.Clone(data), []string{"isolated fake; native permissions unverified"}, nil
	}}
	port := func(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
		return &additionalPort{setup: setup.Clone(), ledger: ledger}, nil
	}
	entry := execution.ExecutionRegistration{ServiceStopBehavior: executionbackend.ContinueExecutionOnServiceStop, Key: key, ProfileKey: "fixture/stream-v1", WorkspaceBranchPrefix: "fixture", LocalWorkspace: true, ProfileCodecKeys: []string{additionalKey}, QuestionCodecKey: additionalKey, QuestionCodecVersion: 1, Host: "fixture",
		Lifecycle: port, Target: func(execution.Config) executionbackend.ExecutionTarget {
			return executionbackend.ExecutionTarget{ID: "fixture:fixed"}
		}, Backend: func(execution.Config) executionbackend.ExecutionBackend { return &additionalPort{ledger: ledger} },

		ValidateConfig: func(context.Context, execution.Config) error { return nil }, Preflight: func(context.Context, execution.Config) error { return nil }, PrepareClaim: func(context.Context, execution.Config) (execution.ClaimPreparation, error) {
			return execution.ClaimPreparation{}, nil
		}, ReadinessRequired: func(execution.Config) bool { return false },
		BuildPolicyResource: func(_ context.Context, r execution.PolicyResourceBuildRequest) (execution.PolicyResourceBuildResult, error) {
			e, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{ImplementationKey: additionalKey, CodecVersion: 1, Purpose: executionbackend.ImplementationResourcePurpose, ProjectID: r.Resource.ProjectID, ExecutionID: r.Resource.ExecutionID, AttemptID: r.Resource.AttemptID, Generation: r.Resource.Generation, ConfigDigest: r.Resource.ConfigDigest}, []byte(`{"policy":"isolated-read-only"}`))
			return execution.PolicyResourceBuildResult{Envelope: e}, err
		}, VerifyPolicyResource: func(e executionbackend.ImplementationDataEnvelope, id execution.PolicyResourceIdentity) error {
			if e.ImplementationKey != additionalKey || e.CodecVersion != 1 {
				return errors.New("fixture resource codec changed")
			}
			return e.ValidateResourceIdentity(id.ProjectID, id.ExecutionID, id.AttemptID, id.Generation, id.ConfigDigest)
		},
		LinkedWorktreeWriteArgs: func(context.Context, execution.Config, string) ([]string, error) { return nil, nil }, ValidTurnID: func(value string) bool { return len(value) == 32 }, CleanupCapabilities: []string{"observe", "resource_settlement"},
		ValidateRef: func(ref executionbackend.ExecutionRef) error {
			if ref.Protocol != additionalProtocol || ref.Target.ID != "fixture:fixed" || ref.External.Kind != "fixture" || ref.External.Incarnation != "fixture-original" {
				return errors.New("fixture binding identity changed")
			}
			return nil
		},
		ValidatePublicationPolicy: func(policy executionbackend.PublicationPolicy, _ publication.Config) error {
			if policy.Publisher != executionbackend.PublisherWorker {
				return errors.New("fixture uses worker publication")
			}
			return nil
		},
	}
	directory, err := execution.NewExecutionDirectory("fixture-directory-v1", []execution.ImplementationProfileCodec{codec}, []execution.ExecutionRegistration{entry})
	require(t, err)
	return directory
}

func TestCD67AdditionalImplementation(t *testing.T) {
	t.Run("neutral_fixed_input_restart_and_fresh_manual_answer", testAdditionalImplementation)
	t.Run("supported_app_server_receipt_barriers", testIntegratedAppServerTwoTurns)
	t.Run("supported_terminal_recovery_and_original_identity", func(t *testing.T) { testIntegratedNativeSessionGate(t, true) })
	t.Run("remote_protocol_is_not_permission_authentication", testCD64RemoteProtocolAssertions)
	t.Run("remote_model_and_cancellation_admission", TestSupervisedOrcaRequiresPinnedModelAndKeepsStopUnsupported)
	t.Run("remote_claim_requires_permission_evidence", TestRemotePublicationClaimRequiresPermissionEvidence)
}

func testAdditionalImplementation(t *testing.T) {
	testAdditionalImplementationWithCandidateGuard(t, false)
}

func testAdditionalImplementationWithCandidateGuard(t *testing.T, candidateGuard bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	fixture := newLifecycleProjectBase(t, "local", false)
	provider := newAdditionalProvider(t)
	fixture.remoteURL = provider.server.URL + "/owner/repo.git"
	fixtureGitCommand(t, "-C", fixture.repository, "remote", "set-url", "origin", fixture.remoteURL)
	t.Setenv("STEERLINE_TEST_REMOTE_URL", fixture.remoteURL)
	ledger := &additionalLedger{path: filepath.Join(filepath.Dir(fixture.configPath), "fixture-ledger.json"), observers: map[string]*additionalObserver{}, publicationURL: provider.server.URL + "/owner/repo/pulls/901"}
	directory := additionalDirectory(t, ledger)
	global := map[string]any{"version": 2, "default_execution_profile": "fixture", "execution_profiles": []any{map[string]any{"id": "fixture", "name": "Neutral fixture", "host": "fixture", "agent": "fixture", "implementation_configs": []executionbackend.ImplementationConfigInput{{ImplementationKey: additionalKey, CodecVersion: 1, Payload: []byte(`{"target":"fixture:fixed"}`)}}}}}
	require(t, os.WriteFile(steerlineapp.GlobalExecutionConfigPath(), mustJSON(global), 0600))
	projectData, err := os.ReadFile(fixture.configPath)
	require(t, err)
	var project map[string]any
	require(t, json.Unmarshal(projectData, &project))
	project["publication_policy"] = executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}
	project["pr"] = publication.Config{Kind: "gitea", BaseURL: provider.server.URL, Repository: "owner/repo"}
	project["pr_poll"] = "1s"
	require(t, os.WriteFile(fixture.configPath, mustJSON(project), 0600))
	fixture.config, err = steerlineapp.LoadConfigWithDirectory(fixture.configPath, directory, steerlineapp.NewProcessHost(10*time.Second))
	require(t, err)
	projectStore, err := steerlineapp.OpenNativeTaskProject(ctx, &fixture.config)
	require(t, err)
	require(t, projectStore.Close())
	createIntegrationTask(t, ctx, fixture, 901)
	service := startIntegrationServiceWithSources(t, ctx, fixture.config, directory, additionalSources{})
	claimIntegrationTask(t, ctx, fixture, 901, service.instanceID)
	first := waitIntegrationRun(t, ctx, service, fixture.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute != nil && e.TurnNumber == 1
	})
	originalInput := []byte(nil)
	for _, op := range first.runtime.Operations {
		if op.Kind == "worker_input_value" {
			originalInput = bytes.Clone(op.Request)
		}
	}
	if len(originalInput) == 0 || first.execution.Target.ID != "fixture:fixed" || first.execution.SessionID != "fixture-original-session" {
		t.Fatal("fixture did not persist the original target, session and input")
	}
	oldAttempt, oldProcess := first.run.AttemptID, *first.run.Process
	provider.mu.Lock()
	provider.branch, provider.head = first.execution.Workspace.Branch, first.execution.Workspace.Base
	provider.mu.Unlock()
	assertNoTaskDelivery(t, first.runtime, first.run)
	service.stop(t)
	ledger2 := &additionalLedger{path: ledger.path, observers: map[string]*additionalObserver{}}
	directory2 := additionalDirectory(t, ledger2)
	config2, err := steerlineapp.LoadConfigWithDirectory(fixture.configPath, directory2, steerlineapp.NewProcessHost(10*time.Second))
	require(t, err)
	if config2.ExecutionDirectory == directory {
		t.Fatal("restart reused a mutable directory")
	}
	service2 := startIntegrationServiceWithSources(t, ctx, config2, directory2, additionalSources{})
	defer service2.stop(t)
	recovered := waitIntegrationRun(t, ctx, service2, config2, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return e.RecoveryRequired && e.RecoveryState == "live" && run.Decision != nil && run.Decision.Status == "pending"
	})
	ledger2.mu.Lock()
	saved, err := ledger2.read()
	reads := ledger2.reads
	ledger2.mu.Unlock()
	require(t, err)
	if reads == 0 || len(saved.Inputs) != 1 || len(saved.Answers) != 0 || recovered.run.AttemptID != oldAttempt || *recovered.run.Process != oldProcess {
		t.Fatal("restart mutated input or replaced the original identity before manual confirmation")
	}
	for _, op := range recovered.runtime.Operations {
		if op.Kind == "worker_input_value" && !bytes.Equal(op.Request, originalInput) {
			t.Fatal("restart changed persistent original input")
		}
	}
	submitIntegrationAnswer(t, ctx, config2, recovered.run)
	settled := waitIntegrationRun(t, ctx, service2, config2, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return e.LastStoppedTurnID == first.execution.CurrentTurnID && !e.RecoveryRequired && run.Decision != nil && run.Decision.Status == "pending" && run.Decision.Request.QuestionRoute == nil
	})
	ledger2.mu.Lock()
	saved, err = ledger2.read()
	ledger2.mu.Unlock()
	require(t, err)
	if len(saved.Inputs) != 1 || len(saved.Answers) != 1 || settled.execution.TurnNumber != 1 || settled.execution.SessionID != first.execution.SessionID || *settled.run.Process != oldProcess || !decisionWasAccepted(settled.runtime, recovered.run.Decision.ID) {
		t.Fatal("manual answer repeated input or lost its original receipt")
	}
	assertNoTaskDelivery(t, settled.runtime, settled.run)
	provider.mu.Lock()
	provider.merged, provider.mergedAt = true, time.Now().UTC()
	provider.mu.Unlock()
	mergedInfo, err := (&publication.Client{Config: publication.Config(config2.PR), Timeout: 10 * time.Second}).Query(ctx, ledger.publicationURL)
	require(t, err)
	if !mergedInfo.Merged || mergedInfo.State != "merged" || mergedInfo.MergeCommit == nil || *mergedInfo.MergeCommit != fixture.commit {
		t.Fatal("isolated provider did not expose its original merged receipt")
	}
	// Settled executions leave the active runtime projection. Verify delivery
	// through the public task detail, which retains its authoritative receipt.
	var delivered storageports.NativeTaskPage
	var deliveryServiceError string
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("last task delivery query: %s; service error=%v", mustJSON(delivered), deliveryServiceError)
		}
	})
	waitFor(t, 2*time.Minute, func() bool {
		var err error
		delivered, err = service2.queries.ShowNativeTask(ctx, "901")
		if observed, queryErr := steerlineapp.LoadDatabaseRuntime(config2); queryErr == nil && observed != nil && observed.Service.Error != nil {
			deliveryServiceError = *observed.Service.Error
			t.Fatalf("production delivery reported an error: %s", deliveryServiceError)
		}
		return err == nil && len(delivered.Tasks) == 1 && delivered.Tasks[0].Status == "closed" && delivered.Tasks[0].Delivery != nil
	})
	view, err := service2.queries.ReadProjectView(ctx, "901", 100)
	require(t, err)
	if view.SelectedTask == nil || view.SelectedTask.Delivery == nil || view.SelectedTask.Delivery.SelectedWorkflowRunID != first.run.RunID || view.SelectedTask.Delivery.MergeCommit != fixture.commit || view.SelectedTask.Delivery.PRURL != ledger.publicationURL {
		t.Fatal("product task page lost its original selected workflow/provider delivery")
	}
	observed, err := steerlineapp.LoadDatabaseRuntime(config2)
	require(t, err)
	if observed == nil {
		t.Fatal("execution reconstruction lost its database snapshot")
	}
	count := 0
	for _, delivery := range observed.Deliveries {
		if delivery.TaskID == "901" {
			count++
			if delivery.SelectedWorkflowRunID != first.run.RunID || delivery.MergeCommit != fixture.commit || delivery.PRURL != ledger.publicationURL {
				t.Fatal("delivery replaced original selected workflow/provider merge fact")
			}
		}
	}
	if count != 1 {
		t.Fatalf("production provider delivery count=%d, want one", count)
	}
	if delivery := delivered.Tasks[0].Delivery; delivery.SelectedWorkflowRunID != first.run.RunID || delivery.MergeCommit != fixture.commit || delivery.PRURL != ledger.publicationURL {
		t.Fatal("task detail lost its original selected workflow/provider delivery")
	}
	if candidateGuard {
		store, err := steerlineapp.OpenNativeTaskCommandStore(ctx, &config2)
		require(t, err)
		defer store.Close()
		command := appExecutionCommands(t, store, config2)
		_, err = command.AbandonNativeCandidate(ctx, config2.ExecutionConfig(), execution.CandidateAbandonOptions{TaskID: "901", WorkflowRunID: first.run.RunID, RequestID: newID(), ExpectedTaskVersion: delivered.Tasks[0].Version, ExpectedCandidateVersion: first.run.ControlRevision, RequestedBy: "operator", Reason: "must reject known merged candidate"})
		if err == nil || !strings.Contains(err.Error(), "already_merged") {
			t.Fatalf("real merged delivery allowed exit: %v", err)
		}
		t.Logf("real provider merge %s; candidate abandon rejected: %v", ledger.publicationURL, err)
	}
	waitFor(t, 20*time.Second, func() bool {
		ledger2.mu.Lock()
		defer ledger2.mu.Unlock()
		value, err := ledger2.read()
		return err == nil && value.CleanupCount == 1
	})
	ledger2.mu.Lock()
	saved, err = ledger2.read()
	ledger2.mu.Unlock()
	require(t, err)
	if saved.CleanupCount != 1 || len(saved.Inputs) != 1 || len(saved.Answers) != 1 {
		t.Fatal("delivery cleanup duplicated an external mutation or lost original input")
	}
	provider.mu.Lock()
	queries := provider.queries
	provider.mu.Unlock()
	if queries == 0 {
		t.Fatal("delivery bypassed the production publication query")
	}
	// The fake does not certify missing tools, permission evidence or alternate
	// targets. The same accepted catalog refuses each unsupported new claim.
	for _, target := range []string{"fixture:changed", ""} {
		_, _, err := directory.NormalizeProfile([]executionbackend.ImplementationConfigInput{{ImplementationKey: additionalKey, CodecVersion: 1, Payload: mustJSON(map[string]string{"target": target})}}, "fixture", "fixture", config2.Identity(), "fixture", config2.LoadedConfigDigest())
		if err == nil {
			t.Fatal("alternate target admitted")
		}
	}
	if !strings.Contains(strings.Join(config2.AvailableExecutionProfiles()[0].Freeze(config2.Identity()).ImplementationDisplay, " "), "unverified") {
		t.Fatal("fixture hid its native authentication limit")
	}
}
