package testenv

import (
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"os"
	"os/exec"
	"path/filepath"
)

func RefactoringMaintenance(s *Scenario) {
	p := s.NewProject("refactoring")
	p.PrepareControlledEnvironment("controlled")
	p.CreateTasks()
	project, e := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(e)
	identity := project.Identity()
	project.Close()
	client, e := applicationclient.Open(s.Context, p.Home)
	s.Must(e)
	defer client.Close()
	prefix := "/projects/" + identity.ProjectID
	var record struct {
		ID         string
		Revision   uint64
		Phase      string
		Round      int
		Diagnostic string
		Reports    []ref.Report
		Operations []ref.Operation
		PR         *struct{ URL string }
		Question   *ref.Question
	}
	command := func(action, target string, revision uint64, payload any) httpcontract.CommandEnvelope {
		body, e := json.Marshal(payload)
		s.Must(e)
		request := httpcontract.CommandEnvelope{RequestID: ref.ID(), ProjectID: identity.ProjectID, InstanceID: identity.InstanceID, Action: action, TargetID: target, ExpectedVersion: revision, Payload: body}
		var response httpcontract.CommandResponse
		s.Must(client.Post(s.Context, prefix+"/commands", request, &response))
		if response.Status != "applied" {
			s.t.Fatalf("%s: %s", action, response.Diagnostic)
		}
		return request
	}
	web := startAdvisoryWeb(s, p)
	node, e := exec.LookPath("node")
	s.Must(e)
	out := filepath.Join(s.Output, "browser")
	s.Must(os.MkdirAll(out, 0700))
	browser := func(id string) {
		cmd := p.Command(node, filepath.Join(s.Run.Root, "web", "node_modules", "@playwright", "test", "cli.js"), "test", "tests/refactoring-maintenance.spec.ts")
		cmd.Dir = filepath.Join(s.Run.Root, "web")
		cmd.Env = append(cmd.Env, "STEERLINE_MAINTENANCE_WEB="+web, "STEERLINE_MAINTENANCE_HOME="+p.Home, "STEERLINE_MAINTENANCE_PROJECT="+identity.ProjectID, "STEERLINE_MAINTENANCE_ID="+id, "STEERLINE_WEB_EVIDENCE="+out)
		_, err := s.RunCommand(cmd, "maintenance-browser-"+id)
		s.Must(err)
	}
	browser("")
	var request httpcontract.CommandEnvelope
	raw, e := os.ReadFile(filepath.Join(out, "start.json"))
	s.Must(e)
	s.Must(json.Unmarshal(raw, &request))
	if request.Action != "refactoring_start" || request.ProjectID != identity.ProjectID || request.InstanceID != identity.InstanceID || request.TargetID != identity.RepositoryIdentity {
		s.t.Fatal("browser start identity mismatch", request)
	}
	wait := func(phase string) {
		s.Wait("maintenance "+phase, func() (bool, error) {
			e := client.Get(s.Context, prefix+"/refactoring/"+request.RequestID, &record)
			if e == nil && record.Phase == "blocked" {
				s.t.Fatal(record.Diagnostic)
			}
			return e == nil && record.Phase == phase, e
		})
	}
	wait("waiting_input")
	if record.Question == nil {
		s.t.Fatal("missing question")
	}
	// GETs and command replay must not create work or refresh identities.
	var replay httpcontract.CommandResponse
	s.Must(client.Post(s.Context, prefix+"/commands", request, &replay))
	if replay.Status != "applied" {
		s.t.Fatal(replay)
	}
	command("refactoring_answer", record.ID, record.Revision, map[string]string{"question_id": record.Question.ID, "answer": "保留公共接口"})
	wait("delivered")
	if record.Round != 2 || len(record.Reports) != 1 || record.PR == nil {
		s.t.Fatal("delivery missing", record)
	}
	original := record.PR.URL
	var fixture controlled.RunnerFixture
	raw, e = os.ReadFile(filepath.Join(runnerstatepath.Directory(p.Home), "caller-fixture.json"))
	s.Must(e)
	s.Must(json.Unmarshal(raw, &fixture))
	writes := 0
	latest := ""
	identityKey := ""
	origin := ""
	for _, o := range record.Operations {
		run, ok := fixture.Runs[o.RunID]
		if !ok || run.Workflow.ID != "execute" {
			continue
		}
		writes++
		r := &run
		if r.ReuseWorktreeFromRunID != latest || latest != "" && (r.WorkspaceIdentity != identityKey || r.WorkspaceOriginRunID != origin) {
			s.t.Fatal("broken worktree ownership", r)
		}
		latest = r.RunID
		identityKey = r.WorkspaceIdentity
		origin = r.WorkspaceOriginRunID
	}
	if writes != 4 {
		s.t.Fatalf("first, repair, second, publication: got %d", writes)
	}
	for _, n := range fixture.SubmitCalls {
		if n != 1 {
			s.t.Fatal("submit repeated")
		}
	}
	command("refactoring_conflict", record.ID, record.Revision, struct{}{})
	wait("delivered")
	if len(record.Reports) != 2 || record.PR.URL != original || record.Round != 2 {
		s.t.Fatal("conflict changed original action/PR")
	}
	var activities api.AsyncActivities
	s.Must(client.Get(s.Context, prefix+"/activities?filter=recent", &activities))
	found := false
	for _, a := range activities.Items {
		if a.Source == "refactoring" && a.SourceID == record.ID {
			found = a.SteerAgent != nil && a.SteerAgent.Type == "maintenance" && a.Details.Target == record.ID
		}
	}
	if !found {
		s.t.Fatal("missing maintenance activity")
	}
	browser(record.ID)
	s.Must(controlled.Write(filepath.Join(s.Output, "refactoring-proof.json"), map[string]any{"rounds": 2, "executes": writes, "same_workspace": true, "original_pr": true, "independent_audit": true, "browser": true, "head": record.Reports[1].Head, "pr": original}))
	s.Must(s.FinishBody())
}
