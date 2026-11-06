package remotetest

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
	progresstools "github.com/erranliu/SteerLine/tools"
)

func profileFixture(t *testing.T) (string, executionconfig.Profile) {
	t.Helper()
	root := t.TempDir()
	exe := filepath.Join(root, "orca.exe")
	if err := os.WriteFile(exe, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	settings, _ := json.Marshal(map[string]any{"transport": map[string]string{"executable": exe}, "agent_launch": map[string]string{"executable": "C:/Tools/codex.exe", "model": "selected", "reasoning_effort": "high"}, "access_mode": "full_access"})
	p := executionconfig.Profile{ID: "orca-remote-desktop64g", Name: "64G", Execution: runnercontract.ExecutionInput{Adapter: "orca-terminal", Target: runnercontract.ExecutionTarget{Kind: "orca_environment", Environment: "e68f4edf-8a83-42c2-a9da-6d404d3aa3d8"}, Settings: settings}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}
	return filepath.Join(root, "profile.json"), p
}
func TestRemoteL4FrozenConfigurationAndInputs(t *testing.T) {
	path, p := profileFixture(t)
	raw, _ := json.Marshal(p)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"STEERLINE_L4_REMOTE_PROFILE": path}
	get := func(k string) string { return vars[k] }
	in, err := Load(Discovery, get)
	if err != nil {
		t.Fatal(err)
	}
	if in.ExecutionDigest != Digest(p.Execution) || in.RoundSeconds != 0 {
		t.Fatal("directory acquired task prerequisites")
	}
	if err = os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if Digest(in.Profile.Execution) != in.ExecutionDigest {
		t.Fatal("frozen profile changed with its source")
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(Progress, get); err == nil {
		t.Fatal("missing project/budget accepted")
	}
	vars["STEERLINE_L4_ROUND_TIMEOUT_SECONDS"] = "1200"
	repo, commit := projectFixture(t, p.Execution)
	vars["STEERLINE_PROGRESS_PROJECT"] = repo
	vars["STEERLINE_PROGRESS_COMMIT"] = commit
	in, err = Load(Progress, get)
	if err != nil {
		t.Fatal(err)
	}
	if in.Timeout(Progress) != 90*time.Minute {
		t.Fatal("three rounds or observation/setup deadline clipped")
	}
	config := filepath.Join(t.TempDir(), "308.json")
	projectRaw, _ := json.Marshal(map[string]string{"repository": repo, "branch": "AIRefactor", "remote": "origin", "task_authority": "native"})
	if err = os.WriteFile(config, projectRaw, 0600); err != nil {
		t.Fatal(err)
	}
	vars["STEERLINE_L4_PROJECT_CONFIG"], vars["STEERLINE_L4_PROJECT_COMMIT"] = config, commit
	in, err = Load(Environment, get)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(config, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if string(in.ProjectConfig) != string(projectRaw) || in.Project != repo || in.Commit != commit {
		t.Fatal("308 project input was not frozen")
	}
	vars["STEERLINE_PROGRESS_COMMIT"] = strings.Repeat("a", 40)
	if _, err = Load(Progress, get); err == nil {
		t.Fatal("unknown commit/missing committed materials accepted")
	}
	vars["STEERLINE_L4_ROUND_TIMEOUT_SECONDS"] = "9223372036854775807"
	if _, err = Load(Progress, get); err == nil {
		t.Fatal("overflowing three-round deadline accepted")
	}
	if unrelated, err := Load("smoke.delivery", func(string) string { return "" }); err != nil || unrelated != nil {
		t.Fatal("controlled test now requires remote config")
	}
}

func projectFixture(t *testing.T, execution runnercontract.ExecutionInput) (string, string) {
	t.Helper()
	repo := t.TempDir()
	testingRules, err := prompts.TestingRules()
	if err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(env.Manifest{Version: 5, Script: env.ScriptPath("ps1"), Interpreter: "powershell.exe", Dependencies: []string{}})
	for path, body := range map[string]string{tm.ProtocolPath: string(testingRules), tm.WriterPath: progresstools.PowerShellWriter, "SteerlineFlow/environment-project.json": `{"version":1,"requirements":[],"testing_catalogue":"SteerlineFlow/testing/test-list.json"}`, env.DocumentPath(): "```steerline-environment\n" + string(manifest) + "\n```\n", env.ScriptPath("ps1"): "throw 'fixture only'\n"} {
		full := filepath.Join(repo, filepath.FromSlash(path))
		if e := os.MkdirAll(filepath.Dir(full), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(full, []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
	}
	run := func(args ...string) string {
		t.Helper()
		raw, e := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if e != nil {
			t.Fatal(e, string(raw))
		}
		return strings.TrimSpace(string(raw))
	}
	run("init", "-q")
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "add", ".")
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "materials")
	return repo, run("rev-parse", "HEAD")
}
func TestRemoteL4RejectsWrongTargetAndMissingInputs(t *testing.T) {
	for _, change := range []func(*executionconfig.Profile){func(p *executionconfig.Profile) { p.Execution.Target.Kind = "local" }, func(p *executionconfig.Profile) { p.Execution.Target.Environment = "name:64G" }, func(p *executionconfig.Profile) { p.Workflow.Version = 1 }, func(p *executionconfig.Profile) { p.Execution.Settings = json.RawMessage(`{}`) }} {
		path, p := profileFixture(t)
		change(&p)
		raw, _ := json.Marshal(p)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(Discovery, func(string) string { return path }); err == nil {
			t.Fatal("invalid remote profile accepted")
		}
	}
	if _, err := Load(Discovery, func(string) string { return "relative.json" }); err == nil {
		t.Fatal("relative shared profile accepted")
	}
}

func TestLocalEnvironmentL4FrozenProfileAndTarget(t *testing.T) {
	path, p := profileFixture(t)
	p.Execution.Target = runnercontract.ExecutionTarget{Kind: "local"}
	b, _ := json.Marshal(p)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	repo, commit := projectFixture(t, p.Execution)
	config := filepath.Join(t.TempDir(), "project.json")
	b, _ = json.Marshal(map[string]string{"repository": repo, "branch": "project-owned-source", "task_authority": "native"})
	if err := os.WriteFile(config, b, 0600); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"STEERLINE_L4_LOCAL_PROFILE": path, "STEERLINE_L4_PROJECT_CONFIG": config, "STEERLINE_L4_PROJECT_COMMIT": commit, "STEERLINE_L4_ROUND_TIMEOUT_SECONDS": "900"}
	in, err := Load(LocalEnvironment, func(k string) string { return vars[k] })
	if err != nil || in.Profile.Execution.Target.Kind != "local" || in.Commit != commit {
		t.Fatal(in, err)
	}
	if Selected("testing.project-environment-generation") {
		t.Fatal("retired generation is still selected")
	}
	generation := executionconfig.Profile{ID: "generation-local", Name: "Explicit generation", Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"full_access"}`)}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}
	generationPath := filepath.Join(t.TempDir(), "generation.json")
	raw, _ := json.Marshal(generation)
	if err := os.WriteFile(generationPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	vars["STEERLINE_L4_GENERATION_PROFILE"] = generationPath
	in, err = Load(LocalEnvironment, func(k string) string { return vars[k] })
	if err != nil || in.GenerationProfile == nil || in.GenerationProfile.ID != generation.ID || in.ExecutionDigest != Digest(p.Execution) {
		t.Fatal("explicit generator changed the tested execution", in, err)
	}
	_ = os.WriteFile(generationPath, []byte("changed"), 0600)
	if in.GenerationProfile.ID != generation.ID {
		t.Fatal("generator snapshot changed with source file")
	}
	generation.ID = p.ID
	raw, _ = json.Marshal(generation)
	_ = os.WriteFile(generationPath, raw, 0600)
	if _, err := Load(LocalEnvironment, func(k string) string { return vars[k] }); err == nil {
		t.Fatal("duplicate tested/generation profile ID accepted")
	}
	delete(vars, "STEERLINE_L4_GENERATION_PROFILE")
	p.Execution.Target = runnercontract.ExecutionTarget{Kind: "orca_environment", Environment: "remote"}
	b, _ = json.Marshal(p)
	_ = os.WriteFile(path, b, 0600)
	if _, err := Load(LocalEnvironment, func(k string) string { return vars[k] }); err == nil {
		t.Fatal("local detection accepted another target")
	}
}

func TestEnvironmentL4RejectsStaleAdmissionMaterialsBeforeFixture(t *testing.T) {
	for _, path := range []string{tm.ProtocolPath, tm.WriterPath} {
		t.Run(path, func(t *testing.T) {
			profilePath, profile := profileFixture(t)
			raw, _ := json.Marshal(profile)
			if err := os.WriteFile(profilePath, raw, 0600); err != nil {
				t.Fatal(err)
			}
			repo, _ := projectFixture(t, profile.Execution)
			if err := os.WriteFile(filepath.Join(repo, path), []byte("old fixed material"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"add", "."}, {"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "stale material"}} {
				if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
					t.Fatal(err, string(out))
				}
			}
			commit, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
			if err != nil {
				t.Fatal(err)
			}
			vars := map[string]string{"STEERLINE_L4_REMOTE_PROFILE": profilePath, "STEERLINE_PROGRESS_PROJECT": repo, "STEERLINE_PROGRESS_COMMIT": strings.TrimSpace(string(commit)), "STEERLINE_L4_ROUND_TIMEOUT_SECONDS": "900"}
			if _, err := Load(Progress, func(k string) string { return vars[k] }); err == nil || !strings.Contains(err.Error(), "admission probe") {
				t.Fatal("stale admission material reached fixture preparation", err)
			}
		})
	}
}
