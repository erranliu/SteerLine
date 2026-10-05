package host

import (
	"context"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	scheduler "github.com/erranliu/SteerLine/internal/execution/scheduler"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

// RunService binds host timers and file transport to the injected scheduling port.
func RunService[T any](ctx context.Context, actor scheduler.Scheduler[T], c execution.Config, mode string) error {
	return scheduler.Run(ctx, actor, scheduler.Schedule{TaskPoll: duration(c.TaskPoll), PRPoll: duration(c.PRPoll), Heartbeat: duration(c.Heartbeat), FindStop: func(instance string, seen map[string]bool) (*productapi.ControlRequest, string, time.Time) {
		return findStopRequestCached(c, instance, seen)
	}}, mode)
}
