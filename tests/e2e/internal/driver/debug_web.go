package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	host "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

// run owns only debug Git/provider helpers. Both product processes are started
// through the same installed coordinator as a user's start.bat.
func (d *debugRelease) run() (retErr error) {
	dir := filepath.Join(d.root, "runtime-launcher")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	lease, err := host.LockRepository(dir)
	if err != nil {
		return err
	}
	defer lease.Close()
	token := localservice.Token()
	if err = localservice.Write(dir, "owner.json", map[string]string{"token": token}); err != nil {
		return err
	}
	os.Remove(filepath.Join(dir, "stop.json"))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	scopes := []*debugRelease{d}
	if d.webProjects == 2 {
		scopes = append(scopes, d.secondary())
	}
	var helpers []*testprocess.Process
	var files []*os.File
	defer func() {
		stopCtx, done := context.WithTimeout(context.Background(), 130*time.Second)
		stop := d.command(d.binary, "stop")
		retErr = errors.Join(retErr, testprocess.Run(stopCtx, stop))
		done()
		for _, helper := range helpers {
			cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
			retErr = errors.Join(retErr, helper.Stop(cleanup))
			done()
		}
		for _, file := range files {
			file.Close()
		}
	}()
	var selected struct {
		Scheme string `json:"scheme"`
	}
	raw, err := os.ReadFile(filepath.Join(d.root, "scheme.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &selected); err != nil {
		return err
	}
	for _, scope := range scopes {
		if selected.Scheme == "simulate" {
			addressPath := filepath.Join(filepath.Dir(scope.provider), "address.json")
			os.Remove(addressPath)
			log, err := os.OpenFile(filepath.Join(d.root, "logs", "git-server"+scope.suffix()+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			files = append(files, log)
			cmd := scope.command(d.helper, "git-server", "--root", scope.remoteRoot(), "--address-file", addressPath, "--e2e-state", scope.provider)
			cmd.Stdout, cmd.Stderr = log, log
			helper, err := testprocess.Start(cmd)
			if err != nil {
				return err
			}
			helpers = append(helpers, helper)
			readyCtx, done := context.WithTimeout(ctx, 20*time.Second)
			var address struct {
				Address string `json:"address"`
			}
			for readyCtx.Err() == nil {
				raw, e := os.ReadFile(addressPath)
				if e == nil && json.Unmarshal(raw, &address) == nil && address.Address != "" {
					break
				}
				select {
				case <-helper.Done:
					done()
					return errors.New("Git helper exited")
				case <-time.After(50 * time.Millisecond):
				}
			}
			done()
			if address.Address == "" {
				return errors.New("Git helper readiness timed out")
			}
			state, err := controlled.Read(scope.provider)
			if err != nil {
				return err
			}
			state.GitURL = "http://" + address.Address + "/" + debugRepository + ".git"
			if err = controlled.Write(scope.provider, state); err != nil {
				return err
			}
		} else {
			cmd := scope.command("git", "-C", scope.repo, "-c", "credential.helper=", "push", "origin", scope.base+":refs/heads/"+scope.base)
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if err = cmd.Run(); err != nil {
				return err
			}
		}
	}
	startCtx, done := context.WithTimeout(ctx, 190*time.Second)
	start := d.command(d.binary, "start", "--no-open")
	start.Stdout, start.Stderr = os.Stdout, os.Stderr
	err = testprocess.Run(startCtx, start)
	done()
	if err != nil {
		return err
	}
	readyCtx, done := context.WithTimeout(ctx, 180*time.Second)
	defer done()
	client, err := applicationclient.Open(readyCtx, d.home)
	if err != nil {
		return err
	}
	defer client.Close()
	for {
		var catalog contract.Catalog
		if err = client.Get(readyCtx, "/projects", &catalog); err != nil {
			return err
		}
		count := 0
		for _, project := range catalog.Projects {
			if project.RuntimeState == "failed" {
				controlled.Write(filepath.Join(d.root, "evidence", "startup-failure.json"), catalog)
				return fmt.Errorf("project %s failed: %s", project.ConfigID, project.Diagnostic)
			}
			if project.Online {
				count++
			}
		}
		if count == len(scopes) {
			break
		}
		select {
		case <-readyCtx.Done():
			return fmt.Errorf("required projects not ready: %+v", catalog)
		case <-time.After(100 * time.Millisecond):
		}
	}
	for _, scope := range scopes {
		if err = scope.seedTasks(); err != nil {
			return err
		}
	}
	if err = localservice.Write(dir, "ready.json", map[string]string{"token": token}); err != nil {
		return err
	}
	fmt.Println("Unified debug host ready:", d.home)
	if !contains(os.Args, "--no-open") {
		web := filepath.Join(d.home, "bin", "steerline-web.exe")
		cmd := d.command(web, "open")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err = cmd.Run(); err != nil {
			return err
		}
	}
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		var stop struct {
			Token string `json:"token"`
		}
		if localservice.ReadPrivateJSON(filepath.Join(dir, "stop.json"), &stop, 1024) == nil && stop.Token == token {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}
func (d *debugRelease) stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 130*time.Second)
	defer cancel()
	command := d.command(d.binary, "stop")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	stopErr := testprocess.Run(ctx, command)
	dir := filepath.Join(d.root, "runtime-launcher")
	var owner struct {
		Token string `json:"token"`
	}
	if err := localservice.ReadPrivateJSON(filepath.Join(dir, "owner.json"), &owner, 1024); err != nil {
		return errors.Join(stopErr, err)
	}
	return errors.Join(stopErr, localservice.Write(dir, "stop.json", owner))
}
