package orca

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOrcaInputLocalRuntime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ORCA_USER_DATA_PATH", dir)
	if _, err := LocalRuntimeID(); err == nil {
		t.Fatal("missing local bootstrap accepted")
	}
	path := filepath.Join(dir, "orca-runtime.json")
	if err := os.WriteFile(path, []byte(`{"runtimeId":"original-local","authToken":"private"}`), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := LocalRuntimeID()
	if err != nil || id != "original-local" {
		t.Fatal(id, err)
	}
	if err := os.WriteFile(path, []byte(`{"authToken":"private"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LocalRuntimeID(); err == nil {
		t.Fatal("identity-free bootstrap accepted")
	}
}
