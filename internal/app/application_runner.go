package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

// applicationRunner binds shutdown and settlement to the original selected daemon.
// Its retry semantics match the caller contract: only an unconfirmed stop may
// repeat, with the same request ID; a failed final read can be read again.
type applicationRunner struct {
	generations       *testmanagement.GenerationQueue
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

func (r *applicationRunner) start(ctx context.Context, g RunnerExecutionConfig, home string, timeout time.Duration, prepare ...func(context.Context, *runnerclient.Client) ([]runnercontract.PendingStop, error)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := g.Validate(); err != nil {
		return err
	}
	limit, _ := executionconfig.GenerateConcurrency(g.MaxGenerateConcurrency)
	r.generations = testmanagement.NewGenerationQueue(limit)
	if err := runnerstatepath.Validate(home); err != nil {
		return err
	}
	cli, version, err := g.Runner.Open(ctx, runnerstatepath.Directory(home), timeout)
	if err != nil {
		return err
	}
	var old runnercontract.BuildPin
	if readErr := localservice.ReadPrivateJSON(filepath.Join(home, "application-api", "runner-build.json"), &old, 64<<10); readErr == nil && !runnerbinding.SameBuild(old, cli.BuildPin()) {
		proof, proofErr := cli.VerifyTakeover(ctx)
		if proofErr != nil {
			return proofErr
		}
		status, statusErr := cli.ServiceStatus(ctx)
		if statusErr != nil || status.Status != "not_running" && status.ExecutableSHA256 != cli.SHA256 {
			return errors.New("original Runner service must exit before compatible build takeover")
		}
		cli.Takeover = &proof
	}
	var stops []runnercontract.PendingStop
	for _, collect := range prepare {
		pending, collectErr := collect(ctx, &cli)
		if collectErr != nil {
			return collectErr
		}
		stops = append(stops, pending...)
	}
	var proofs []runnercontract.TakeoverReport
	if cli.Takeover != nil {
		proofs = append(proofs, *cli.Takeover)
	}
	if err = bindApplicationRunnerBuild(home, cli.BuildPin(), proofs...); err != nil {
		return err
	}
	if err = runnerstatepath.Prepare(home); err != nil {
		return err
	}
	r.client = cli
	r.attempted = true
	cli.UsagePolicy = append([]byte(nil), g.UsagePolicy...)
	serviceStarted := time.Now()
	info, err := cli.StartServiceWithStops(ctx, version, stops)
	fmt.Fprintf(os.Stderr, "startup Runner service elapsed=%s error=%v\n", time.Since(serviceStarted), err)
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
	catalogStarted := time.Now()
	catalog, err := cli.Catalog(ctx, version)
	fmt.Fprintf(os.Stderr, "startup Runner catalog elapsed=%s error=%v\n", time.Since(catalogStarted), err)
	if err == nil {
		cli = cli.WithCatalog(catalog)
	}
	r.client, r.catalog = cli, catalog
	return err
}

// The caller holds the home lease. A changed transport pin requires fresh public
// compatibility evidence; original candidate/check pins remain immutable.
func bindApplicationRunnerBuild(home string, pin runnercontract.BuildPin, proofs ...runnercontract.TakeoverReport) error {
	dir := filepath.Join(home, "application-api")
	var old runnercontract.BuildPin
	err := localservice.ReadPrivateJSON(filepath.Join(dir, "runner-build.json"), &old, 64<<10)
	if err == nil {
		if !runnerbinding.SameBuild(old, pin) {
			if len(proofs) != 1 || !runnerbinding.Compatible(proofs[0], pin) || canonicalPath(old.StateDir) != canonicalPath(pin.StateDir) {
				return errors.New("runner_build_changed: compatible takeover evidence is required")
			}
			return localservice.Write(dir, "runner-build.json", pin)
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
