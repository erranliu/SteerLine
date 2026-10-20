package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	host "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// The home is owned and no project actors have started. Take the usual project
// locks only while preserving the caller's existing durable stop outbox. Runner
// remains the only execution-state authority and admits stops before workers.
func (d *ApplicationDirectory) prepareRunnerStops(ctx context.Context, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	var pending []runnercontract.PendingStop
	for _, entry := range d.entries {
		if entry.ref.RunDir == "" {
			if entry.view.RuntimeState == "failed" {
				return nil, errors.New("Runner startup cannot inspect an unresolved project census")
			}
			continue
		}
		if _, err := os.Stat(filepath.Join(entry.ref.RunDir, "steerline.db")); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		c, err := loadConfigSnapshot(entry.ref.ConfigPath, entry.data, d.global, nil, false)
		if err != nil {
			return nil, err
		}
		stops, err := prepareProjectRunnerStops(ctx, &c, cli)
		if err != nil {
			return nil, fmt.Errorf("%s startup recovery: %w", entry.view.ConfigID, err)
		}
		pending = append(pending, stops...)
	}
	return pending, nil
}

func prepareProjectRunnerStops(ctx context.Context, c *Config, cli *runnerclient.Client) ([]runnercontract.PendingStop, error) {
	if err := c.ResolveIdentity(ctx); err != nil {
		return nil, err
	}
	repoLock, err := host.LockRepository(c.Identity())
	if err != nil {
		return nil, err
	}
	defer repoLock.Close()
	runLock, err := host.LockRepository(c.RunDir)
	if err != nil {
		return nil, err
	}
	defer runLock.Close()
	store, err := OpenNativeTaskCommandStore(ctx, c)
	if err != nil {
		return nil, err
	}
	defer store.Close()
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
		return nil, nil
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
	var pending []runnercontract.PendingStop
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
