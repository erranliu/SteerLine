package driver

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"time"

	host "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
)

// The launcher owns each product process independently. Stopping one project
// does not close another project's Job or the application/Web Jobs.
func (d *debugRelease) run() error {
	launcherDir := filepath.Join(d.root, "runtime-launcher")
	if err := os.MkdirAll(launcherDir, 0700); err != nil {
		return err
	}
	lease, err := host.LockRepository(launcherDir)
	if err != nil {
		return err
	}
	defer lease.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	var children []*testprocess.Process
	var files []*os.File
	defer func() {
		for i := len(children) - 1; i >= 0; i-- {
			closeCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
			children[i].Stop(closeCtx)
			done()
		}
		for _, f := range files {
			f.Close()
		}
	}()
	start := func(cmd *exec.Cmd, name string) error {
		f, err := os.OpenFile(filepath.Join(d.root, "logs", name+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		files = append(files, f)
		cmd.Stdout, cmd.Stderr = f, f
		p, err := testprocess.Start(cmd)
		if err == nil {
			children = append(children, p)
		}
		return err
	}
	web := filepath.Join(d.root, "bin", "steerline-web.exe")
	if _, err := os.Stat(web); err == nil {
		if err = start(d.command(d.binary, "serve-app"), "application"); err != nil {
			return err
		}
		readyCtx, done := context.WithTimeout(ctx, 15*time.Second)
		defer done()
		ready := false
		for readyCtx.Err() == nil {
			client, e := localservice.Open(readyCtx, d.home, filepath.Join(d.home, "application-api"), contract.Protocol, "/v1/application/identity")
			if e == nil {
				client.Close()
				ready = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !ready {
			return errors.New("application startup failed; see logs/application.log")
		}
		cmd := d.command(web, "serve")
		if origin := os.Getenv("STEERLINE_WEB_DEV_ORIGIN"); origin != "" {
			cmd.Env = append(cmd.Env, "STEERLINE_WEB_DEV_ORIGIN="+origin)
		}
		if err = start(cmd, "web"); err != nil {
			return err
		}
	}
	scopes := []*debugRelease{d}
	if d.webProjects == 2 {
		scopes = append(scopes, d.secondary())
	}
	completed := make(chan error, len(scopes))
	for _, scope := range scopes {
		go func(scope *debugRelease) { completed <- scope.runProjectContext(ctx) }(scope)
	}
	remaining := len(scopes)
	var failures []error
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		allDone := remaining == 0
		for _, child := range children {
			select {
			case <-child.Done:
			default:
				allDone = false
			}
		}
		if allDone {
			for _, child := range children {
				failures = append(failures, child.Wait(context.Background()))
			}
			return errors.Join(failures...)
		}
		select {
		case err := <-completed:
			remaining--
			failures = append(failures, err)
		case <-tick.C:
		case <-ctx.Done():
			for _, entry := range []struct {
				exe  string
				args []string
			}{{web, []string{"stop"}}, {d.binary, []string{"stop-app"}}} {
				if _, err := os.Stat(entry.exe); err == nil {
					stopCtx, done := context.WithTimeout(context.Background(), 45*time.Second)
					failures = append(failures, testprocess.Run(stopCtx, d.command(entry.exe, entry.args...)))
					done()
				}
			}
			deadline := time.NewTimer(45 * time.Second)
			defer deadline.Stop()
			for remaining > 0 {
				select {
				case err := <-completed:
					remaining--
					failures = append(failures, err)
				case <-deadline.C:
					return errors.Join(append(failures, errors.New("project normal stop timed out"))...)
				}
			}
			return errors.Join(failures...)
		}
	}
}
