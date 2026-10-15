package app

import (
	"fmt"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func resolveCommand(c *executionbackend.Command) error {
	p, e := exec.LookPath(c.Executable)
	if e != nil {
		return e
	}
	ext := strings.ToLower(filepath.Ext(p))
	if ext == ".cmd" || ext == ".bat" || ext == ".ps1" {
		return fmt.Errorf("use a native executable instead of shell shim: %s", p)
	}
	c.Executable = p
	return nil
}
func positiveDuration(value string, fallback time.Duration) time.Duration {
	d, e := time.ParseDuration(value)
	if e != nil || d <= 0 {
		return fallback
	}
	return d
}
