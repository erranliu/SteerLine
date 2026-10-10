package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	apphost "github.com/erranliu/SteerLine/internal/app/host"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	orcaremote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	api "github.com/erranliu/SteerLine/internal/product/api"
	p "github.com/erranliu/SteerLine/internal/storage/ports"
)

type orcaSessionSpec struct {
	profile orcaremote.ProfileConfig
	ref     b.ExecutionRef
}

// Resolve only the selected candidate's immutable accepted profile and current
// original PTY. Never use today's default profile to choose a viewing host.
func resolveOrcaSession(records p.ExecutionSnapshot, project string, target api.NativeSessionTarget) (orcaSessionSpec, error) {
	var spec orcaSessionSpec
	if project == "" || records.ProjectIdentity != project {
		return spec, errors.New("候选项目身份不匹配")
	}
	for _, e := range records.Executions {
		if e.ExecutionID != target.ExecutionID || e.WorkflowRunID != target.RunID || e.ProjectIdentity != project {
			continue
		}
		if target.SessionID == "" || target.SessionGeneration == "" || e.CurrentAttemptID != target.AttemptID || e.SessionID != target.SessionID || e.SessionGeneration != target.SessionGeneration || e.CurrentTurnID == "" || e.Backend != string(b.BackendOrca) || e.Protocol != string(b.ProtocolOrcaSupervised) || e.Process == nil {
			return spec, errors.New("原 Orca 候选会话已变化或尚未开始；刷新后重试")
		}
		if e.SessionID != orcaremote.SessionID(*e.Process) || e.SessionGeneration != e.Process.Incarnation {
			return spec, errors.New("原 Orca 会话与终端身份不匹配")
		}
		for _, run := range records.WorkflowRuns {
			if run.ProjectIdentity != project || run.WorkflowRunID != target.RunID || run.ExecutionProfile == nil {
				continue
			}
			profile := run.ExecutionProfile
			if profile.Host != "orca_supervised" || profile.Agent != e.Agent {
				return spec, errors.New("原 Orca 执行方案身份不匹配")
			}
			found := false
			for _, envelope := range profile.ImplementationData {
				if envelope.ImplementationKey != orcaremote.ProfileCodecKey {
					continue
				}
				if found {
					return spec, errors.New("原 Orca 执行配置重复")
				}
				found = true
				if err := envelope.ValidateProfileIdentity(project, profile.ID, profile.AcceptedConfigVersion); err != nil {
					return spec, err
				}
				if envelope.CodecVersion != orcaremote.ProfileCodecVersion || envelope.Purpose != b.ImplementationConfigPurpose {
					return spec, errors.New("原 Orca 执行配置版本不匹配")
				}
				var err error
				spec.profile, err = orcaremote.DecodeProfileConfig(envelope.Payload)
				if err != nil {
					return spec, err
				}
			}
			if !found {
				return spec, errors.New("原 Orca 执行配置缺失")
			}
			spec.ref = b.ExecutionRef{Backend: b.BackendOrca, Protocol: b.ProtocolOrcaSupervised, RunID: e.WorkflowRunID, AttemptID: e.CurrentAttemptID, Target: e.Target, Agent: b.AgentSpec{Name: e.Agent}, External: *e.Process, WorkspacePath: e.Workspace.Worktree, WorkspaceBranch: e.Workspace.Branch, BaseCommit: e.Workspace.Base}
			return spec, nil
		}
		return spec, errors.New("原 Orca 候选执行方案缺失")
	}
	return spec, errors.New("原 Orca 候选不存在；刷新后重试")
}

func orcaViewerClient(config Config, spec orcaSessionSpec) orcaremote.Client {
	return orcaremote.New(orcaremote.Config{Repository: config.Repository, Agent: spec.ref.Agent.Name, ExpectedRemoteRuntime: spec.ref.External.RuntimeID,
		Settings: orcaremote.Settings{Target: spec.profile.Target, ConnectionID: spec.profile.ConnectionID, EnvironmentID: spec.profile.EnvironmentID, RepositorySelector: spec.profile.RepositorySelector},
		Call: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
			command := b.LaunchCommand{Executable: spec.profile.Executable, Args: append(append([]string(nil), spec.profile.Args...), args...), WorkingDirectory: config.Repository, Timeout: timeout,
				BlockedEnvironment: []string{"ORCA_ENVIRONMENT", "ORCA_PAIRING_CODE", "ORCA_REMOTE_PAIRING"}}
			if len(args) > 0 && args[0] == "open" {
				return apphost.OutputDesktopOpen(ctx, command, resourceprocess.FilteredEnvironment(command.BlockedEnvironment))
			}
			return config.ProcessHost.OutputCommand(ctx, command)
		}})
}

