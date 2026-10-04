package steerline

import (
	"context"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/host"
)

// RunService binds host timers and file transport to the injected scheduling port.
func RunService[T any](ctx context.Context, actor host.Scheduler[T], c execution.Config, mode string) error {
	return host.Run(ctx, actor, host.Schedule{TaskPoll: duration(c.TaskPoll), PRPoll: duration(c.PRPoll), Heartbeat: duration(c.Heartbeat), FindStop: func(instance string, seen map[string]bool) (*productapi.ControlRequest, string, time.Time) {
		return findStopRequestCached(c, instance, seen)
	}}, mode)
}
