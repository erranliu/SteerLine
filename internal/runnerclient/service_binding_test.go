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

func TestRunnerFullBindingAdmission(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "invocation")
	t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", "accepted")
	t.Setenv("STEERLINE_RUNNER_CLI_CALL", path)
	digest := sha256.Sum256(image)
	c := Client{Executable: exe, InstanceID: "original", SHA256: hex.EncodeToString(digest[:]), BuildVersion: "fixed", ResourceDigest: strings.Repeat("b", 64), StateDir: t.TempDir(), Timeout: 10 * time.Second}
	catalog := &Catalog{}
	catalog.Capabilities.InstanceBinding.FullBinding = &ServiceBindingCapability{Contract: ServiceBindingContract, BeforeEffects: true, NoImplicitStart: true, Commands: []string{"submit"}}
	c = c.WithCatalog(catalog)
	request := Request{Protocol: Protocol, RequestID: "fixed-original", Input: json.RawMessage(`{"original":"body"}`)}
	if _, err := c.Submit(context.Background(), request); err != nil {
		t.Fatal("covered call performed a prequery or lost binding", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	var actual ExpectedServiceBinding
	for i, line := range lines {
		if line == "--expected-binding" && i+1 < len(lines) {
			err = json.Unmarshal([]byte(lines[i+1]), &actual)
		}
	}
	if err != nil || actual.Contract != ServiceBindingContract || actual.InstanceID != c.InstanceID || actual.StateDir != c.StateDir || actual.ExecutableSHA256 != c.SHA256 || actual.Version != c.BuildVersion || actual.ResourceDigest != c.ResourceDigest || !strings.Contains(string(raw), `"request_id":"fixed-original"`) {
		t.Fatal("full binding/original request lost", string(raw), err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	broken := c
	broken.ResourceDigest = ""
	if _, err = broken.Submit(context.Background(), request); err == nil {
		t.Fatal("partial identity downgraded")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("incomplete binding reached executable")
	}
	// Uncovered commands keep the existing check. This fixture's service-status
	// response is deliberately not a service identity, so the prequery must fail.
	c.FullBindingCommands = nil
	if _, err := c.Submit(context.Background(), request); err == nil {
		t.Fatal("missing capability bypassed prequery")
	}
	raw, err = os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(raw), "service\nstatus\n") {
		t.Fatal("uncovered command skipped status", string(raw), err)
	}
}

func TestRunnerFullBindingPublicCLI(t *testing.T) {
	exe := os.Getenv("STEERRUNNER_EXE")
	if exe == "" {
		t.Skip("explicit built Runner required for public CLI integration")
	}
	selection := Selection{Executable: exe, Version: os.Getenv("STEERRUNNER_VERSION"), SHA256: os.Getenv("STEERRUNNER_SHA256")}
	c, version, err := selection.Open(context.Background(), t.TempDir(), 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	info, err := c.StartService(context.Background(), version)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.StopService(context.Background(), info.InstanceID, "binding-contract-stop"); err != nil {
			t.Error(err)
		}
		if err := c.VerifyServiceSettlement(context.Background(), info.InstanceID, "binding-contract-stop"); err != nil {
			t.Error(err)
		}
	})
	c.InstanceID = info.InstanceID
	catalog, err := c.Catalog(context.Background(), version)
	if err != nil {
		t.Fatal(err)
	}
	c = c.WithCatalog(catalog)
	if _, covered, err := c.fullBinding("capabilities"); !covered || err != nil {
		t.Fatal("built Runner did not publish full binding", err)
	}
	if _, err := c.Call(context.Background(), "capabilities", nil, nil); err != nil {
		t.Fatal("correct binding rejected", err)
	}
	if !c.SupportsObservation() {
		t.Fatal("built Runner did not publish same-snapshot observation")
	}
	if _, err := c.Observe(context.Background(), "absent-run", ""); err == nil {
		t.Fatal("absent Run acquired an observation")
	} else if failure, ok := err.(*Failure); !ok || failure.Code != "run_not_found" || failure.Unknown {
		t.Fatal("public observe did not reach the bound read-only endpoint", err)
	}
	for _, field := range []string{"instance", "version", "resource", "exe"} {
		bad := c
		switch field {
		case "instance":
			bad.InstanceID = "replacement"
		case "version":
			bad.BuildVersion = "foreign"
		case "resource":
			bad.ResourceDigest = strings.Repeat("c", 64)
		case "exe":
			// Use the real CLI with a deliberately wrong resident expectation,
			// bypassing only the caller's local EXE guard in this protocol test.
			binding := ExpectedServiceBinding{ServiceBindingContract, Protocol, c.InstanceID, c.StateDir, strings.Repeat("d", 64), c.BuildVersion, c.ResourceDigest}
			body, _ := json.Marshal(binding)
			unbound := c
			unbound.InstanceID = ""
			_, err = unbound.Call(context.Background(), "capabilities", []string{"--expected-binding", string(body)}, nil)
		}
		if field != "exe" {
			_, err = bad.Call(context.Background(), "capabilities", nil, nil)
		}
		failure, ok := err.(*Failure)
		if !ok || failure.Unknown || failure.Code != "binding_mismatch" && failure.Code != "stale_instance" {
			t.Fatal("binding mismatch was not a definite rejection", field, err)
		}
	}
	t.Logf("idle public CLI verified: version=%s sha256=%s resource=%s", c.BuildVersion, c.SHA256, c.ResourceDigest)
}
