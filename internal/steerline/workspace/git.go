// Package workspace owns local Git worktree operations and metadata resolution.
// It accepts only fixed repository identities and commands; it has no access to
// SteerLine configuration or business persistence.
package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/execution"
)

type Git struct {
	Command execution.LaunchCommand
	Timeout time.Duration
	Start   func(*exec.Cmd) (execution.StopHandle, error)
}

type outputLimit struct {
	bytes.Buffer
	max int
}

func (b *outputLimit) Write(data []byte) (int, error) {
	if b.Len()+len(data) > b.max {
		return 0, fmt.Errorf("Git output exceeds %d bytes", b.max)
	}
	return b.Buffer.Write(data)
}

// Output runs a fixed Git executable and argument vector in the given directory.
func (g Git) Output(ctx context.Context, directory string, args ...string) ([]byte, error) {
	if g.Command.Executable == "" || g.Start == nil || g.Timeout <= 0 {
		return nil, errors.New("Git command, process starter and positive timeout are required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, g.Timeout)
	defer cancel()
	argv := append(append([]string(nil), g.Command.Args...), args...)
	cmd := exec.Command(g.Command.Executable, argv...)
	cmd.Dir = directory
	cmd.Env = overrideEnv(cmd.Environ(), map[string]string{"GIT_TERMINAL_PROMPT": "0"})
	var stdout outputLimit
	stdout.max = 16 << 20
	var stderr outputLimit
	stderr.max = 1 << 20
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	process, err := g.Start(cmd)
	if err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		process.Kill()
		<-done
		err = ctx.Err()
	}
	process.Close()
	if err != nil {
		return stdout.Bytes(), fmt.Errorf("%s failed: %w (%s)", g.Command.Executable, err, clip(stderr.String(), 1000))
	}
	return stdout.Bytes(), nil
}

func overrideEnv(base []string, values map[string]string) []string {
	out := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "GIT_TERMINAL_PROMPT") {
			out = append(out, entry)
		}
	}
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}

func clip(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}
