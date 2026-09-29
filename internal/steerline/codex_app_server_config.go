package steerline

import (
	"context"
	"errors"
	"fmt"
)

// codexAppServerCommand applies the frozen profile before the subcommand. The
// app-server receives model and sandbox settings through native thread/turn
// parameters rather than exec-only flags.
func codexAppServerCommand(c Config, endpoint string) (Command, error) {
	if c.Codex.Executable == "" || endpoint == "" {
		return Command{}, errors.New("Codex app-server command is incomplete")
	}
	executable, err := codexAppServerExecutable(c.Codex)
	if err != nil {
		return Command{}, err
	}
	args := append([]string(nil), c.Codex.Args...)
	if c.Profile != "" {
		args = append(args, "--profile", c.Profile)
	}
	args = append(args, "app-server", "--listen", endpoint)
	return Command{Executable: executable, Args: args}, nil
}

func codexAppThreadStart(c Config, worktree string, dynamicTools []any) (map[string]any, error) {
	if worktree == "" {
		return nil, errors.New("Codex app-server thread has no fixed worktree")
	}
	params := map[string]any{"cwd": worktree, "sandbox": c.Sandbox, "approvalPolicy": "never"}
	if c.Model != "" {
		params["model"] = c.Model
	}
	if len(dynamicTools) > 0 {
		params["dynamicTools"] = dynamicTools
	}
	return params, nil
}

func codexAppTurnStart(ctx context.Context, c Config, worktree, threadID, commandID, prompt string) (map[string]any, error) {
	if !codexThreadID.MatchString(threadID) || !stableExecutionID(commandID) || worktree == "" || prompt == "" {
		return nil, errors.New("Codex app-server turn lacks a fixed thread, command or input")
	}
	policy := map[string]any{}
	switch c.Sandbox {
	case "read-only":
		policy["type"] = "readOnly"
	case "danger-full-access":
		policy["type"] = "dangerFullAccess"
	case "workspace-write":
		policy["type"] = "workspaceWrite"
		args, err := codexLinkedWorktreeWriteArgs(ctx, c, worktree)
		if err != nil {
			return nil, err
		}
		roots := make([]string, 0, len(args)/2)
		for i := 0; i < len(args); i += 2 {
			if i+1 >= len(args) || args[i] != "--add-dir" {
				return nil, fmt.Errorf("unexpected Codex Git sandbox argument at %d", i)
			}
			roots = append(roots, args[i+1])
		}
		policy["writableRoots"] = roots
	default:
		return nil, fmt.Errorf("unsupported Codex sandbox %q", c.Sandbox)
	}
	params := map[string]any{"threadId": threadID, "input": []map[string]string{{"type": "text", "text": prompt}}, "clientUserMessageId": commandID, "cwd": worktree, "sandboxPolicy": policy, "approvalPolicy": "never"}
	if c.Model != "" {
		params["model"] = c.Model
	}
	if c.ReasoningEffort != "" {
		params["effort"] = c.ReasoningEffort
	}
	return params, nil
}
