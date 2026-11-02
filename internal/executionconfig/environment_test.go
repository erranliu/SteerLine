package executionconfig

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestEnvironmentsGroupTargetsWithoutProjectOrModel(t *testing.T) {
	local := runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"model":"one"}`)}
	orca := local
	orca.Adapter = "orca-terminal"
	remote := orca
	remote.Target = runnercontract.ExecutionTarget{Kind: "orca_environment", Environment: "remote-id"}
	profiles := []Profile{{ID: "one", Execution: local}, {ID: "orca", Execution: orca}, {ID: "another", Execution: local}, {ID: "remote", Execution: remote, Workflow: runnercontract.Workflow{ID: "generate"}}}
	groups := Environments(profiles)
	if len(groups) != 3 || len(groups[0].ProfileIDs) != 2 || groups[0].ProfileIDs[1] != "another" || groups[1].ProfileIDs[0] != "orca" || groups[2].Target != remote.Target {
		t.Fatal(groups)
	}
	if Target(local) != `{"Adapter":"codex-app-server","Target":{"kind":"local"}}` {
		t.Fatal("target occupancy representation changed", Target(local))
	}
	changed := local
	changed.Settings = json.RawMessage(`{"model":"other","agent":"another","access_mode":"sandbox"}`)
	if Target(changed) != Target(local) || Target(orca) == Target(local) {
		t.Fatal("target identity depends on agent/model or ignores adapter")
	}
	remote.Target.Environment = ""
	if Target(remote) != "" {
		t.Fatal("unidentified remote target accepted")
	}
}
