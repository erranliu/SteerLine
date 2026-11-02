//go:build runneracceptance

package runnerclient

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunnerFullBindingPublicCLI(t *testing.T) {
	exe := os.Getenv("STEERRUNNER_EXE")
	if exe == "" {
		t.Fatal("explicit built Runner required for public CLI integration")
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
