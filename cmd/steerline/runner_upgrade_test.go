package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestRunnerUpgradePreflight(t *testing.T) {
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "preflight-fixture")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])
	home := t.TempDir()
	if err := runRunnerUpgrade([]string{"--runner-exe", exe, "--runner-sha256", sha, "--home", home, "--json"}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--runner-exe", exe, "--home", home}, {"--runner-exe", exe, "--runner-sha256", "bad", "--home", home}, {"--runner-exe", exe, "--runner-sha256", sha, "--home", filepath.Join("relative", "home")}} {
		if runRunnerUpgrade(args) == nil {
			t.Fatal("invalid preflight accepted", args)
		}
	}
}
