//go:build runneracceptance

package runnerclient

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"os"
	"testing"
	"time"
)

func TestExecutionConfigurationPublishedRunner(t *testing.T) {
	ctx := context.Background()
	selection := Selection{Executable: os.Getenv("STEERRUNNER_EXE"), Version: os.Getenv("STEERRUNNER_VERSION"), SHA256: os.Getenv("STEERRUNNER_SHA256")}
	client, version, err := selection.Open(ctx, t.TempDir(), 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := client.OfflineCatalog(ctx, version)
	if err != nil {
		t.Fatal(err)
	}
	execution := runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"full_access","executable":"C:/not-installed/codex.exe","model":"selected","reasoning_effort":"high"}`)}
	description, err := client.ConfigureExecution(ctx, catalog, runnercontract.ExecutionConfigurationRequest{Operation: "describe", Execution: &execution, Workflow: runnercontract.Workflow{ID: "development", Version: 2}})
	if err != nil || description.Identity == nil {
		t.Fatal(description, err)
	}
	derived, err := client.ConfigureExecution(ctx, catalog, runnercontract.ExecutionConfigurationRequest{Operation: "derive", Execution: &execution, Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, AccessMode: "read_only"})
	if err != nil || derived.Execution == nil || derived.View.AccessMode != "read_only" || derived.View.Model == nil || *derived.View.Model != "selected" {
		t.Fatal(derived, err)
	}
	write, err := client.ConfigureExecution(ctx, catalog, runnercontract.ExecutionConfigurationRequest{Operation: "derive", Execution: &execution, Workflow: runnercontract.Workflow{ID: "execute", Version: 1}, AccessMode: "full_access"})
	if err != nil || write.Execution == nil {
		t.Fatal(write, err)
	}
	if err = catalog.ValidateMaintenance(*write.Execution, *derived.Execution); err != nil {
		t.Fatal("maintenance capability/derivation", err)
	}
	model := "changed"
	composed, err := client.ConfigureExecution(ctx, catalog, runnercontract.ExecutionConfigurationRequest{Operation: "compose", BaseExecution: &execution, Workflow: runnercontract.Workflow{ID: "development", Version: 2}, Selection: &runnercontract.ExecutionSelection{Adapter: execution.Adapter, Target: execution.Target, Agent: "codex", AccessMode: "full_access", Model: &model}, Unset: []string{"reasoning_effort"}})
	if err != nil || composed.Identity == nil || composed.View.ReasoningEffort != nil {
		t.Fatal(composed, err)
	}
	first, _ := runnercontract.ExecutionContextKey(execution, *description.Identity)
	second, err := runnercontract.ExecutionContextKey(*composed.Execution, *composed.Identity)
	if err != nil || first != second || description.Identity.SourceExecutionDigest == composed.Identity.SourceExecutionDigest {
		t.Fatal("preference-only change affected identity", first, second, err)
	}
	t.Logf("fixed Runner configuration: version=%s sha256=%s resource=%s", client.BuildVersion, client.SHA256, catalog.Capabilities.ResourceDigest)
}
