package native

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSandboxReadinessRejectsMissingAndStaleState(t *testing.T) {
	home := t.TempDir()
	if CheckSandboxReady(home) == nil {
		t.Fatal("missing setup accepted")
	}
	for _, dir := range []string{".sandbox", ".sandbox-secrets"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, raw := range map[string]string{
		".sandbox/setup_marker.json":          `{"version":4,"offline_username":"CodexSandboxOffline","online_username":"CodexSandboxOnline"}`,
		".sandbox-secrets/sandbox_users.json": `{"version":4,"offline":{"username":"CodexSandboxOffline","password":"invalid"},"online":{"username":"CodexSandboxOnline","password":"invalid"}}`,
	} {
		if err := os.WriteFile(filepath.Join(home, path), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if CheckSandboxReady(home) == nil {
		t.Fatal("stale setup accepted")
	}
}

func TestSandboxReadinessExistingAccounts(t *testing.T) {
	home := os.Getenv("STEERLINE_E2E_SANDBOX_PREFLIGHT_HOME")
	if home == "" {
		t.Skip("set STEERLINE_E2E_SANDBOX_PREFLIGHT_HOME for the no-token native credential check")
	}
	if err := CheckSandboxReady(home); err != nil {
		t.Fatal(err)
	}
}
