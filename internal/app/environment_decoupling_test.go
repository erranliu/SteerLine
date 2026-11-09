package app

import (
	"context"
	"encoding/json"
	"errors"
	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func commitEnvironmentMaterials(t *testing.T, c Config) string {
	t.Helper()
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "update project interface")
	fixtureGit(t, c.Repository, "push", "origin", "main")
	return pmGit(t, c.Repository, "rev-parse", "HEAD")
}

func seedDetectedEnvironment(t *testing.T, p *environmentPort, profile string) envmodel.Record {
	t.Helper()
	prepared, e := p.prepare(context.Background(), profile)
	require(t, e)
	r := envmodel.Record{FormatVersion: 1, ID: envmodel.ID(), Project: p.config.Identity(), Revision: 1, Kind: "adaptation", Profile: profile, Execution: prepared.Input.Execution, Prepared: prepared, State: "ready", CreatedAt: time.Now().UTC(), Requests: []envmodel.Request{{ID: envmodel.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []envmodel.Round{{Number: 1, Settled: true}}}
	require(t, p.store.StartEnvironment(context.Background(), r, 0))
	return r
}
func TestEnvironmentDetectionNeedsNoProjectCatalogueOrPlatformCopies(t *testing.T) {
	p, c, _ := environmentFixture(t)
	for _, path := range []string{"SteerlineFlow/environment-project.json", "SteerlineFlow/testing/test-list.json", "SteerlineFlow/environment-generation.md", "SteerlineFlow/environment-execution.md", "SteerlineProjectGuideDoc/environment-adaptation.md"} {
		e := os.Remove(filepath.Join(c.Repository, path))
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			t.Fatal(e)
		}
	}
	// Missing files are intentional; the fixture already commits only invocation files.
	prepared, e := p.prepare(context.Background(), "remote")
	require(t, e)
	if prepared.Input.Workflow.ID != "execute" || prepared.Input.Publication.Provider != "" || len(prepared.Input.Documents.References) != 0 {
		t.Fatal(prepared.Input)
	}
}
func TestEnvironmentStableBasisExcludesUnrelatedCommitAndModel(t *testing.T) {
	p, c, store := environmentFixture(t)
	before, e := p.prepare(context.Background(), "remote")
	require(t, e)
	require(t, os.WriteFile(filepath.Join(c.Repository, "unrelated.md"), []byte("notes"), 0600))
	commitEnvironmentMaterials(t, c)
	for i := range p.config.RunnerExecution.Profiles {
		if p.config.RunnerExecution.Profiles[i].ID == "remote" {
			p.config.RunnerExecution.Profiles[i].Execution.Settings = json.RawMessage(`{"agent_launch":{"executable":"codex","model":"different","reasoning_effort":"high"},"access_mode":"full_access"}`)
		}
	}
	after, e := p.prepare(context.Background(), "remote")
	require(t, e)
	if before.Commit == after.Commit || before.Inputs != after.Inputs {
		t.Fatal("ephemeral commit/model entered basis", before.Inputs, after.Inputs)
	}
	t.Run("effective-project-requirements", func(t *testing.T) {
		document, err := os.ReadFile("../../SteerlineFlow/environment.md")
		require(t, err)
		manifest, err := envmodel.ParseManifest(document)
		require(t, err)
		module, err := os.ReadFile("../../go.mod")
		require(t, err)
		goVersion := regexp.MustCompile(`(?m)^go\s+(\d+\.\d+(?:\.\d+)?)\s*$`).FindSubmatch(module)
		if len(goVersion) != 2 || manifest.Requirements["go"] != string(goVersion[1]) || len(manifest.Dependencies) != 0 {
			t.Fatal("project requirements drifted from Go minimum or bind unrelated dependencies", manifest)
		}
		path := filepath.Join(c.Repository, filepath.FromSlash(envmodel.DocumentPath()))
		require(t, os.WriteFile(path, document, 0600))
		commitEnvironmentMaterials(t, c)
		r := seedDetectedEnvironment(t, p, "remote")
		// Prose, JSON formatting/order, CRLF and ordinary module dependency changes
		// must preserve the successful basis and its original source identity.
		body, err := json.MarshalIndent(map[string]any{"requirements": manifest.Requirements, "dependencies": manifest.Dependencies, "script": manifest.Script, "version": manifest.Version, "interpreter": manifest.Interpreter}, "", "    ")
		require(t, err)
		reformatted := strings.ReplaceAll("# New explanatory prose\n```steerline-environment\n"+string(body)+"\n```\nNew help link.\n", "\n", "\r\n")
		require(t, os.WriteFile(path, []byte(reformatted), 0600))
		require(t, os.WriteFile(filepath.Join(c.Repository, "go.mod"), []byte("module fixture\n\ngo "+manifest.Requirements["go"]+"\n\nrequire example.invalid/library v1.2.3\n"), 0600))
		commit := commitEnvironmentMaterials(t, c)
		require(t, p.observeBasis(context.Background(), commit))
		proof, record, err := p.admission(context.Background(), "remote", r.Prepared.Commit, false)
		require(t, err)
		if proof.Basis != r.Prepared.Inputs || record.Prepared.Commit != r.Prepared.Commit {
			t.Fatal("cosmetic/module changes altered original qualification", proof, record.Prepared)
		}
		changed := strings.Replace(reformatted, `"go": "`+manifest.Requirements["go"]+`"`, `"go": "99.0"`, 1)
		require(t, os.WriteFile(path, []byte(changed), 0600))
		commit = commitEnvironmentMaterials(t, c)
		options, err := p.readinessOptions(context.Background())
		require(t, err)
		want := "环境要求 go：" + manifest.Requirements["go"] + " → 99.0；须重新检测"
		found := false
		for _, item := range options.(environmentOptions).Environments {
			if item.Key == mustEnvironmentKey(t, r.Execution) {
				found = true
				if item.Stage != "stale" || item.Reason != want {
					t.Fatal("query lost specific requirement change", item)
				}
			}
		}
		current, _, err := p.current(context.Background(), "remote")
		require(t, err)
		if !found || current.Invalidated {
			t.Fatal("read-only query wrote revocation or omitted target", current)
		}
		if _, _, err := p.admission(context.Background(), "remote", "", false); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatal("admission lost requirement change", err)
		}
		require(t, p.observeBasis(context.Background(), commit))
		current, _, err = p.current(context.Background(), "remote")
		require(t, err)
		if !current.Invalidated || current.Reason != want {
			t.Fatal("durable revocation lost requirement change", current)
		}
		// A newly passed current basis cannot admit an older candidate. Rechecking
		// that same current basis would not resolve the candidate source mismatch.
		replacement := r
		replacement.ID = envmodel.ID()
		replacement.Prepared, err = p.materialAt(context.Background(), "remote", commit)
		require(t, err)
		replacement.Requests = []envmodel.Request{{ID: envmodel.ID(), Original: json.RawMessage(`{}`)}}
		require(t, store.StartEnvironment(context.Background(), replacement, current.Revision))
		if _, _, err := p.admission(context.Background(), "remote", r.Prepared.Commit, false); err == nil || !strings.Contains(err.Error(), strings.TrimSuffix(want, "；须重新检测")) || !strings.Contains(err.Error(), "请更新候选来源") || strings.Contains(err.Error(), "须重新检测") {
			t.Fatal("candidate mismatch gave an ineffective recheck action or reversed the change", err)
		}
		_, _, err = p.admission(context.Background(), "remote", "", false)
		require(t, err)
	})
}
func TestEnvironmentQualificationIgnoresProjectTestsAndBusinessDocuments(t *testing.T) {
	p, c, store := environmentFixture(t)
	r := seedDetectedEnvironment(t, p, "remote")
	names := []string{"SteerlineFlow/environment-project.json", "SteerlineFlow/testing/test-list.json", "SteerlineFlow/testing.md", "SteerlineFlow/spec/task.spec.md", "SteerlineFlow/plans/task.plan.md", "SteerlineFlow/Roadmap/execution-map.md", "README.md", "tools/project-tests.py"}
	for _, remove := range []bool{false, true} {
		for _, name := range names {
			path := filepath.Join(c.Repository, filepath.FromSlash(name))
			if remove {
				require(t, os.Remove(path))
			} else {
				require(t, os.MkdirAll(filepath.Dir(path), 0700))
				// These deliberately invalid project files are not environment inputs.
				require(t, os.WriteFile(path, []byte("invalid project content; never execute\n"), 0600))
			}
		}
		commit := commitEnvironmentMaterials(t, c)
		require(t, p.observeBasis(context.Background(), commit))
		proof, _, err := p.admission(context.Background(), "remote", commit, false)
		require(t, err)
		if proof.Basis != r.Prepared.Inputs {
			t.Fatal("project checks entered the runtime basis")
		}
		current, err := store.CurrentEnvironment(context.Background(), r.Project, mustEnvironmentKey(t, r.Execution))
		require(t, err)
		if current.Invalidated || current.Revision != 1 {
			t.Fatal("project content revoked runtime qualification", current)
		}
	}
}

