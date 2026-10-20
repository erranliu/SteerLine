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
