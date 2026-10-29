package testenv

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/google/uuid"
)

// PrepareControlledRemoteEnvironment prepares versioned fixture files and uses
// the public adaptation command. It never writes readiness into caller storage.
func (p *Project) PrepareControlledRemoteEnvironment(profile string) {
	s := p.Scenario
	s.t.Helper()
	if p.Chain != "" || s.Run.Remote != nil {
		s.t.Fatal("controlled environment helper cannot prepare a live project")
	}
	const base = "fixture-environment-validation"
	const script = "scripts/verify-environment.ps1"
	rules, err := prompts.EnvironmentExecutionRules()
	s.Must(err)
	for path, body := range map[string][]byte{
		env.DocumentPath:                   []byte("# Controlled environment\n```steerline-environment\n{\"version\":1,\"script\":\"" + script + "\",\"interpreter\":\"pwsh\",\"validation_base\":\"" + base + "\",\"dependencies\":[]}\n```\n"),
		script:                             []byte("throw 'Scripted public peer fixture; native execution is forbidden'\n"),
		env.ExecutionRulesPath:             rules,
		"SteerlineFlow/plans/task.plan.md": []byte("# Controlled recovery plan\nNo automated tests are required for this task. The catalogue's fixture.environment entry belongs to the separate scripted environment prerequisite. Recovery assertions are owned by the external E2E harness.\n"),
		testmanagement.CataloguePath:       []byte(`{"tests":[{"id":"fixture.environment","name":"Controlled environment prerequisite","level":"L2","environment":"controlled","command":"controlled peer assertion","check":"scripted sealed report"}]}`),
	} {
		target := filepath.Join(p.Repo, path)
		s.Must(os.MkdirAll(filepath.Dir(target), 0700))
		s.Must(os.WriteFile(target, body, 0600))
	}
	p.git(p.Repo, "add", ".")
	p.git(p.Repo, "commit", "--quiet", "-m", "Prepare controlled environment admission")
	provider, err := controlled.Read(p.Provider)
	s.Must(err)
	p.git(p.Repo, "push", "--quiet", provider.GitURL, "main", "HEAD:refs/heads/"+base)
	commit := p.git(p.Repo, "rev-parse", "HEAD")
	p.StartService()
	project, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	defer project.Close()
	client, err := applicationclient.Open(s.Context, p.Home)
	s.Must(err)
	defer client.Close()
	identity := project.Identity()
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	payload, _ := json.Marshal(env.Start{Profile: profile, TimeoutSeconds: 90})
	request := httpcontract.CommandEnvelope{RequestID: id, ProjectID: identity.ProjectID, InstanceID: identity.InstanceID, Action: "environment_start", TargetID: string(identity.ProjectID), ExpectedVersion: 1, Payload: payload}
	prefix := "/projects/" + string(identity.ProjectID)
	var response httpcontract.CommandResponse
	s.Must(client.Post(s.Context, prefix+"/commands", request, &response))
	if response.Status != "applied" {
		s.t.Fatalf("controlled adaptation rejected: %s %s", response.Code, response.Diagnostic)
	}
	var view struct {
		Status, Diagnostic string
		Settled            bool
		Rounds             []struct {
			RunID  string `json:"run_id"`
			Result *runnercontract.ResultFile
			Report *env.Report
		}
	}
	s.Wait("controlled environment sealed and ready", func() (bool, error) {
		err := client.Get(s.Context, prefix+"/environment/"+id, &view)
		if err == nil && view.Settled && view.Status != "ready" {
			s.t.Fatalf("controlled adaptation failed: %s", view.Diagnostic)
		}
		return err == nil && view.Settled && view.Status == "ready", err
	})
	if len(view.Rounds) != 1 || view.Rounds[0].Result == nil || view.Rounds[0].Report == nil {
		s.t.Fatal("missing original adaptation result")
	}
	round := view.Rounds[0]
	_, err = runnerclient.ValidateResultFile(*round.Result, round.RunID)
	s.Must(err)
	raw, err := p.Driver.RunHelper(s.Context, s.Run.Helper, "inspect", "--state-dir", runnerstatepath.Directory(p.Home), "--run-id", round.RunID, "--json")
	s.Must(err)
	var observed struct {
		Data struct{ Run runnercontract.Run }
	}
	s.Must(json.Unmarshal(raw, &observed))
	if round.Result.CollectionStatus != "closed" || round.Result.Version != 1 || observed.Data.Run.StopStatus != "stopped" || round.Report.Commit != commit || !round.Report.Passed(base, observed.Data.Run.Execution.Branch) {
		s.t.Fatal("unverified controlled adaptation provenance")
	}
	s.Must(controlled.Write(filepath.Join(s.Output, p.Name+"-environment-proof.json"), view))
}
