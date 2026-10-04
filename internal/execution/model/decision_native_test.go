package model

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"strings"
	"testing"
)

func TestNativeDecisionRequestAndAnswerValidation(t *testing.T) {
	request := DecisionRequest{FormatVersion: DecisionRequestFormatVersion, Question: "  choose  ", Options: []executionbackend.DecisionOption{{ID: " keep ", Label: " Keep "}, {ID: "replace", Label: "Replace"}}, Recommended: "keep", AllowFreeText: true}
	if err := ValidateDecisionRequest(&request); err != nil {
		t.Fatal(err)
	}
	if request.Question != "choose" || request.Options[0].ID != "keep" || request.Recommended != "keep" {
		t.Fatalf("valid decision fields were not normalized: %+v", request)
	}

	invalid := []DecisionRequest{
		{},
		{Question: "question"},
		{Question: "question", Options: []executionbackend.DecisionOption{{ID: "", Label: "missing id"}}},
		{Question: "question", Options: []executionbackend.DecisionOption{{ID: "same", Label: "one"}, {ID: "same", Label: "two"}}},
		{Question: "question", Options: []executionbackend.DecisionOption{{ID: "keep", Label: "Keep"}}, Recommended: "other"},
		{Question: strings.Repeat("界", 2001), Options: []executionbackend.DecisionOption{{ID: "keep", Label: "Keep"}}},
	}
	for i, candidate := range invalid {
		candidate.FormatVersion = DecisionRequestFormatVersion
		originalQuestion := candidate.Question
		if err := ValidateDecisionRequest(&candidate); err == nil {
			t.Fatalf("invalid decision request %d was accepted", i)
		}
		if len([]rune(originalQuestion)) > 2000 && len([]rune(candidate.Question)) != len([]rune(originalQuestion)) {
			t.Fatalf("overlong question %d was truncated: %d runes", i, len([]rune(candidate.Question)))
		}
	}

	answer := DecisionAnswer{DecisionID: "decision", RunID: "run", OptionID: "keep", AnsweredBy: "operator"}
	if err := ValidateDecisionAnswer(&request, &answer); err != nil {
		t.Fatalf("valid native answer was rejected: %v", err)
	}
	bad := answer
	bad.OptionID = "unknown"
	if err := ValidateDecisionAnswer(&request, &bad); err == nil {
		t.Fatal("answer using an unoffered option was accepted")
	}
	custom := DecisionAnswer{DecisionID: "decision", RunID: "run", AnsweredBy: "operator"}
	if err := ValidateDecisionAnswer(&request, &custom); err == nil {
		t.Fatal("free-text answer without a note was accepted")
	}
	custom.Note = "because"
	if err := ValidateDecisionAnswer(&request, &custom); err != nil {
		t.Fatalf("valid free-text answer was rejected: %v", err)
	}
}
