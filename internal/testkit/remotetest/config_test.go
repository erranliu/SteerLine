package remotetest

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
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
	repo, commit := projectFixture(t)
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

func projectFixture(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	for path, body := range map[string]string{"SteerlineFlow/environment.md": "```steerline-environment\n{\"version\":1,\"script\":\"scripts/verify-environment.ps1\",\"interpreter\":\"pwsh\",\"validation_base\":\"validation\",\"dependencies\":[]}\n```\n", "scripts/verify-environment.ps1": "throw 'fixture only'\n"} {
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
