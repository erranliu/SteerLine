package steerlineapp

import (
	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
)

type clientCommands struct {
	clientTaskCommands
	clientDiscardCommands
	store persistence.Connection
	lock  *steerline.RepoLock
}

func newClientCommands(store persistence.Connection, config execution.Config, lock *steerline.RepoLock) (*clientCommands, error) {
	execution, err := NewCommandService(store, config)
	if err != nil {
		return nil, err
	}
	return &clientCommands{clientTaskCommands: newClientTaskCommands(store, config), clientDiscardCommands: clientDiscardCommands{service: execution, config: config}, store: store, lock: lock}, nil
}

func (r *clientCommands) Close() error {
	err := r.store.Close()
	if r.lock != nil {
		r.lock.Close()
	}
	return err
}
