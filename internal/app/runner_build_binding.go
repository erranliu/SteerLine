package app

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

// bindRunnerBuild checks project snapshots against the application's shared client.
// The home-wide immutable build gate has already run before service start.
func bindRunnerBuild(ctx context.Context, c Config, store *sqlite.SQLiteStore) error {
	cli, err := c.runnerClient(ctx)
	if err != nil {
		return err
	}
	pin := cli.BuildPin()
	snapshot, err := store.ReadRunnerSnapshot(ctx, c.Identity())
	if err != nil {
		return err
	}
	var checkpoint hostcontract.ServiceCheckpoint
	if snapshot.Checkpoint != nil {
		checkpoint = *snapshot.Checkpoint
	}
	if checkpoint.Service.RunnerBuild != nil {
		if !sameRunnerBuild(*checkpoint.Service.RunnerBuild, pin) {
			return errors.New("runner_build_changed: restore the original Runner to reconcile/stop its resources; a new build requires a fresh environment")
		}
	}
	for _, candidate := range snapshot.Candidates {
		if !sameRunnerBuild(candidate.RunnerBuild, pin) {
			return errors.New("runner_build_changed: candidate belongs to another frozen Runner; restart with the original build")
		}
	}
	checks, err := store.Checks(ctx, c.Identity())
	if err != nil {
		return err
	}
	for _, check := range checks {
		if check.Selection.SHA256 != pin.SHA256 || check.Selection.Version != pin.Version || canonicalPath(check.Selection.Executable) != canonicalPath(pin.Executable) || check.ResourceDigest != pin.ResourceDigest || canonicalPath(check.StateDir) != canonicalPath(pin.StateDir) {
			return errors.New("runner_build_changed: original environment check must be reconciled with its frozen Runner")
		}
	}
	if checkpoint.Service.RunnerBuild != nil {
		return nil
	}
	checkpoint.SchemaVersion = hostcontract.ServiceCheckpointSchema
	checkpoint.Repository = hostcontract.Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.Identity()}
	checkpoint.Service.RunnerBuild = &pin
	_, err = store.SaveCheckpoint(ctx, checkpoint)
	return err
}

func sameRunnerBuild(a, b runnercontract.BuildPin) bool {
	return a.SHA256 == b.SHA256 && a.ResourceDigest == b.ResourceDigest && a.Version == b.Version && canonicalPath(a.StateDir) == canonicalPath(b.StateDir) && canonicalPath(a.Executable) == canonicalPath(b.Executable)
}
