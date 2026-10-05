package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/execution"
)

func TestDecisionSubmissionRequiresNativeAuthorityBeforeStateAccess(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "state")
	c := execution.Config{RunDir: runDir}
	if _, _, err := SubmitAnswerDecision(context.Background(), c, "run", "decision", "yes", "", "operator", 1, 1, "instance", newCommandID()); err == nil || !strings.Contains(err.Error(), "task_authority=native") {
		t.Fatalf("direct answer command bypassed explicit native authority: %v", err)
	}
	if _, _, err := SubmitDecisionAnswer(context.Background(), c, AnswerOptions{RunID: "run"}); err == nil || !strings.Contains(err.Error(), "task_authority=native") {
		t.Fatalf("decision CLI wrapper bypassed explicit native authority: %v", err)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Fatalf("rejected decision submission created state: %v", err)
	}
}
