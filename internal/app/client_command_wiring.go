package app

import (
	"context"
	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type applicationStore interface {
	taskStorage
	taskrunner.DiscardStore
	Close() error
}

type clientCommands struct {
	clientTaskCommands
	clientDiscardCommands
	store applicationStore
	lock  *steerline.RepoLock
}

func newClientCommands(store applicationStore, config Config, lock *steerline.RepoLock) (*clientCommands, error) {
	return &clientCommands{clientTaskCommands: newClientTaskCommands(store, config), clientDiscardCommands: clientDiscardCommands{service: offlineRunnerDiscard{config: config, store: store}}, store: store, lock: lock}, nil
}

type offlineRunnerDiscard struct {
	config Config
	store  taskrunner.DiscardStore
}

func (s offlineRunnerDiscard) Discard(ctx context.Context, id string, intent taskmodel.DiscardIntent) (taskmodel.TaskDiscardReceipt, error) {
	_, found, err := s.store.LookupCommand(ctx, s.config.Identity(), id)
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	var cli runnerclient.Client
	if !found {
		rows, err := s.store.RunnerCandidates(ctx, s.config.Identity(), intent.TaskID)
		if err != nil {
			return taskmodel.TaskDiscardReceipt{}, err
		}
		for _, candidate := range rows {
			if candidate.RunID != "" {
				cli, err = s.config.runnerClient(ctx)
				if err != nil {
					return taskmodel.TaskDiscardReceipt{}, err
				}
				break
			}
		}
	}
	return runnerTaskService(s.config, s.store, cli).Discard(ctx, s.store, id, intent)
}

func (r *clientCommands) Close() error {
	err := r.store.Close()
	if r.lock != nil {
		r.lock.Close()
	}
	return err
}
