package steerline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNativeOrcaQuestionUsesSharedNativeAnswerShape(t *testing.T) {
	turn := OrcaNativeTurn{TurnID: strings.Repeat("1", 32), TaskID: "task_first", DispatchID: "ctx_first"}
	for _, tc := range []struct {
		name, payload, option, note, want string
	}{
		{"free_text", `{"taskId":"task_first","dispatchId":"ctx_first","question":"Which approach?","options":[]}`, "", "Use the direct path", "Use the direct path"},
		{"option_and_note", `{"taskId":"task_first","dispatchId":"ctx_first","question":"Which approach?","options":[{"id":"direct","label":"Direct path","detail":"Use Orca"}]}`, "direct", "Keep this session", "Direct path\n\nKeep this session"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded, _ := json.Marshal(tc.payload) // Real Orca inbox payload is a JSON string.
			message := OrcaInboxMessage{ID: "msg_question", FromHandle: "dispatch:ctx_first", Type: "question", Body: "Which approach?", Payload: encoded}
			if !nativeOrcaMessageMatches(message, turn, "term_agent") {
				t.Fatal("the original Dispatch question was not recognized")
			}
			run := &Run{AttemptID: strings.Repeat("2", 32)}
			decision, err := WriteDecisionRequest(run, nativeOrcaQuestion(message, "run_original", turn))
			if err != nil {
				t.Fatal(err)
			}
			answer := DecisionAnswer{AnsweredBy: "operator", OptionID: tc.option, Note: tc.note}
			if err := normalizeNativeDecisionAnswer(&decision.Request, &answer); err != nil {
				t.Fatal(err)
			}
			if got := nativeOrcaAnswerText(answer, decision.Request); got != tc.want {
				t.Fatalf("Orca reply body = %q, want %q", got, tc.want)
			}
			if len(answer.Answers) != 1 || len(answer.Answers["msg_question"]) == 0 {
				t.Fatalf("shared native answer lost the original message ID: %+v", answer.Answers)
			}
		})
	}
}

func TestNativeOrcaRejectsLateDispatchMessages(t *testing.T) {
	turn := OrcaNativeTurn{TaskID: "task_new", DispatchID: "ctx_new"}
	payload, _ := json.Marshal(`{"taskId":"task_old","dispatchId":"ctx_old","outcome":"succeeded"}`)
	message := OrcaInboxMessage{ID: "msg_old", FromHandle: "term_agent", Type: "worker_done", Payload: payload}
	if nativeOrcaMessageMatches(message, turn, "term_agent") {
		t.Fatal("an old worker_done was attributed to the new Dispatch")
	}
	message.Payload, _ = json.Marshal(`{"taskId":"task_new","dispatchId":"ctx_new","outcome":"failed"}`)
	if !nativeOrcaMessageMatches(message, turn, "term_agent") {
		t.Fatal("current worker_done was not recognized")
	}
	if outcome, err := nativeOrcaDoneOutcome(message, turn); err != nil || outcome != "failed" {
		t.Fatalf("outcome = %q / %v", outcome, err)
	}
}

func TestNativeOrcaAgentSelectionUsesFrozenProfile(t *testing.T) {
	base := Config{Host: "orca", Agent: "codex", Sandbox: "workspace-write"}
	for _, agent := range []string{"codex", "opencode"} {
		profile := ExecutionProfile{ID: "orca-" + agent, Name: "Orca " + agent, Host: "orca", Agent: agent, Sandbox: "workspace-write", Orca: OrcaConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "2m"}}
		base.ExecutionProfiles = []ExecutionProfile{profile}
		if err := validateExecutionProfiles(base); err != nil {
			t.Fatal(err)
		}
		frozen := profile.Freeze()
		if !frozen.VerifyWorkerFlow() {
			t.Fatal("profile digest changed while freezing")
		}
		selected := base.ApplyFixedExecutionProfile(frozen)
		if selected.agentName() != agent || selected.needsCodexExecutable() {
			t.Fatalf("Orca agent selection was tied to Codex: %+v", selected)
		}
	}
}
