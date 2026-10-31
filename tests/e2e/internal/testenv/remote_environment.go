//go:build e2e

package testenv

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/internal/testkit/remotetest"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/google/uuid"
)

func (p *Project) verifyRemoteExecution(raw json.RawMessage) {
	var input runnercontract.RunInput
	p.Scenario.Must(json.Unmarshal(raw, &input))
	if remotetest.Digest(input.Execution) != p.Scenario.Run.Remote.ExecutionDigest {
		p.Scenario.t.Fatal("effective execution differs from frozen shared remote profile")
	}
}

// Only rewrites the fixture's validation target, before its source commit is
// frozen. The project's script and standards remain versioned original bytes.
func (p *Project) bindValidationBase() string {
	s := p.Scenario
	_ = p.environmentRules()
	path := filepath.Join(p.Repo, env.DocumentPath)
	raw, err := os.ReadFile(path)
	s.Must(err)
	manifest, err := env.ParseManifest(raw)
	s.Must(err)
	base := strings.TrimSuffix(p.BaseBranch, "/base") + "/validation/" + s.ID + "/base"
	manifest.ValidationBase = base
	fence := regexp.MustCompile("(?s)(```steerline-environment[\\t ]*\\r?\\n)(.*?)(\\r?\\n```)")
	if len(fence.FindAllSubmatch(raw, -1)) != 1 {
		s.t.Fatal("missing unique environment manifest")
	}
	data, err := json.Marshal(manifest)
	s.Must(err)
	raw = fence.ReplaceAllFunc(raw, func(old []byte) []byte {
		parts := fence.FindSubmatch(old)
		return append(append(append([]byte{}, parts[1]...), data...), parts[3]...)
	})
	s.Must(os.WriteFile(path, raw, 0600))
	return base
}

func (p *Project) ensureRemoteReady(seconds int) env.Record {
	record := p.checkRemoteEnvironment(seconds)
	if record.Status() != "ready" {
		p.Scenario.t.Fatal("project environment did not pass native readiness")
	}
	return record
}

