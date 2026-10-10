package app

import (
	"context"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecisionSubmissionRequiresNativeAuthorityBeforeStateAccess(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "state")
	c := Config{RunDir: runDir}
	if _, _, err := SubmitAnswerDecision(context.Background(), c, "run", "decision", "yes", "", "operator", 1, 1, "instance", newCommandID()); err == nil || !strings.Contains(err.Error(), "task_authority must be explicitly set to native") {
		t.Fatalf("direct answer command bypassed explicit native authority: %v", err)
	}
	if _, _, err := SubmitDecisionAnswer(context.Background(), c, AnswerOptions{RunID: "run"}); err == nil || !strings.Contains(err.Error(), "task_authority must be explicitly set to native") {
		t.Fatalf("decision CLI wrapper bypassed explicit native authority: %v", err)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Fatalf("rejected decision submission created state: %v", err)
	}
}

func TestBoundAnswerRejectsAttemptOrRecoveryRetarget(t *testing.T) {
	run := &reconstructiontypes.RunView{AttemptID: "attempt-a", RecoveryVersion: 4}
	for _, binding := range []answerBinding{{AttemptID: "attempt-b", RecoveryVersion: 4}, {AttemptID: "attempt-a", RecoveryVersion: 5}} {
		if err := validateAnswerBinding(run, binding); err == nil || !strings.Contains(err.Error(), "stale_recovery") {
			t.Fatalf("changed answer source was retargeted: binding=%+v err=%v", binding, err)
		}
	}
	if err := validateAnswerBinding(run, answerBinding{AttemptID: "attempt-a", RecoveryVersion: 4}); err != nil {
		t.Fatalf("original answer source was rejected: %v", err)
	}
}
