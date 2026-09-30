package steerline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
)

func TestAppServerThreadUsesNoninteractiveApprovalPolicy(t *testing.T) {
	params, err := codexworker.ThreadParams("C:/fixed/worktree", "workspace-write", "", []any{codexapp.AskDynamicTool()})
	if err != nil || params["approvalPolicy"] != "never" || params["sandbox"] != "workspace-write" {
		t.Fatalf("worker thread could request an unsupported host approval: %+v %v", params, err)
	}
}

func TestCodexAskCallPreservesNativeBindingAndAllQuestions(t *testing.T) {
	thread, turn := "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	params := map[string]any{"threadId": thread, "turnId": turn, "callId": "call-1", "namespace": nil, "tool": codexapp.AskToolName, "arguments": map[string]any{"questions": []any{map[string]any{"id": "branch", "header": "Branch", "question": "Use which branch?", "options": []any{map[string]any{"id": "a", "label": "A"}, map[string]any{"id": "b", "label": "B"}}, "allow_free_text": true}, map[string]any{"id": "scope", "header": "Scope", "question": "What scope?", "options": []any{}, "allow_free_text": true}}}}
	call, err := codexworker.ParseAskCall(codexapp.Envelope{ID: json.RawMessage(`17`), Method: "item/tool/call", Params: mustJSON(params)}, thread, turn)
	if err != nil {
		t.Fatal(err)
	}
	if string(call.RequestID) != "17" || call.CallID != "call-1" || len(call.Questions) != 2 || call.Questions[1].ID != "scope" {
		t.Fatalf("lost native binding or question: %+v", call)
	}
	call.Generation = "generation"
	decision, err := appServerAskDecision(&Run{}, call)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"[branch]", "Use which branch?", "a：A", "b：B", "[scope]", "What scope?"} {
		if !strings.Contains(decision.Request.Context, text) {
			t.Fatalf("multi-question UI omitted %q: %s", text, decision.Request.Context)
		}
	}
	params["turnId"] = "00000000-0000-0000-0000-000000000003"
	if _, err := codexworker.ParseAskCall(codexapp.Envelope{ID: json.RawMessage(`17`), Method: "item/tool/call", Params: mustJSON(params)}, thread, turn); err == nil {
		t.Fatal("accepted another turn's question")
	}
}
