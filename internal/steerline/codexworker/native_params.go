package codexworker

import (
	"errors"
	"fmt"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func AppServerLaunch(executable string, args []string, profile, endpoint string) (executionapi.LaunchCommand, error) {
	if executable == "" || endpoint == "" {
		return executionapi.LaunchCommand{}, errors.New("Codex app-server command is incomplete")
	}
	native, err := NativeExecutable(executable)
	if err != nil {
		return executionapi.LaunchCommand{}, err
	}
	launchArgs := append([]string(nil), args...)
	if profile != "" {
		launchArgs = append(launchArgs, "--profile", profile)
	}
	launchArgs = append(launchArgs, "app-server", "--listen", endpoint)
	return executionapi.LaunchCommand{Executable: native, Args: launchArgs}, nil
}

func ThreadStartParams(worktree, sandbox, model string, dynamicTools []any) (map[string]any, error) {
	if worktree == "" {
		return nil, errors.New("Codex app-server thread has no fixed worktree")
	}
	params := map[string]any{"cwd": worktree, "sandbox": sandbox, "approvalPolicy": "never"}
	if model != "" {
		params["model"] = model
	}
	if len(dynamicTools) > 0 {
		params["dynamicTools"] = dynamicTools
	}
	return params, nil
}

// WritableRoots decodes the fixed Codex --add-dir arguments for native policy.
func WritableRoots(args []string) ([]string, error) {
	roots := make([]string, 0, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) || args[i] != "--add-dir" {
			return nil, fmt.Errorf("unexpected Codex Git sandbox argument at %d", i)
		}
		roots = append(roots, args[i+1])
	}
	return roots, nil
}

func TurnStartParams(worktree, threadID, commandID, prompt, sandbox, model, effort string, writableRoots []string) (map[string]any, error) {
	if worktree == "" || threadID == "" || commandID == "" || prompt == "" {
		return nil, errors.New("Codex app-server turn lacks a fixed thread, command or input")
	}
	policy := map[string]any{}
	switch sandbox {
	case "read-only":
		policy["type"] = "readOnly"
		policy["networkAccess"] = false
	case "danger-full-access":
		policy["type"] = "dangerFullAccess"
	case "workspace-write":
		policy["type"] = "workspaceWrite"
		policy["writableRoots"] = append([]string(nil), writableRoots...)
		policy["networkAccess"] = true
		policy["excludeTmpdirEnvVar"] = true
		policy["excludeSlashTmp"] = true
	default:
		return nil, fmt.Errorf("unsupported Codex sandbox %q", sandbox)
	}
	params := map[string]any{"threadId": threadID, "input": []map[string]string{{"type": "text", "text": prompt}}, "clientUserMessageId": commandID, "cwd": worktree, "sandboxPolicy": policy, "approvalPolicy": "never"}
	if model != "" {
		params["model"] = model
	}
	if effort != "" {
		params["effort"] = effort
	}
	return params, nil
}
