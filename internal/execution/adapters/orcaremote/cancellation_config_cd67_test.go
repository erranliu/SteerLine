package orca_remote_worker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoadConfigRequiresExplicitRemoteCancellationFalse(t *testing.T) {
	value := ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "2m", ConnectionID: "peer", EnvironmentID: "env", RepositorySelector: "repo", CoordinatorTerminal: "terminal", PollInterval: "1s", CapabilityReportVersion: 1}
	raw, err := EncodeProfileConfig(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "remote_cancel_available")
	missing, _ := json.Marshal(fields)
	if _, err := DecodeProfileConfig(missing); err == nil || !strings.Contains(err.Error(), "explicitly set to false") {
		t.Fatalf("missing cancellation policy accepted: %v", err)
	}
	fields["remote_cancel_available"] = json.RawMessage("true")
	enabled, _ := json.Marshal(fields)
	if _, err := DecodeProfileConfig(enabled); err == nil || !strings.Contains(err.Error(), "fixed unavailable") {
		t.Fatalf("enabled cancellation accepted: %v", err)
	}
}
