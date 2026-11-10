//go:build runneracceptance

package suggestionbridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/steeragent/suggestion/contract"
)

// Uses only the existing public Runner generate path and an isolated state-dir.
// It neither creates a development task nor publishes a PR.
func TestAdvisoryLocalCodexGeneratePublished(t *testing.T) {
	exe, native := os.Getenv("STEERRUNNER_EXE"), os.Getenv("MAIN_AGENT_CODEX_EXE")
	if !filepath.IsAbs(exe) || !filepath.IsAbs(native) {
		t.Fatal("explicit absolute Runner and Codex EXE paths required")
	}
	raw, e := os.ReadFile(exe)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	root, e := os.MkdirTemp("", "steerline-advisory-generate-live-")
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("isolated root=%s; retained if settlement fails", root)
	cli := runnerclient.Client{Executable: exe, SHA256: hex.EncodeToString(sum[:]), StateDir: filepath.Join(root, "runner"), Timeout: 35 * time.Second}
	version, e := cli.Version(ctx)
	if e != nil {
		t.Fatal(e)
	}
	info, e := cli.StartService(ctx, version)
	if e != nil {
		t.Fatal(e)
	}
	cli.InstanceID, cli.BuildVersion, cli.ResourceDigest = info.InstanceID, info.Version, info.ResourceDigest
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 45*time.Second)
		defer stop()
		id := "stop-advisory-live-" + info.InstanceID
		if e := cli.StopService(cleanup, info.InstanceID, id); e != nil {
			t.Error(e)
		}
		if e := cli.VerifyServiceSettlement(cleanup, info.InstanceID, id); e != nil {
			t.Error(e)
			return
		}
		if e := os.RemoveAll(root); e != nil {
			t.Error(e)
		}
	})
	catalog, e := cli.Catalog(ctx, version)
	if e != nil {
		t.Fatal(e)
	}
	cli = cli.WithCatalog(catalog)
	model, effort := "gpt-6.1-sol", "high"
	settings, _ := json.Marshal(map[string]string{"executable": native, "access_mode": "full_access"})
	configured, e := cli.ConfigureExecution(ctx, catalog, runnercontract.ExecutionConfigurationRequest{Operation: "compose", BaseExecution: &runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: settings}, Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Selection: &runnercontract.ExecutionSelection{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Agent: "codex", Model: &model, ReasoningEffort: &effort, AccessMode: "read_only"}})
	if e != nil || configured.Execution == nil {
		t.Fatal(e)
	}
	service, source, _ := fixture()
	service.Runner, service.RunnerBuild = cli, cli.BuildPin()
	service.Prepare = func(context.Context) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
		return *configured.Execution, catalog, nil
	}
	source.Goal = "根据提供的空输入要求及执行者问题，建议一段用于继续讨论的中文用户提示词。"
	service.Build = func(_ context.Context, a Source, id string) (contract.Request, error) {
		return contract.Request{Protocol: contract.Protocol, Capability: contract.Capability, RequestID: id, Context: contract.Context{Goal: a.Goal, Materials: []contract.Material{{ID: "spec", Label: "原任务 Spec", Body: "An empty request must return an error. No new implementation is requested."}, {ID: "question", Label: "执行者原问题", Body: "Should an empty request return a default value or an error?"}}}}, nil
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if e := service.Close(cleanup); e != nil {
			t.Error(e)
		}
	})
	if e = service.Sync(ctx, []Source{source}); e != nil {
		t.Fatal(e)
	}
	id := SourceID(service.Project, source)
	var result Record
	for ctx.Err() == nil {
		if e = service.ProcessNext(ctx); e != nil {
			t.Fatal(e)
		}
		result, e = service.Get(ctx, id)
		if e != nil {
			t.Fatal(e)
		}
		if result.Complete() {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(300 * time.Millisecond):
		}
	}
	if result.State != "ready" || result.Suggestion == nil || result.Projection == nil || result.Projection.StopStatus != "stopped" {
		t.Fatalf("generate failed: state=%s diagnostic=%s", result.State, result.Diagnostic)
	}
	if out := os.Getenv("MAIN_AGENT_LIVE_EVIDENCE"); out != "" {
		raw, _ := json.MarshalIndent(result, "", "  ")
		if e = os.WriteFile(out, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("Runner=%s SHA=%s resource=%s Run=%s request=%s session=%s configured=%s/%s state=%s stop=%s", info.Version, cli.SHA256, info.ResourceDigest, result.RunID, result.RunnerRequest.RequestID, result.Projection.Execution.SessionID, model, effort, result.State, result.Projection.StopStatus)
}
