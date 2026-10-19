package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

// applicationRunner binds shutdown and settlement to the original selected daemon.
// Its retry semantics match the caller contract: only an unconfirmed stop may
// repeat, with the same request ID; a failed final read can be read again.
type applicationRunner struct {
	client            runnerclient.Client
	info              runnerclient.ServiceInfo
	attempted         bool
	stop, settle      func(context.Context) error
	mu                sync.Mutex
	shutdownConfirmed bool
	settlementErr     error
	requestID         string
	catalog           *runnerclient.Catalog
}

func (r *applicationRunner) start(ctx context.Context, g RunnerExecutionConfig, home string, timeout time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := g.Validate(); err != nil {
		return err
	}
	if err := runnerstatepath.Validate(home); err != nil {
		return err
	}
	cli, version, err := g.Runner.Open(ctx, runnerstatepath.Directory(home), timeout)
	if err != nil {
		return err
	}
	if err = bindApplicationRunnerBuild(home, cli.BuildPin()); err != nil {
		return err
	}
	if err = runnerstatepath.Prepare(home); err != nil {
		return err
	}
	r.client = cli
	r.attempted = true
	info, err := cli.StartService(ctx, version)
	r.info = info
	if err != nil {
		return err
	}
	// The home lease and build gate establish ownership. Once the selected
	// instance is verified, later catalog failure cannot abandon its cleanup.
	cli.InstanceID = info.InstanceID
	r.client = cli
	r.requestID = "sl-service-stop-" + info.InstanceID
	r.stop = func(ctx context.Context) error { return cli.StopService(ctx, info.InstanceID, r.requestID) }
	r.settle = func(ctx context.Context) error { return cli.VerifyServiceSettlement(ctx, info.InstanceID, r.requestID) }
	catalog, err := cli.Catalog(ctx, version)
	if err == nil {
		cli.NativeSessionAdapters = append([]string(nil), catalog.Capabilities.NativeSessionViewer.Adapters...)
	}
	r.client, r.catalog = cli, catalog
	return err
}

// The caller holds the application's home lease. Never let a new build open old state.
func bindApplicationRunnerBuild(home string, pin runnercontract.BuildPin) error {
	dir := filepath.Join(home, "application-api")
	var old runnercontract.BuildPin
	err := localservice.ReadPrivateJSON(filepath.Join(dir, "runner-build.json"), &old, 64<<10)
	if err == nil {
		if !sameRunnerBuild(old, pin) {
			return errors.New("runner_build_changed: shared Runner requires its original build; use a fresh environment")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if _, err = os.Lstat(pin.StateDir); !os.IsNotExist(err) {
		return errors.New("shared Runner directory exists without its original build pin; use a fresh environment")
	}
	return localservice.Write(dir, "runner-build.json", pin)
}

func (r *applicationRunner) validateProject(c Config) error {
	if r.catalog == nil {
		return errors.New("shared Runner catalog is unavailable")
	}
	if err := r.catalog.ValidatePublication(c.runnerPublication()); err != nil {
		return err
	}
	for _, p := range c.RunnerExecution.Profiles {
		if err := r.catalog.ValidateConfiguration(p.Execution, p.Workflow); err != nil {
			return fmt.Errorf("Runner workflow %s: %w", p.ID, err)
		}
	}
	return nil
}

func (r *applicationRunner) result() *contract.RunnerShutdown {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := &contract.RunnerShutdown{Attempted: r.attempted, ResourcesSettled: !r.attempted}
	if !r.attempted {
		return result
	}
	result.StateDir, result.InstanceID = r.client.StateDir, r.info.InstanceID
	result.ExecutableSHA256, result.Version, result.ResourceDigest = r.client.SHA256, r.client.BuildVersion, r.client.ResourceDigest
	result.RequestID, result.Stopped = r.requestID, r.shutdownConfirmed
	if err := r.resourceError(); err != nil {
		result.Diagnostic = err.Error()
	} else {
		result.ResourcesSettled = true
	}
	return result
}

func (r *applicationRunner) shutdown(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.attempted {
		return nil
	}
	if r.shutdownConfirmed {
		if r.settlementErr != nil && r.settle != nil {
			r.settlementErr = r.settle(ctx)
		}
		return nil
	}
	if r.stop == nil {
		return r.resourceError()
	}
	err := r.stop(ctx)
	r.shutdownConfirmed = err == nil
	r.settlementErr = err
	if err == nil {
		if r.settle == nil {
			r.settlementErr = errors.New("original Runner resource settlement evidence is unavailable")
		} else {
			r.settlementErr = r.settle(ctx)
		}
	}
	return err
}
func (r *applicationRunner) ResourceSettlementError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resourceError()
}
func (r *applicationRunner) resourceError() error {
	if !r.attempted {
		return nil
	}
	if !r.shutdownConfirmed {
		return errors.Join(errors.New("original Runner shutdown remains unverified"), r.settlementErr)
	}
	return r.settlementErr
}
