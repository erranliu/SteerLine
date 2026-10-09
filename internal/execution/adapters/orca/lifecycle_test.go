package orcaworker

import (
	"encoding/json"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestCleanupProgressRequiresConfirmedOriginalResource(t *testing.T) {
	resource := executionbackend.ProcessIdentity{Kind: "orca", Terminal: "original", RuntimeID: "runtime", Incarnation: "incarnation"}
	snapshot := executionbackend.LifecycleSnapshot{Ref: executionbackend.ExecutionRef{External: resource}, Session: executionbackend.SessionContext{ExecutionID: "execution"}}
	input, err := json.Marshal(executionbackend.CleanupInput{Resources: []executionbackend.ProcessIdentity{resource}})
	if err != nil {
		t.Fatal(err)
	}
	operation := executionbackend.Operation{Action: string(executionbackend.LifecycleCleanup), Status: executionbackend.OperationUnknown, Request: input, Ref: &snapshot.Ref, LifecycleIdentity: &executionbackend.LifecycleOperationIdentity{ExecutionID: "execution"}}
	snapshot.Operations = []executionbackend.Operation{operation}
	if cleanupConfirmed(snapshot, resource) {
		t.Fatal("unknown cleanup advanced progress")
	}
	snapshot.Operations[0].Status = executionbackend.OperationConfirmed
	if !cleanupConfirmed(snapshot, resource) {
		t.Fatal("committed cleanup did not advance its original resource")
	}
	replacement := resource
	replacement.Incarnation = "replacement"
	if cleanupConfirmed(snapshot, replacement) {
		t.Fatal("original receipt closed a replacement resource")
	}
}

func stableIDValue(last byte) string {
	const hex = "0123456789abcdef"
	value := make([]byte, 32)
	for i := range value {
		value[i] = hex[0]
	}
	value[len(value)-1] = hex[int(last)]
	return string(value)
}
