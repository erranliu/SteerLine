package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestConfigurationSnapshotKeepsFactsAndRefreshesOnlyExplicitFailures(t *testing.T) {
	execution := runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"full_access","model":"one"}`)}
	request := runnercontract.ExecutionConfigurationRequest{Operation: "describe", Execution: &execution, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}
	calls := 0
	broken := true
	snapshot := &ConfigurationSnapshot{Query: func(ctx context.Context, in runnercontract.ExecutionConfigurationRequest) (runnercontract.ExecutionConfigurationResult, error) {
		calls++
		if broken {
			return runnercontract.ExecutionConfigurationResult{}, errors.New("transient public CLI failure")
		}
		identity := runnercontract.ExecutionIdentity{Contract: runnercontract.ExecutionIdentityContract, SourceExecutionDigest: runnercontract.ConfigurationValueDigest(*in.Execution), Context: runnercontract.ExecutionInput{Adapter: in.Execution.Adapter, Target: in.Execution.Target, Settings: json.RawMessage(`{"access_mode":"full_access"}`)}}
		return runnercontract.ExecutionConfigurationResult{Identity: &identity}, nil
	}}
	for i := 0; i < 3; i++ {
		if _, err := snapshot.Resolve(context.Background(), request); err == nil {
			t.Fatal("failure lost")
		}
	}
	if calls != 1 {
		t.Fatal("ordinary polling repeated a failed query", calls)
	}
	broken = false
	snapshot.RefreshFailures()
	result, err := snapshot.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	original := result.Identity.Context.Settings
	result.Identity.Context.Settings = json.RawMessage(`{"forged":true}`)
	identity, err := snapshot.Identity(execution)
	if err != nil || string(identity.Context.Settings) != string(original) {
		t.Fatal("caller mutated trusted facts", err)
	}
	snapshot.RefreshFailures()
	_, err = snapshot.Resolve(context.Background(), request)
	if err != nil || calls != 2 {
		t.Fatal("explicit refresh discarded successful facts", calls, err)
	}
	changed := execution
	changed.Settings = json.RawMessage(`{"access_mode":"full_access","model":"different"}`)
	if _, err = snapshot.Identity(changed); err == nil {
		t.Fatal("unobserved source inherited trusted identity")
	}
}
