package backend

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/hostcontract"
)

func TestExecutionRecordsKeepWireIdentity(t *testing.T) {
	ref := ExecutionRef{Version: 1, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV3, Target: ExecutionTarget{ID: "target"}, Agent: AgentSpec{Name: "codex"}, RunID: "run", AttemptID: "attempt", External: hostcontract.ProcessIdentity{Kind: "orca", Terminal: "terminal", RuntimeID: "runtime", Incarnation: "incarnation"}}
	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"backend":"orca","protocol":"orca_terminal_v3","target":{"id":"target"},"agent":{"name":"codex"},"run_id":"run","attempt_id":"attempt","external":{"kind":"orca","terminal":"terminal","runtime_id":"runtime","incarnation":"incarnation"}}`
	if string(data) != want {
		t.Fatalf("binding representation changed: %s", data)
	}
	var restored ExecutionRef
	if err := json.Unmarshal(data, &restored); err != nil || restored != ref {
		t.Fatalf("native binding identity changed: %+v / %v", restored, err)
	}
	question := Question{ID: "choice", Header: "Choice", Question: "Continue?", Options: []DecisionOption{{ID: "yes", Label: "Yes"}}, AllowFreeText: true}
	data, err = json.Marshal(question)
	if err != nil || string(data) != `{"id":"choice","header":"Choice","question":"Continue?","options":[{"id":"yes","label":"Yes"}],"allow_free_text":true}` {
		t.Fatalf("persisted native question representation changed: %s / %v", data, err)
	}
	hostOption := DecisionOption{ID: "recheck_environment", Label: "Recheck", Receiver: "center", Action: "recheck_environment"}
	data, err = json.Marshal(hostOption)
	if err != nil || string(data) != `{"id":"recheck_environment","label":"Recheck","receiver":"center","action":"recheck_environment"}` {
		t.Fatalf("host action routing changed: %s / %v", data, err)
	}
}
