package orcatransport

import (
	"context"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/reliable"
	"strings"
	"testing"
)

func TestPreparationApplicabilityOriginalTerminal(t *testing.T) {
	for _, environment := range []string{"", "saved-environment"} {
		t.Run(environment, func(t *testing.T) {
			l, f, s := fixtureOn(t, environment)
			l.setup.AgentLaunchCommand = "original-launch"
 f.commandMode=true
			next, err := reliable.Next(context.Background(), l, s)
			if err != nil || next.Action != b.LifecyclePrepareWorkspace {
				t.Fatalf("workspace omitted: %+v %v", next, err)
			}
			r, _, err := l.Execute(context.Background(), request(s, next.Action, next.Input))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			next, err = reliable.Next(context.Background(), l, s)
			if err != nil || next.Action != b.LifecycleCreateResource {
				t.Fatalf("required launch skipped: %+v %v", next, err)
			}
			// The already launched terminal path explicitly declares this mechanism
			// not applicable; its original native input preparation remains required.
			l.setup.AgentLaunchCommand = ""
			s.PendingTurn = &b.TurnInput{CommandID: strings.Repeat("c", 32), Prompt: "saved"}
			next, err = reliable.Next(context.Background(), l, s)
			if err != nil || next.Action != b.LifecyclePrepareSession {
				t.Fatalf("native input preparation skipped: %+v %v", next, err)
			}
		})
	}
}
