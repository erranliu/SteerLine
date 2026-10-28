package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
	progresstools "github.com/erranliu/SteerLine/tools"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPathCatalog(t *testing.T) *runnerclient.Catalog {
	t.Helper()
	raw, e := os.ReadFile("../runnerclient/testdata/content-capabilities.json")
	require(t, e)
	var caps runnerclient.Capabilities
	require(t, json.Unmarshal(raw, &caps))
	cat, e := runnerclient.CompileCatalog(caps)
	require(t, e)
	return cat
}
func TestTMCommittedPlanSnapshot(t *testing.T) {
	m, store, _ := pmsFixture(t)
	c := m.config
	work := callerFrozenWork(t, m, store.contract)
	raw := work.FrozenTestPlan
	run := callerTestRun()
	got, e := c.prepareTestPlan(context.Background(), work, &run)
	require(t, e)
	if string(got) != string(raw) || len(run.Documents.References) != 5 || !strings.Contains(run.Workflow.PromptOverrides["implement.execute"], testPromptContext(t, raw, true)) {
		t.Fatal("system context was not frozen", run)
	}
	if run.ProgressFile == nil {
		t.Fatal("progress was not frozen with candidate")
	}
	for _, key := range []string{"implement.report", "publish", "publish.report"} {
		if !strings.HasSuffix(run.Workflow.PromptOverrides[key], testmodel.TestingReadOnlyInstructions) {
			t.Fatal("report/publication can overwrite test progress", key)
		}
	}
	initial, err := base64.StdEncoding.DecodeString(run.ProgressFile.InitialContentBase64)
	require(t, err)
	if string(initial) != `{"test.one":false}` {
		t.Fatal(string(initial))
	}
	for _, ref := range run.Documents.References {
		if ref.Kind != "" || ref.Commit != "" || ref.Content != nil || strings.Contains(ref.Path, "steerline-test") {
			t.Fatal("pretend document path", ref)
		}
	}
	if _, e = (Config{}).prepareTestPlan(context.Background(), work, &run); e == nil {
		t.Fatal("missing published prompts accepted")
	}
	work.FrozenTestPlan = nil
	got, e = c.prepareTestPlan(context.Background(), work, &run)
	require(t, e)
	var missing testmanagement.Snapshot
	require(t, json.Unmarshal(got, &missing))
	if missing.Status != "missing" || run.ProgressFile != nil {
		t.Fatal("missing snapshot fell back to repository arrangement")
	}
}

