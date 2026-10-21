// Package nativequery owns bounded metadata-query processes. It has no shell,
// task, session, or business execution API.
package nativequery

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/processwindow"
	"io"
	"os/exec"
	"sync"
	"time"
)

const Limit = 1 << 20

type Process struct {
	Input    io.WriteCloser
	Output   io.ReadCloser
	cmd      *exec.Cmd
	done     chan struct{}
	err      error
	kill     func() error
	once     sync.Once
	waitOnce sync.Once
}

func Start(ctx context.Context, executable string, args ...string) (*Process, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	processwindow.Hide(cmd)
	cmd.WaitDelay = 5 * time.Second
	p := &Process{cmd: cmd, done: make(chan struct{})}
	prepare(cmd)
	var err error
	if p.Input, err = cmd.StdinPipe(); err != nil {
		return nil, err
	}
	if p.Output, err = cmd.StdoutPipe(); err != nil {
		p.Input.Close()
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		p.Input.Close()
		p.Output.Close()
		return nil, errors.New("cannot start metadata query executable")
	}
	p.kill, err = own(cmd)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		p.Input.Close()
		p.Output.Close()
		return nil, err
	}
	go func() {
		select {
		case <-ctx.Done():
			p.kill()
		case <-p.done:
		}
	}()
	return p, nil
}
func (p *Process) Wait() error { p.startWait(); <-p.done; return p.err }
func (p *Process) startWait() {
	p.waitOnce.Do(func() { go func() { p.err = p.cmd.Wait(); close(p.done) }() })
}
func (p *Process) Close() error {
	var result error
	p.once.Do(func() {
		p.Input.Close()
		p.kill()
		p.startWait()
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			result = errors.New("metadata query cleanup remains unverified")
		}
		p.Output.Close()
	})
	return result
}
func Run(ctx context.Context, executable string, args []string) (raw []byte, err error) {
	p, err := Start(ctx, executable, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if e := p.Close(); e != nil {
			err = e
		}
	}()
	p.Input.Close()
	raw, err = io.ReadAll(io.LimitReader(p.Output, Limit+1))
	if err != nil {
		return nil, errors.New("cannot read metadata query")
	}
	if len(raw) > Limit {
		return nil, errors.New("metadata query exceeds 1 MiB")
	}
	if err = p.Wait(); err != nil {
		return nil, errors.New("metadata query failed or timed out")
	}
	return raw, nil
}
