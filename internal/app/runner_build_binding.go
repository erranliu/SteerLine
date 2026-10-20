package app

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

// bindRunnerBuild checks project snapshots against the application's shared client.
// The home-wide current build gate has already run before service start;
// original candidate/check build facts remain immutable after takeover.
func bindRunnerBuild(ctx context.Context, c Config, store *sqlite.SQLiteStore) error {
	cli, err := c.runnerClient(ctx)
	if err != nil {
		return err
	}
	pin := cli.BuildPin()
	compatible := cli.Takeover != nil && runnerbinding.Compatible(*cli.Takeover, pin)
	snapshot, err := store.ReadRunnerSnapshot(ctx, c.Identity())
	if err != nil {
		return err
	}
	var checkpoint hostcontract.ServiceCheckpoint
	if snapshot.Checkpoint != nil {
		checkpoint = *snapshot.Checkpoint
	}
	if checkpoint.Service.RunnerBuild != nil {
		if !runnerbinding.SameBuild(*checkpoint.Service.RunnerBuild, pin) && (!compatible || canonicalPath(checkpoint.Service.RunnerBuild.StateDir) != canonicalPath(pin.StateDir)) {
			return errors.New("runner_build_changed: restore the original Runner to reconcile/stop its resources; a new build requires a fresh environment")
		}
	}
	checks, err := store.Checks(ctx, c.Identity())
	if err != nil {
		return err
	}
	if err := runnerbinding.ValidateBuildBindings(pin, cli.Takeover, snapshot.Candidates, checks); err != nil {
		return err
	}
	generations, err := store.Generations(ctx, c.Identity())
	if err != nil {
		return err
	}
	for _, g := range generations {
		if !runnerbinding.SameBuild(g.Runner, pin) {
			if err := verifyGenerationTakeover(g, cli); err != nil {
				return err
			}
		}
	}
	if checkpoint.Service.RunnerBuild != nil && runnerbinding.SameBuild(*checkpoint.Service.RunnerBuild, pin) {
		return nil
	}
	checkpoint.SchemaVersion = hostcontract.ServiceCheckpointSchema
	checkpoint.Repository = hostcontract.Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.Identity()}
	checkpoint.Service.RunnerBuild = &pin
	_, err = store.SaveCheckpoint(ctx, checkpoint)
	return err
}
