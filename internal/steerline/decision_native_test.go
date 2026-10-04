package steerline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

func TestDecisionSubmissionRequiresNativeAuthorityBeforeStateAccess(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "state")
	c := Config{Persistence: sqlite.Factory{}, RunDir: runDir}
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
	request := executionmodel.DecisionRequest{FormatVersion: executionmodel.DecisionRequestFormatVersion, Question: "  choose  ", Options: []executionbackend.DecisionOption{{ID: " keep ", Label: " Keep "}, {ID: "replace", Label: "Replace"}}, Recommended: "keep", AllowFreeText: true}
	if err := executionmodel.ValidateDecisionRequest(&request); err != nil {
		t.Fatal(err)
	}
	if request.Question != "choose" || request.Options[0].ID != "keep" || request.Recommended != "keep" {
		t.Fatalf("valid decision fields were not normalized: %+v", request)
	}

	invalid := []executionmodel.DecisionRequest{
		{},
		{Question: "question"},
		{Question: "question", Options: []executionbackend.DecisionOption{{ID: "", Label: "missing id"}}},
		{Question: "question", Options: []executionbackend.DecisionOption{{ID: "same", Label: "one"}, {ID: "same", Label: "two"}}},
		{Question: "question", Options: []executionbackend.DecisionOption{{ID: "keep", Label: "Keep"}}, Recommended: "other"},
		{Question: strings.Repeat("界", 2001), Options: []executionbackend.DecisionOption{{ID: "keep", Label: "Keep"}}},
	}
	for i, candidate := range invalid {
		candidate.FormatVersion = executionmodel.DecisionRequestFormatVersion
		originalQuestion := candidate.Question
		if err := executionmodel.ValidateDecisionRequest(&candidate); err == nil {
			t.Fatalf("invalid decision request %d was accepted", i)
		}
		if len([]rune(originalQuestion)) > 2000 && len([]rune(candidate.Question)) != len([]rune(originalQuestion)) {
			t.Fatalf("overlong question %d was truncated: %d runes", i, len([]rune(candidate.Question)))
		}
	}

	answer := executionmodel.DecisionAnswer{DecisionID: "decision", RunID: "run", OptionID: "keep", AnsweredBy: "operator"}
	if err := executionmodel.ValidateDecisionAnswer(&request, &answer); err != nil {
		t.Fatalf("valid native answer was rejected: %v", err)
	}
	bad := answer
	bad.OptionID = "unknown"
	if err := executionmodel.ValidateDecisionAnswer(&request, &bad); err == nil {
		t.Fatal("answer using an unoffered option was accepted")
	}
	custom := executionmodel.DecisionAnswer{DecisionID: "decision", RunID: "run", AnsweredBy: "operator"}
	if err := executionmodel.ValidateDecisionAnswer(&request, &custom); err == nil {
		t.Fatal("free-text answer without a note was accepted")
	}
	custom.Note = "because"
	if err := executionmodel.ValidateDecisionAnswer(&request, &custom); err != nil {
		t.Fatalf("valid free-text answer was rejected: %v", err)
	}
}
