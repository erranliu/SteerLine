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
// original build facts remain immutable and never grant per-Run authority.
func bindRunnerBuild(ctx context.Context, c Config, store *sqlite.SQLiteStore) error {
	cli, err := c.runnerClient(ctx)
	if err != nil {
		return err
	}
	pin := cli.BuildPin()
	refactorings, err := store.Refactorings(ctx, c.Identity())
	if err != nil {
		return err
	}
	for _, r := range refactorings {
		if !runnerbinding.SamePath(r.Frozen.Runner.StateDir, pin.StateDir) {
			return errors.New("maintenance Runner namespace differs")
		}
	}
	if err := verifyEnvironmentBindings(ctx, c, store, cli); err != nil {
		return err
	}
	snapshot, err := store.ReadRunnerSnapshot(ctx, c.Identity())
	if err != nil {
		return err
	}
	var checkpoint hostcontract.ServiceCheckpoint
	if snapshot.Checkpoint != nil {
		checkpoint = *snapshot.Checkpoint
	}
	if checkpoint.Service.RunnerBuild != nil && !runnerbinding.SamePath(checkpoint.Service.RunnerBuild.StateDir, pin.StateDir) {
		return errors.New("project belongs to another Runner namespace; use a fresh application home")
	}
	if err := runnerbinding.ValidateBuildBindings(pin, snapshot.Candidates); err != nil {
		return err
	}
	roadmaps, err := store.Roadmaps(ctx, c.Identity())
	if err != nil {
		return err
	}
	for _, r := range roadmaps {
		if r.Submission != "pending" && !runnerbinding.SamePath(r.Runner.StateDir, pin.StateDir) {
			return errors.New("Roadmap Runner namespace differs")
		}
	}
	advisories, err := advisoryRecords(ctx, store, c.Identity())
	if err != nil {
		return err
	}
	for _, r := range advisories {
		if r.Submission == "started" && !runnerbinding.SamePath(r.Runner.StateDir, pin.StateDir) {
			return errors.New("advisory Runner namespace differs")
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
