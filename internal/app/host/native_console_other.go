//go:build !windows

package host

import (
	"context"
	"errors"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func OpenNativeConsole(context.Context, executionbackend.LaunchCommand) error {
	return errors.New("当前平台不支持打开独立原生终端窗口")
}
