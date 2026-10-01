// Package testprocess owns test child processes and their descendants.
package testprocess

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

type Process struct {
	Cmd  *exec.Cmd
	Done chan struct{}
	err  error
	mu   sync.Mutex
	stop func() error
}

func (p *Process) Wait(ctx context.Context) error {
	select {
	case <-p.Done:
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (p *Process) Stop(ctx context.Context) error {
	select {
	case <-p.Done:
		return nil
	default:
	}
	if err := p.stop(); err != nil {
		return err
	}
	select {
	case <-p.Done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func Run(ctx context.Context, cmd *exec.Cmd) error {
	p, err := Start(cmd)
	if err != nil {
		return err
	}
	select {
	case <-p.Done:
		return p.Wait(ctx)
	case <-ctx.Done():
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := p.Stop(stopCtx)
		if err != nil {
			return err
		}
		return ctx.Err()
	}
}