func TestEnvironmentRuntimeDependenciesBindBasisWithoutExpandingReadingReferences(t *testing.T) {
	p, c, _ := environmentFixture(t)
	before, e := p.prepare(context.Background(), "remote")
	require(t, e)
	if len(before.Input.Documents.References) != 0 || len(before.Manifest.Dependencies) != 1 {
		t.Fatal(before)
	}
	require(t, os.WriteFile(filepath.Join(c.Repository, "go.mod"), []byte("module changed.invalid\n"), 0600))
	commitEnvironmentMaterials(t, c)
	after, e := p.prepare(context.Background(), "remote")
	require(t, e)
	if before.Inputs == after.Inputs {
		t.Fatal("declared dependency ignored")
	}
	require(t, os.Remove(filepath.Join(c.Repository, "go.mod")))
	commitEnvironmentMaterials(t, c)
	if _, e := p.prepare(context.Background(), "remote"); e == nil {
		t.Fatal("missing declared dependency ignored")
	}
	t.Run("dependency-order-is-not-a-change", func(t *testing.T) {
		for _, name := range []string{"a.json", "b.json"} {
			require(t, os.WriteFile(filepath.Join(c.Repository, name), []byte("{}"), 0600))
		}
		path := filepath.Join(c.Repository, filepath.FromSlash(envmodel.DocumentPath()))
		require(t, os.WriteFile(path, []byte(environmentFixtureDocument("ps1", []string{"a.json", "b.json"})), 0600))
		commitEnvironmentMaterials(t, c)
		first, err := p.prepare(context.Background(), "remote")
		require(t, err)
		require(t, os.WriteFile(path, []byte(environmentFixtureDocument("ps1", []string{"b.json", "a.json"})), 0600))
		commitEnvironmentMaterials(t, c)
		second, err := p.prepare(context.Background(), "remote")
		require(t, err)
		if first.Inputs != second.Inputs {
			t.Fatal("dependency list order invalidated qualification")
		}
	})
	t.Run("specific-reason-fields", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			change func(*envmodel.Prepared)
			want   string
		}{
			{"version", func(p *envmodel.Prepared) { p.Manifest.Version++ }, "环境接口版本变化"},
			{"interpreter", func(p *envmodel.Prepared) { p.Manifest.Interpreter = "bash" }, "解释器：powershell.exe → bash"},
			{"script", func(p *envmodel.Prepared) { p.Manifest.Script = envmodel.ScriptPath("sh") }, "检测入口：" + envmodel.ScriptPath("ps1") + " → " + envmodel.ScriptPath("sh")},
			{"added-dependency", func(p *envmodel.Prepared) { p.Manifest.Dependencies = []string{"go.mod", "tools/check.ps1"} }, "新增检测依赖：tools/check.ps1"},
			{"removed-dependency", func(p *envmodel.Prepared) { p.Manifest.Dependencies = []string{} }, "移除检测依赖：go.mod"},
			{"removed-requirement", func(p *envmodel.Prepared) { p.Manifest.Requirements = map[string]string{} }, "移除环境要求 go（原值 1.26.0）"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				first := before
				first.Manifest.Requirements = map[string]string{"go": "1.26.0"}
				next := first
				tc.change(&next)
				if reason := environmentBasisChange(first, next); !strings.Contains(reason, tc.want) {
					t.Fatal(reason, tc.want)
				}
			})
		}
	})
}
func TestEnvironmentReadFailureDoesNotPersistRevocation(t *testing.T) {
	p, _, store := environmentFixture(t)
	r := seedDetectedEnvironment(t, p, "remote")
	ctx := context.Background()
	git := p.config.Git
	repository := p.config.Repository
	for _, failure := range []string{"bare-executable", "absolute-executable", "working-directory"} {
		p.config.Git, p.config.Repository = git, repository
		switch failure {
		case "bare-executable":
			p.config.Git.Executable = "missing-environment-git-command"
		case "absolute-executable":
			p.config.Git.Executable = filepath.Join(t.TempDir(), "missing-git.exe")
		case "working-directory":
			p.config.Repository = filepath.Join(t.TempDir(), "missing-worktree")
		}
		if e := p.observeBasis(ctx, r.Prepared.Commit); e == nil || strings.Contains(e.Error(), "项目尚未接入检测") {
			t.Fatal("read failure lost its cause or became a missing project document", failure, e)
		}
		current, e := store.CurrentEnvironment(ctx, r.Project, mustEnvironmentKey(t, r.Execution))
		require(t, e)
		if current.Invalidated || current.Revision != 1 {
			t.Fatal("read outage revoked", failure, current)
		}
	}
	p.config.Git, p.config.Repository = git, repository
	require(t, p.observeBasis(ctx, r.Prepared.Commit))
	if _, _, e := p.admission(ctx, "remote", "", false); e != nil {
		t.Fatal(e)
	}
}
func TestEnvironmentKnownDeletedDependencyPermanentlyRevokes(t *testing.T) {
	p, c, store := environmentFixture(t)
	r := seedDetectedEnvironment(t, p, "remote")
	require(t, os.Remove(filepath.Join(c.Repository, "go.mod")))
	commit := commitEnvironmentMaterials(t, c)
	require(t, p.observeBasis(context.Background(), commit))
	current, e := store.CurrentEnvironment(context.Background(), r.Project, mustEnvironmentKey(t, r.Execution))
	require(t, e)
	if !current.Invalidated {
		t.Fatal("deleted dependency did not revoke")
	}
}

