package testenv

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/google/uuid"
)

// PrepareControlledEnvironment prepares versioned fixture files and uses
// the public adaptation command. It never writes readiness into caller storage.
func (p *Project) WriteControlledEnvironment() string {
	s := p.Scenario
	s.t.Helper()
	if p.Chain != "" || s.Run.Remote != nil {
		s.t.Fatal("controlled environment helper cannot prepare a live project")
	}
	script := env.ScriptPath("ps1")
	manifest, err := json.Marshal(env.Manifest{Version: 6, Requirements: map[string]string{}, Script: script, Interpreter: "powershell.exe", Dependencies: []string{}})
	s.Must(err)
	files := map[string][]byte{
		env.DocumentPath(): []byte("# Controlled environment\n```steerline-environment\n" + string(manifest) + "\n```\n"),
		script:             []byte("throw 'Scripted public peer fixture; native execution is forbidden'\n"),
	}
	for path, body := range files {
		target := filepath.Join(p.Repo, path)
		s.Must(os.MkdirAll(filepath.Dir(target), 0700))
		s.Must(os.WriteFile(target, body, 0600))
	}
	p.git(p.Repo, "add", ".")
	if p.git(p.Repo, "status", "--porcelain") != "" {
		p.git(p.Repo, "commit", "--quiet", "-m", "Prepare controlled environment admission")
	}
	provider, err := controlled.Read(p.Provider)
	s.Must(err)
	p.git(p.Repo, "push", "--quiet", provider.GitURL, "main")
	return p.git(p.Repo, "rev-parse", "HEAD")
}

func (p *Project) PrepareControlledEnvironment(profile string) {
	p.validateControlledEnvironment(profile, p.WriteControlledEnvironment())
}

func (p *Project) ValidateDeclaredControlledEnvironment(profile string) {
	p.validateControlledEnvironment(profile, p.git(p.Repo, "rev-parse", "HEAD"))
}

func (p *Project) validateControlledEnvironment(profile, commit string) {
	s := p.Scenario
	script, err := os.ReadFile(filepath.Join(p.Repo, env.ScriptPath("ps1")))
	s.Must(err)
	expectedScriptDigest := env.Digest(script)
	p.StartService()
	project, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	defer project.Close()
	client, err := applicationclient.Open(s.Context, p.Home)
	s.Must(err)
	defer client.Close()
	identity := project.Identity()
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	revision := uint64(0)
	for _, option := range p.EnvironmentOptions() {
		if option.ID == profile {
			revision = option.CurrentRevision
		}
	}
	payload, _ := json.Marshal(env.Start{EnvironmentKey: p.environmentKey(profile), Profile: profile, TimeoutSeconds: 90, ExpectedCurrentRevision: revision})
	request := httpcontract.CommandEnvelope{RequestID: id, ProjectID: identity.ProjectID, InstanceID: identity.InstanceID, Action: "environment_start", TargetID: string(identity.ProjectID), ExpectedVersion: 1, Payload: payload}
	prefix := "/projects/" + string(identity.ProjectID)
	var response httpcontract.CommandResponse
	s.Must(client.Post(s.Context, prefix+"/commands", request, &response))
	if response.Status != "applied" {
		s.t.Fatalf("controlled adaptation rejected: %s %s", response.Code, response.Diagnostic)
	}
	var view struct {
		ScriptDigest       string `json:"script_digest"`
		Status, Diagnostic string
		Settled            bool
		Rounds             []struct {
			RunID  string `json:"run_id"`
			Result *runnercontract.ResultFile
			Report *env.Report
		}
	}
	s.Wait("project environment sealed and ready", func() (bool, error) {
		err := client.Get(s.Context, prefix+"/environment/"+id, &view)
		if err == nil && view.Settled && view.Status != "ready" {
			s.t.Fatalf("controlled adaptation failed: %s", view.Diagnostic)
		}
		return err == nil && view.Settled && view.Status == "ready", err
	})
	if len(view.Rounds) != 1 || view.Rounds[0].Result == nil || view.Rounds[0].Report == nil {
		s.t.Fatal("missing original adaptation result")
	}
	if expectedScriptDigest != "" && view.ScriptDigest != expectedScriptDigest {
		s.t.Fatal("adaptation did not consume the generated script")
	}
	round := view.Rounds[0]
	_, err = runnerclient.ValidateResultFile(*round.Result, round.RunID)
	s.Must(err)
	var raw []byte
	if p.Chain != "" {
		command := exec.CommandContext(s.Context, s.Run.Runner, "inspect", "--state-dir", runnerstatepath.Directory(p.Home), "--run-id", round.RunID, "--json")
		command.Env = p.Driver.Environment(os.Environ())
		raw, err = command.CombinedOutput()
	} else {
		raw, err = p.Driver.RunHelper(s.Context, s.Run.Helper, "inspect", "--state-dir", runnerstatepath.Directory(p.Home), "--run-id", round.RunID, "--json")
	}
	s.Must(err)
	var observed struct {
		Data struct{ Run runnercontract.Run }
	}
	s.Must(json.Unmarshal(raw, &observed))
	// Preserve the original public result even when its binding assertion fails.
	s.Must(controlled.Write(filepath.Join(s.Output, p.Name+"-"+profile+"-environment-proof.json"), view))
	if round.Result.CollectionStatus != "closed" || round.Result.Version != 1 || observed.Data.Run.StopStatus != "stopped" || round.Report.Commit != commit || !round.Report.Passed() {
		s.t.Fatalf("unverified controlled adaptation provenance: result=%s/%d stop=%s commit=%s expected=%s ready=%v", round.Result.CollectionStatus, round.Result.Version, observed.Data.Run.StopStatus, round.Report.Commit, commit, round.Report.Passed())
	}
	s.Must(controlled.Write(filepath.Join(s.Output, p.Name+"-environment-proof.json"), view))
}

func (p *Project) environmentProfiles() []executionconfig.Profile {
	if p.Scenario.Run.Remote != nil {
		return []executionconfig.Profile{p.Scenario.Run.Remote.Profile}
	}
	raw, err := os.ReadFile(filepath.Join(p.Home, "execution.json"))
	p.Scenario.Must(err)
	var file struct {
		Profiles []executionconfig.Profile `json:"execution_profiles"`
	}
	p.Scenario.Must(json.Unmarshal(raw, &file))
	return file.Profiles
}
func (p *Project) environmentKey(id string) string {
	for _, profile := range p.EnvironmentOptions() {
		if profile.ID == id && profile.Key != "" {
			return profile.Key
		}
	}
	p.Scenario.t.Fatalf("environment profile has no public Runner identity: %s", id)
	return ""
}
