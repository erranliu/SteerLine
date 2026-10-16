package app

import (
	"context"
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"path/filepath"
	"strings"
)

// Only the pinned public CLI may locate or open native execution resources.
func openRunnerNativeSession(ctx context.Context, config Config, target api.NativeSessionTarget) error {
	reader, err := OpenNativeTaskReader(ctx, &config)
	if err != nil {
		return err
	}
	defer reader.Close()
	candidate, err := reader.Candidate(ctx, config.Identity(), target.RunID)
	if err != nil {
		return err
	}
	if candidate.RunID == "" || candidate.RunID != target.ExecutionID || candidate.Projection == nil || candidate.Projection.Execution.AttemptID != target.AttemptID || candidate.Projection.Execution.SessionID != target.SessionID || candidate.Projection.Execution.Generation != target.SessionGeneration {
		return errors.New("原候选的执行或会话绑定已变化，请刷新后重试")
	}
	cli, err := config.runnerClient(ctx)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(candidate.StateDir), filepath.Clean(cli.StateDir)) {
		return errors.New("Runner 状态目录与原候选不一致")
	}
	inspection, err := cli.Inspect(ctx, "run-id", candidate.RunID, false)
	if err != nil {
		return err
	}
	run := inspection.Run
	if run.InputDigest != candidate.InputDigest || run.Execution.Branch != candidate.Branch {
		return errors.New("Runner 执行与原候选的冻结输入或分支不一致")
	}
	return cli.OpenNativeSession(ctx, runnerclient.NativeSessionTarget{RunID: target.ExecutionID, AttemptID: target.AttemptID, SessionID: target.SessionID, Generation: target.SessionGeneration})
}
