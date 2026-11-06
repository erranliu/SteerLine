package testprotocol

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"io"
	"os"
	"strings"
)

//go:embed execution_configuration_capability.json
var ExecutionConfigurationCapability json.RawMessage

// RunExecutionConfigurationFixture is a public CLI peer, with no execution or
// project state. The schema is frozen from the independently built Runner.
func RunExecutionConfigurationFixture(version string) bool {
	if len(os.Args) < 2 || os.Args[1] != "execution-config" {
		return false
	}
	raw, _ := io.ReadAll(io.LimitReader(os.Stdin, runnercontract.ExecutionConfigurationLimit+1))
	var input runnercontract.ExecutionConfigurationRequest
	if json.Unmarshal(raw, &input) != nil {
		return false
	}
	result, err := ExecutionConfigurationFixture(context.Background(), input)
	executable, _ := os.Executable()
	body, _ := os.ReadFile(executable)
	sha := sha256.Sum256(body)
	result.Source = runnercontract.ExecutionConfigurationSource{Version: version, ExecutableSHA256: hex.EncodeToString(sha[:]), ResourceDigest: strings.Repeat("a", 64)}
	envelope := map[string]any{"protocol": runnercontract.Protocol, "ok": err == nil, "request_id": nil, "data": result, "error": nil}
	if err != nil {
		envelope["data"] = nil
		envelope["error"] = map[string]string{"code": "unsupported_capability", "message": "fixture configuration unsupported"}
	}
	_ = json.NewEncoder(os.Stdout).Encode(envelope)
	if err != nil {
		os.Exit(3)
	}
	return true
}

// ExecutionIdentity supplies synthetic public facts for caller transaction
// fixtures. It is not a native validation oracle: schema/derivation semantics
// are verified against the independently built Runner executable.
func ExecutionIdentity(execution runnercontract.ExecutionInput) runnercontract.ExecutionIdentity {
	context := execution
	var settings map[string]json.RawMessage
	_ = json.Unmarshal(execution.Settings, &settings)
	if execution.Adapter == "codex-app-server" {
		delete(settings, "model")
		delete(settings, "reasoning_effort")
	} else if raw, ok := settings["agent_launch"]; ok {
		var launch map[string]json.RawMessage
		_ = json.Unmarshal(raw, &launch)
		delete(launch, "model")
		delete(launch, "reasoning_effort")
		if len(launch) == 0 {
			delete(settings, "agent_launch")
		} else {
			settings["agent_launch"], _ = json.Marshal(launch)
		}
	}
	context.Settings, _ = json.Marshal(settings)
	return runnercontract.ExecutionIdentity{Contract: runnercontract.ExecutionIdentityContract, SourceExecutionDigest: runnercontract.ConfigurationValueDigest(execution), Context: context}
}

func ExecutionConfigurationFixture(_ context.Context, in runnercontract.ExecutionConfigurationRequest) (runnercontract.ExecutionConfigurationResult, error) {
	result := runnercontract.ExecutionConfigurationResult{Contract: runnercontract.ExecutionConfigurationContract, Operation: in.Operation, InputDigest: runnercontract.ConfigurationValueDigest(in)}
	if in.Operation == "compose" && in.Selection != nil {
		selected := in.Selection
		execution := runnercontract.ExecutionInput{Adapter: selected.Adapter, Target: selected.Target, Settings: json.RawMessage(`{"access_mode":"full_access"}`)}
		if in.BaseExecution != nil && in.BaseExecution.Adapter == selected.Adapter && in.BaseExecution.Target == selected.Target {
			description, _ := ExecutionConfigurationFixture(context.Background(), runnercontract.ExecutionConfigurationRequest{Operation: "describe", Execution: in.BaseExecution})
			if description.View.Agent == selected.Agent {
				execution = *in.BaseExecution
			}
		}
		var settings map[string]any
		_ = json.Unmarshal(execution.Settings, &settings)
		fields := settings
		if selected.Adapter == "orca-terminal" {
			settings["agent"] = selected.Agent
			fields = map[string]any{}
			if old, ok := settings["agent_launch"].(map[string]any); ok {
				fields = old
			}
			settings["agent_launch"] = fields
		}
		settings["access_mode"] = selected.AccessMode
		if selected.Model != nil {
			fields["model"] = *selected.Model
		}
		if selected.ReasoningEffort != nil {
			fields["reasoning_effort"] = *selected.ReasoningEffort
		}
		for _, key := range in.Unset {
			delete(fields, key)
		}
		execution.Settings, _ = json.Marshal(settings)
		identity := ExecutionIdentity(execution)
		result.Execution, result.Identity, result.View = &execution, &identity, *selected
		return result, nil
	}
	if in.Execution == nil {
		return result, errors.New("fixture requires original execution")
	}
	execution := *in.Execution
	identity := ExecutionIdentity(execution)
	var settings map[string]json.RawMessage
	_ = json.Unmarshal(execution.Settings, &settings)
	modelSettings := settings
	result.View = runnercontract.ExecutionSelection{Adapter: execution.Adapter, Target: execution.Target, Agent: "codex"}
	_ = json.Unmarshal(settings["access_mode"], &result.View.AccessMode)
	if execution.Adapter == "orca-terminal" {
		modelSettings = map[string]json.RawMessage{}
		var agent string
		_ = json.Unmarshal(settings["agent"], &agent)
		if agent != "" {
			result.View.Agent = agent
		}
		_ = json.Unmarshal(settings["agent_launch"], &modelSettings)
	}
	_ = json.Unmarshal(modelSettings["model"], &result.View.Model)
	_ = json.Unmarshal(modelSettings["reasoning_effort"], &result.View.ReasoningEffort)
	if in.Operation == "describe" {
		result.Identity = &identity
		return result, nil
	}
	if in.Operation != "derive" || in.Workflow.Version != 1 {
		return result, errors.New("fixture configuration operation unsupported")
	}
	if in.Workflow.ID == "generate" {
		if execution.Adapter != "codex-app-server" || execution.Target.Kind != "local" || in.AccessMode != "read_only" {
			return result, errors.New("fixture requires local content profile")
		}
		settings["access_mode"] = json.RawMessage(`"read_only"`)
		execution.Settings, _ = json.Marshal(settings)
		result.View.AccessMode = "read_only"
	} else if in.Workflow.ID != "execute" || in.AccessMode != "full_access" {
		return result, errors.New("fixture derivation unsupported")
	}
	result.SourceIdentity = &identity
	result.Execution = &execution
	return result, nil
}
