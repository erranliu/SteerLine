package testprotocol

import (
	_ "embed"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"time"
)

// Public schema copied byte-for-byte from SteerRunner 5df57b69e98382abcd63676b04a73079fd939fa6,
// internal/resources/assets/execution-wait.schema.json. It is a public contract
// fixture, not evidence that the native Runner/Orca flow has passed.
//
//go:embed testdata/execution-wait.schema.json
var executionWaitSchema json.RawMessage

func RunnerExecutionWaitCapability() map[string]any {
	return map[string]any{"contract": runnercontract.ExecutionWaitContract, "same_snapshot": true, "run_property": "execution_wait", "schema": executionWaitSchema}
}

func RunnerExecutionWait(at time.Time) *runnercontract.ExecutionWait {
	return &runnercontract.ExecutionWait{Contract: runnercontract.ExecutionWaitContract, OperationID: "original-preparation", Query: "repository_inventory", Stage: "queue", ReasonCode: "orca_cli_busy", StartedAt: at, UpdatedAt: at, DeadlineAt: at.Add(90 * time.Second), MaxCalls: 5, Diagnostic: "Orca CLI channel is busy."}
}