func callerFrozenWork(t *testing.T, m *projectMaintenance, contract taskmodel.TaskContract) taskmodel.FixedWork {
	t.Helper()
	body, err := m.read(context.Background(), contract.PlanRef.Commit, testmodel.CataloguePath)
	require(t, err)
	catalogue, err := testmanagement.ParseCatalogue(body)
	require(t, err)
	snap := testmanagement.Snapshot{Status: "ready", PlanPath: contract.PlanRef.Path, Commit: contract.PlanRef.Commit, GenerationID: "original", Definitions: catalogue.Tests, Digest: testmanagement.Digest(catalogue.Tests)}
	raw, err := json.Marshal(snap)
	require(t, err)
	return taskmodel.FixedWork{FrozenTestPlan: raw, Contract: contract}
}
func callerTestRun() runnerclient.RunInput {
	return runnerclient.RunInput{Execution: runnerclient.ExecutionInput{Adapter: "codex-app-server", Target: runnerclient.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{}`)}, Workflow: runnerclient.Workflow{ID: "development", Version: 2}}
}

func TestCallerPromptConvergencePinnedInputsAndWriter(t *testing.T) {
	m, store, _ := pmsFixture(t)
	ctx := context.Background()
	for _, mode := range []string{"missing", "same", "crlf", "different"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(m.config.Repository, testmodel.WriterPath)
			defer func() {
				pmGit(t, m.config.Repository, "restore", "--", testmodel.CataloguePath)
				_ = os.WriteFile(path, []byte(progresstools.PowerShellWriter), 0600)
			}()
			require(t, os.MkdirAll(filepath.Dir(path), 0700))
			switch mode {
			case "missing":
				require(t, os.Remove(path))
			case "same":
				require(t, os.WriteFile(path, []byte(progresstools.PowerShellWriter), 0600))
			case "crlf":
				require(t, os.WriteFile(path, []byte(strings.ReplaceAll(strings.ReplaceAll(progresstools.PowerShellWriter, "\r\n", "\n"), "\n", "\r\n")), 0600))
			case "different":
				require(t, os.WriteFile(path, []byte(progresstools.PowerShellWriter+"\n# different"), 0600))
			}
			pmGit(t, m.config.Repository, "add", ".")
			pmGit(t, m.config.Repository, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", mode)
			contract := store.contract
			contract.PlanRef.Commit = pmGit(t, m.config.Repository, "rev-parse", "HEAD")
			work := callerFrozenWork(t, m, contract)
			// Mutable checkout must not influence the pinned source decision.
			require(t, os.WriteFile(path, []byte("uncommitted different writer"), 0600))
			require(t, os.WriteFile(filepath.Join(m.config.Repository, testmodel.CataloguePath), []byte("uncommitted invalid catalogue"), 0600))
			run := callerTestRun()
			full, err := m.config.prepareTestPlan(ctx, work, &run)
			referenced := mode == "same" || mode == "crlf"
			if !referenced {
				if err == nil {
					t.Fatal("missing or changed writer accepted")
				}
			} else {
				require(t, err)
				body := run.Workflow.PromptOverrides["implement.execute"]
				if string(full) != string(work.FrozenTestPlan) || !strings.Contains(body, testPromptContext(t, full, referenced)) {
					t.Fatal("frozen arrangement or source changed")
				}
				if strings.Contains(body, progresstools.PowerShellWriter) == referenced {
					t.Fatal("wrong writer distribution", mode)
				}
				if !strings.Contains(run.Workflow.PromptOverrides["publish"], testReadOnlyPrompt(t)) || strings.Contains(run.Workflow.PromptOverrides["publish"], "Set-RuntimeProgress") {
					t.Fatal("publication got writer")
				}
				if len(run.Documents.References) != 5 {
					t.Fatal("writer path not bound")
				}
			}
			// Restore only our own fixture files before the next commit.
			pmGit(t, m.config.Repository, "restore", "--", testmodel.CataloguePath)
			if mode == "missing" {
				require(t, os.WriteFile(path, []byte(progresstools.PowerShellWriter), 0600))
			} else {
				pmGit(t, m.config.Repository, "restore", "--", testmodel.WriterPath)
			}
		})
	}
	work := callerFrozenWork(t, m, store.contract)
	for _, mode := range []string{"digest", "definition", "note", "commit", "plan-digest", "missing-protocol", "wrong-protocol", "missing-arrangement", "wrong-arrangement", "missing-rules", "missing-catalogue"} {
		t.Run(mode, func(t *testing.T) {
			next := work
			var snap testmanagement.Snapshot
			require(t, json.Unmarshal(next.FrozenTestPlan, &snap))
			switch mode {
			case "digest":
				snap.Digest = "wrong"
			case "definition":
				snap.Definitions[0].Command = "changed"
				snap.Digest = testmanagement.Digest(snap.Definitions)
			case "note":
				snap.Definitions[0].Note = "lost note"
				snap.Digest = testmanagement.Digest(snap.Definitions)
			case "commit":
				snap.Commit = strings.Repeat("f", 40)
			case "plan-digest":
				next.Contract.PlanRef.SHA256 = strings.Repeat("0", 64)
			default:
				pmGit(t, m.config.Repository, "restore", "--source", store.contract.PlanRef.Commit, "--staged", "--worktree", "--", ".")
				path := testmodel.RulesPath
				if mode == "missing-catalogue" {
					path = testmodel.CataloguePath
				}
				switch mode {
				case "missing-protocol", "wrong-protocol":
					path = testmodel.ProtocolPath
				case "missing-arrangement", "wrong-arrangement":
					arrangement, err := testmodel.ArrangementPath(snap.PlanPath)
					require(t, err)
					path = arrangement
				}
				if strings.HasPrefix(mode, "wrong-") {
					require(t, os.WriteFile(filepath.Join(m.config.Repository, path), []byte(`{"tests":[]}`), 0600))
					pmGit(t, m.config.Repository, "add", ".")
				} else {
					pmGit(t, m.config.Repository, "rm", path)
				}
				pmGit(t, m.config.Repository, "-c", "commit.gpgsign=false", "commit", "-qm", mode)
				snap.Commit = pmGit(t, m.config.Repository, "rev-parse", "HEAD")
				next.Contract.PlanRef.Commit = snap.Commit
			}
			next.FrozenTestPlan, _ = json.Marshal(snap)
			run := callerTestRun()
			if _, err := m.config.prepareTestPlan(ctx, next, &run); err == nil {
				t.Fatal("invalid pinned source accepted", mode)
			}
		})
	}
}

func testPromptContext(t *testing.T, snapshot []byte, referenced bool) string {
	t.Helper()
	body, err := prompts.TestingContext(snapshot, referenced)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func testReadOnlyPrompt(t *testing.T) string {
	t.Helper()
	body, err := prompts.TestingReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	return body
}
