package app

import (
	"context"
	"encoding/json"
	"errors"
	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"os"
	"path/filepath"
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
	r := envmodel.Record{ID: envmodel.ID(), Project: p.config.Identity(), Revision: 1, Kind: "adaptation", Profile: profile, Execution: prepared.Input.Execution, Prepared: prepared, State: "ready", CreatedAt: time.Now().UTC(), Requests: []envmodel.Request{{ID: envmodel.ID(), Original: json.RawMessage(`{}`)}}, Rounds: []envmodel.Round{{Number: 1, Settled: true}}}
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
	p, c, _ := environmentFixture(t)
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
}
func TestEnvironmentReadFailureDoesNotPersistRevocation(t *testing.T) {
	p, _, store := environmentFixture(t)
	r := seedDetectedEnvironment(t, p, "remote")
	ctx := context.Background()
	git := p.config.Git
	p.config.Git.Executable = "missing-environment-git-command"
	if e := p.observeBasis(ctx, r.Prepared.Commit); e == nil {
		t.Fatal("read failure accepted")
	}
	current, e := store.CurrentEnvironment(ctx, r.Project, mustEnvironmentKey(t, r.Execution))
	require(t, e)
	if current.Invalidated || current.Revision != 1 {
		t.Fatal("read outage revoked", current)
	}
	p.config.Git = git
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
