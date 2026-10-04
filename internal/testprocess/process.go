// Package testprocess owns test child processes and their descendants.
package testprocess

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"
)

type Process struct {
	Cmd        *exec.Cmd
	Done       chan struct{}
	err        error
	cleanupErr error
	mu         sync.Mutex
	stop       func() error
}

func (p *Process) Wait(ctx context.Context) error {
	select {
	case <-p.Done:
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.cleanupErr == nil {
			return p.err
		}
		return errors.Join(p.err, p.cleanupErr)
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (p *Process) Stop(ctx context.Context) error {
	select {
	case <-p.Done:
		return p.cleanupResult()
	default:
	}
	if err := p.stop(); err != nil {
		return err
	}
	select {
	case <-p.Done:
		return p.cleanupResult()
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (p *Process) cleanupResult() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cleanupErr
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
