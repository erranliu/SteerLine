package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSandboxScenarioRequiresBothOriginalMergedTasks(t *testing.T) {
	var candidates []any
	for i, task := range []string{"1", "2"} {
		var usage []any
		for _, turn := range []string{"implement", "report", "publish"} {
			usage = append(usage, map[string]any{"session_id": task, "turn_id": turn, "model": "gpt-6-luna", "reasoning_effort": "medium", "tokens": map[string]int{"total_tokens": 100}})
		}
		candidates = append(candidates, map[string]any{"task_id": task, "run_id": task, "session_id": task, "worktree": task, "native_stages": 3, "usage": usage, "merged_pr": map[string]any{"url": "https://github.com/erranliu/SteerlineTest/pull/" + []string{"215", "216"}[i], "state": "MERGED", "mergedAt": "2026-10-12T03:00:00Z", "mergeCommit": map[string]string{"oid": strings.Repeat(task, 40)}, "baseRefName": "codex/debug/owned/base"}})
	}
	good := map[string]any{"passed": true, "cleanup_complete": true, "usage_verified": true, "manual_confirmation": false, "deliveries": 2, "candidates": candidates}
	raw, err := json.Marshal(good)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"complete", func(map[string]any) {}},
		{"one_delivery", func(p map[string]any) { p["deliveries"] = 1 }},
		{"cleanup_failed", func(p map[string]any) { p["cleanup_complete"] = false }},
		{"manual_answer", func(p map[string]any) { p["manual_confirmation"] = true }},
		{"reused_session", func(p map[string]any) { p["candidates"].([]any)[1].(map[string]any)["session_id"] = "1" }},
		{"unmerged", func(p map[string]any) {
			p["candidates"].([]any)[1].(map[string]any)["merged_pr"].(map[string]any)["state"] = "OPEN"
		}},
		{"missing_merge_commit", func(p map[string]any) {
			p["candidates"].([]any)[1].(map[string]any)["merged_pr"].(map[string]any)["mergeCommit"] = map[string]string{"oid": ""}
		}},
		{"malformed_merge_time", func(p map[string]any) {
			p["candidates"].([]any)[1].(map[string]any)["merged_pr"].(map[string]any)["mergedAt"] = "not-a-time"
		}},
		{"zero_merge_time", func(p map[string]any) {
			p["candidates"].([]any)[1].(map[string]any)["merged_pr"].(map[string]any)["mergedAt"] = "0001-01-01T00:00:00Z"
		}},
		{"foreign_usage", func(p map[string]any) {
			p["candidates"].([]any)[1].(map[string]any)["usage"].([]any)[0].(map[string]any)["session_id"] = "1"
		}},
		{"zero_usage", func(p map[string]any) {
			p["candidates"].([]any)[1].(map[string]any)["usage"].([]any)[0].(map[string]any)["tokens"].(map[string]any)["total_tokens"] = 0
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(raw, &value); err != nil {
				t.Fatal(err)
			}
			tc.change(value)
			changed, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var proof sandboxScenarioProof
			if err := json.Unmarshal(changed, &proof); err != nil {
				t.Fatal(err)
			}
			if err := validateSandboxScenarioProof(&proof); (err == nil) != (tc.name == "complete") {
				t.Fatalf("proof validation: %v", err)
			}
		})
	}
	if validateSandboxScenarioProof(nil) == nil {
		t.Fatal("missing sandbox evidence accepted")
	}
}
