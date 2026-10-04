package steerline

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func validServiceCommandKind(kind string) bool {
	switch kind {
	case "retry_run", "mark_stopped", "resolve_command":
		return true
	default:
		return false
	}
}

type Runner struct {
	Dir         string
	Timeout     time.Duration
	Env         map[string]string
	Environment []string
	Input       []byte
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

func (r Runner) Output(ctx context.Context, command executionbackend.Command, args ...string) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.Command(command.Executable, append(append([]string{}, command.Args...), args...)...)
	cmd.Dir = r.Dir
	if r.Environment != nil {
		cmd.Env = append([]string{}, r.Environment...)
	}
	if r.Input != nil {
		cmd.Stdin = bytes.NewReader(r.Input)
	}
	if len(r.Env) > 0 {
		cmd.Env = overrideEnv(cmd.Environ(), r.Env)
	}
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
		return stdout.Bytes(), fmt.Errorf("%s failed: %w (%s)", command.Executable, e, clip(stderr.String(), 1000))
	}
	return stdout.Bytes(), nil
}

func overrideEnv(base []string, values map[string]string) []string {
	out := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		replace := false
		for name := range values {
			if strings.EqualFold(key, name) {
				replace = true
				break
			}
		}
		if !replace {
			out = append(out, entry)
		}
	}
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}

func unsetEnv(base []string, names []string) []string {
	if len(names) == 0 {
		return base
	}
	blocked := make(map[string]bool, len(names))
	prefixes := []string{}
	for _, name := range names {
		upper := strings.ToUpper(name)
		if strings.HasSuffix(upper, "*") {
			prefixes = append(prefixes, strings.TrimSuffix(upper, "*"))
		} else {
			blocked[upper] = true
		}
	}
	out := make([]string, 0, len(base))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		upperKey := strings.ToUpper(key)
		remove := blocked[upperKey]
		for _, prefix := range prefixes {
			if strings.HasPrefix(upperKey, prefix) {
				remove = true
				break
			}
		}
		if !remove {
			out = append(out, entry)
		}
	}
	return out
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
