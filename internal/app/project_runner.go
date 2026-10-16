package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

// projectRunner binds shutdown and settlement to the original selected daemon.
// Its retry semantics match the caller contract: only an unconfirmed stop may
// repeat, with the same request ID; a failed final read can be read again.
type projectRunner struct {
	client            runnerclient.Client
	info              runnerclient.ServiceInfo
	attempted         bool
	stop, settle      func(context.Context) error
	mu                sync.Mutex
	shutdownConfirmed bool
	settlementErr     error
}

func (r *projectRunner) start(ctx context.Context, g RunnerExecutionConfig, runDir string, timeout time.Duration, publication runnerclient.Publication) error {
	if err := g.Validate(); err != nil {
		return err
	}
	if err := runnerstatepath.Prepare(runDir); err != nil {
		return err
	}
	cli, version, err := g.Runner.Open(ctx, runnerstatepath.Directory(runDir), timeout)
	if err != nil {
		return err
	}
	r.client = cli
	before, err := cli.ServiceStatus(ctx)
	if err != nil {
		return err
	}
	r.attempted = true
	info, err := cli.StartService(ctx, version)
	r.info = info // Retain observed identity without treating a failed start as ownership.
	if err != nil {
		return err
	}
	catalog, err := cli.Catalog(ctx, version)
	if err == nil {
		cli.NativeSessionAdapters = append([]string(nil), catalog.Capabilities.NativeSessionViewer.Adapters...)
	}
	if err == nil {
		err = catalog.ValidatePublication(publication)
	}
	if err == nil {
		for _, profile := range g.Profiles {
			if e := catalog.ValidateExecution(profile.Execution); e != nil {
				err = fmt.Errorf("Runner profile %s: %w", profile.ID, e)
				break
			}
			if e := catalog.ValidateWorkflow(profile.Workflow); e != nil {
				err = fmt.Errorf("Runner workflow %s: %w", profile.ID, e)
				break
			}
		}
	}
	r.client, r.info = cli, info
	// A rejected startup owns only a daemon absent before this attempt. Keep
	// the existing request IDs and never stop a previously ready daemon here.
	if err == nil || before.Status == "not_running" {
		requestID := "sl-service-stop-" + info.InstanceID
		if err != nil {
			requestID = "sl-startup-rejected-" + info.InstanceID
		}
		r.stop = func(ctx context.Context) error { return cli.StopService(ctx, info.InstanceID, requestID) }
		r.settle = func(ctx context.Context) error { return cli.VerifyServiceSettlement(ctx, info.InstanceID, requestID) }
	}
	return err
}

func (r *projectRunner) shutdown(ctx context.Context) error {
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
func (r *projectRunner) ResourceSettlementError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resourceError()
}
func (r *projectRunner) resourceError() error {
	if !r.attempted {
		return nil
	}
	if !r.shutdownConfirmed {
		return errors.Join(errors.New("original Runner shutdown remains unverified"), r.settlementErr)
	}
	return r.settlementErr
}
