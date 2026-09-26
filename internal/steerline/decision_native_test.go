package steerline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecisionSubmissionRequiresNativeAuthorityBeforeStateAccess(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "state")
	c := Config{RunDir: runDir, RuntimeFile: "runtime.json"}
	if _, _, err := SubmitAnswerDecision(context.Background(), c, "run", "decision", "yes", "", "operator", 1, 1, "instance", newID()); err == nil || !strings.Contains(err.Error(), "task_authority=native") {
		t.Fatalf("direct answer command bypassed explicit native authority: %v", err)
	}
	if _, _, err := SubmitDecisionAnswer(context.Background(), c, AnswerOptions{RunID: "run"}); err == nil || !strings.Contains(err.Error(), "task_authority=native") {
		t.Fatalf("decision CLI wrapper bypassed explicit native authority: %v", err)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Fatalf("rejected decision submission created state: %v", err)
	}
}

func TestNativeDecisionRequestAndAnswerValidation(t *testing.T) {
	request := DecisionRequest{Question: "  choose  ", Options: []DecisionOption{{ID: " keep ", Label: " Keep "}, {ID: "replace", Label: "Replace"}}, Recommended: "keep", AllowFreeText: true}
	if err := validateDecisionRequest(&request); err != nil {
		t.Fatal(err)
	}
	if request.Question != "choose" || request.Options[0].ID != "keep" || request.Recommended != "keep" {
		t.Fatalf("valid decision fields were not normalized: %+v", request)
	}

	invalid := []DecisionRequest{
		{},
		{Question: "question"},
		{Question: "question", Options: []DecisionOption{{ID: "", Label: "missing id"}}},
		{Question: "question", Options: []DecisionOption{{ID: "same", Label: "one"}, {ID: "same", Label: "two"}}},
		{Question: "question", Options: []DecisionOption{{ID: "keep", Label: "Keep"}}, Recommended: "other"},
		{Question: strings.Repeat("界", 2001), Options: []DecisionOption{{ID: "keep", Label: "Keep"}}},
	}
	for i, candidate := range invalid {
		originalQuestion := candidate.Question
		if err := validateDecisionRequest(&candidate); err == nil {
			t.Fatalf("invalid decision request %d was accepted", i)
		}
		if len([]rune(originalQuestion)) > 2000 && len([]rune(candidate.Question)) != len([]rune(originalQuestion)) {
			t.Fatalf("overlong question %d was truncated: %d runes", i, len([]rune(candidate.Question)))
		}
	}

	answer := DecisionAnswer{DecisionID: "decision", RunID: "run", OptionID: "keep", AnsweredBy: "operator"}
	if err := validateDecisionAnswer(&request, &answer); err != nil {
		t.Fatalf("valid native answer was rejected: %v", err)
	}
	bad := answer
	bad.OptionID = "unknown"
	if err := validateDecisionAnswer(&request, &bad); err == nil {
		t.Fatal("answer using an unoffered option was accepted")
	}
	custom := DecisionAnswer{DecisionID: "decision", RunID: "run", AnsweredBy: "operator"}
	if err := validateDecisionAnswer(&request, &custom); err == nil {
		t.Fatal("free-text answer without a note was accepted")
	}
	custom.Note = "because"
	if err := validateDecisionAnswer(&request, &custom); err != nil {
		t.Fatalf("valid free-text answer was rejected: %v", err)
	}
}

func TestNativeDecisionEligibilityRequiresFixedWorkflowBinding(t *testing.T) {
	attempt := strings.Repeat("a", 32)
	run := &Run{
		RunID:           strings.Repeat("b", 32),
		AttemptID:       attempt,
		WorkflowVersion: MinimalWorkflowVersion,
		ExecutionID:     strings.Repeat("c", 32),
		Backend:         string(BackendLocal),
		Protocol:        string(ProtocolLocalProcess),
		Decision:        &DecisionState{ID: strings.Repeat("d", 32), AttemptID: attempt, Status: "pending"},
	}
	config := Config{TaskAuthority: "native"}
	if err := DecisionReplyEligibility(context.Background(), config, run); err != nil {
		t.Fatalf("fixed local native decision was rejected: %v", err)
	}
	run.Decision.AttemptID = strings.Repeat("e", 32)
	if err := DecisionReplyEligibility(context.Background(), config, run); err == nil {
		t.Fatal("decision bound to another attempt was accepted")
	}
	run.Decision.AttemptID = attempt
	run.ExecutionID = ""
	if err := DecisionReplyEligibility(context.Background(), config, run); err == nil {
		t.Fatal("decision without fixed execution identity was accepted")
	}
}

func TestMinimalDecisionApplicationEvidenceBindsOriginalDecisionAttemptAndAnswer(t *testing.T) {
	oldAttempt := strings.Repeat("a", 32)
	run := &Run{
		RunID:           strings.Repeat("b", 32),
		AttemptID:       strings.Repeat("c", 32),
		ExecutionID:     strings.Repeat("d", 32),
		WorkflowVersion: MinimalWorkflowVersion,
		Decision: &DecisionState{
			ID: strings.Repeat("e", 32), AttemptID: oldAttempt, Status: "reply_accepted",
			OptionID: "keep", Note: "preserve data", AnsweredBy: "operator",
			Request: DecisionRequest{Question: "choose", Options: []DecisionOption{{ID: "keep", Label: "Keep"}}},
		},
	}
	answer, err := ReadDecisionAnswer(run)
	if err != nil {
		t.Fatal(err)
	}
	result := StepGateResult{
		AppliedDecisionID:          run.Decision.ID,
		AppliedDecisionAttemptID:   oldAttempt,
		AppliedDecisionExecutionID: run.ExecutionID,
		AppliedAnswerSHA256:        decisionAnswerDigest(answer),
	}
	if !minimalDecisionApplicationMatches(run, result) {
		t.Fatal("matching native step evidence did not apply the decision")
	}
	for name, mutate := range map[string]func(*StepGateResult){
		"decision":  func(v *StepGateResult) { v.AppliedDecisionID = strings.Repeat("f", 32) },
		"attempt":   func(v *StepGateResult) { v.AppliedDecisionAttemptID = strings.Repeat("f", 32) },
		"execution": func(v *StepGateResult) { v.AppliedDecisionExecutionID = strings.Repeat("f", 32) },
		"answer":    func(v *StepGateResult) { v.AppliedAnswerSHA256 = strings.Repeat("0", 64) },
	} {
		candidate := result
		mutate(&candidate)
		if minimalDecisionApplicationMatches(run, candidate) {
			t.Fatalf("decision application accepted mismatched %s evidence", name)
		}
	}
}
