//go:build windows

package main

import (
	"context"
	"github.com/erranliu/SteerLine/internal/testprocess"
	"os/exec"
)

func runOwned(ctx context.Context, cmd *exec.Cmd) error { return testprocess.Run(ctx, cmd) }
