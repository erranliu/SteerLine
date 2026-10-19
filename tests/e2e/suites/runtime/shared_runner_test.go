//go:build e2e

package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
)

func TestSharedRunnerProjects(t *testing.T) {
	t.Parallel()
	s := testenv.NewScenario(t, "runtime.shared-runner-projects")
	a, b := s.NewProject("a"), s.NewProject("b")
	raw, err := os.ReadFile(b.Config)
	s.Must(err)
	b.Home = a.Home
	b.Config = filepath.Join(a.Home, "etc", "Secondary.json")
	b.Driver.Home, b.Driver.Config = b.Home, b.Config
	s.Must(os.WriteFile(b.Config, raw, 0600))
	s.Must(controlled.Write(filepath.Join(a.Home, "fixture-routes.json"), map[string]string{a.Repo: a.Provider, b.Repo: b.Provider}))
	home, err := b.Driver.Run(s.Context, "home", "-config", b.Config)
	s.Must(err)
	for _, line := range strings.Split(string(home), "\n") {
		if strings.HasPrefix(line, "run_dir ") {
			b.Database = filepath.Join(strings.TrimSpace(strings.TrimPrefix(line, "run_dir")), "steerline.db")
		}
	}
	a.CreateTasks()
	b.Service = a.Service
	b.CreateTasks()
	requestID := strings.Repeat("5", 32)
	for _, p := range []*testenv.Project{a, b} {
		s.Must(p.Driver.Control(s.Context, s.Run.Helper, "resume_dispatch", ""))
		snap, err := p.Snapshot()
		s.Must(err)
		var version uint64
		for _, task := range snap.Page.Tasks {
			if task.TaskID == p.Tasks["a"].ID {
				version = task.Version
			}
		}
		receipt, err := p.Driver.Candidate(s.Context, s.Run.Helper, driver.CandidateCommand{Action: "start_task", RequestID: requestID, TaskID: p.Tasks["a"].ID, TaskVersion: version, GraphRevision: snap.Page.GraphRevision, ProfileID: "controlled"})
		s.Must(err)
		if receipt.Control == nil || receipt.Control.Status != "applied" {
			t.Fatal("shared project start failed", receipt)
		}
	}
	first, second := a.WaitPhase("waiting_pr_merge"), b.WaitPhase("waiting_pr_merge")
	x, y := first.Runner.Candidates[0], second.Runner.Candidates[0]
	if first.Page.ProjectID == second.Page.ProjectID || first.Page.DatabaseID == second.Page.DatabaseID || x.ID != requestID || y.ID != requestID || x.Submit.RequestID == y.Submit.RequestID || x.RunID == y.RunID || x.StateDir != y.StateDir {
		t.Fatal("shared service crossed project/request identity")
	}
	if first.Runtime.Service.PID != second.Runtime.Service.PID || first.Runtime.Service.InstanceID == second.Runtime.Service.InstanceID {
		t.Fatal("project instances and shared host conflated")
	}
	if x.Projection.Correlation["steerline_project"] != first.Page.ProjectID || y.Projection.Correlation["steerline_project"] != second.Page.ProjectID || x.Projection.Execution.Workspace == y.Projection.Execution.Workspace {
		t.Fatal("Run repository/correlation crossed projects")
	}
	if x.Projection.PR == nil || y.Projection.PR == nil || x.Projection.PR.HeadBranch == y.Projection.PR.HeadBranch || first.Helper.Requests[x.Submit.RequestID] != "submit" || first.Helper.Requests[y.Submit.RequestID] != "" || second.Helper.Requests[y.Submit.RequestID] != "submit" || second.Helper.Requests[x.Submit.RequestID] != "" {
		t.Fatal("PR or provider receipts crossed projects")
	}
	a.Record("shared-before", first)
	b.Record("shared-before", second)
	requests, _ := json.Marshal(a.CallerRequests())
	a.StopService()
	b.Service = nil
	a.StartService()
	b.Service = a.Service
	a.Record("shared-after", a.WaitPhase("waiting_pr_merge"))
	b.Record("shared-after", b.WaitPhase("waiting_pr_merge"))
	after, _ := json.Marshal(a.CallerRequests())
	if string(requests) != string(after) {
		t.Fatal("host restart submitted new business input")
	}
	a.StopService()
	b.Service = nil
	s.Must(s.FinishBody())
}
