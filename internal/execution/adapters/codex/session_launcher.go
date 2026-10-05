package codexworker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const codexHistoryBindingFile = "codex-history.v1.json"

// The launcher runs inside Orca's terminal environment, which may use a managed
// CODEX_HOME. Only a non-model bootstrap user item is injected to persist the new
// empty session. Every assistant phase below still comes from native history.
type SessionLaunch struct {
	Binding     HistoryBinding `json:"binding"`
	BindingPath string         `json:"binding_path"`
	Args        []string       `json:"args"`
	Prompt      string         `json:"prompt"`
	Sandbox     string         `json:"sandbox"`
}

func RunSessionLauncher(ctx context.Context, requestPath string, start func(*exec.Cmd) (executionbackend.StopHandle, error), writeAtomic func(string, []byte) error) error {
	raw, err := os.ReadFile(requestPath)
	if err != nil {
		return err
	}
	var request SessionLaunch
	if err = json.Unmarshal(raw, &request); err != nil {
		return err
	}
	b := request.Binding
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if b.Version != 1 || b.Backend != executionbackend.BackendOrca || b.Protocol != executionbackend.ProtocolOrcaTerminalV2 || b.SessionID != "" || b.Command.Executable == "" || canonicalPath(cwd) != canonicalPath(b.Workspace) || !filepath.IsAbs(requestPath) || canonicalPath(filepath.Dir(request.BindingPath)) != canonicalPath(filepath.Dir(requestPath)) || filepath.Base(request.BindingPath) != codexHistoryBindingFile {
		return errors.New("invalid Codex session launch binding")
	}
	if _, err = os.Stat(request.BindingPath); !os.IsNotExist(err) {
		return errors.New("Codex session binding already exists; refusing another launch")
	}
	b.Home = EnvironmentHome()
	setupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client, err := OpenHistory(setupCtx, b.Command, cwd, b.Home, start)
	if err != nil {
		return err
	}
	b.SessionID, err = client.BootstrapSession(setupCtx, cwd, request.Sandbox, "SteerLine prepared this session for execution "+b.ExecutionID+", attempt "+b.AttemptID+". Await the task instruction.")
	client.Close()
	if err != nil {
		return err
	}
	if err = b.Validate(); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err = writeAtomic(request.BindingPath, append(encoded, '\n')); err != nil {
		return err
	}
	command := ResumeCommand(b.Command, b.SessionID, request.Args, request.Prompt, cwd)
	cmd := exec.Command(command.Executable, command.Args...)
	cmd.Dir = cwd
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
