package codexworker

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/execution/adapters/protocol/codexapp"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// NativeViewerCommand attaches the TUI to the existing worker server. Omitting
// a prompt keeps opening the viewer from starting another business turn.
func NativeViewerCommand(executable, workspace string, binding AppServerBinding) (executionbackend.LaunchCommand, error) {
	if !ValidNativeID(binding.SessionID) || binding.Generation == "" || !filepath.IsAbs(workspace) {
		return executionbackend.LaunchCommand{}, errors.New("原生会话尚未绑定完整身份或工作目录")
	}
	if err := codexapp.ValidateEndpoint(binding.Endpoint); err != nil {
		return executionbackend.LaunchCommand{}, err
	}
	native, err := NativeExecutable(executable)
	if err != nil {
		return executionbackend.LaunchCommand{}, err
	}
	return executionbackend.LaunchCommand{Executable: native, WorkingDirectory: workspace, Args: []string{"resume", "--remote", binding.Endpoint, "--cd", workspace, binding.SessionID}}, nil
}

// VerifyNativeViewer reads the live thread without resuming or sending input.
func VerifyNativeViewer(ctx context.Context, binding AppServerBinding, workspace string) error {
	client, err := ConnectAppServer(ctx, binding.Endpoint)
	if err != nil {
		return err
	}
	defer client.Close()
	var response struct {
		Thread struct {
			ID  string `json:"id"`
			Cwd string `json:"cwd"`
		} `json:"thread"`
	}
	if err := client.Call(ctx, "thread/read", map[string]any{"threadId": binding.SessionID, "includeTurns": false}, &response); err != nil {
		return err
	}
	if response.Thread.ID != binding.SessionID || !sameWorkspace(response.Thread.Cwd, workspace) {
		return errors.New("原生会话或工作目录与候选不匹配")
	}
	return nil
}
