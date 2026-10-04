package testprocess

import (
	"context"
	"errors"
	"testing"
)

func TestProcessReportsDescendantCleanupFailure(t *testing.T) {
	rootErr := errors.New("root exit")
	cleanupErr := errors.New("descendant exit unverified")
	p := &Process{Done: make(chan struct{}), err: rootErr, cleanupErr: cleanupErr}
	close(p.Done)
	if err := p.Wait(context.Background()); !errors.Is(err, rootErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("Wait lost root or cleanup failure: %v", err)
	}
	if err := p.Stop(context.Background()); !errors.Is(err, cleanupErr) {
		t.Fatalf("Stop reported successful cleanup after descendant failure: %v", err)
	}
}
