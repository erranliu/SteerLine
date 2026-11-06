package discovery

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDiscoveryCancellationDoesNotReuseDrainingFlight(t *testing.T) {
	m, q := fixture(t)
	initial, err := m.Query(context.Background(), q)
	if err != nil || initial.State != "ready" {
		t.Fatal(initial, err)
	}
	// Freeze an old native query in its cleanup phase after its only subscriber
	// cancels, without depending on OS scheduling or a particular cleanup speed.
	old := &pending{key: initial.ContextKey, subscribers: 1, done: make(chan struct{}), cancel: func() {}}
	m.mu.Lock()
	m.flights[old.key] = old
	m.mu.Unlock()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = m.await(canceled, old); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, finish := context.WithTimeout(context.Background(), 5*time.Second)
	defer finish()
	result, err := m.Query(ctx, q)
	if err != nil || result.State != "ready" {
		t.Fatal("new subscriber joined the canceled native query", result, err)
	}
	replacement := &pending{key: old.key}
	m.mu.Lock()
	m.flights[old.key] = replacement
	m.removeFlight(old)
	remaining := m.flights[old.key]
	delete(m.flights, old.key)
	m.mu.Unlock()
	if remaining != replacement {
		t.Fatal("old cleanup removed a replacement query")
	}
}
