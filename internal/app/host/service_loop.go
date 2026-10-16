package host

import (
	"context"

	"github.com/erranliu/SteerLine/internal/execution/scheduler"
)

// RunService binds host timers to the caller scheduling port.
func RunService[T any](ctx context.Context, actor scheduler.Scheduler[T], c Config, mode string) error {
	return scheduler.Run(ctx, actor, scheduler.Schedule{TaskPoll: duration(c.TaskPoll), PRPoll: duration(c.PRPoll), Heartbeat: duration(c.Heartbeat)}, mode)
}