type orcaViewerRequest struct {
	Project    string                  `json:"project"`
	Home       string                  `json:"home"`
	ConfigPath string                  `json:"config_path"`
	Target     api.NativeSessionTarget `json:"target"`
}

type orcaViewerBinding struct {
	Runtime  string              `json:"runtime"`
	Terminal orcaworker.Terminal `json:"terminal"`
}

func openOrcaSession(ctx context.Context, config Config, records p.ExecutionSnapshot, target api.NativeSessionTarget) error {
	spec, err := resolveOrcaSession(records, config.Identity(), target)
	if err != nil {
		return err
	}
	client := orcaViewerClient(config, spec)
	// Open the local desktop only. Subsequent native replies must still match the
	// recorded runtime, so opening cannot silently rebind an execution after restart.
	if _, err := client.CenterCall(ctx, "open", "--json"); err != nil {
		return err
	}
	if err := client.VerifyViewer(ctx, spec.ref); err != nil {
		return err
	}
	if spec.profile.Target == "local" {
		if err := client.FocusViewerTerminal(ctx, spec.ref.External.RuntimeID, spec.ref.External.Terminal, spec.ref.External.WorktreeID); err != nil {
			return err
		}
		return client.VerifyViewer(ctx, spec.ref)
	}
	// Current Orca CLI host navigation cannot target the caller's paired desktop.
	// A local display terminal reads the exact remote PTY; it never resumes it.
	if _, err := client.ReadViewerScreen(ctx, spec.ref); err != nil {
		return err
	}
	request := orcaViewerRequest{Project: config.Identity(), Home: HomeDir(), ConfigPath: config.ConfigPath, Target: target}
	raw, err := json.Marshal(request)
	if err != nil {
		return err
	}
	key := sha256.Sum256(raw)
	requestPath := filepath.Join(config.RunDir, "session-viewers", hex.EncodeToString(key[:])+".json")
	if err := config.ProcessHost.WriteExecutionFile(requestPath, raw); err != nil {
		return err
	}
	return openOrcaRemoteDisplay(ctx, config, spec, client, requestPath)
}

