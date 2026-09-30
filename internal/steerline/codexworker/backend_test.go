package codexworker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type capturedProcess struct {
	identity executionapi.ProcessIdentity
	closed   bool
}

func (p *capturedProcess) Identity() executionapi.ProcessIdentity { return p.identity }
func (p *capturedProcess) Close()                                 { p.closed = true }
func (p *capturedProcess) Kill()                                  {}
func TestPrepareSessionRetainsPartialProcessReceiptOnWriteFailure(t *testing.T) {
	root := t.TempDir()
	sentinel := errors.New("receipt persistence failed")
	process := &capturedProcess{identity: executionapi.ProcessIdentity{PID: 123, Created: "456", Executable: os.Args[0]}}
	backend := Backend{Config: Config{Policy: policyFixture(), Executable: os.Args[0], Workspace: root, RunDir: root, RunID: strings.Repeat("a", 32), ServerDir: filepath.Join(root, "server"), Sandbox: "workspace-write", WindowsSandbox: "elevated", Environment: func(env []string) []string { return env }}, Process: ProcessPort{Start: func(*exec.Cmd) (executionapi.StopHandle, error) { return process, nil }, NewGeneration: func() string { return "new-generation" }, WriteAtomic: func(path string, raw []byte) error {
		if filepath.Base(path) == "process.json" {
			return sentinel
		}
		return os.WriteFile(path, raw, 0600)
	}}}
	handle, binding, err := backend.PrepareSession(context.Background(), Binding{}, executionapi.TurnInput{CommandID: strings.Repeat("b", 32), Prompt: "fixed input"})
	if !errors.Is(err, sentinel) || handle != nil || binding.Process == nil || *binding.Process != process.identity || binding.Endpoint == "" || binding.Generation != "new-generation" || binding.SessionID != "" || !process.closed {
		t.Fatalf("partial binding/cleanup lost: %+v %v closed=%v", binding, err, process.closed)
	}
}
func TestPrepareSessionRejectsInvalidSettingsBeforeAnySideEffect(t *testing.T) {
	root := t.TempDir()
	called := false
	backend := Backend{Config: Config{Policy: policyFixture(), Executable: os.Args[0], Workspace: root, RunDir: root, RunID: strings.Repeat("a", 32), ServerDir: filepath.Join(root, "server"), Sandbox: "invalid"}, Process: ProcessPort{Start: func(*exec.Cmd) (executionapi.StopHandle, error) { called = true; return nil, nil }, WriteAtomic: func(string, []byte) error { called = true; return nil }}}
	_, _, err := backend.PrepareSession(context.Background(), Binding{}, executionapi.TurnInput{CommandID: strings.Repeat("b", 32), Prompt: "fixed input"})
	if err == nil || called {
		t.Fatal("invalid settings reached side effects")
	}
	if _, err = os.Stat(filepath.Join(root, "runs")); !os.IsNotExist(err) {
		t.Fatal("invalid settings created turn resources")
	}
}
