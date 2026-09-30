package steerline

import (
	"context"
	"os/exec"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func codexProcessStart(cmd *exec.Cmd) (executionapi.StopHandle, error) { return startProcess(cmd) }
func openCodexHistory(ctx context.Context, command Command, dir, home string) (*codexworker.HistoryClient, error) {
	return codexworker.OpenHistory(ctx, command, dir, home, codexProcessStart)
}
