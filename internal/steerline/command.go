package steerline

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type Runner struct {
	Dir     string
	Timeout time.Duration
}
type limitedBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		return 0, fmt.Errorf("command output exceeds %d bytes", b.max)
	}
	return b.Buffer.Write(p)
}
func (r Runner) Output(ctx context.Context, command Command, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.Command(command.Executable, append(append([]string{}, command.Args...), args...)...)
	cmd.Dir = r.Dir
	var stdout = limitedBuffer{max: 16 << 20}
	var stderr = limitedBuffer{max: 1 << 20}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	p, e := startProcess(cmd)
	if e != nil {
		return nil, e
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case e = <-done:
	case <-ctx.Done():
		p.Kill()
		<-done
		e = ctx.Err()
	}
	p.Close()
	if e != nil {
		return nil, fmt.Errorf("%s failed: %w (%s)", command.Executable, e, clip(stderr.String(), 1000))
	}
	return stdout.Bytes(), nil
}
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
