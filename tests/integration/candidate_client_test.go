package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	app "github.com/erranliu/SteerLine/internal/app"
	codex "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

// Keep candidate admission, original-request replay and scoped feedback coverage
// through the public client after retiring the terminal UI.
func TestCandidateClientWiring(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	f := newLifecycleProject(t, "local")
	var profiles []any
	for i := 0; i < 2; i++ {
		payload, err := codex.EncodeProfileConfig(codex.ProfileConfig{Executable: os.Args[0], Model: fmt.Sprintf("fixture-model-%d", i)})
		require(t, err)
		profiles = append(profiles, map[string]any{"id": fmt.Sprintf("p%d", i), "name": fmt.Sprintf("隔离方案%d", i), "host": "local", "agent": "codex", "implementation_configs": []backend.ImplementationConfigInput{{ImplementationKey: codex.ProfileCodecKey, CodecVersion: 2, Payload: payload}}})
	}
	require(t, os.WriteFile(app.GlobalExecutionConfigPath(), mustJSON(map[string]any{"version": 2, "default_execution_profile": "p0", "execution_profiles": profiles}), 0600))
	var err error
	f.config, err = app.LoadConfig(f.configPath)
	require(t, err)
	createIntegrationTask(t, ctx, f, 901)
	svc := startIntegrationService(t, ctx, f.config, &integrationLifecycleResolver{})
	defer svc.stop(t)
	client, err := app.OpenClient(ctx, f.configPath)
	require(t, err)
	defer client.Close()
	submit := func(action api.ControlAction, payload api.ControlPayload) api.ControlInput {
		t.Helper()
		snap := client.ReadProject(ctx, "901", api.ProjectSnapshot{})
		if snap.Connection.Snapshot.Page == nil {
			t.Fatal("service snapshot unavailable")
		}
		revision := snap.Connection.Snapshot.Page.Control.Revision
		in := api.ControlInput{Action: action, RequestID: newID(), InstanceID: svc.instanceID, ExpectedRevision: &revision, Payload: payload, RequestedBy: "operator"}
		_, err := client.SubmitControl(ctx, in)
		require(t, err)
		return in
	}
	waitApplied := func(in api.ControlInput) {
		t.Helper()
		receipt, err := client.WaitControl(ctx, in.RequestID, 100*time.Millisecond)
		require(t, err)
		if receipt.Status != "applied" {
			t.Fatalf("%s: %+v", in.Action, receipt)
		}
	}
	var primary *api.CandidateSummary
	var second api.ControlInput
	accepted := client.Project().Presentation.Profiles
	for i := 0; i < 2; i++ {
		profile := accepted[i]
		payload := api.ControlPayload{TaskID: "901", ExecutionProfileID: profile.ID, ExecutionProfileDigest: profile.Digest}
		if i > 0 {
			page, err := client.ShowTask(ctx, "901")
			require(t, err)
			payload.ExpectedTaskVersion = page.Tasks[0].Version
			payload.ExpectedGraphRevision = page.GraphRevision
		}
		action := api.StartCandidate
		if i == 0 {
			action = api.StartTask
		}
		in := submit(action, payload)
		waitApplied(in)
		if i == 1 {
			second = in
		}
		if i == 0 {
			waitFor(t, 2*time.Minute, func() bool {
				snap := client.ReadProject(ctx, "901", api.ProjectSnapshot{})
				if snap.Observation.Page == nil {
					return false
				}
				for _, run := range snap.Observation.Page.Runs {
					if run.Decision != nil && run.SessionID != "" {
						primary = run
						return true
					}
				}
				return false
			})
		}
	}
	request, err := client.Request(ctx, second.RequestID)
	require(t, err)
	var payload api.ControlPayload
	require(t, json.Unmarshal(request.Payload, &payload))
	if payload.TaskID != "901" || payload.ExecutionProfileID != "p1" || payload.ExpectedTaskVersion == 0 || payload.ExpectedGraphRevision == 0 {
		t.Fatalf("candidate start lost fixed identity: %+v", payload)
	}
	_, err = client.SubmitControl(ctx, second)
	require(t, err)
	waitApplied(second)
	batch, err := client.RecentActivity(ctx, primary.RunID, primary.ExecutionID, primary.AttemptID)
	require(t, err)
	if len(batch.Activity) > 10 || batch.Cursor.ExecutionID != primary.ExecutionID || batch.Cursor.AttemptID != primary.AttemptID {
		t.Fatalf("activity lost original scope: %+v", batch)
	}
	snap := client.ReadProject(ctx, "901", api.ProjectSnapshot{})
	for _, run := range snap.Observation.Page.Runs {
		if run.RunID == primary.RunID {
			primary = run
		}
	}
	answerID := newID()
	originalDecision, originalTurn := primary.Decision.ID, primary.TurnNumber
	receipt, _, err := client.SubmitDecision(ctx, api.DecisionInput{CommandID: answerID, RequestedBy: "operator", Answer: api.DecisionAnswer{DecisionID: primary.Decision.ID, RunID: primary.RunID, OptionID: primary.Decision.Request.Options[0].ID, Note: "精确原候选：原地中文\n第二行", ExpectedDecisionVersion: primary.Decision.Version, ExpectedRunRevision: primary.ControlRevision, SourceAttemptID: primary.AttemptID, ExpectedRecoveryVersion: primary.RecoveryVersion, ServiceInstanceID: svc.instanceID}})
	require(t, err)
	if receipt.Status == "rejected" {
		t.Fatalf("feedback rejected: %+v", receipt)
	}
	waitFor(t, time.Minute, func() bool {
		s := client.ReadProject(ctx, "901", api.ProjectSnapshot{})
		if s.Observation.Page == nil {
			return false
		}
		for _, run := range s.Observation.Page.Runs {
			if run.RunID == primary.RunID && run.TurnNumber > originalTurn && run.Decision != nil && run.Decision.ID != originalDecision && run.Decision.Status == "pending" {
				primary = run
				return true
			}
		}
		return false
	})
	waitApplied(submit(api.StopCandidate, api.ControlPayload{TaskID: "901", RunID: primary.RunID, ExpectedRunRevision: primary.ControlRevision}))
	waitFor(t, 30*time.Second, func() bool {
		s := client.ReadProject(ctx, "901", api.ProjectSnapshot{})
		if s.Observation.Page == nil {
			return false
		}
		if len(s.Observation.Page.Runs) != 2 {
			t.Fatal("original request replay changed candidate count")
		}
		for _, run := range s.Observation.Page.Runs {
			if run.RunID == primary.RunID {
				return run.WorkflowStatus == "abandoned" && run.CandidateStopReason == "manual"
			}
		}
		return false
	})
}
