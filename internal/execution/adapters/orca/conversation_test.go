package orcaworker

import (
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestTerminalWaitMapsAsHostFactWithoutBusinessInput(t *testing.T) {
	input := executionvalues.ConversationInput{Event: executionvalues.ConversationEvent{ProjectID: "project", TaskID: "task", WorkflowID: "workflow", ExecutionID: "execution", AttemptID: "attempt",
		Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaTerminalV2, Agent: "codex", TargetID: "target"}, Source: "orca-terminal-v2", Segment: "terminal", Identity: "terminal-source", Session: "terminal-session", InputWait: true}
	batch, err := MapTerminalConversation(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Writes) != 1 || batch.Writes[0].Event.Type != executionvalues.ConversationInputWait || batch.Writes[0].Event.Status != "waiting" ||
		batch.Writes[0].Event.Attributes["wait_origin"] != "host" || batch.Writes[0].Event.Attributes["business_decision"] != false {
		t.Fatalf("host wait was not kept separate from business input: %+v", batch)
	}
}
