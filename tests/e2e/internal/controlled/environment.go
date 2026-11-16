package controlled

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// This public peer scripts a sealed adaptation result so recovery tests can
// pass the real caller admission gate. It proves no native environment or PR.
func scriptedEnvironment(input runnercontract.RunInput, run *runnercontract.Run) (runnercontract.ResultFile, error) {
	initial, err := base64.StdEncoding.Strict().DecodeString(input.ResultFile.InitialContentBase64)
	if err != nil {
		return runnercontract.ResultFile{}, err
	}
	var report env.Report
	if err = json.Unmarshal(initial, &report); err != nil {
		return runnercontract.ResultFile{}, err
	}
	report.Completed = true
	report.Ready = true
	report.Message = "scripted project result; not native qualification"
	report.Details = json.RawMessage(`{"format":"fixture-owned","checks":[{"id":"opaque-project-only","status":"failed"}]}`)
	body, err := json.Marshal(report)
	if err != nil {
		return runnercontract.ResultFile{}, err
	}
	hash := sha256.Sum256(body)
	encoded := base64.StdEncoding.EncodeToString(body)
	now := time.Now().UTC()
	run.Status, run.Phase, run.StopStatus, run.NewInputsAllowed = "completed", "closed", "stopped", false
	run.Closed = true
	run.Controls = runnercontract.Controls{}
	run.Final = &runnercontract.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "original-command", SessionID: "original-session", TurnID: "original-turn"}
	return runnercontract.ResultFile{Contract: runnercontract.ResultContract, RunID: run.RunID, Version: 1, ObservedAt: &now, Encoding: "base64", ContentBase64: &encoded, SizeBytes: len(body), SHA256: hex.EncodeToString(hash[:]), CollectionStatus: "closed"}, nil
}

func advertiseResult(caps map[string]any) {
	caps["checkout_probe"] = map[string]string{"contract": "steerrunner-checkout-probe/v1", "submit_property": "checkout_probe", "workspace": "existing_checkout"}
	caps["offline_inspect"] = map[string]any{"command": "inspect --offline", "read_only": true, "writer_absent": true, "starts_workers": false}
	found := false
	for _, raw := range caps["workflows"].([]any) {
		w := raw.(map[string]any)
		found = found || w["id"] == "execute"
	}
	if !found {
		caps["workflows"] = append(caps["workflows"].([]any), map[string]any{"id": "execute", "version": 1, "supported_adapters": []string{"codex-app-server", "orca-terminal"}, "prompt_steps": []string{"execute"}, "states": map[string]any{"execute": map[string]any{"kind": "execute", "contract": "final", "prompt_key": "execute"}, "closed": map[string]any{"kind": "terminal"}}})
	}

	// Derive the final-only schema from this peer's existing public file shape.
	raw, _ := json.Marshal(caps["progress_file_schema"])
	var schema map[string]any
	_ = json.Unmarshal(raw, &schema)
	schema["$id"] = runnercontract.ResultContract
	properties := schema["properties"].(map[string]any)
	properties["contract"] = map[string]any{"const": runnercontract.ResultContract}
	properties["version"] = map[string]any{"type": "integer", "minimum": 0, "maximum": 1}
	properties["collection_status"] = map[string]any{"type": "string", "enum": []string{"disabled", "pending", "closed"}}
	caps["result_file_schema"] = schema
	caps["result_file"] = map[string]any{"contract": runnercontract.ResultContract, "command": "result-file --run-id R --json", "submit_property": "result_file", "environment_variable": "STEERLINE_RESULT_FILE", "max_bytes": 65536, "encoding": "base64", "digest": "sha256", "collection": "end_only", "query_reads_file": false, "supported": map[string]any{"adapters": []string{"codex-app-server", "orca-terminal"}, "target_kinds": []string{"local", "orca_environment"}, "workflows": []string{"execute/1", "development/1", "development/2", "development-audited/2"}}}
}
