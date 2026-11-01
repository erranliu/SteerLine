package testenv

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/prompts"
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
	const base = "fixture-environment-validation"
	profiles := p.environmentProfiles()
	entry, err := pm.ReadProjectEntry(func(path string) ([]byte, error) { return os.ReadFile(filepath.Join(p.Repo, path)) })
	s.Must(err)
	files := map[string][]byte{}
	for _, profile := range profiles {
		key, err := env.Key(profile.Execution)
		s.Must(err)
		script := env.ScriptPath(key, "ps1")
		manifest, err := json.Marshal(env.Manifest{Version: 3, RequiredChecks: []string{}, Gaps: []string{}, EnvironmentKey: key, Script: script, Interpreter: "powershell.exe", ValidationBase: base, Dependencies: []string{}})
		s.Must(err)
		files[env.DocumentPath(key)] = []byte("# Controlled environment\n```steerline-environment\n" + string(manifest) + "\n```\n")
		files[script] = []byte("throw 'Scripted public peer fixture; native execution is forbidden'\n")
	}
	rules, err := prompts.EnvironmentExecutionRules()
	s.Must(err)
	generationRules, err := prompts.EnvironmentGenerationRules()
	s.Must(err)
	for path, body := range map[string][]byte{
		env.GenerationRulesPath:            generationRules,
		env.ExecutionRulesPath:             rules,
		"SteerlineFlow/plans/task.plan.md": []byte("# Controlled recovery plan\nNo automated tests are required for this task. The catalogue's fixture.environment entry belongs to the separate scripted environment prerequisite. Recovery assertions are owned by the external E2E harness.\n"),
		entry.TestingCatalogue:             []byte(`{"tests":[{"id":"fixture.environment","name":"Controlled environment prerequisite","level":"L2","environment":"controlled","command":"controlled peer assertion","check":"scripted sealed report"}]}`),
	} {
		files[path] = body
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
	p.git(p.Repo, "push", "--quiet", provider.GitURL, "main", "HEAD:refs/heads/"+base)
	return p.git(p.Repo, "rev-parse", "HEAD")
}

func (p *Project) GenerateControlledEnvironment(profile string) string {
	s := p.Scenario
	if p.Chain != "" || s.Run.Remote != nil {
		s.t.Fatal("controlled generation is not native proof")
	}
	key := p.environmentKey(profile)
	project, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	defer project.Close()
	client, err := applicationclient.Open(s.Context, p.Home)
	s.Must(err)
	defer client.Close()
	prefix := "/projects/" + string(project.Identity().ProjectID)
	var material struct {
		Facts json.RawMessage `json:"target_facts"`
		Ready bool            `json:"generation_ready"`
	}
	s.Must(client.Get(s.Context, prefix+"/environment/materials?environment_key="+key, &material))
	if len(material.Facts) == 0 || !material.Ready {
		s.t.Fatal("missing frozen target facts or project sources")
	}
	commit := func(message string) {
		p.git(p.Repo, "add", ".")
		p.git(p.Repo, "commit", "--quiet", "--allow-empty", "-m", message)
		provider, err := controlled.Read(p.Provider)
		s.Must(err)
		p.git(p.Repo, "push", "--quiet", provider.GitURL, "main", "HEAD:refs/heads/fixture-environment-validation")
	}
	response := p.EnvironmentCommand("environment_generate", env.Generate{TargetKey: key, Profile: "controlled"})
	if response.Status != "applied" {
		s.t.Fatal("controlled generation rejected", response)
	}
	var record struct {
		ID, State string
		Settled   bool
		Generated *env.Generated `json:"generated"`
	}
	s.Must(json.Unmarshal(response.Environment, &record))
	id := record.ID
	s.Wait("controlled target generation settled", func() (bool, error) {
		err := client.Get(s.Context, prefix+"/environment/"+id, &record)
		if err == nil && record.Settled && record.State != "generated" {
			s.t.Fatal("controlled generation failed", record.State)
		}
		return err == nil && record.Settled && record.State == "generated", err
	})
	if record.Generated == nil {
		s.t.Fatal("controlled generation missing output")
	}
	manifest, err := env.ParseManifest([]byte(record.Generated.Document))
	s.Must(err)
	if manifest.EnvironmentKey != key {
		s.t.Fatal("controlled generation switched target")
	}
	s.Must(os.WriteFile(filepath.Join(p.Repo, env.DocumentPath(key)), []byte(record.Generated.Document), 0600))
	s.Must(os.WriteFile(filepath.Join(p.Repo, manifest.Script), []byte(record.Generated.Script), 0600))
	commit("Review controlled generation output")
	return env.Digest([]byte(record.Generated.Script))
}

func (p *Project) PrepareControlledEnvironment(profile string) {
	p.validateControlledEnvironment(profile, p.WriteControlledEnvironment(), "")
}

func (p *Project) ValidateGeneratedControlledEnvironment(profile, scriptDigest string) {
	p.validateControlledEnvironment(profile, p.git(p.Repo, "rev-parse", "HEAD"), scriptDigest)
}

func (p *Project) validateControlledEnvironment(profile, commit, expectedScriptDigest string) {
	s := p.Scenario
	const base = "fixture-environment-validation"
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
	if expectedScriptDigest != "" && view.ScriptDigest != expectedScriptDigest {
		s.t.Fatal("adaptation did not consume the generated script")
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
	profiles := p.environmentProfiles()
	for _, profile := range profiles {
		if profile.ID == id {
			key, err := env.Key(profile.Execution)
			p.Scenario.Must(err)
			return key
		}
	}
	p.Scenario.t.Fatalf("environment profile unavailable: %s", id)
	return ""
}
