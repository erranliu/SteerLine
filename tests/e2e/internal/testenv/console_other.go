//go:build !windows

package testenv

import (
	"errors"
	"os/exec"
)

func (s *Scenario) RunPseudoConsole(*exec.Cmd, string, string) ([]byte, error) {
	return nil, errors.New("interactive console acceptance requires Windows ConPTY")
}

func (s *Scenario) RunPseudoConsoleSteps(*exec.Cmd, string, string, ...ConsoleStep) ([]byte, error) {
	return nil, errors.New("interactive console acceptance requires Windows ConPTY")
}
