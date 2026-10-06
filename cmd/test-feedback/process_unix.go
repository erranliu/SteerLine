//go:build !windows

package main

import (
	"context"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"os/exec"
)

func runOwned(ctx context.Context, cmd *exec.Cmd) error { return testprocess.Run(ctx, cmd) }
