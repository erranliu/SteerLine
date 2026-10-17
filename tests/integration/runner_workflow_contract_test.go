//go:build runnercontract

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestRunnerPublishedWorkflowContract(t *testing.T) {
	exe := os.Getenv("STEERRUNNER_EXE")
	if exe == "" {
		t.Fatal("STEERRUNNER_EXE must explicitly select the actual Runner build")
	}
	exe, err := filepath.Abs(exe)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	cli := runnerclient.Client{Executable: exe, SHA256: hex.EncodeToString(sum[:]), StateDir: filepath.Join(t.TempDir(), "runner"), Timeout: 10 * time.Second}
	ctx := context.Background()
	version, err := cli.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	info, err := cli.StartService(ctx, version)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cli.StopService(ctx, info.InstanceID, "contract-stop-"+info.InstanceID); err != nil {
			t.Error(err)
		}
		if err := cli.VerifyServiceSettlement(ctx, info.InstanceID, "contract-stop-"+info.InstanceID); err != nil {
			t.Error(err)
		}
	})
	cli.InstanceID, cli.BuildVersion, cli.ResourceDigest = info.InstanceID, info.Version, info.ResourceDigest
	cat, err := cli.Catalog(ctx, version)
	if err != nil {
		t.Fatal(err)
	}
	caps, err := cli.Call(ctx, "capabilities", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Persist capabilities only in the test evidence, never as a runtime fixture.
	if dir := os.Getenv("RUNNER_CONTRACT_EVIDENCE"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, "capabilities.json"), caps.Data, 0600); err != nil {
			t.Fatal(err)
		}
	}

	// Exercise Runner's public enforcement directly, independently of the caller's
	// preflight. A stale bound call must not fall through to the replacement.
	stale := exec.CommandContext(ctx, exe, "capabilities", "--state-dir", cli.StateDir, "--json", "--expected-instance-id", "different-instance")
	out, staleErr := stale.Output()
	var failure *exec.ExitError
	if !errors.As(staleErr, &failure) || failure.ExitCode() != 5 || !strings.Contains(string(out), `"stale_instance"`) {
		t.Fatalf("actual Runner did not fence stale request: %s %v", out, staleErr)
	}
	execution := runnerclient.ExecutionInput{Adapter: "codex-app-server", Target: runnerclient.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"executable":"codex","model":"contract-execution","reasoning_effort":"medium"}`)}
	plain := runnerclient.Workflow{ID: "development", Version: 1}
	audited := runnerclient.Workflow{ID: "development-audited", Version: 1, AuditModel: &runnercontract.ModelSettings{Model: "contract-audit", ReasoningEffort: "high"}}
	for _, w := range []runnerclient.Workflow{plain, audited} {
		if err := cat.ValidateConfiguration(execution, w); err != nil {
			t.Fatalf("%s: %v", w.ID, err)
		}
	}
	for _, tc := range []struct {
		name string
		e    runnerclient.ExecutionInput
		w    runnerclient.Workflow
	}{
		{"padded plain model", runnerclient.ExecutionInput{Adapter: "codex-app-server", Target: runnerclient.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"model":" x "}`)}, plain},
		{"missing audit model", execution, runnerclient.Workflow{ID: "development-audited", Version: 1}},
		{"audit on plain", execution, runnerclient.Workflow{ID: "development", Version: 1, AuditModel: audited.AuditModel}},
		{"unknown workflow", execution, runnerclient.Workflow{ID: "unknown", Version: 1}},
		{"wrong workflow version", execution, runnerclient.Workflow{ID: "development", Version: 99}},
		{"unknown override", execution, runnerclient.Workflow{ID: audited.ID, Version: 1, AuditModel: audited.AuditModel, PromptOverrides: map[string]string{"unknown": "x"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := cat.ValidateConfiguration(tc.e, tc.w); err == nil {
				t.Fatal("invalid combination accepted")
			}
		})
	}
	body, err := json.Marshal(runnerclient.RunInput{Execution: execution, Workflow: audited})
	if err != nil {
		t.Fatal(err)
	}
	var encoded struct {
		Workflow struct {
			AuditModel struct {
				Model  string `json:"model"`
				Effort string `json:"reasoning_effort"`
			} `json:"audit_model"`
		} `json:"workflow"`
	}
	if err := json.Unmarshal(body, &encoded); err != nil {
		t.Fatal(err)
	}
	if encoded.Workflow.AuditModel.Model != "contract-audit" || encoded.Workflow.AuditModel.Effort != "high" {
		t.Fatalf("nested model was lost: %s", body)
	}
	t.Logf("EXE=%s SHA=%s version=%s protocol=%s resource=%s workflows=%d instance=%s; no model/PR invoked", exe, cli.SHA256, version.Version, runnerclient.Protocol, version.ResourceDigest, len(cat.Capabilities.Workflows), info.InstanceID)
}
