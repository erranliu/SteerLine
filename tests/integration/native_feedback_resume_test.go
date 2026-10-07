package integration

import (
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/testkit/protocol"
)

func TestNativeOrcaConsecutiveFeedbackKeepsOriginalSession(t *testing.T) {
	testNativeOrcaConsecutiveFeedback(t, false)
}

func TestNativeOrcaOpenPRFeedbackResumesSavedTask(t *testing.T) {
	testNativeOrcaConsecutiveFeedback(t, true)
}

func testNativeOrcaConsecutiveFeedback(t *testing.T, openPR bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	f := newLifecycleProjectBase(t, "orca", !openPR)
	initial := testprotocol.NativeOrcaState{EnvironmentMode: true, ReadinessMode: "success"}
	var provider *additionalProvider
	if openPR {
		provider = newAdditionalProvider(t)
		f.remoteURL = provider.server.URL + "/owner/repo.git"
		fixtureGitCommand(t, "-C", f.repository, "remote", "set-url", "origin", f.remoteURL)
		t.Setenv("STEERLINE_TEST_REMOTE_URL", f.remoteURL)
		raw, err := os.ReadFile(f.configPath)
		require(t, err)
		var config map[string]any
		require(t, json.Unmarshal(raw, &config))
		config["pr"] = publication.Config{Kind: "gitea", BaseURL: provider.server.URL, Repository: "owner/repo"}
		config["pr_poll"] = "100ms"
		require(t, os.WriteFile(f.configPath, mustJSON(config), 0600))
		f.config, err = steerlineapp.LoadConfig(f.configPath)
		require(t, err)
		project, err := steerlineapp.OpenNativeTaskProject(ctx, &f.config)
		require(t, err)
		require(t, project.Close())
		initial.CompletionPRURL = provider.server.URL + "/owner/repo/pulls/901"
	}
	require(t, testprotocol.WriteNativeOrcaState(f.nativeStateDir, initial))
	createIntegrationTask(t, ctx, f, 901)
	service := startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
	defer func() { service.stop(t) }()
	claimIntegrationTask(t, ctx, f, 901, service.instanceID)
	if provider != nil {
		waitFor(t, 30*time.Second, func() bool {
			state, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
			if err != nil || state.Branch == "" {
				return false
			}
			provider.mu.Lock()
			provider.branch, provider.head = state.Branch, f.commit
			provider.mu.Unlock()
			return true
		})
	}
	current := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
		return run.Decision != nil && run.Decision.Status == "pending" && e.TurnNumber == 1 && e.CurrentTurnID == e.LastStoppedTurnID
	})
	original := *current.execution.Process
	session := current.execution.SessionID
	if openPR {
		service.stop(t)
		service = startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
		current = waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return run.Decision != nil && run.Decision.Status == "pending" && e.TurnNumber == 1 && e.RecoveryState != "unverified" && e.RecoveryState != ""
		})
	}
	for _, turn := range []uint64{2, 3} {
		submitIntegrationAnswer(t, ctx, f.config, current.run)
		current = waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return run.Decision != nil && run.Decision.Status == "pending" && e.TurnNumber == turn && e.CurrentTurnID == e.LastStoppedTurnID
		})
		if current.execution.Process == nil || !sameNativePhysicalProcess(*current.execution.Process, original) || current.execution.SessionID != session {
			t.Fatal("feedback replaced the original session or physical terminal")
		}
	}
	state, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
	require(t, err)
	if state.Dispatches != 4 || len(state.Tasks) != 4 {
		t.Fatalf("original feedback duplicated or lost its readiness/business dispatch: tasks=%d dispatches=%d", len(state.Tasks), state.Dispatches)
	}
}
