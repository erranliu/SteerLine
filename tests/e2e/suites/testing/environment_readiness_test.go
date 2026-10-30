//go:build e2e

package testing

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectEnvironmentReadiness(t *testing.T) {
	s := testenv.NewScenario(t, "testing.project-environment-readiness")
	p, q := s.NewProject("first"), s.NewProject("second")
	for _, project := range []*testenv.Project{p, q} {
		path := filepath.Join(project.Home, "execution.json")
		raw, err := os.ReadFile(path)
		s.Must(err)
		var config map[string]any
		s.Must(json.Unmarshal(raw, &config))
		profiles := config["execution_profiles"].([]any)
		profile := map[string]any{}
		for key, value := range profiles[0].(map[string]any) {
			profile[key] = value
		}
		profile["id"] = "controlled-orca"
		profile["execution"] = map[string]any{"adapter": "orca-terminal", "target": map[string]string{"kind": "orca_environment", "environment": "controlled-peer"}, "settings": map[string]any{"agent_launch": map[string]any{"executable": "codex"}}}
		config["execution_profiles"] = append(profiles, profile)
		s.Must(controlled.Write(path, config))
		project.WriteControlledEnvironment()
		project.CreateTasks()
		project.StartService()
		project.ClaimExpectedStatus("a", "rejected")
		before, err := project.Snapshot()
		s.Must(err)
		if len(before.Runner.Candidates) != 0 {
			t.Fatal("unprepared project allocated candidate")
		}
		project.Record("unprepared", before)
	}
	p.PrepareControlledEnvironment("controlled")
	q.PrepareControlledEnvironment("controlled-orca")
	// Ready only for the exact project/context combination, through the public query.
	for _, project := range []*testenv.Project{p, q} {
		ready := 0
		for _, option := range project.EnvironmentOptions() {
			if option.Status == "ready" {
				ready++
			}
		}
		if ready != 1 {
			t.Fatal("qualification leaked across project or environment", project.Name, ready)
		}
	}
	p.Driver.Profile = "controlled-orca"
	p.ClaimExpectedStatus("a", "rejected")
	q.ClaimExpectedStatus("a", "rejected")
	p.Driver.Profile = "controlled"
	p.Claim("a")
	open := p.WaitPhase("waiting_pr_merge")
	p.AssertRunnerBinding(open)
	p.Record("qualified", open)
	for _, option := range p.EnvironmentOptions() {
		if option.ID == "controlled" {
			response := p.EnvironmentCommand("environment_invalidate", map[string]any{"environment_key": option.Key, "expected_current_revision": option.CurrentRevision, "reason": "controlled rebuild"})
			if response.Status != "applied" {
				t.Fatal(response)
			}
		}
	}
	// Existing authorization remains usable after invalidation; new Start is denied.
	for _, option := range p.EnvironmentOptions() {
		if option.ID == "controlled" && option.Status == "ready" {
			t.Fatal("revoked context stayed ready")
		}
	}
	var taskVersion uint64
	for _, task := range open.Page.Tasks {
		if task.TaskID == p.Tasks["a"].ID {
			taskVersion = task.Version
		}
	}
	receipt, err := p.Driver.Candidate(s.Context, s.Run.Helper, driver.CandidateCommand{Action: "start_candidate", RequestID: strings.ReplaceAll(uuid.NewString(), "-", ""), TaskID: p.Tasks["a"].ID, TaskVersion: taskVersion, GraphRevision: open.Page.GraphRevision, ProfileID: "controlled"})
	s.Must(err)
	if receipt.Control == nil || receipt.Control.Status != "rejected" || receipt.Control.Code != "environment_not_ready" {
		t.Fatal("new candidate did not reach environment gate", receipt)
	}
	after, err := p.Snapshot()
	s.Must(err)
	if len(after.Runner.Candidates) != 1 || after.Runner.Candidates[0].ID != open.Runner.Candidates[0].ID {
		t.Fatal("invalidation created or replaced a candidate")
	}
	p.Merge()
	done := p.WaitPhase("completed")
	p.Record("delivered-after-invalidation", done)
	p.AssertClosedDeliveryQuiescent(done)
	q.Driver.Profile = "controlled-orca"
	q.Claim("a")
	other := q.WaitPhase("waiting_pr_merge")
	q.AssertRunnerBinding(other)
	q.Record("other-context-still-ready", other)
	q.Merge()
	other = q.WaitPhase("completed")
	q.AssertClosedDeliveryQuiescent(other)
	p.WaitStoppedCandidates()
	q.WaitStoppedCandidates()
	p.StopService()
	q.StopService()
	s.Must(controlled.Write(filepath.Join(s.Output, "readiness-proof.json"), map[string]any{"projects": 2, "contexts": 2, "missing_rejected": true, "isolated": true, "invalidated_rejected": true, "original_delivery": true, "candidates": 2}))
}
