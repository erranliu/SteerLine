package app

import (
	steerline "github.com/erranliu/SteerLine/internal/app/host"
)

type applicationStore interface {
	commandWiringStore
	Close() error
}

type clientCommands struct {
	clientTaskCommands
	clientDiscardCommands
	store applicationStore
	lock  *steerline.RepoLock
}

func newClientCommands(store applicationStore, config Config, lock *steerline.RepoLock) (*clientCommands, error) {
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
