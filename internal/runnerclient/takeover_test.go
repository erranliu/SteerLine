package runnerclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func takeoverCLI(t *testing.T) Client {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", "check-response")
	t.Setenv("STEERLINE_RUNNER_CLI_CALL", filepath.Join(t.TempDir(), "calls"))
	return Client{Executable: exe, SHA256: hex.EncodeToString(sum[:]), StateDir: t.TempDir(), Timeout: 5 * time.Second}
}
func fixtureResponse(t *testing.T, data any) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"protocol": Protocol, "ok": true, "request_id": nil, "data": data, "error": nil})
	t.Setenv("STEERLINE_CHECK_RESPONSE", string(raw))
}

func TestTakeoverPublicContractRejectsChangedIdentityAndUnsupportedState(t *testing.T) {
	c := takeoverCLI(t)
	proof := TakeoverReport{Contract: "steerrunner-takeover/v1", StateDir: c.StateDir, CandidateSHA256: c.SHA256, Compatible: true, Items: []TakeoverItem{{ID: "run", Kind: "run", Revision: 7, InputDigest: "original", Status: "compatible"}}}
	fixtureResponse(t, proof)
	if _, err := c.VerifyTakeover(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"hash", "directory", "status", "duplicate", "contract"} {
		bad := proof
		bad.Items = append([]TakeoverItem(nil), proof.Items...)
		switch change {
		case "hash":
			bad.CandidateSHA256 = strings.Repeat("a", 64)
		case "directory":
			bad.StateDir += "other"
		case "status":
			bad.Items[0].Status = "unsupported"
		case "duplicate":
			bad.Items = append(bad.Items, bad.Items[0])
		case "contract":
			bad.Contract = "unknown"
		}
		fixtureResponse(t, bad)
		if _, err := c.VerifyTakeover(context.Background()); err == nil {
			t.Fatalf("accepted %s", change)
		}
	}
}

func TestStartupStopsKeepBareOriginalEnvelopeAndKnownRejection(t *testing.T) {
	c := takeoverCLI(t)
	stops := []PendingStop{{RunID: "original-run", Request: Request{Protocol: Protocol, RequestID: "original-stop", Input: json.RawMessage(`{"expected_revision":7}`)}}}
	info := ServiceInfo{Protocol: Protocol, Status: "ready", StateDir: c.StateDir, InstanceID: "new-instance", Version: "candidate", ResourceDigest: "resources", ExecutableSHA256: c.SHA256}
	// Call the one startup invocation directly to inspect the public wire shape;
	// StartServiceWithStops additionally handles an already-ready current owner.
	fixtureResponse(t, info)
	if _, err := c.call(context.Background(), "service start", nil, nil, &ServiceStartInput{PendingStops: stops}); err != nil {
		t.Fatal(err)
	}
	wire, err := os.ReadFile(os.Getenv("STEERLINE_RUNNER_CLI_CALL"))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(wire), "--stdin--\n", 2)
	if len(parts) != 2 {
		t.Fatal(string(wire))
	}
	var actual ServiceStartInput
	if json.Unmarshal([]byte(parts[1]), &actual) != nil || len(actual.PendingStops) != 1 || actual.PendingStops[0].Request.RequestID != "original-stop" || string(actual.PendingStops[0].Request.Input) != string(stops[0].Request.Input) {
		t.Fatal("changed original startup stop", parts[1])
	}
	t.Setenv("STEERLINE_CHECK_RESPONSE", `{"protocol":"steerrunner-cli/v1","ok":false,"request_id":"original-stop","data":null,"error":{"code":"stale_revision","message":"original rejection"}}`)
	t.Setenv("STEERLINE_CHECK_ERROR", "1")
	_, err = c.call(context.Background(), "service start", nil, nil, &ServiceStartInput{PendingStops: stops})
	failure, ok := err.(*Failure)
	if !ok || failure.Unknown || failure.Code != "stale_revision" {
		t.Fatal("known original rejection became unknown", err)
	}
}
