//go:build !windows

package main

import (
	"errors"
	"os/exec"
	"time"
)

func provisioningCommand(string, string, time.Duration) (*exec.Cmd, error) {
	return nil, errors.New("requires native Windows")
}
