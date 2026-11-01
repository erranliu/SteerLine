//go:build e2e

package testenv

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	model "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/internal/testkit/remotetest"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
)

// Real generate Runs, with an isolated local source remote. Neither target is
// granted readiness and no external Git ref or PR is written by this scenario.
func RunEnvironmentGeneration(t *testing.T) {
	s := NewScenario(t, remotetest.Generation)
	in := s.Run.Remote
	if in == nil || in.GenerationTarget == nil {
		t.Fatal("explicit local generator and remote target required")
	}
	config, err := driver.DecodeProjectConfig(in.ProjectConfig)
	s.Must(err)
	p := &Project{Scenario: s, Name: "generation", Root: filepath.Join(s.Root, "generation"), Home: filepath.Join(s.Root, "generation", "home"), Repo: filepath.Join(s.Root, "generation", "repo"), Config: filepath.Join(s.Root, "generation", "home", "etc", "generation.json"), Provider: filepath.Join(s.Root, "generation", "provider.json"), Executable: s.Run.Binary, Chain: "local-environment-generation", Tasks: map[string]Task{}}
	s.projects = append(s.projects, p)
	s.register("project", p.Root, 0)
	for _, path := range []string{p.Root, filepath.Dir(p.Config), filepath.Join(p.Home, "temp")} {
		s.Must(os.MkdirAll(path, 0700))
	}
	p.Driver = driver.Client{Binary: p.Executable, Home: p.Home, Config: p.Config, StatePath: p.Provider, LogDir: filepath.Join(s.Output, "generation-commands"), Profile: in.Profile.ID, NativeHome: s.Run.AuthHome}
	s.Must(os.MkdirAll(p.Driver.LogDir, 0700))
	s.Must(controlled.Write(p.Provider, controlled.State{}))
	clone := exec.Command("git", "clone", "--no-hardlinks", "--no-checkout", in.Project, p.Repo)
	clone.Env = p.Driver.Environment(os.Environ())
	_, err = s.RunCommandFor(clone, "generation-clone", 10*time.Minute)
	s.Must(err)
	p.BaseBranch = "pew-generation-source"
	p.git(p.Repo, "checkout", "-b", p.BaseBranch, in.Commit)
	p.RemoteURL = p.git(in.Project, "config", "--get", "remote."+config.Remote+".url")
	p.git(p.Repo, "remote", "set-url", "origin", p.RemoteURL)
	bare := filepath.Join(p.Root, "source.git")
	p.git(p.Repo, "clone", "--bare", "--no-hardlinks", p.Repo, bare)
	config.Repository, config.RunDir, config.Branch, config.Remote = p.Repo, "", p.BaseBranch, "origin"
	config.Git.Executable = "git"
	config.Git.Args = []string{"-c", "url." + filepath.ToSlash(bare) + ".insteadOf=" + p.RemoteURL}
	s.Must(controlled.Write(p.Config, config))
	selection, err := driver.RunnerSelection(s.Run.Runner)
	s.Must(err)
	s.Must(controlled.Write(filepath.Join(p.Home, "execution.json"), map[string]any{"version": 3, "runner": selection, "default_execution_profile": in.Profile.ID, "execution_profiles": []any{in.Profile, *in.GenerationTarget}}))
	for _, asset := range []struct {
		path string
		read func() ([]byte, error)
	}{{model.GenerationRulesPath, prompts.EnvironmentGenerationRules}, {model.ExecutionRulesPath, prompts.EnvironmentExecutionRules}} {
		body, err := asset.read()
		s.Must(err)
		s.Must(os.MkdirAll(filepath.Dir(filepath.Join(p.Repo, asset.path)), 0700))
		s.Must(os.WriteFile(filepath.Join(p.Repo, asset.path), body, 0600))
	}
	// A test-owned declared base, not a guessed production branch or a model
	// instruction. The existing project verifiers/requirements remain unchanged.
	validationBase := "pew-generated-validation"
	commonPath := filepath.Join(p.Repo, "SteerlineFlow", "environment-common.md")
	common, readErr := os.ReadFile(commonPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		s.Must(readErr)
	}
	common = append(common, []byte("\n# Test source publication facts\n\nvalidation_base: "+validationBase+"\nThe validation base is pre-created in this isolated source remote.\n")...)
	s.Must(os.WriteFile(commonPath, common, 0600))
	commit := func(message string) string {
		p.git(p.Repo, "add", ".")
		p.git(p.Repo, "-c", "user.name=SteerLine Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "--allow-empty", "-m", message)
		p.git(p.Repo, "push", bare, "HEAD:refs/heads/"+p.BaseBranch)
		return p.git(p.Repo, "rev-parse", "HEAD")
	}
	commit("Prepare same-version generation rules")
	p.git(p.Repo, "push", bare, "HEAD:refs/heads/"+validationBase)
	raw, err := p.Driver.Run(s.Context, "home", "-config", p.Config)
	s.Must(err)
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "run_dir ") {
			p.Database = filepath.Join(strings.TrimSpace(strings.TrimPrefix(line, "run_dir")), "steerline.db")
		}
	}
	if !filepath.IsAbs(p.Database) {
		t.Fatal("generation project database unavailable")
	}
	commonDir := p.git(p.Repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	p.ProjectID = strings.ToLower(filepath.Clean(filepath.FromSlash(commonDir)))
	p.StartService()
	reader, err := sqlite.OpenSQLiteReader(p.Database)
	s.Must(err)
	defer reader.Close()
	client, err := applicationclient.Open(s.Context, p.Home)
	s.Must(err)
	defer client.Close()
	project, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	defer project.Close()
	prefix := "/projects/" + string(project.Identity().ProjectID)
	keys := []string{}
	for _, profile := range []struct{ id string }{{in.Profile.ID}, {in.GenerationTarget.ID}} {
		var key string
		for _, option := range p.EnvironmentOptions() {
			if option.ID == profile.id {
				key = option.Key
			}
		}
		if !model.ValidKey(key) {
			t.Fatal("target missing from loaded options", profile.id)
		}
		keys = append(keys, key)
		var material struct {
			Expected string `json:"expected_context"`
			Path     string `json:"context_path"`
		}
		s.Must(client.Get(s.Context, prefix+"/environment/materials?environment_key="+key, &material))
		if material.Path != model.GenerationContextPath(key) || material.Expected == "" {
			t.Fatal("bootstrap target context absent")
		}
		s.Must(os.MkdirAll(filepath.Dir(filepath.Join(p.Repo, material.Path)), 0700))
		s.Must(os.WriteFile(filepath.Join(p.Repo, material.Path), []byte(material.Expected), 0600))
	}
	generationSource := commit("Pin target-specific generation contexts")
	proof := []map[string]any{}
	for _, key := range keys {
		response := p.EnvironmentCommand("environment_generate", model.Generate{TargetKey: key, Profile: in.Profile.ID})
		if response.Status != "applied" {
			t.Fatal("generation rejected", response)
		}
		var activity struct {
			ID, State  string
			Diagnostic string            `json:"diagnostic"`
			Rounds     []json.RawMessage `json:"rounds"`
			Settled    bool
			Generated  *model.Generated `json:"generated"`
		}
		var original model.Record
		s.Wait("native generation final and settlement", func() (bool, error) {
			var history struct {
				Records []struct {
					ID, Kind, State string
					Settled         bool
				}
			}
			if err := client.Get(s.Context, prefix+"/environment?environment_key="+key, &history); err != nil {
				return false, err
			}
			if len(history.Records) == 0 {
				return false, nil
			}
			record := history.Records[0]
			if record.Kind != "generation" {
				t.Fatal("unexpected adaptation during generation")
			}
			if err := client.Get(s.Context, prefix+"/environment/"+record.ID, &activity); err != nil {
				return false, err
			}
			if activity.Settled {
				original, err = reader.Environment(s.Context, p.ProjectID, activity.ID)
				if err != nil {
					return false, err
				}
				s.Must(controlled.Write(filepath.Join(s.Output, "generation-original-"+key+".json"), original))
			}
			if activity.Settled && activity.State != "generated" {
				s.Must(controlled.Write(filepath.Join(s.Output, "generation-"+key+".json"), activity))
				t.Fatal("native generation failed", activity.State, activity.Diagnostic)
			}
			return activity.Settled && activity.State == "generated", nil
		})
		if activity.Generated == nil {
			t.Fatal("missing native output")
		}
		if original.TargetKey != key || original.Prepared.Commit != generationSource || len(original.Rounds) != 1 {
			t.Fatal("generation target/source/round differs from frozen request")
		}
		round := original.Rounds[0]
		if !round.Settled || round.Run == nil || round.Run.StopStatus != "stopped" || round.Run.ContentResult == nil || !round.Run.ContentResult.Source.Verified || round.Run.RepositorySource == nil || round.Run.RepositorySource.Commit != generationSource {
			t.Fatal("native generation source or stop not verified")
		}
		s.Must(controlled.Write(filepath.Join(s.Output, "generation-"+key+".json"), activity))
		manifest, err := model.ParseManifest([]byte(activity.Generated.Document))
		s.Must(err)
		if manifest.EnvironmentKey != key {
			t.Fatal("native output changed target")
		}
		s.Must(os.WriteFile(filepath.Join(p.Repo, model.DocumentPath(key)), []byte(activity.Generated.Document), 0600))
		s.Must(os.WriteFile(filepath.Join(p.Repo, manifest.Script), []byte(activity.Generated.Script), 0600))
		proof = append(proof, map[string]any{"target_environment_key": key, "activity_id": activity.ID, "settled": activity.Settled, "document_path": model.DocumentPath(key), "script_path": manifest.Script, "gaps": activity.Generated.Gaps})
	}
	fixture := commit("Review and pin generated environment materials")
	for _, option := range p.EnvironmentOptions() {
		if option.Status == "ready" {
			t.Fatal("generation incorrectly granted readiness")
		}
	}
	retained := filepath.Join(s.Output, "generated-source")
	p.git(p.Repo, "clone", "--no-hardlinks", p.Repo, retained)
	p.git(retained, "remote", "set-url", "origin", p.RemoteURL)
	config.Repository = retained
	config.Git.Args = nil
	s.Must(controlled.Write(filepath.Join(s.Output, "generated-project.json"), config))
	s.Must(controlled.Write(filepath.Join(s.Output, "generation-proof.json"), map[string]any{"source_commit": in.Commit, "fixture_commit": fixture, "execution_digest": in.ExecutionDigest, "targets": proof, "target_count": len(proof), "generation_only": true, "project": retained}))
}
