package orcaworker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkerStageNativeFinalProvenance(t *testing.T) {
	for _, scenario := range []string{"valid", "commentary", "tool", "wrong_task", "task_prefix", "subagent", "not_stopped"} {
		t.Run(scenario, func(t *testing.T) {
			cwd := t.TempDir()
			task := "task-2"
			role, phase := "assistant", "final_answer"
			if scenario == "wrong_task" {
				task = "task-1"
			}
			if scenario == "task_prefix" {
				task = "task-20"
			}
			if scenario == "commentary" {
				phase = "commentary"
			}
			if scenario == "tool" {
				role = "tool"
			}
			meta := map[string]any{"id": "original-session", "cwd": cwd}
			if scenario == "subagent" {
				meta["source"] = map[string]any{"subagent": map[string]string{"parent_thread_id": "parent"}}
			}
			events := []any{map[string]any{"type": "session_meta", "payload": meta}, map[string]any{"type": "event_msg", "payload": map[string]string{"type": "task_started", "turn_id": "turn"}}, map[string]any{"type": "response_item", "payload": map[string]any{"type": "message", "role": "user", "content": []any{map[string]string{"type": "input_text", "text": "Your task ID is: " + task}}}}, map[string]any{"type": "response_item", "payload": map[string]any{"type": "message", "role": role, "phase": phase, "content": []any{map[string]string{"type": "output_text", "text": `{"outcome":"done"}`}}}}}
			if scenario != "not_stopped" {
				events = append(events, map[string]any{"type": "event_msg", "payload": map[string]string{"type": "task_complete", "turn_id": "turn"}})
			}
			var raw strings.Builder
			for _, e := range events {
				b, _ := json.Marshal(e)
				raw.Write(b)
				raw.WriteByte('\n')
			}
			session, turn, final, item, err := codexTranscriptFinal([]byte(raw.String()), cwd, "task-2")
			if scenario == "valid" {
				if err != nil || session != "original-session" || turn != "turn" || final != `{"outcome":"done"}` || item == "" {
					t.Fatal("valid native final lost its binding", err)
				}
			} else if err == nil {
				t.Fatal("noncanonical source became final")
			}
		})
	}
}
