package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	host "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

// The home is owned and no project actors have started. Take the usual project
// locks only while preserving the caller's existing durable stop outbox. Runner
// remains the only execution-state authority and admits stops before workers.
func (d *ApplicationDirectory) prepareRunnerStops(ctx context.Context, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	started := time.Now()
	defer func() { fmt.Fprintf(os.Stderr, "startup recovery preparation elapsed=%s\n", time.Since(started)) }()
	type prepared struct {
		config  Config
		store   *sqlite.SQLiteStore
		release func()
		err     error
	}
	preparedProjects := make([]prepared, len(d.entries))
	// Projects have passed the common-dir/run-dir conflict census. Only their
	// local preparation runs concurrently; takeover and stop outbox decisions
	// below still use one original Runner client in deterministic project order.
	slots := make(chan struct{}, 2)
	var wg sync.WaitGroup
	for i, entry := range d.entries {
		wg.Add(1)
		go func(i int, entry *catalogEntry) {
			defer wg.Done()
			p := &preparedProjects[i]
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				p.err = ctx.Err()
				return
			}
			defer func() { <-slots }()
			if entry.ref.RunDir == "" {
				if entry.view.RuntimeState == "failed" {
					p.err = errors.New("Runner startup cannot inspect an unresolved project census")
				}
				return
			}
			if _, err := os.Stat(filepath.Join(entry.ref.RunDir, "steerline.db")); os.IsNotExist(err) {
				return
			} else if err != nil {
				p.err = err
				return
			}
			p.config, p.err = loadConfigSnapshotWithQueries(entry.ref.ConfigPath, entry.data, d.global, nil, false, newStartupQueries(ctx))
			if p.err != nil {
				return
			}
			p.config.runtimeGenerations = d.runner.generations
			p.store, p.release, p.err = openStartupStopStore(ctx, &p.config)
		}(i, entry)
	}
	wg.Wait()
	defer func() {
		for i := range preparedProjects {
			if preparedProjects[i].release != nil {
				preparedProjects[i].release()
			}
		}
	}()
	var pending []runnercontract.PendingStop
	for i, entry := range d.entries {
		p := &preparedProjects[i]
		if p.err != nil {
			return nil, fmt.Errorf("%s startup recovery: %w", entry.view.ConfigID, p.err)
		}
		if p.store == nil {
			continue
		}
		stops, err := collectProjectRunnerStops(ctx, &p.config, p.store, cli)
		if err != nil {
			return nil, fmt.Errorf("%s startup recovery: %w", entry.view.ConfigID, err)
		}
		pending = append(pending, stops...)
		c := p.config
		c.finishStartupQueries()
		entry.startupConfig = &c
	}
	return pending, nil
}

func prepareProjectRunnerStops(ctx context.Context, c *Config, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	store, release, err := openStartupStopStore(ctx, c)
	if err != nil {
		return nil, err
	}
	defer release()
	return collectProjectRunnerStops(ctx, c, store, cli)
}

func openStartupStopStore(ctx context.Context, c *Config) (*sqlite.SQLiteStore, func(), error) {
	if err := c.ResolveIdentity(ctx); err != nil {
		return nil, nil, err
	}
	repoLock, err := host.LockRepository(c.Identity())
	if err != nil {
		return nil, nil, err
	}
	runLock, err := host.LockRepository(c.RunDir)
	if err != nil {
		repoLock.Close()
		return nil, nil, err
	}
	if c.startup != nil {
		c.resetStartupQueries(ctx)
	}
	store, err := OpenNativeTaskCommandStore(ctx, c)
	if err != nil {
		runLock.Close()
		repoLock.Close()
		return nil, nil, err
	}
	return store, func() { _ = store.Close(); runLock.Close(); repoLock.Close() }, nil
}

func collectProjectRunnerStops(ctx context.Context, c *Config, store *sqlite.SQLiteStore, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	if c.runtimeGenerations != nil {
		rows, err := store.Generations(ctx, c.Identity())
		if err != nil {
			return nil, err
		}
		for _, g := range rows {
			c.runtimeGenerations.Update(g)
		}
	}
	roadmapStops, err := prepareRoadmapStops(ctx, c, store, cli)
	if err != nil {
		return nil, err
	}
	generationStops, err := prepareGenerationStops(ctx, store, c.Identity(), cli)
	if err != nil {
		return nil, err
	}
	generationStops = append(generationStops, roadmapStops...)
	snapshot, err := store.ReadRunnerSnapshot(ctx, c.Identity())
	if err != nil {
		return nil, err
	}
	need := false
	for _, candidate := range snapshot.Candidates {
		if !runnerbinding.SameBuild(candidate.RunnerBuild, cli.BuildPin()) || candidate.StopRequested && !candidate.InputsRevoked {
			need = true
		}
	}
	checks, err := store.Checks(ctx, c.Identity())
	if err != nil {
		return nil, err
	}
	for _, check := range checks {
		if check.Selection.SHA256 != cli.SHA256 || check.ResourceDigest != cli.ResourceDigest {
			need = true
		}
	}
	if !need {
		return generationStops, nil
	}
	if cli.Takeover == nil {
		proof, err := cli.VerifyTakeover(ctx)
		if err != nil {
			return nil, err
		}
		cli.Takeover = &proof
	}
	if !runnerbinding.Compatible(*cli.Takeover, cli.BuildPin()) {
		return nil, errors.New("original Runner state is incompatible")
	}
	for _, check := range checks {
		if !runnerbinding.Check(*cli.Takeover, check) {
			return nil, errors.New("original environment check differs from takeover proof")
		}
	}
	pending := generationStops
	bridge := &runnerbridge.Bridge{Store: store}
	for _, candidate := range snapshot.Candidates {
		if canonicalPath(candidate.StateDir) != canonicalPath(cli.StateDir) {
			return nil, errors.New("candidate Runner directory changed")
		}
		item, bound := runnerbinding.Candidate(*cli.Takeover, candidate)
		if candidate.RunID != "" && !bound {
			return nil, errors.New("original candidate is absent or input digest differs from takeover proof")
		}
		if !candidate.StopRequested || candidate.InputsRevoked {
			continue
		}
		if !bound {
			if candidate.SubmitState == "pending" || candidate.SubmitState == "rejected" || candidate.SubmitState == "cancelled" {
				continue
			}
			// A request not present in the complete offline command projection
			// stays observe-only. Starting workers cannot create its absent Run.
			if candidate.RunID == "" {
				continue
			}
			return nil, errors.New("original cancellation target is unverified")
		}
		if item.Status != "compatible" {
			return nil, errors.New("read-only original Run cannot accept its pending stop")
		}
		stop, err := bridge.PrepareStartupStop(ctx, candidate, item.ID, item.Revision)
		if err != nil {
			return nil, err
		}
		if stop != nil {
			pending = append(pending, *stop)
		}
	}
	return pending, nil
}