func (p *Project) checkRemoteEnvironment(seconds int) env.Record {
	s := p.Scenario
	p.StartService()
	public, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	defer public.Close()
	identity := public.Identity()
	common := p.git(p.Repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	p.ProjectID = strings.ToLower(filepath.Clean(filepath.FromSlash(common)))
	client, err := applicationclient.Open(s.Context, p.Home)
	s.Must(err)
	defer client.Close()
	prefix := "/projects/" + string(identity.ProjectID)
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	payload, _ := json.Marshal(env.Start{Profile: p.Driver.Profile, TimeoutSeconds: seconds})
	request := httpcontract.CommandEnvelope{RequestID: id, ProjectID: identity.ProjectID, InstanceID: identity.InstanceID, Action: "environment_start", TargetID: string(identity.ProjectID), ExpectedVersion: 1, Payload: payload}
	var response httpcontract.CommandResponse
	s.Must(controlled.Write(filepath.Join(s.Output, "environment-start.json"), request))
	// A failed write is reconciled under this exact ID, never re-posted.
	err = client.Post(s.Context, prefix+"/commands", request, &response)
	if err != nil {
		s.Must(client.Get(s.Context, prefix+"/commands/"+id, &response))
	}
	if response.Status != "applied" {
		s.t.Fatalf("environment rejected: %s %s", response.Code, response.Diagnostic)
	}
	var view struct {
		State, Status, Diagnostic string
		Settled                   bool
	}
	s.Wait("project environment adaptation", func() (bool, error) {
		err := client.Get(s.Context, prefix+"/environment/"+id, &view)
		if err == nil {
			_ = controlled.Write(filepath.Join(s.Output, "environment-latest.json"), view)
		}
		return err == nil && view.Settled, err
	})
	reader, err := sqlite.OpenSQLiteReader(p.Database)
	s.Must(err)
	defer reader.Close()
	r, err := reader.Environment(s.Context, p.ProjectID, id)
	s.Must(err)
	if len(r.Rounds) == 0 || len(r.Rounds) > 3 {
		s.t.Fatal("invalid ready/budget evidence")
	}
	if r.Prepared.Commit != p.git(p.Repo, "rev-parse", "HEAD") {
		s.t.Fatal("adaptation source differs from the pinned fixture commit")
	}
	for _, round := range r.Rounds {
		p.verifyRemoteExecution(round.Request.Input)
	}
	last := r.Rounds[len(r.Rounds)-1]
	if last.Run == nil || last.Result == nil || last.Result.CollectionStatus != "closed" || last.Result.Version != 1 || last.Run.StopStatus != "stopped" || last.Report == nil || r.Status() == "ready" && !last.Report.Passed(r.Prepared.Manifest.ValidationBase, last.Run.Execution.Branch) {
		s.t.Fatal("missing stopped final report")
	}
	// Retain only caller-owned/public evidence, never Runner private storage.
	s.Must(controlled.Write(filepath.Join(s.Output, "environment-proof.json"), map[string]any{"execution_digest": s.Run.Remote.ExecutionDigest, "source_commit": r.Prepared.Commit, "activity_id": id, "state": r.State, "rounds": len(r.Rounds), "round_details": r.Rounds, "run_id": last.RunID, "run": last.Run, "result": last.Result, "report": last.Report, "l2_ids": r.Prepared.L2, "round_timeout_seconds": seconds}))
	if r.Status() == "ready" {
		p.verifyEnvironmentEvidence(r)
	} else {
		p.verifyUnreadyEnvironmentEvidence(r)
	}
	return r
}

func RunRemoteEnvironment(t *testing.T) { runProjectEnvironment(t, remotetest.Environment) }
func RunLocalEnvironment(t *testing.T)  { runProjectEnvironment(t, remotetest.LocalEnvironment) }
func runProjectEnvironment(t *testing.T, id string) {
	s := NewScenario(t, id)
	if s.Run.Remote == nil {
		t.Fatal("formal remote L4 input required")
	}
	in := s.Run.Remote
	config, err := driver.DecodeProjectConfig(in.ProjectConfig)
	s.Must(err)
	p := &Project{Scenario: s, Name: "308", Root: filepath.Join(s.Root, "308"), Home: filepath.Join(s.Root, "308", "home"), Repo: filepath.Join(s.Root, "308", "merge-h-308-code"), Config: filepath.Join(s.Root, "308", "home", "etc", "308.json"), Provider: filepath.Join(s.Root, "308", "provider.json"), Executable: s.Run.Binary, Chain: "environment-live", Tasks: map[string]Task{}}
	s.projects = append(s.projects, p)
	s.register("project", p.Root, 0)
	for _, dir := range []string{p.Root, filepath.Join(p.Home, "etc"), filepath.Join(p.Home, "temp"), filepath.Join(p.Home, "codex")} {
		s.Must(os.MkdirAll(dir, 0700))
	}
	p.Driver = driver.Client{Binary: p.Executable, Home: p.Home, Config: p.Config, StatePath: p.Provider, LogDir: filepath.Join(s.Output, "308-commands"), Profile: in.Profile.ID, NativeHome: s.Run.AuthHome}
	s.Must(os.MkdirAll(p.Driver.LogDir, 0700))
	s.Must(controlled.Write(p.Provider, controlled.State{}))
	clone := exec.Command("git", "clone", "--no-hardlinks", "--no-checkout", in.Project, p.Repo)
	clone.Env = p.Driver.Environment(os.Environ())
	_, err = s.RunCommandFor(clone, "308-clone", 10*time.Minute)
	s.Must(err)
	if p.git(p.Repo, "rev-parse", in.Commit+"^{commit}") != in.Commit {
		t.Fatal("308 source commit differs")
	}
	p.RemoteURL = p.git(in.Project, "remote", "get-url", config.Remote)
	p.git(p.Repo, "remote", "set-url", "origin", p.RemoteURL)
	p.BaseBranch = "codex/e2e/" + strings.ReplaceAll(uuid.NewString(), "-", "") + "/" + s.ID + "/base"
	p.git(p.Repo, "-c", "filter.lfs.process=", "-c", "filter.lfs.smudge=", "-c", "filter.lfs.required=false", "checkout", "-b", p.BaseBranch, in.Commit)
	base := p.bindValidationBase()
	for file, body := range map[string]string{"SteerlineFlow/spec/task.spec.md": "# Remote admission probe\nDo not implement, commit, push or publish. Remain blocked awaiting the caller's explicit stop.\n\n## Acceptance\nThe caller verifies the original Run binding, then explicitly stops it. No implementation or publication is authorized.\n", "SteerlineFlow/plans/task.tests.json": `{"tests":[]}`, "SteerlineFlow/plans/task.plan.md": "# Remote admission probe\nRead the bound Spec. No implementation or publication is authorized; await explicit stop.\n\nNo automated tests are registered for this caller probe. The required test arrangement for this admission probe is {\"tests\":[]}. Environment validation is owned by the separate caller flow.\n"} {
		s.Must(os.MkdirAll(filepath.Dir(filepath.Join(p.Repo, file)), 0700))
		s.Must(os.WriteFile(filepath.Join(p.Repo, file), []byte(body), 0600))
	}
	p.git(p.Repo, "add", "--", env.DocumentPath, "SteerlineFlow/spec/task.spec.md", "SteerlineFlow/plans/task.plan.md", "SteerlineFlow/plans/task.tests.json")
	p.git(p.Repo, "commit", "--quiet", "--allow-empty", "-m", "Freeze isolated environment validation base")
	fixture := p.git(p.Repo, "rev-parse", "HEAD")
	config.Repository, config.RunDir, config.Branch, config.Remote = p.Repo, "", p.BaseBranch, "origin"
	config.Git.Executable = "git"
	config.Git.Args = nil
	s.Must(controlled.Write(p.Config, config))
	selection, err := driver.RunnerSelection(s.Run.Runner)
	s.Must(err)
	codex, err := driver.CodexExecutable("codex")
	s.Must(err)
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), map[string]any{"version": 3, "runner": selection, "default_execution_profile": "material-generation", "execution_profiles": []any{map[string]any{"id": "material-generation", "name": "Local material generation", "execution": map[string]any{"adapter": "codex-app-server", "target": map[string]string{"kind": "local"}, "settings": map[string]string{"executable": codex, "model": "gpt-6-luna", "reasoning_effort": "medium"}}, "workflow": map[string]any{"id": "development", "version": 2}}, in.Profile}}))
	raw, err := p.Driver.Run(s.Context, "home", "-config", p.Config)
	s.Must(err)
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "run_dir ") {
			p.Database = filepath.Join(strings.TrimSpace(strings.TrimPrefix(line, "run_dir")), "steerline.db")
		}
	}
	if !filepath.IsAbs(p.Database) {
		t.Fatal("project database identity unavailable")
	}
	t.Cleanup(func() { p.cleanupRemoteValidation(base) })
	s.Must(controlled.Write(filepath.Join(s.Output, "remote-ref-intent.json"), map[string]any{"source_branch": p.BaseBranch, "validation_base": base, "commit": fixture}))
	p.git(p.Repo, "push", "origin", "HEAD:refs/heads/"+p.BaseBranch, "HEAD:refs/heads/"+base)
	if !strings.HasPrefix(p.git(p.Repo, "ls-remote", "origin", "refs/heads/"+p.BaseBranch), fixture+"\t") {
		t.Fatal("308 test source tip changed")
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "source.json"), map[string]any{"source_commit": in.Commit, "fixture_commit": fixture, "source_branch": p.BaseBranch, "validation_base": base, "execution_digest": in.ExecutionDigest}))
	// Runner locates the actual remote repository using its registered identity.
	if in.Profile.Execution.Adapter == "orca-terminal" {
		r := loadOrcaTarget(s, p)
		r.Fetch(s, p, fixture)
	}
	p.CreateTasks()
	s.Must(p.Driver.Control(s.Context, s.Run.Helper, "resume_dispatch", ""))
	deniedRaw, err := p.Driver.RunHelper(s.Context, s.Run.Helper, "control", "-config", p.Config, "-action", "start_task", "-task-id", p.Tasks["a"].ID, "-profile", p.Driver.Profile, "-expected-status", "rejected")
	s.Must(err)
	var denied productapi.ControlReceipt
	s.Must(json.Unmarshal(deniedRaw, &denied))
	if denied.Status != "rejected" || denied.Code != "environment_not_ready" || !strings.Contains(denied.Message, "环境未就绪") {
		t.Fatal("candidate was not denied by the environment gate", denied)
	}
	s.Must(controlled.Write(filepath.Join(s.Output, "admission-before.json"), denied))
	before, err := p.Snapshot()
	s.Must(err)
	for _, candidate := range before.Runner.Candidates {
		if candidate.RunID != "" {
			t.Fatal("unready project dispatched a remote Run")
		}
	}
	activity := p.checkRemoteEnvironment(in.RoundSeconds)
	if activity.Status() != "ready" {
		raw, err := p.Driver.RunHelper(s.Context, s.Run.Helper, "control", "-config", p.Config, "-action", "start_task", "-task-id", p.Tasks["a"].ID, "-profile", p.Driver.Profile, "-expected-status", "rejected")
		s.Must(err)
		var receipt productapi.ControlReceipt
		s.Must(json.Unmarshal(raw, &receipt))
		if receipt.Code != "environment_not_ready" {
			t.Fatal("failed requirements did not enforce admission", receipt)
		}
		s.Must(controlled.Write(filepath.Join(s.Output, "admission-after.json"), receipt))
		snapshot, err := p.Snapshot()
		s.Must(err)
		if len(snapshot.Runner.Candidates) != 0 {
			t.Fatal("not_ready project reserved a candidate")
		}
		s.Must(controlled.Write(filepath.Join(s.Output, "admission-proof.json"), map[string]any{"before_rejected": true, "after_rejected": true, "after_run_id": "", "stopped": true, "execution_digest": in.ExecutionDigest, "source_commit": in.Commit, "fixture_commit": fixture}))
		s.Must(s.FinishBody())
		p.StopService()
		t.Fatal("native environment remains not_ready; negative evidence was preserved and does not count as a positive L4 pass")
	}
	p.Claim("a")
	var admitted, admittedCandidate, admittedDigest string
	s.Wait("ready candidate actual Run", func() (bool, error) {
		snap, e := p.Snapshot()
		if e != nil {
			return false, e
		}
		for _, c := range snap.Runner.Candidates {
			if c.RunID != "" && c.InputDigest != "" && c.Projection != nil {
				p.verifyRemoteExecution(c.Submit.Input)
				admitted = c.RunID
				admittedCandidate, admittedDigest = c.ID, c.InputDigest
				return true, nil
			}
		}
		return false, nil
	})
	runner, _, err := selection.Open(s.Context, runnerstatepath.Directory(p.Home), 30*time.Second)
	s.Must(err)
	service, err := runner.ServiceStatus(s.Context)
	s.Must(err)
	runner.InstanceID = service.InstanceID
	s.stopOriginalRun(runner, admitted, admittedDigest, "admission-")
	s.Wait("caller observed original admission probe stop", func() (bool, error) {
		state, e := p.Snapshot()
		if e != nil {
			return false, e
		}
		for _, c := range state.Runner.Candidates {
			if c.ID == admittedCandidate {
				if c.RunID != admitted || c.InputDigest != admittedDigest || c.Participation != "active" {
					t.Fatal("native stop changed the original participating candidate binding")
				}
				s.Must(controlled.Write(filepath.Join(s.Output, "admission-stopped-candidate.json"), c))
				return c.Projection != nil && c.Projection.StopStatus == "stopped", nil
			}
		}
		return false, nil
	})
	s.Must(controlled.Write(filepath.Join(s.Output, "admission-proof.json"), map[string]any{"before_rejected": true, "after_run_id": admitted, "stopped": true, "execution_digest": in.ExecutionDigest, "source_commit": in.Commit, "fixture_commit": fixture}))
	s.Must(s.FinishBody())
	p.StopService()
}
