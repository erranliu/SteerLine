package server

import (
	"os"
	"path/filepath"
	"testing"

	privatefs "github.com/erranliu/SteerLine/internal/product/httpfs"
)

func TestWebSwitchHomeConfigPreservesPermissions(t *testing.T) {
	home := t.TempDir()
	checkPermissions := homePermissionsFixture(t, home)
	bat := filepath.Join(home, "service-SteerlineTest.bat")
	const content = "@echo off\r\nrem existing launcher\r\n"
	if err := os.WriteFile(bat, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	address, err := EnsureAddress(home)
	if err != nil || address != "127.0.0.1:18742" {
		t.Fatalf("default config: %q, %v", address, err)
	}
	checkPermissions()
	if data, err := os.ReadFile(bat); err != nil || string(data) != content {
		t.Fatalf("existing launcher became inaccessible or changed: %q, %v", data, err)
	}
	config := filepath.Join(home, "web.json")
	if err := privatefs.VerifyPrivateFile(config); err != nil {
		t.Fatalf("new config is not private: %v", err)
	}
	if loaded, err := LoadAddress(home); err != nil || loaded != address {
		t.Fatalf("default config was not persisted: %q, %v", loaded, err)
	}
	custom := []byte(`{"version":1,"address":"127.0.0.1:18743"}`)
	if err := os.WriteFile(config, custom, 0600); err != nil {
		t.Fatal(err)
	}
	if address, err = EnsureAddress(home); err != nil || address != "127.0.0.1:18743" {
		t.Fatalf("existing custom config: %q, %v", address, err)
	}
	if data, err := os.ReadFile(config); err != nil || string(data) != string(custom) {
		t.Fatalf("existing config was overwritten: %q, %v", data, err)
	}
	checkPermissions()
}
