package orcaworker

import (
	"encoding/json"
	"testing"
)

func TestPromptSubmissionRequiresMatchingPendingPaste(t *testing.T) {
	receipt := json.RawMessage(`{"prompt":{"provider":"codex","processIncarnation":"inc-original","stages":["input_accepted"]}}`)
	if !PromptAwaitingSubmission(receipt, "inc-original") || !screenHasPendingPaste([]string{"› [Pasted Content 6859 chars]"}) {
		t.Fatal("the original unsubmitted Codex paste was not recognized")
	}
	if PromptAwaitingSubmission(receipt, "inc-other") ||
		PromptAwaitingSubmission(json.RawMessage(`{"prompt":{"provider":"codex","processIncarnation":"inc-original","stages":["input_accepted","turn_started"]}}`), "inc-original") ||
		screenHasPendingPaste([]string{"• Working (5s)", "› Ask Codex to do anything"}) {
		t.Fatal("a started or different worker prompt would receive an extra Enter")
	}
}
