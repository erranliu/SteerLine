package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
)

func commitEnvironmentMaterials(t *testing.T, c Config) string {
	t.Helper()
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "update project interface")
	fixtureGit(t, c.Repository, "push", "origin", "main")
	return pmGit(t, c.Repository, "rev-parse", "HEAD")
}

func TestEnvironmentProjectSourcesUseCustomCatalogueWithoutLegacyFiles(t *testing.T) {
	p, c, _ := environmentFixture(t)
	for old, next := range map[string]string{"SteerlineFlow/testing.md": "quality/policy.md", "SteerlineFlow/testing/test-list.json": "quality/cases.json"} {
		body, err := os.ReadFile(filepath.Join(c.Repository, old))
		require(t, err)
		require(t, os.MkdirAll(filepath.Join(c.Repository, "quality"), 0700))
		require(t, os.WriteFile(filepath.Join(c.Repository, next), body, 0600))
		require(t, os.Remove(filepath.Join(c.Repository, old)))
	}
	entry := pm.ProjectEntry{Version: 1, Requirements: []string{"quality/policy.md"}, TestingCatalogue: "quality/cases.json"}
	body, _ := json.Marshal(entry)
	require(t, os.WriteFile(filepath.Join(c.Repository, pm.ProjectEntryPath), body, 0600))
	commit := commitEnvironmentMaterials(t, c)
	prepared, err := p.materialAt(context.Background(), "fixture", commit, false)
	require(t, err)
	refs, _ := json.Marshal(prepared.Input.Documents.References)
	if !strings.Contains(string(refs), "quality/cases.json") || !strings.Contains(string(refs), "quality/policy.md") || strings.Contains(string(refs), "SteerlineFlow/testing.md") || strings.Contains(string(refs), "SteerlineFlow/testing/test-list.json") {
		t.Fatal("project sources were replaced by fixed paths", string(refs))
	}
	key, _ := envmodel.Key(c.RunnerExecution.Profiles[0].Execution)
	in, err := p.generationInput(context.Background(), envmodel.Generate{TargetKey: key, Profile: "fixture"})
	require(t, err)
	if in.Generate.Repository.Paths == nil {
		t.Fatal("valid explicit references unnecessarily surveyed source")
	}
}

func TestEnvironmentBlockingGapsAreReadWithoutGenerationRecords(t *testing.T) {
	_, c, _ := environmentFixture(t)
	key, _ := envmodel.Key(c.RunnerExecution.Profiles[0].Execution)
	manifest, err := envmodel.ParseManifest([]byte(environmentFixtureDocument(key, "ps1", "environment-validation", []string{"go.mod"})))
	require(t, err)
	manifest.Gaps = []string{"project runtime version is unknown"}
	body, _ := json.Marshal(manifest)
	require(t, os.WriteFile(filepath.Join(c.Repository, envmodel.DocumentPath(key)), []byte("# Manual project material\n```steerline-environment\n"+string(body)+"\n```\n"), 0600))
	commit := commitEnvironmentMaterials(t, c)
	// This owner has no store or generation history to consult.
	cold := newEnvironmentPort(c, nil, runnerclient.Client{}, nil)
	if _, err := cold.materialAt(context.Background(), "fixture", commit, false); err == nil || !strings.Contains(err.Error(), manifest.Gaps[0]) {
		t.Fatal("standalone material failed to block", err)
	}
	value, err := cold.materials(context.Background(), key)
	require(t, err)
	got := value.(map[string]any)
	if len(got["gaps"].([]string)) != 1 || got["verification_diagnostic"] == nil || got["expected_context"] != nil {
		t.Fatal("blocking material depended on generation history", got)
	}
}

