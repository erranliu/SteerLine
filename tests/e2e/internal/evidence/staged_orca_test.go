package evidence

import (
	"encoding/json"
	"testing"

	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func TestNativeOrcaStageLaunchEvidenceRequiresActualFrozenSelection(t *testing.T) {
	e := executionmodel.ExecutionRecord{ProjectIdentity: "project", ExecutionID: "execution", CurrentAttemptID: "attempt", Generation: "generation", ConfigurationDigest: "config", Protocol: string(executionbackend.ProtocolOrcaTerminalV2)}
	process := executionbackend.ProcessIdentity{Kind: "orca", Terminal: "terminal", RuntimeID: "runtime", Incarnation: "incarnation"}
	e.Process = &process
	profile := &executionmodel.FixedExecutionProfile{ID: "profile", AcceptedConfigVersion: "accepted", Host: "orca", Agent: "opaque-agent"}
	payload, _ := orcaworker.EncodeProfileConfig(orcaworker.ProfileConfig{Executable: "orca", Args: []string{}, Model: "gpt-6-luna", ReasoningEffort: "medium"})
	envelope, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{CodecVersion: orcaworker.ProfileCodecVersion, Purpose: executionbackend.ImplementationConfigPurpose, ImplementationKey: orcaworker.ProfileCodecKey, ProjectID: e.ProjectIdentity, ProfileID: profile.ID, AcceptedConfigVersion: profile.AcceptedConfigVersion}, payload)
	profile.ImplementationData = []executionbackend.ImplementationDataEnvelope{must(t, envelope, err)}
	state := orcaworker.NativeExecutionState{Turns: []orcaworker.NativeTurnState{{TurnID: "command", TaskID: "task", DispatchID: "dispatch"}}}
	data, err := orcaworker.NewExecutionStateEnvelope(orcaworker.ExecutionStateIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest}, state)
	e.ImplementationData = []executionbackend.ImplementationDataEnvelope{must(t, data, err)}
	raw := []byte(`{"taskId":"task","dispatchId":"dispatch","launch":{"effective":{"agent":"opaque-agent","model":"gpt-6-luna","effort":"medium"}}}`)
	op := executionmodel.OperationRecord{Kind: "start_worker_turn", Receipt: raw, LifecycleIdentity: &executionbackend.LifecycleOperationIdentity{ExecutionID: e.ExecutionID, Ref: executionbackend.ExecutionRef{AttemptID: e.CurrentAttemptID}}}
	domain := reconstruction.Snapshot{Executions: []executionmodel.ExecutionRecord{e}, WorkflowRuns: []executionmodel.WorkflowRunRecord{{ExecutionProfile: profile}}, Operations: []executionmodel.OperationRecord{op}}
	if err := FixedNativeLaunch(domain); err != nil {
		t.Fatal(err)
	}
	selection, err := ModelSelection(domain)
	if err != nil || selection.Model != "gpt-6-luna" || selection.Effort != "medium" {
		t.Fatalf("native profile selection changed: %+v %v", selection, err)
	}
	for _, field := range []string{"taskId", "dispatchId", "model", "effort", "agent"} {
		t.Run(field, func(t *testing.T) {
			var receipt map[string]any
			if err := json.Unmarshal(raw, &receipt); err != nil {
				t.Fatal(err)
			}
			if field == "taskId" || field == "dispatchId" {
				receipt[field] = "other"
			} else {
				receipt["launch"].(map[string]any)["effective"].(map[string]any)[field] = "other"
			}
			domain.Operations[0].Receipt, _ = json.Marshal(receipt)
			if err := FixedNativeLaunch(domain); err == nil {
				t.Fatal("changed native launch selection was accepted")
			}
		})
	}
}
