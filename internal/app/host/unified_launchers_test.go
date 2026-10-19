package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestUnifiedStartupLaunchersRetireOnlyOwnedTemplates(t *testing.T) {
	home := filepath.Join(t.TempDir(), "中文 home")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	old, err := launcherFS.ReadFile("retired-launchers/start-app.bat")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(home, "start-app.bat"), toCRLF(old), 0600); err != nil {
		t.Fatal(err)
	}
	custom := []byte("@echo off\r\nset STEERLINE_HOME=%~dp0\r\ncustom user commands\r\n")
	for _, name := range []string{"start-web.bat", "my-tool.bat"} {
		if err = os.WriteFile(filepath.Join(home, name), custom, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = InstallLaunchers(home); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(home, "start-app.bat")); !os.IsNotExist(err) {
		t.Fatal("owned old launcher retained", err)
	}
	for _, name := range []string{"start-web.bat", "my-tool.bat"} {
		data, err := os.ReadFile(filepath.Join(home, name))
		if err != nil || !bytes.Equal(data, custom) {
			t.Fatal("custom launcher modified", name, err)
		}
	}
	for _, name := range []string{"start.bat", "restart.bat", "stop.bat"} {
		data, err := os.ReadFile(filepath.Join(home, name))
		if err != nil || !bytes.Contains(data, []byte("set \"STEERLINE_HOME=%~dp0\"")) || !bytes.Contains(data, []byte("exit /b %RESULT%")) {
			t.Fatal("launcher home/exit code binding lost", name, err)
		}
	}
	if _, err = os.Stat(filepath.Join(home, "console.bat")); !os.IsNotExist(err) {
		t.Fatal("TUI launcher generated")
	}
}
