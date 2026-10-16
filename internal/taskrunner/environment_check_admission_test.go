package taskrunner

import (
	"context"
	"errors"
	"testing"
)

func TestEnvironmentCheckSharesOriginalRunnerAdmission(t *testing.T) {
	expected := errors.New("original Runner instance changed")
	calls := 0
	actor := &Actor{options: ActorOptions{Tasks: &Service{Admit: func(context.Context) error { calls++; return expected }}}}
	if err := actor.VerifyRunnerAdmission(context.Background()); !errors.Is(err, expected) || calls != 1 {
		t.Fatal("original Runner gate bypassed", err, calls)
	}
	actor.options.Tasks.Admit = nil
	if err := actor.VerifyRunnerAdmission(context.Background()); err == nil {
		t.Fatal("unbound instance gate allowed check writes")
	}
}
