package app

import (
	"context"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

// bindRunnerBuild runs under the project ownership locks, before service start.
// A different build must use a fresh environment, never reinterpret old state.
func bindRunnerBuild(ctx context.Context, c Config, store *sqlite.SQLiteStore) error {
	cli, _, err := c.RunnerExecution.Runner.Open(ctx, runnerstatepath.Directory(c.RunDir), positiveDuration(c.CommandTimeout, 30*time.Second))
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
		return nil
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
		if check.Selection.SHA256 != pin.SHA256 || check.ResourceDigest != pin.ResourceDigest {
			return errors.New("runner_build_changed: original environment check must be reconciled with its frozen Runner")
		}
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
