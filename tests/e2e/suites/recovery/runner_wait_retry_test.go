//go:build e2e

package recovery

import (
	"encoding/json"
	"fmt"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	protocol "github.com/erranliu/SteerLine/internal/testkit/protocol"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The peer publishes scripted snapshots, never a native retry implementation.
func TestRunnerWaitRetry(t *testing.T) {
	t.Parallel()
	s := testenv.NewScenario(t, "recovery.runner-wait-retry")
	newProject := func(name string) *testenv.Project {
		p := s.NewProject(name)
		raw, err := os.ReadFile(filepath.Join(p.Home, "execution.json"))
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
		p.Driver.Profile = "controlled-orca"
		s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), config))
		p.PrepareControlledEnvironment("controlled-orca")
		return p
	}
	setWait := func(p *testenv.Project, w *runnercontract.ExecutionWait, hold bool) {
		state, err := controlled.Read(p.Provider)
		s.Must(err)
		state.Hold, state.PreparationWait = hold, w
		s.Must(controlled.Write(p.Provider, state))
	}
	waitFor := func(p *testenv.Project, stage string) api.CandidateSummary {
		var got api.CandidateSummary
		s.Wait(p.Name+" public wait "+stage, func() (bool, error) {
			client, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
			if err != nil {
				return false, err
			}
			defer client.Close()
			snap := client.ReadProject(s.Context, p.Tasks["a"].ID, api.ProjectSnapshot{})
			if snap.Connection.Snapshot.Page == nil {
				return false, nil
			}
			for _, c := range snap.Connection.Snapshot.Page.Runs {
				if c.TaskID != p.Tasks["a"].ID || c.RunnerDelivery == nil || !c.RunnerDelivery.Available {
					continue
				}
				w := c.RunnerDelivery.ExecutionWait
				if stage == "cleared" && w == nil || w != nil && w.Stage == stage {
					got = *c
					return true, nil
				}
			}
			return false, nil
		})
		s.Must(controlled.Write(filepath.Join(s.Output, p.Name+"-api-"+stage+".json"), got))
		return got
	}
	p := newProject("resume")
	p.CreateTasks()
	w := protocol.RunnerExecutionWait(time.Now().UTC())
	setWait(p, w, true)
	p.Claim("a")
	first := waitFor(p, "queue")
	if first.Decision != nil || first.RecoveryRequired || first.RunnerDelivery.ExecutionWait.CallsStarted != 0 {
		t.Fatal("queue demanded resume or invented a call", first)
	}
	requests, _ := json.Marshal(p.CallerRequests())
	p.RestartService()
	after := waitFor(p, "queue")
	if after.RunID != first.RunID || after.ExecutionID != first.ExecutionID || after.AttemptID != first.AttemptID || !after.RunnerDelivery.ExecutionWait.DeadlineAt.Equal(w.DeadlineAt) {
		t.Fatal("restart replaced identity or wait budget")
	}
	w = w.Clone()
	w.Stage, w.ReasonCode, w.CallsStarted = "calling", "orca_query_running", 1
	w.UpdatedAt = time.Now().UTC()
	setWait(p, w, true)
	waitFor(p, "calling")
	w = w.Clone()
	w.Stage, w.ReasonCode = "backoff", "orca_query_transient"
	w.UpdatedAt = time.Now().UTC()
	next := w.UpdatedAt.Add(time.Second)
	w.NextAttemptAt = &next
	setWait(p, w, true)
	backoff := waitFor(p, "backoff")
	if backoff.Decision != nil || backoff.RunnerDelivery.ExecutionWait.CallsStarted != 1 {
		t.Fatal("backoff created a decision or extra call")
	}
	current, _ := json.Marshal(p.CallerRequests())
	if string(current) != string(requests) {
		t.Fatal("observation/restart sent new input")
	}
	setWait(p, nil, true)
	waitFor(p, "cleared")
	client, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	app, err := applicationclient.Open(s.Context, p.Home)
	s.Must(err)
	var activity httpcontract.ActivityPage
	route := fmt.Sprintf("/projects/%s/runs/%s/activity?execution_id=%s&attempt_id=%s&recent=true", client.Identity().ProjectID, first.RunID, first.ExecutionID, first.AttemptID)
	s.Must(app.Get(s.Context, route, &activity))
	s.Must(controlled.Write(filepath.Join(s.Output, "resume-wait-activity.json"), activity))
	waitEvents := 0
	seenEvents := map[string]bool{}
	for _, event := range activity.Page.Activity {
		if event.Kind != "execution_wait" {
			continue
		}
		if seenEvents[event.ItemID] {
			t.Fatal("duplicate wait activity", event.ItemID)
		}
		seenEvents[event.ItemID] = true
		if _, present := event.Meta["execution_wait"]; !present {
			t.Fatal("wait activity lost explicit object/null")
		}
		waitEvents++
	}
	if waitEvents != 4 {
		t.Fatalf("expected queue/calling/backoff/clear activity, got %d", waitEvents)
	}
	client.Close()
	app.Close()
	setWait(p, nil, false)
	p.WaitPhase("waiting_pr_merge")
	p.Merge()
	done := p.WaitPhase("completed")
	p.AssertOneDelivery(done)
	p.Record("completed", done)
	p.WaitStoppedCandidates()
	p.StopService()

	q := newProject("exhaust")
	q.CreateTasks()
	w = protocol.RunnerExecutionWait(time.Now().UTC())
	setWait(q, w, true)
	q.Claim("a")
	waitFor(q, "queue")
	w = w.Clone()
	w.Stage, w.ReasonCode, w.CallsStarted = "exhausted", "query_attempts_exhausted", 5
	w.UpdatedAt = time.Now().UTC()
	setWait(q, w, true)
	exhausted := waitFor(q, "exhausted")
	if !exhausted.RecoveryRequired || exhausted.Decision == nil {
		t.Fatal("exhausted fact lost original recovery eligibility")
	}
	requests, _ = json.Marshal(q.CallerRequests())
	waitFor(q, "exhausted")
	current, _ = json.Marshal(q.CallerRequests())
	if string(requests) != string(current) {
		t.Fatal("exhausted observation auto-resumed")
	}
	project, err := applicationclient.OpenProject(s.Context, q.Home, filepath.Base(q.Config))
	s.Must(err)
	page, err := project.ShowTask(s.Context, q.Tasks["a"].ID)
	s.Must(err)
	_, err = project.DiscardTask(s.Context, api.DiscardInput{TaskID: q.Tasks["a"].ID, ExpectedVersion: page.Tasks[0].Version, RequestID: strings.Repeat("d", 32), Reason: "cancel controlled waiting candidate"})
	// The existing discard guard requires a verified PR binding. Waiting does
	// not bypass that authority or imply that the Runner resource is stopped.
	if err == nil || !strings.Contains(err.Error(), "merge_state_unknown") {
		t.Fatal("unverified discard changed its original qualification", err)
	}
	afterReject := waitFor(q, "exhausted")
	if afterReject.TaskStatus != "in_progress" || afterReject.StopRequested || afterReject.RunID != exhausted.RunID {
		t.Fatal("rejected discard changed candidate authority")
	}
	project.Close()
	q.StopService()
	s.Must(s.FinishBody())
}
