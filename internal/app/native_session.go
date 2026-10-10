package app

import (
	"context"
	"errors"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	api "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func openNativeSession(ctx context.Context, config Config, target api.NativeSessionTarget) error {
	reader, err := OpenNativeTaskReader(ctx, &config)
	if err != nil {
		return err
	}
	snapshot, err := reader.ReadExecutionStateSnapshot(ctx, config.Identity())
	closeErr := reader.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return err
	}
	for _, execution := range snapshot.Records.Executions {
		if execution.ExecutionID == target.ExecutionID && execution.Protocol == string(executionbackend.ProtocolOrcaTerminalV3) {
			return openOrcaSession(ctx, config, snapshot.Records, target)
		}
	}
	command, binding, process, err := nativeSessionCommand(snapshot.Records, config.Identity(), target)
	if err != nil {
		return err
	}
	if resourceprocess.InspectProcess(process) != "matching process still exists" {
		return errors.New("原 Codex app-server 已退出或进程身份无法核实；无法查看实时会话")
	}
	if err := codexworker.VerifyNativeViewer(ctx, binding, command.WorkingDirectory); err != nil {
		return err
	}
	return steerline.OpenNativeConsole(ctx, command)
}

func nativeSessionCommand(records storageports.ExecutionSnapshot, project string, target api.NativeSessionTarget) (executionbackend.LaunchCommand, codexworker.AppServerBinding, *executionbackend.ProcessIdentity, error) {
	var command executionbackend.LaunchCommand
	var binding codexworker.AppServerBinding
	fail := func(message string) (executionbackend.LaunchCommand, codexworker.AppServerBinding, *executionbackend.ProcessIdentity, error) {
		return command, binding, nil, errors.New(message)
	}
	if records.ProjectIdentity != project || project == "" {
		return fail("候选项目身份不匹配")
	}
	var execution *executionmodel.ExecutionRecord
	for i := range records.Executions {
		e := &records.Executions[i]
		if e.ExecutionID == target.ExecutionID && e.ProjectIdentity == project && e.WorkflowRunID == target.RunID {
			execution = e
			break
		}
	}
	if execution == nil || execution.CurrentAttemptID != target.AttemptID || execution.SessionID != target.SessionID || execution.SessionGeneration != target.SessionGeneration || target.SessionID == "" || target.SessionGeneration == "" {
		return fail("原候选会话已变化；刷新后重试")
	}
	e := execution
	if e.CurrentTurnID == "" {
		return fail("原生执行轮次尚未开始；Codex CLI 暂无可恢复历史")
	}
	if e.Backend != string(executionbackend.BackendLocal) || e.Protocol != string(executionbackend.ProtocolLocalCodexAppServer) || e.Agent != "codex" || e.Process == nil {
		return fail("仅支持已绑定原进程的本机 Codex app-server 会话")
	}
	var profile *executionmodel.FixedExecutionProfile
	for _, run := range records.WorkflowRuns {
		if run.WorkflowRunID == target.RunID && run.ProjectIdentity == project {
			profile = run.ExecutionProfile
			break
		}
	}
	if profile == nil || profile.Host != "local" || profile.Agent != "codex" {
		return fail("原候选执行方案不可用")
	}
	settings, err := codexworker.DecodeProfileConfigEnvelope(profile.ImplementationData, codexworker.ProfileConfigIdentity{ProjectID: project, ProfileID: profile.ID, AcceptedConfigVersion: profile.AcceptedConfigVersion})
	if err != nil {
		return command, binding, nil, err
	}
	if len(e.ImplementationData) != 1 {
		return fail("原 Codex app-server 绑定缺失或重复")
	}
	binding, err = codexworker.DecodeAppServerBindingEnvelope(e.ImplementationData[0], codexworker.AppServerBindingIdentity{ProjectID: project, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: e.ConfigurationDigest})
	if err != nil {
		return command, binding, nil, err
	}
	if binding.SessionID != target.SessionID || binding.Generation != target.SessionGeneration {
		return fail("原 Codex app-server 会话身份不匹配")
	}
	command, err = codexworker.NativeViewerCommand(settings.Executable, e.Workspace.Worktree, binding)
	return command, binding, e.Process, err
}
