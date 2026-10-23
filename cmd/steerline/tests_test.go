package main

import (
	"strings"
	"testing"
)

func TestTMCLIRejectsIncompleteCommandsBeforeConnection(t *testing.T) {
	id := strings.Repeat("a", 32)
	for _, args := range [][]string{
		{}, {"other", "-request-id", id}, {"generate", "-request-id", id},
		{"generation", "-request-id", "bad"}, {"generation", "-request-id", id, "-plan", "wrong.plan.md"},
		{"stop-generation", "-request-id", id}, {"generation", "-request-id", id, "extra"},
	} {
		if err := runTestsCommand(args); err == nil || strings.Contains(err.Error(), "application") {
			t.Fatalf("invalid command reached application: %v: %v", args, err)
		}
	}
}

func TestPMCLIApplyRequiresCompleteReview(t *testing.T) {
	id := strings.Repeat("a", 32)
	for _, args := range [][]string{{"apply", "-request-id", id}, {"apply", "-request-id", id, "-generation-id", id, "-expected-revision", "1"}, {"generation", "-request-id", id, "-allow-empty"}, {"generate", "-request-id", id, "-plan", "SteerlineFlow/plans/a.plan.md", "-execution-profile", "local", "-expected-revision", "1"}} {
		if e := runTestsCommand(args); e == nil || strings.Contains(e.Error(), "application") {
			t.Fatalf("incomplete review reached application: %v %v", args, e)
		}
	}
}
