package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/localservice"
	fs "github.com/erranliu/SteerLine/internal/product/httpfs"
	"golang.org/x/sys/windows"
)

// Exercise the actual deployment writer against the recovery reader. A Go-only
// private fixture conceals differences in the PowerShell writer's Windows ACL.
func TestDeploymentRecordScriptMeetsRecoveryACL(t *testing.T) {
	helper, err := filepath.Abs("../../scripts/deployment-files.ps1")
	require(t, err)
	shells := []string{"powershell.exe"}
	if _, err := exec.LookPath("pwsh.exe"); err == nil {
		shells = append(shells, "pwsh.exe")
	}
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "deployment.json")
			input := filepath.Join(dir, "input.json")
			script := filepath.Join(dir, "write.ps1")
			require(t, os.WriteFile(script, []byte(`param($Helper, $Path, $InputPath)
$ErrorActionPreference = 'Stop'
. $Helper
Write-JSON $Path (Get-Content -LiteralPath $InputPath -Raw | ConvertFrom-Json) -Private
`), 0600))
			write := func(want installedDeployment, success bool) {
				t.Helper()
				data, err := json.Marshal(want)
				require(t, err)
				require(t, os.WriteFile(input, data, 0600))
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, shell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, helper, path, input)
				output, err := cmd.CombinedOutput()
				if (err == nil) != success {
					t.Fatalf("writer success=%v: %v\n%s", success, err, output)
				}
				pending, err := filepath.Glob(path + ".*.tmp")
				require(t, err)
				if len(pending) != 0 {
					t.Fatal("writer left temporary records", pending)
				}
			}
			read := func(want installedDeployment) {
				t.Helper()
				var got installedDeployment
				require(t, localservice.ReadPrivateJSON(path, &got, 64<<10))
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("recovery read different record: %+v", got)
				}
			}
			record := installedDeployment{Home: dir, RunnerVersion: "test", Files: map[string]string{"steerrunner.exe": "original"}}
			write(record, true)
			read(record)
			require(t, os.Remove(path))
			require(t, os.WriteFile(path, []byte(`{"home":"inherited ACL"}`), 0600))
			if fs.VerifyPrivateFile(path) == nil {
				t.Fatal("ordinary inherited record unexpectedly meets private recovery ACL")
			}
			write(record, true)
			read(record)
			record.Files["steerrunner.exe"] = "replacement"
			write(record, true)
			read(record)

			// A denied atomic rename must preserve the old record and clean the
			// private temporary file rather than remove/truncate the destination.
			before, err := os.ReadFile(path)
			require(t, err)
			name, err := windows.UTF16PtrFromString(path)
			require(t, err)
			handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
			require(t, err)
			defer windows.CloseHandle(handle)
			record.Files["steerrunner.exe"] = "must not replace"
			write(record, false)
			after, err := os.ReadFile(path)
			require(t, err)
			if !bytes.Equal(before, after) {
				t.Fatal("failed replacement changed original deployment record")
			}
			require(t, fs.VerifyPrivateFile(path))
		})
	}
}
