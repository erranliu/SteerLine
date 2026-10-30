package model_test

import (
	"strings"
	"testing"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
)

func TestEnvironmentReadinessRequiresCurrentTTYResult(t *testing.T) {
	for _, status := range []string{"missing", "failed", "incomplete", "passed"} {
		t.Run(status, func(t *testing.T) {
			r := env.Report{Version: env.ReportVersion, Completed: true, Checks: []env.Check{{ID: "git", Status: "passed"}}, L2: []env.Item{{ID: "l2", Status: "passed"}}, PR: env.PR{URL: "https://example.invalid/repo/pulls/1", Base: "validation", Head: "head", HeadRevision: strings.Repeat("c", 40), CleanupCompleted: true}}
			if status != "missing" {
				r.Checks = append(r.Checks, env.Check{ID: "terminal.tty", Status: status})
			}
			if r.Passed("validation", "head") != (status == "passed") {
				t.Fatal("TTY result did not determine admission", status)
			}
		})
	}
}