func TestEnvironmentStartRejectsPendingOrSupersededBaseline(t *testing.T) {
	for _, phase := range []string{"observation-pending", "new-baseline-published"} {
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			p, c, store := environmentFixture(t)
			m := newProjectMaintenance(c, nil)
			p.config.runtimeMaintenance = m
			m.environmentObservation = p.observeBasis
			prepared, err := p.prepare(ctx, "remote")
			require(t, err)
			var expected uint64
			if phase == "observation-pending" {
				old := seedDetectedEnvironment(t, p, "remote")
				current, err := store.CurrentEnvironment(ctx, old.Project, mustEnvironmentKey(t, old.Execution))
				require(t, err)
				expected = current.Revision
			}
			// The original operation already froze A; pause before selecting Current.
			p.service.Prepare = func(context.Context, string) (envmodel.Prepared, error) { return prepared, nil }
			require(t, os.WriteFile(filepath.Join(c.Repository, "go.mod"), []byte("module newer.invalid\n"), 0600))
			commitEnvironmentMaterials(t, c)
			var finish chan error
			var release chan struct{}
			if phase == "observation-pending" {
				entered := make(chan struct{})
				release = make(chan struct{})
				m.environmentObservation = func(ctx context.Context, commit string) error {
					close(entered)
					<-release
					return p.observeBasis(ctx, commit)
				}
				finish = make(chan error, 1)
				go func() { _, err := m.check(ctx); finish <- err }()
				select {
				case <-entered:
				case <-time.After(10 * time.Second):
					t.Fatal("observation did not reach environment callback")
				}
			} else {
				_, err = m.check(ctx)
				require(t, err)
			}
			id := envmodel.ID()
			_, startErr := p.service.Start(ctx, id, json.RawMessage(`{}`), envmodel.Start{EnvironmentKey: mustEnvironmentKey(t, prepared.Input.Execution), Profile: "remote", ExpectedCurrentRevision: expected, TimeoutSeconds: 60})
			if release != nil {
				close(release)
				require(t, <-finish)
			}
			if startErr == nil {
				t.Fatal("a prepared old baseline became current after a newer observation began")
			}
			if _, err := store.Environment(ctx, c.Identity(), id); !errors.Is(err, envmodel.ErrNotFound) {
				t.Fatal("rejected stale start left an activity", err)
			}
		})
	}
}
