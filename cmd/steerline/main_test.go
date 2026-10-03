package main

import (
	"os"
	"strings"
	"testing"
)

func TestCLIExecutionProfileFlagRejectsUnrelatedActions(t *testing.T) {
	for _, args := range [][]string{
		{"check", "-execution-profile", "orca"},
		{"control", "-action", "resume_dispatch", "-execution-profile", "orca"},
		{"control", "-action", "start_task", "-request-id", "receipt", "-execution-profile", "orca"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			old := os.Args
			os.Args = append([]string{"steerline"}, args...)
			defer func() { os.Args = old }()
			if err := run(); err == nil || !strings.Contains(err.Error(), "only valid with control -action start_task") {
				t.Fatalf("execution selection accepted for another action: %v", err)
			}
		})
	}
}

func TestProjectsMenuCanOpenCurrentConsole(t *testing.T) {
	for _, action := range []string{"console", "1"} {
		got, err := pickAction(action)
		if err != nil || got != "console" {
			t.Fatalf("pickAction(%q) = %q, %v; want console", action, got, err)
		}
	}
}
