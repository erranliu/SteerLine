package orca_remote_worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

// VerifyViewer reads native facts only. It never launches/resumes an agent.
func (c Client) VerifyViewer(ctx context.Context, ref b.ExecutionRef) error {
	r := FromExecutionRef(ref)
	if err := validateSupervisedRef(c.config, r); err != nil {
		return err
	}
	if !c.config.local() {
		if err := verifySavedEnvironment(ctx, c, c.config.Settings.EnvironmentID, c.config.Settings.ConnectionID, ref.External.RuntimeID); err != nil {
			return err
		}
	}
	show, err := c.Show(ctx, StartReceipt{RunID: ref.External.OrchestrationRun, TaskID: ref.External.Task, DispatchID: ref.External.Dispatch, WorktreeID: ref.External.WorktreeID, CenterRuntimeID: ref.External.CoordinatorRuntimeID})
	if err != nil {
		return err
	}
	if show.Identity != ref.External || show.Liveness != "live" || show.WorktreePath != ref.WorkspacePath || show.Branch != strings.TrimPrefix(ref.WorkspaceBranch, "refs/heads/") {
		return errors.New("原 Orca worker 身份已变化或不可达；未打开会话")
	}
	_, err = c.viewerTerminal(ctx, ref)
	return err
}

func (c Client) viewerTerminal(ctx context.Context, ref b.ExecutionRef) (orcaworker.Terminal, error) {
	args := []string{"terminal", "show", "--terminal", ref.External.Terminal, "--json"}
	if !c.config.local() {
		args = append(args, "--environment", c.config.Settings.EnvironmentID)
	}
	env, err := c.CenterCall(ctx, args...)
	if err != nil {
		return orcaworker.Terminal{}, err
	}
	var result struct {
		Terminal orcaworker.Terminal `json:"terminal"`
	}
	if err := json.Unmarshal(env.Payload, &result); err != nil {
		return result.Terminal, err
	}
	t := result.Terminal
	// Orphaned is renderer ownership, not proof that the original PTY exited.
	if env.RuntimeID != ref.External.RuntimeID || t.Handle != ref.External.Terminal || t.IncarnationID != ref.External.Incarnation || t.WorktreeID != ref.External.WorktreeID || t.WorktreePath != ref.WorkspacePath || t.AgentIdentity != ref.Agent.Name || !t.Connected || t.Exited() {
		return t, errors.New("原 Orca agent 终端已退出、被替换或身份不匹配")
	}
	return t, nil
}

// ReadViewerScreen pins both sides of the read to the original PTY identity.
// Screen-unavailable is kept explicit; accumulated output is never called a screen.
func (c Client) ReadViewerScreen(ctx context.Context, ref b.ExecutionRef) (string, error) {
	if err := c.VerifyViewer(ctx, ref); err != nil {
		return "", err
	}
	args := []string{"terminal", "read", "--terminal", ref.External.Terminal, "--screen", "--limit", "1000", "--json"}
	if !c.config.local() {
		args = append(args, "--environment", c.config.Settings.EnvironmentID)
	}
	env, err := c.CenterCall(ctx, args...)
	if err != nil {
		return "", err
	}
	var result struct {
		Terminal struct {
			Handle, Source string
			Tail           []string
			Limited        bool
		} `json:"terminal"`
	}
	if err := json.Unmarshal(env.Payload, &result); err != nil {
		return "", err
	}
	if env.RuntimeID != ref.External.RuntimeID || result.Terminal.Handle != ref.External.Terminal || result.Terminal.Source == "" {
		return "", errors.New("Orca 未提供原终端的屏幕读取能力或返回了其他会话")
	}
	if _, err := c.viewerTerminal(ctx, ref); err != nil {
		return "", err
	}
	header := "远端原终端实时查看（只读） · 任务操作请通过 SteerLine\n"
	if result.Terminal.Source == "screen-unavailable" {
		header += "屏幕暂不可用；以下为保留输出，可能包含重绘片段。\n"
	} else if result.Terminal.Source != "screen" {
		return "", errors.New("Orca 返回未知的屏幕来源：" + result.Terminal.Source)
	}
	if result.Terminal.Limited {
		header += "输出已截断。\n"
	}
	return header + strings.Join(result.Terminal.Tail, "\n"), nil
}

// FocusViewerTerminal is local-only: a remote host navigation is not a local view.
func (c Client) FocusViewerTerminal(ctx context.Context, runtime, handle, worktree string) error {
	env, err := c.CenterCall(ctx, "terminal", "switch", "--terminal", handle, "--json")
	if err != nil {
		return err
	}
	var result struct {
		Focus struct {
			Handle, WorktreeID string
			Navigated          bool
		} `json:"focus"`
	}
	if err := json.Unmarshal(env.Payload, &result); err != nil {
		return err
	}
	if env.RuntimeID != runtime || result.Focus.Handle != handle || result.Focus.WorktreeID != worktree || !result.Focus.Navigated {
		return errors.New("本机 Orca 未确认定位到原终端；未启动替代会话")
	}
	return nil
}
