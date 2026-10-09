//go:build windows

package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
	"golang.org/x/sys/windows"
)

func TestNativeConsoleHasInteractiveHandlesAndLiteralArguments(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "native 中文 & workspace")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(dir, "console.json")
	literal := "quoted \"value\" & echo $(nothing)"
	command := b.LaunchCommand{Executable: os.Args[0], WorkingDirectory: dir, Args: []string{"-test.run=^TestNativeConsoleProcessHelper$", "--", evidence, literal}}
	if err := OpenNativeConsole(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(evidence); err == nil {
			var result struct {
				Cwd, Argument string
				Input, Output bool
			}
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			if result.Cwd != dir || result.Argument != literal || !result.Input || !result.Output {
				t.Fatalf("new window lacks terminal handles or changed command: %+v", result)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("native console helper produced no evidence")
}

func TestNativeConsoleProcessHelper(t *testing.T) {
	if len(os.Args) < 5 || os.Args[len(os.Args)-3] != "--" {
		return
	}
	var mode uint32
	input, _ := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	output, _ := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	cwd, _ := os.Getwd()
	result := struct {
		Cwd, Argument string
		Input, Output bool
	}{Cwd: cwd, Argument: os.Args[len(os.Args)-1], Input: windows.GetConsoleMode(input, &mode) == nil, Output: windows.GetConsoleMode(output, &mode) == nil}
	data, err := json.Marshal(result)
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(os.Args[len(os.Args)-2], data, 0600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