func TestEnvironmentAdaptationRepairsObsoleteOutputsWithoutAcceptingThem(t *testing.T) {
	for _, mode := range []string{"missing-document", "deleted-dependency", "deleted-script", "broken-manifest"} {
		t.Run(mode, func(t *testing.T) {
			p, c, _ := environmentFixture(t)
			key, _ := envmodel.Key(c.RunnerExecution.Profiles[0].Execution)
			switch mode {
			case "missing-document":
				require(t, os.Remove(filepath.Join(c.Repository, envmodel.DocumentPath(key))))
			case "deleted-dependency":
				require(t, os.Remove(filepath.Join(c.Repository, "go.mod")))
			case "deleted-script":
				require(t, os.Remove(filepath.Join(c.Repository, envmodel.ScriptPath(key, "ps1"))))
			case "broken-manifest":
				require(t, os.WriteFile(filepath.Join(c.Repository, envmodel.DocumentPath(key)), []byte("# Original requirements\n```steerline-environment\n{\"dependencies\":[\"../outside\"]}\n```\n"), 0600))
			}
			commit := commitEnvironmentMaterials(t, c)
			in, err := p.generationInput(context.Background(), envmodel.Generate{TargetKey: key, Profile: "fixture"})
			require(t, err)
			if in.Generate.Repository.Paths != nil || !strings.Contains(in.Generate.Instruction, "SteerlineProjectGuideDoc/environment-adaptation.md") {
				t.Fatal("repair did not receive actual source and guidance")
			}
			source := appRepositoryManifest(t, *in.Generate.Repository)
			found := false
			for _, file := range source.Files {
				if file.Path == envmodel.DocumentPath(key) {
					found = true
				}
				if strings.HasPrefix(file.Path, "../") {
					t.Fatal("unsafe old reference followed")
				}
			}
			if !found && mode != "missing-document" {
				t.Fatal("original requirements lost during repair")
			}
			if _, err := p.materialAt(context.Background(), "fixture", commit, false); err == nil {
				t.Fatal("invalid old runtime interface was admitted")
			}
		})
	}
}

func TestEnvironmentSurveyBoundsActualSourceAndRequiresGuidance(t *testing.T) {
	for _, mode := range []string{"oversized-source", "missing-guidance", "missing-policy"} {
		t.Run(mode, func(t *testing.T) {
			p, c, _ := environmentFixture(t)
			key, _ := envmodel.Key(c.RunnerExecution.Profiles[0].Execution)
			require(t, os.Remove(filepath.Join(c.Repository, envmodel.DocumentPath(key))))
			switch mode {
			case "oversized-source":
				require(t, os.WriteFile(filepath.Join(c.Repository, "actual-source.txt"), []byte(strings.Repeat("x", 1<<20)), 0600))
			case "missing-guidance":
				require(t, os.Remove(filepath.Join(c.Repository, "SteerlineProjectGuideDoc/environment-adaptation.md")))
			case "missing-policy":
				require(t, os.Remove(filepath.Join(c.Repository, "SteerlineFlow/testing.md")))
			}
			commit := commitEnvironmentMaterials(t, c)
			if _, _, err := p.generationFiles(context.Background(), commit, key); err == nil {
				t.Fatal("unsafe survey was offered", mode)
			}
		})
	}
}

func TestEnvironmentRuntimeDependenciesBindBasisWithoutExpandingReadingReferences(t *testing.T) {
	p, c, _ := environmentFixture(t)
	commit := pmGit(t, c.Repository, "rev-parse", "HEAD")
	before, err := p.materialAt(context.Background(), "fixture", commit, false)
	require(t, err)
	for _, ref := range before.Input.Documents.References {
		if ref.Path == "go.mod" {
			t.Fatal("runtime helper repeated as Agent reading reference")
		}
	}
	if before.Manifest.Dependencies[0] != "go.mod" {
		t.Fatal("runtime dependency removed from authority")
	}
	require(t, os.WriteFile(filepath.Join(c.Repository, "go.mod"), []byte("module changed.invalid/runtime\n"), 0600))
	commit = commitEnvironmentMaterials(t, c)
	after, err := p.materialAt(context.Background(), "fixture", commit, false)
	require(t, err)
	if after.Inputs == before.Inputs {
		t.Fatal("runtime dependency lost qualification binding")
	}
	require(t, os.Remove(filepath.Join(c.Repository, "go.mod")))
	commit = commitEnvironmentMaterials(t, c)
	if _, err := p.materialAt(context.Background(), "fixture", commit, false); err == nil {
		t.Fatal("missing runtime dependency accepted")
	}
}
