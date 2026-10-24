package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCandidateStopReliabilityRequestArbitration(t *testing.T) {
	server, _, _, _ := newAPIFixture(t)
	registry := server.commands
	release, err := registry.lockRequest(context.Background(), "slow-command")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	other, err := registry.lockRequest(context.Background(), "stop")
	if err != nil {
		t.Fatal(err)
	}
	other()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, err := registry.lockRequest(ctx, "slow-command"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("wait was not bounded", err)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if len(registry.requestGates) != 1 || registry.requestGates["slow-command"].users != 1 {
		t.Fatal("cancelled request leaked arbitration", registry.requestGates)
	}
}
