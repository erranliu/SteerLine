//go:build runneracceptance

package mainagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/mainagent/model"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// One explicit real, read-only local Codex analysis. No task candidate, remote
// repository write, development workflow, or publication is involved.
func TestMainAgentLocalCodexPublishedAnalysis(t *testing.T) {
	exe := os.Getenv("STEERRUNNER_EXE")
	if exe == "" || !filepath.IsAbs(exe) {
		t.Fatal("explicit absolute STEERRUNNER_EXE required")
	}
	raw, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	root, err := os.MkdirTemp("", "steerline-main-agent-live-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("isolated live root=%s (retained if settlement fails)", root)
	cli := runnerclient.Client{Executable: exe, SHA256: hex.EncodeToString(sum[:]), StateDir: filepath.Join(root, "runner"), Timeout: 35 * time.Second}
	version, err := cli.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	info, err := cli.StartService(ctx, version)
	if err != nil {
		t.Fatal(err)
	}
	cli.InstanceID, cli.BuildVersion, cli.ResourceDigest = info.InstanceID, info.Version, info.ResourceDigest
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		id := "stop-main-agent-live-" + info.InstanceID
		if err := cli.StopService(cleanup, info.InstanceID, id); err != nil {
			t.Error(err)
		}
		if err := cli.VerifyServiceSettlement(cleanup, info.InstanceID, id); err != nil {
			t.Error(err)
			return
		}
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	cat, err := cli.Catalog(ctx, version)
	if err != nil {
		t.Fatal(err)
	}
	cli = cli.WithCatalog(cat)
	modelID, effort := "gpt-6.1-sol", "high"
	var baseExecution *runnercontract.ExecutionInput
	if executable := os.Getenv("MAIN_AGENT_CODEX_EXE"); executable != "" {
		if !filepath.IsAbs(executable) {
			t.Fatal("MAIN_AGENT_CODEX_EXE must be absolute")
		}
		resolved, resolveErr := filepath.EvalSymlinks(executable)
		file, openErr := os.Open(resolved)
		if resolveErr != nil || openErr != nil {
			t.Fatalf("native executable filesystem precondition: resolved=%q resolve=%v open=%v", resolved, resolveErr, openErr)
		}
		_ = file.Close()
		settings, _ := json.Marshal(map[string]string{"executable": executable, "access_mode": "full_access"})
		baseExecution = &runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: settings}
	} else {
		t.Fatal("explicit absolute MAIN_AGENT_CODEX_EXE required")
	}

	config, err := cli.ConfigureExecution(ctx, cat, runnercontract.ExecutionConfigurationRequest{Operation: "compose", BaseExecution: baseExecution, Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Selection: &runnercontract.ExecutionSelection{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Agent: "codex", Model: &modelID, ReasoningEffort: &effort, AccessMode: "read_only"}})
	if err != nil || config.Execution == nil {
		t.Fatal(err)
	}
	t.Logf("explicit native configuration=%s", config.Execution.Settings)
	repository := filepath.Join(root, "project")
	if err = os.MkdirAll(filepath.Join(repository, "SteerlineFlow"), 0700); err != nil {
		t.Fatal(err)
	}
	rules, _ := prompts.MainAgentRules()
	if err = os.WriteFile(filepath.Join(repository, filepath.FromSlash(model.RulesPath)), rules, 0600); err != nil {
		t.Fatal(err)
	}
	body := "# Task\nAn empty request must return an error. No implementation is requested.\n"
	if err = os.WriteFile(filepath.Join(repository, "task.spec.md"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		c := exec.CommandContext(ctx, "git", args...)
		c.Dir = repository
		b, e := c.CombinedOutput()
		if e != nil {
			t.Fatal(e, string(b))
		}
		return strings.TrimSpace(string(b))
	}
	git("init", "-b", "main")
	git("add", ".")
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "analysis fixture")
	commit := git("rev-parse", "HEAD")
	doc := model.Document{Path: "task.spec.md", Commit: commit, SHA256: model.Digest([]byte(body)), Body: body}
	store := &memory{rows: map[string]model.Record{}}
	service := &Service{Project: "main-agent-live", Repository: repository, Store: store, Runner: cli, Build: cli.BuildPin(), Read: func(_ context.Context, revision, path string) ([]byte, error) {
		command := exec.CommandContext(ctx, "git", "show", revision+":"+path)
		command.Dir = repository
		return command.Output()
	}, Context: func(context.Context, model.Start) (model.Context, error) {
		return model.Context{CandidateID: strings.Repeat("b", 32), CandidateRevision: 1, RepositoryCommit: commit, OriginalBase: commit, Original: []model.Document{doc}, Current: []model.Document{doc}, SpecPath: doc.Path, PlanPath: "task.plan.md", Question: &runnercontract.Question{ID: "empty-input", Text: "What should an empty request do?", AllowFreeText: true}}, nil
	}, Prepare: func(context.Context) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
		return *config.Execution, cat, nil
	}}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := service.Close(cleanup); err != nil {
			t.Error(err)
		}
	})
	record, err := service.Start(ctx, strings.Repeat("a", 32), model.Start{CandidateID: strings.Repeat("b", 32), CandidateRevision: 1, Kind: "clarification", Note: "What should an empty request do? Explain the existing requirement. No new decision or document revision is requested."})
	if err != nil {
		t.Fatal(err)
	}
	for !record.Complete() && ctx.Err() == nil {
		record, err = service.Observe(ctx, record.ID)
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(300 * time.Millisecond):
		}
	}
	if record.State != "review" || record.Result == nil || record.Result.Disposition != "clarification" || record.Result.Answer == nil || len(record.Result.Changes) > 0 {
		t.Fatalf("native analysis did not produce a verified clarification: %s %s", record.State, record.Diagnostic)
	}
	if out := os.Getenv("MAIN_AGENT_LIVE_EVIDENCE"); out != "" {
		raw, _ := json.MarshalIndent(record, "", "  ")
		if err = os.WriteFile(out, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("Runner=%s SHA=%s resource=%s Run=%s request=%s session=%s model=%s/%s state=%s stopped=%s", version.Version, cli.SHA256, version.ResourceDigest, record.RunID, record.Request.RequestID, record.Projection.Execution.SessionID, modelID, effort, record.State, record.Projection.StopStatus)
}
