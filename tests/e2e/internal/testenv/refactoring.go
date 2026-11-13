package testenv

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/internal/executionconfig"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

func RefactoringMaintenance(s *Scenario) {
	const profile = "controlled"
	const agentModel, runnerModel = "fixture-steer-agent", "fixture-runner"
	p := s.NewProject("refactoring")
	configPath := filepath.Join(p.Home, "execution.json")
	configBytes, e := os.ReadFile(configPath)
	s.Must(e)
	var executionConfig executionconfig.File[executionconfig.Pin]
	s.Must(executionconfig.Decode(configBytes, &executionConfig))
	executionConfig.SteerAgent = &executionconfig.SteerAgent{Model: agentModel, ReasoningEffort: "medium"}
	var runnerSettings map[string]string
	s.Must(json.Unmarshal(executionConfig.Profiles[0].Execution.Settings, &runnerSettings))
	runnerSettings["model"], runnerSettings["reasoning_effort"] = runnerModel, "high"
	executionConfig.Profiles[0].Execution.Settings, e = json.Marshal(runnerSettings)
	s.Must(e)
	s.Must(controlled.Write(configPath, executionConfig))
	p.PrepareControlledEnvironment(profile)
	p.CreateTasks()
	project, e := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(e)
	identity := project.Identity()
	project.Close()
	client, e := applicationclient.Open(s.Context, p.Home)
	s.Must(e)
	defer client.Close()
	prefix := "/projects/" + identity.ProjectID
	directionsDir := filepath.Join(p.Home, "etc", "refactoring-directions")
	s.Must(os.MkdirAll(directionsDir, 0700))
	template := filepath.Join(directionsDir, "shared-fixtures.md")
	const directionPrompt = "# 收拢共享夹具\r\n\r\n优先处理共享测试夹具。  \r\n"
	s.Must(os.WriteFile(template, []byte(directionPrompt), 0600))
	var record struct {
		ID              string
		Revision        uint64
		Phase           string
		Round           int
		Diagnostic      string
		Reports         []ref.Report
		Operations      []ref.Operation
		PR              *struct{ URL string }
		Question        *ref.Question
		DirectionName   string `json:"direction_name"`
		DirectionPrompt string `json:"direction_prompt"`
	}
	stale := ref.Start{Direction: "custom:shared-fixtures", DirectionRevision: ref.Digest([]byte("old")), Profile: profile, Strategy: "cautious", Budget: ref.Budget{Rounds: 1}}
	body, e := json.Marshal(stale)
	s.Must(e)
	staleRequest := httpcontract.CommandEnvelope{RequestID: ref.ID(), ProjectID: identity.ProjectID, InstanceID: identity.InstanceID, Action: "refactoring_start", TargetID: identity.RepositoryIdentity, ExpectedVersion: 1, Payload: body}
	var rejected httpcontract.CommandResponse
	s.Must(client.Post(s.Context, prefix+"/commands", staleRequest, &rejected))
	if rejected.Status != "rejected" || rejected.Code != "refactoring_direction_changed" || rejected.Diagnostic != "重构提示词已更新，请刷新并查看最新正文后重新启动" {
		s.t.Fatal("stale preview did not reject before launch", rejected)
	}
	readiness := func(want string) {
		s.t.Helper()
		s.Wait("maintenance environment "+want, func() (bool, error) {
			var view struct {
				Project     struct{ Status string }
				Environment string
				Reason      string
			}
			err := client.Get(s.Context, prefix+"/refactoring/readiness?profile="+profile, &view)
			if err == nil && view.Project.Status == "ready" && view.Environment == want {
				if want == "not_ready" && !strings.Contains(view.Reason, "controlled qualification revoked") {
					s.t.Fatal("missing environment reason", view)
				}
				return true, nil
			}
			return false, err
		})
	}
	readiness("ready")
	var current EnvironmentOption
	for _, option := range p.EnvironmentOptions() {
		if option.ID == profile {
			current = option
			break
		}
	}
	if current.Key == "" || current.Status != "ready" {
		s.t.Fatal("missing the qualification to revoke", current)
	}
	invalidated := p.EnvironmentCommand("environment_invalidate", map[string]any{
		"environment_key": current.Key, "expected_current_revision": current.CurrentRevision, "reason": "controlled qualification revoked",
	})
	if invalidated.Status != "applied" {
		s.t.Fatal("could not revoke the observed qualification", invalidated)
	}
	fixturePath := filepath.Join(runnerstatepath.Directory(p.Home), "caller-fixture.json")
	before, e := os.ReadFile(fixturePath)
	s.Must(e)
	stale.DirectionRevision = ref.Digest([]byte(directionPrompt))
	staleRequest.RequestID = ref.ID()
	staleRequest.Payload, e = json.Marshal(stale)
	s.Must(e)
	s.Must(client.Post(s.Context, prefix+"/commands", staleRequest, &rejected))
	if rejected.Status != "rejected" || !strings.Contains(rejected.Diagnostic, "controlled qualification revoked") {
		s.t.Fatal("the start command accepted a revoked qualification", rejected)
	}
	readiness("not_ready")
	var empty struct{ Records []json.RawMessage }
	s.Must(client.Get(s.Context, prefix+"/refactoring", &empty))
	after, e := os.ReadFile(fixturePath)
	s.Must(e)
	var beforeRunner, afterRunner controlled.RunnerFixture
	s.Must(json.Unmarshal(before, &beforeRunner))
	s.Must(json.Unmarshal(after, &afterRunner))
	if len(empty.Records) != 0 || !maps.Equal(beforeRunner.SubmitCalls, afterRunner.SubmitCalls) {
		s.t.Fatal("a revoked qualification created an action or submitted a Runner request")
	}
	p.ValidateDeclaredControlledEnvironment(profile)
	readiness("ready")
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
	browser := func(id, mode string) {
		cmd := p.Command(node, filepath.Join(s.Run.Root, "web", "node_modules", "@playwright", "test", "cli.js"), "test", "tests/refactoring-maintenance.spec.ts")
		cmd.Dir = filepath.Join(s.Run.Root, "web")
		cmd.Env = append(cmd.Env, "STEERLINE_MAINTENANCE_WEB="+web, "STEERLINE_MAINTENANCE_HOME="+p.Home, "STEERLINE_MAINTENANCE_PROJECT="+identity.ProjectID, "STEERLINE_MAINTENANCE_ID="+id, "STEERLINE_MAINTENANCE_MODE="+mode, "STEERLINE_WEB_EVIDENCE="+out)
		_, err := s.RunCommand(cmd, "maintenance-browser-"+id)
		s.Must(err)
	}
	browser("", "start")
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
	if record.Question == nil || record.DirectionName != "收拢共享夹具" || record.DirectionPrompt != directionPrompt {
		s.t.Fatal("missing question or direction snapshot", record)
	}
	s.Must(os.WriteFile(template, []byte("# 新模板\n\n更新后的内容。\n"), 0600))
	// GETs and command replay must not create work or refresh identities.
	var replay httpcontract.CommandResponse
	s.Must(client.Post(s.Context, prefix+"/commands", request, &replay))
	if replay.Status != "applied" {
		s.t.Fatal(replay)
	}
	browser(record.ID, "answer")
	wait("delivered")
	if record.Round != 2 || len(record.Reports) != 1 || record.PR == nil || record.Reports[0].Recommendation == "" {
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
		if o.RunID == "" {
			continue
		}
		run, ok := fixture.Runs[o.RunID]
		if !ok {
			s.t.Fatal("missing Runner operation", o.RunID)
		}
		var submitted struct{ Execution runnercontract.ExecutionInput }
		s.Must(json.Unmarshal([]byte(fixture.Bodies[strings.TrimPrefix(o.RunID, "run-")]), &submitted))
		var settings struct {
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoning_effort"`
			AccessMode      string `json:"access_mode"`
		}
		s.Must(json.Unmarshal(submitted.Execution.Settings, &settings))
		if run.Workflow.ID == "generate" {
			if settings.Model != agentModel || settings.ReasoningEffort != "medium" || settings.AccessMode != "read_only" {
				s.t.Fatalf("decision %s used the wrong execution settings: %+v", o.RunID, settings)
			}
			if run.RepositorySource != nil || runnerbinding.SamePath(run.Execution.Workspace, p.Repo) {
				s.t.Fatalf("decision %s received the live project directory: %q", o.RunID, run.Execution.Workspace)
			}
			if o.Purpose != "first" && !runnerbinding.SamePath(run.Execution.Workspace, filepath.Join(runnerstatepath.Directory(p.Home), "runs", o.RunID, "content")) {
				s.t.Fatal("later decision did not use isolated content directory", o.Purpose, run.Execution.Workspace)
			}
			if len(run.Materials) != 3 || run.Materials[1].Path != "maintenance/context.json" {
				s.t.Fatal("missing decision context", run.Materials)
			}
			contextRaw, err := os.ReadFile(run.Materials[1].SnapshotPath)
			s.Must(err)
			var decisionContext struct {
				DirectionPrompt string `json:"direction_prompt"`
				PR              *runnercontract.PRFact
			}
			s.Must(json.Unmarshal(contextRaw, &decisionContext))
			if decisionContext.DirectionPrompt != directionPrompt {
				s.t.Fatal("decision lost original direction after template edit", o.RunID)
			}
			if o.Purpose == "report" {
				version := decisionContext.PR
				report := record.Reports[0]
				if version == nil || version.CheckedAt.IsZero() || version.URL != report.PR || version.HeadRevision != report.Head || version.BaseRevision != report.Base {
					s.t.Fatal("report received an unverified PR version", version, report)
				}
			}
			continue
		}
		if run.Workflow.ID != "execute" {
			continue
		}
		if settings.Model != runnerModel || settings.ReasoningEffort != "high" || settings.AccessMode != "full_access" {
			s.t.Fatalf("write %s used the wrong execution settings: %+v", o.RunID, settings)
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
	provider, e := controlled.Read(fixture.Providers[latest])
	s.Must(e)
	if provider.PRs[original].Body != "Original PR description" {
		s.t.Fatal("maintenance review modified the PR body")
	}
	for _, n := range fixture.SubmitCalls {
		if n != 1 {
			s.t.Fatal("submit repeated")
		}
	}
	command("refactoring_conflict", record.ID, record.Revision, struct{}{})
	wait("delivered")
	if len(record.Reports) != 2 || record.PR.URL != original || record.Round != 2 || record.DirectionPrompt != directionPrompt {
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
	browser(record.ID, "history")
	s.Must(controlled.Write(filepath.Join(s.Output, "refactoring-proof.json"), map[string]any{"rounds": 2, "executes": writes, "same_workspace": true, "original_pr": true, "independent_audit": true, "browser": true, "direction_template": true, "head": record.Reports[1].Head, "pr": original}))
	s.Must(s.FinishBody())
}
