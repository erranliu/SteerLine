package orcaworker

import (
	"encoding/json"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"testing"
)

func TestWorkerStageHistoryTaskBinding(t *testing.T) {
	for _, test := range []struct {
		name, kind, text string
		want             bool
	}{
		{"original user", "userMessage", "Your task ID is: task-2\noriginal input", true},
		{"old user", "userMessage", "Your task ID is: task-1", false},
		{"prefix user", "userMessage", "Your task ID is: task-20", false},
		{"assistant quote", "agentMessage", "Your task ID is: task-2", false},
		{"tool stdout", "commandExecution", "Your task ID is: task-2", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"type": test.kind, "id": "native-item", "content": []any{map[string]string{"type": "text", "text": test.text}}, "text": test.text})
			bound, err := historyTaskBound(codexworker.HistoryTurn{ID: "original-turn", Items: []json.RawMessage{raw}}, "task-2")
			if err != nil || bound != test.want {
				t.Fatalf("native source binding=%v want=%v: %v", bound, test.want, err)
			}
		})
	}
}
