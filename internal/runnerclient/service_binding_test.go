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
