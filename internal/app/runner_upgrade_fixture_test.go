package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"os"
	"path/filepath"
	"testing"
)

func bindTestRunnerPin(home string, pin runnercontract.BuildPin) error {
	return bindApplicationRunnerBuild(context.Background(), home, runnerclient.Client{Executable: pin.Executable, SHA256: pin.SHA256, BuildVersion: pin.Version, ResourceDigest: pin.ResourceDigest, StateDir: pin.StateDir})
}
func startupFixtureCLI(t *testing.T) runnerclient.Client {
	t.Helper()
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	exe, err := os.Executable()
	require(t, err)
	raw, err := os.ReadFile(exe)
	require(t, err)
	sum := sha256.Sum256(raw)
	dir := t.TempDir()
	cli := runnerclient.Client{Executable: exe, StateDir: dir, SHA256: hex.EncodeToString(sum[:]), BuildVersion: "unit-fixture", ResourceDigest: hex.EncodeToString(make([]byte, 32))}
	require(t, os.WriteFile(filepath.Join(dir, "idle-fixture.json"), mustJSON(runnerclient.ServiceInfo{Protocol: runnerclient.Protocol, StateDir: dir, Status: "not_running"}), 0600))
	require(t, os.WriteFile(filepath.Join(dir, "idle-run-fixture.json"), []byte(`{}`), 0600))
	return cli
}
func writeStartupInspection(t *testing.T, cli runnerclient.Client, run runnercontract.Run, requestID string) {
	t.Helper()
	id := run.RunID
	view := runnerclient.Inspection{Run: &run, Command: &runnercontract.Command{Command: "submit", RequestID: requestID, RunID: &id}}
	require(t, os.WriteFile(filepath.Join(cli.StateDir, "idle-run-fixture.json"), mustJSON(view), 0600))
}
