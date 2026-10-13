package orca

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// StateSubscription reads public runtime state. It neither sends agent input nor
// interprets titles, screen text or worktree activity as completion.
type StateSubscription struct {
	socket *stateSocket
	id     string
}

func SubscribeState(ctx context.Context, environment, runtime, worktree string) (*StateSubscription, json.RawMessage, error) {
	if runtime == "" || worktree == "" {
		return nil, nil, errors.New("missing original Orca state target")
	}
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	pairing, err := resolveStatePairing(environment)
	if err != nil {
		return nil, nil, err
	}
	socket, err := openStateSocket(ctx, pairing, runtime)
	if err != nil {
		return nil, nil, err
	}
	id, err := socket.request(ctx, "session.tabs.subscribe", map[string]string{"worktree": worktree})
	if err != nil {
		socket.conn.CloseNow()
		return nil, nil, err
	}
	sub := &StateSubscription{socket: socket, id: id}
	initial, err := sub.Next(ctx)
	if err != nil {
		sub.Close()
		return nil, nil, err
	}
	var snapshot struct{ Type string }
	if json.Unmarshal(initial, &snapshot) != nil || snapshot.Type != "snapshot" {
		sub.Close()
		return nil, nil, errors.New("Orca subscription did not supply an initial snapshot")
	}
	return sub, initial, nil
}

func (s *StateSubscription) Next(ctx context.Context) (json.RawMessage, error) {
	for {
		data, err := s.socket.read(ctx)
		if err != nil {
			return nil, err
		}
		result, err := s.socket.response(data, s.id)
		if err != nil || result != nil {
			return result, err
		}
	}
}

func (s *StateSubscription) Close() { _ = s.socket.conn.CloseNow() }

// A separate bounded connection leaves the streaming reader uninterrupted.
func QueryState(ctx context.Context, environment, runtime, worktree string) (json.RawMessage, error) {
	if runtime == "" || worktree == "" {
		return nil, errors.New("missing original Orca state target")
	}
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	pairing, err := resolveStatePairing(environment)
	if err != nil {
		return nil, err
	}
	socket, err := openStateSocket(ctx, pairing, runtime)
	if err != nil {
		return nil, err
	}
	defer socket.conn.CloseNow()
	return socket.call(ctx, "session.tabs.list", map[string]string{"worktree": worktree})
}

// QueryWorktreeAgents reads the public hook-store projection. Callers must
// select and validate the exact original worktree/pane; the worktree's overall
// status is not a main-agent completion fact.
func QueryWorktreeAgents(ctx context.Context, environment, runtime string) (json.RawMessage, error) {
	if runtime == "" {
		return nil, errors.New("missing original Orca runtime")
	}
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	pairing, err := resolveStatePairing(environment)
	if err != nil {
		return nil, err
	}
	socket, err := openStateSocket(ctx, pairing, runtime)
	if err != nil {
		return nil, err
	}
	defer socket.conn.CloseNow()
	return socket.call(ctx, "worktree.ps", map[string]int{"limit": 100})
}