func openOrcaRemoteDisplay(ctx context.Context, config Config, spec orcaSessionSpec, client orcaremote.Client, requestPath string) error {
	bindingPath := requestPath + ".terminal.json"
	var saved orcaViewerBinding
	if raw, err := os.ReadFile(bindingPath); err == nil {
		if err := json.Unmarshal(raw, &saved); err != nil {
			return err
		}
		env, err := client.CenterCall(ctx, "terminal", "show", "--terminal", saved.Terminal.Handle, "--json")
		if err != nil {
			// A closed tab legitimately loses its handle. Reopen only after the
			// complete original-host inventory proves that this display is absent.
			listing, listErr := client.CenterCall(ctx, "terminal", "list", "--worktree", "path:"+config.Repository, "--limit", "1000", "--json")
			var inventory struct {
				Terminals []orcaworker.Terminal `json:"terminals"`
				Truncated bool                  `json:"truncated"`
			}
			if listErr != nil || json.Unmarshal(listing.Payload, &inventory) != nil || inventory.Truncated || listing.RuntimeID != saved.Runtime || saved.Runtime != spec.ref.External.CoordinatorRuntimeID {
				return fmt.Errorf("原 Orca 查看窗口无法核实；未创建重复窗口：%w", err)
			}
			for _, term := range inventory.Terminals {
				if term.Handle == saved.Terminal.Handle {
					return fmt.Errorf("原 Orca 查看窗口仍存在但无法核实：%w", err)
				}
			}
		} else {
			var result struct {
				Terminal orcaworker.Terminal `json:"terminal"`
			}
			if err := json.Unmarshal(env.Payload, &result); err != nil {
				return err
			}
			t := result.Terminal
			if env.RuntimeID != saved.Runtime || saved.Runtime != spec.ref.External.CoordinatorRuntimeID || t.Handle != saved.Terminal.Handle || t.IncarnationID != saved.Terminal.IncarnationID || t.WorktreeID != saved.Terminal.WorktreeID || t.AgentIdentity != "" {
				return errors.New("原 Orca 查看窗口身份已变化；未打开其他终端")
			}
			if t.Connected && !t.Exited() {
				if _, err := os.Stat(requestPath + ".stopped"); errors.Is(err, os.ErrNotExist) {
					return client.FocusViewerTerminal(ctx, saved.Runtime, t.Handle, t.WorktreeID)
				} else if err != nil {
					return err
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	command := orcaDisplayCommand(executable, requestPath, runtime.GOOS)
	args := []string{"terminal", "create", "--worktree", "path:" + config.Repository, "--title", "SteerLine 远端原终端查看 · " + spec.ref.RunID, "--command", command, "--json"}
	if runtime.GOOS == "windows" {
		args = append(args, "--shell", "powershell.exe")
	}
	env, err := client.CenterCall(ctx, args...)
	if err != nil {
		return err
	}
	var result struct {
		Terminal orcaworker.Terminal `json:"terminal"`
	}
	if err := json.Unmarshal(env.Payload, &result); err != nil {
		return err
	}
	if env.RuntimeID != spec.ref.External.CoordinatorRuntimeID || result.Terminal.Handle == "" {
		return errors.New("本机 Orca 未确认创建远端查看窗口")
	}
	// Creation receipts need not include incarnation; read the new local display
	// identity before recording it for subsequent focus-only opens.
	shown, err := client.CenterCall(ctx, "terminal", "show", "--terminal", result.Terminal.Handle, "--json")
	if err != nil {
		return err
	}
	var identity struct {
		Terminal orcaworker.Terminal `json:"terminal"`
	}
	if err := json.Unmarshal(shown.Payload, &identity); err != nil {
		return err
	}
	if shown.RuntimeID != env.RuntimeID || identity.Terminal.Handle != result.Terminal.Handle || identity.Terminal.IncarnationID == "" || identity.Terminal.AgentIdentity != "" || !identity.Terminal.Connected {
		return errors.New("本机 Orca 查看窗口身份不可核实")
	}
	raw, err := json.Marshal(orcaViewerBinding{Runtime: shown.RuntimeID, Terminal: identity.Terminal})
	if err != nil {
		return err
	}
	if err := config.ProcessHost.WriteExecutionFile(bindingPath, raw); err != nil {
		return err
	}
	return client.FocusViewerTerminal(ctx, shown.RuntimeID, identity.Terminal.Handle, identity.Terminal.WorktreeID)
}

func orcaDisplayCommand(executable, requestPath, goos string) string {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	prefix := ""
	if goos == "windows" {
		quote = func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
		prefix = "& "
	}
	return prefix + quote(executable) + " orca-session-view -config " + quote(requestPath)
}

// RunOrcaSessionViewer owns a display, never the agent. Closing it cancels only
// polling. Re-resolve each frame to follow continuation Dispatches on the same
// original session, while stale attempts or replaced incarnations stop the view.
func RunOrcaSessionViewer(ctx context.Context, requestPath string, output io.Writer) error {
	raw, err := os.ReadFile(requestPath)
	if err != nil {
		return err
	}
	if len(raw) > 64<<10 {
		return errors.New("Orca 查看请求过大")
	}
	var request orcaViewerRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return err
	}
	if request.Project == "" || !filepath.IsAbs(request.Home) || !filepath.IsAbs(request.ConfigPath) {
		return errors.New("Orca 查看请求缺少原项目配置")
	}
	// Orca launches the display in its own environment. Bind the selected
	// SteerLine home explicitly before any config/database lookup.
	if err := os.Setenv("STEERLINE_HOME", request.Home); err != nil {
		return err
	}
	if err := os.Remove(requestPath + ".stopped"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	defer func() {
		_ = NewProcessHost(time.Second).WriteExecutionFile(requestPath+".stopped", []byte("stopped\n"))
	}()
	previous := ""
	for {
		frame, err := readOrcaViewerFrame(ctx, request)
		if err != nil {
			return err
		}
		frame = strings.Map(func(r rune) rune {
			if r < 32 && r != '\n' && r != '\t' || r == 127 {
				return -1
			}
			return r
		}, frame)
		if frame != previous {
			if _, err := fmt.Fprint(output, "\x1b[2J\x1b[H", frame, "\n\nCtrl+C 关闭查看；原 agent 继续运行。\n"); err != nil {
				return err
			}
			previous = frame
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func readOrcaViewerFrame(ctx context.Context, request orcaViewerRequest) (string, error) {
	config, err := LoadQueryConfig(request.ConfigPath)
	if err != nil {
		return "", err
	}
	if err := config.ResolveIdentity(ctx); err != nil {
		return "", err
	}
	if config.Identity() != request.Project {
		return "", errors.New("Orca 查看请求项目已变化")
	}
	reader, err := OpenNativeTaskReader(ctx, &config)
	if err != nil {
		return "", err
	}
	snapshot, readErr := reader.ReadExecutionStateSnapshot(ctx, config.Identity())
	if err := errors.Join(readErr, reader.Close()); err != nil {
		return "", err
	}
	spec, err := resolveOrcaSession(snapshot.Records, request.Project, request.Target)
	if err != nil {
		return "", err
	}
	return orcaViewerClient(config, spec).ReadViewerScreen(ctx, spec.ref)
}
