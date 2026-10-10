package codexworker

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type LaunchConfig struct {
	Executable     string
	Args           []string
	Profile        string
	Workspace      string
	ServerDir      string
	Sandbox        string
	WindowsSandbox string
	Runtime        *workerRuntime
}

// LaunchServer starts the fixed native app-server. The caller supplies its
// platform process starter and persists the returned identity before a turn.
func LaunchServer[T any](config LaunchConfig, start func(*exec.Cmd) (T, error), environment func([]string) []string) (identity T, process *os.Process, endpoint string, err error) {
	if err = (Config{Sandbox: config.Sandbox, WindowsSandbox: config.WindowsSandbox, Args: config.Args, Profile: config.Profile}).Validate(runtime.GOOS); err != nil {
		return identity, nil, "", err
	}
	permissions, err := NativePermissions(runtime.GOOS, "local", "codex")
	if err != nil {
		return identity, nil, "", err
	}
	args := append(append([]string(nil), config.Args...), permissions.Args()...)
	err = ValidateManagedArgs(config.Args)
	if err != nil {
		return identity, nil, "", err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return identity, nil, "", err
	}
	endpoint = "ws://" + listener.Addr().String()
	_ = listener.Close()
	launch, err := AppServerLaunch(config.Executable, args, config.Profile, endpoint)
	if err != nil {
		return identity, nil, "", err
	}
	output, err := os.OpenFile(filepath.Join(config.ServerDir, "appserver.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return identity, nil, "", err
	}
	launch.WorkingDirectory = config.Workspace
	cmd := exec.Command(launch.Executable, launch.Args...)
	cmd.Dir = launch.WorkingDirectory
	cmd.Env = environment(cmd.Environ())
	if config.Runtime != nil {
		cmd.Env, err = config.Runtime.environment(cmd.Env)
		if err != nil {
			_ = output.Close()
			return identity, nil, "", err
		}
	}
	cmd.Stdout, cmd.Stderr = output, output
	identity, err = start(cmd)
	_ = output.Close()
	if err != nil {
		return identity, nil, "", err
	}
	go func() { _ = cmd.Wait() }()
	return identity, cmd.Process, endpoint, nil
}
