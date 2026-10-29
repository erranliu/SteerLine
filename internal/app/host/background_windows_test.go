//go:build windows

package host

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	process "github.com/erranliu/SteerLine/internal/execution/process"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/projectconnection"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"golang.org/x/sys/windows"
)

// Record the actual child console before the CLI fixture or Go test main runs.
func init() {
	if marker := os.Getenv("STEERLINE_BACKGROUND_CONSOLE_PROBE"); marker != "" {
		hwnd, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
		if err := os.WriteFile(marker, []byte(fmt.Sprint(hwnd)), 0600); err != nil {
			os.Exit(2)
		}
		if len(os.Args) > 1 && os.Args[1] == "version" {
			fmt.Print(`{"protocol":"steerrunner-cli/v1","ok":true,"request_id":null,"data":{},"error":null}`)
			os.Exit(0)
		}
		dir, err := os.Getwd()
		if err != nil {
			os.Exit(2)
		}
		fmt.Print(dir)
		os.Exit(0)
	}
}

// Exercise the real callers: a hidden console still fails this test. Checking
// only HideWindow/flags would miss the original unconfigured resolver path.
func TestBackgroundCommandsNoConsole(t *testing.T) {
	if os.Getenv("STEERLINE_BACKGROUND_DETACHED_CALLER") != "1" {
		// Production commands originate in a DETACHED_PROCESS host. A test
		// runner's CREATE_NO_WINDOW context can otherwise hide this regression.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBackgroundCommandsNoConsole$", "-test.v")
		command.WaitDelay = time.Second
		command.Env = append(os.Environ(), "STEERLINE_BACKGROUND_DETACHED_CALLER=1")
		command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS, HideWindow: true}
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("detached caller: %v\n%s", err, out)
		}
		t.Logf("%s", out)
		return
	}
	for _, caller := range []string{"repository-identity", "managed-command", "runner-cli", "desktop-launcher"} {
		t.Run(caller, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "console.txt")
			t.Setenv("STEERLINE_BACKGROUND_CONSOLE_PROBE", marker)
			command := backend.LaunchCommand{Executable: os.Args[0], Args: []string{"--background-console-probe"}, WorkingDirectory: dir, Timeout: 5 * time.Second}
			ctx := context.Background()
			var err error
			switch caller {
			case "repository-identity":
				raw, marshalErr := json.Marshal(map[string]any{
					"repository": dir, "task_authority": "native", "workflow_protocol": "worker-flow-v2",
					"git": map[string]any{"executable": command.Executable, "args": command.Args},
				})
				require(t, marshalErr)
				ref := api.ProjectRef{ConfigPath: filepath.Join(dir, "project.json"), ApplicationHome: dir}
				err = projectconnection.ResolveSnapshot(ctx, &ref, raw)
			case "managed-command":
				_, err = process.New(5*time.Second).OutputCommand(ctx, command)
			case "runner-cli":
				_, err = (runnerclient.Client{Executable: os.Args[0]}).Call(ctx, "version", nil, nil)
			case "desktop-launcher":
				_, err = OutputDesktopOpen(ctx, command, os.Environ())
			}
			require(t, err)
			raw, err := os.ReadFile(marker)
			require(t, err)
			if string(raw) != "0" {
				t.Fatalf("background child allocated or inherited a console: HWND=%s", raw)
			}
		})
	}
}
