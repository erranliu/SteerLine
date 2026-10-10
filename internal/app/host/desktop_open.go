package host

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

// OutputDesktopOpen owns the launcher, not the desktop it opens. Running an
// desktop launcher inside a kill-on-close worker Job would kill a newly opened
// desktop when the CLI exits. All ordinary viewer reads remain managed commands.
func OutputDesktopOpen(ctx context.Context, command b.LaunchCommand, environment []string) ([]byte, error) {
	budget := command.Timeout
	if budget <= 0 {
		budget = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	cmd := exec.CommandContext(ctx, command.Executable, command.Args...)
	cmd.Dir = command.WorkingDirectory
	cmd.Env = environment
	hideDesktopLauncher(cmd)
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("打开桌面失败：%w", err)
	}
	return out, nil
}
