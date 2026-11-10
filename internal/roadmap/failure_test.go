package roadmap

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type failureHistoryRunner struct {
	*roadmapRunner
	change func(map[string]any)
	pages  int
}

func (f *failureHistoryRunner) Call(ctx context.Context, command string, args []string, request *runnercontract.Request) (runnerclient.Response, error) {
	if command != "history" {
		return f.roadmapRunner.Call(ctx, command, args, request)
	}
	f.pages++
	question := map[string]any{"role": "question", "command_id": "docs", "session_id": "session", "turn_id": "turn", "question": map[string]any{"native_questions": []any{map[string]any{"text": "测试 N201-CONTENT 尚未登记，无法满足当前约束"}}}}
	if f.change != nil {
		f.change(question)
	}
	entries := []any{}
	if f.pages == 2 {
		entries = append(entries, question)
	} else {
		entries = append(entries, map[string]any{"role": "user"})
	}
	raw, _ := json.Marshal(map[string]any{"run_id": f.run.RunID, "run": f.run, "entries": entries, "has_more": f.pages == 1, "next_cursor": f.run.RunID + ":" + strings.Repeat("x", f.pages)})
	return runnerclient.Response{Data: raw}, nil
}

func TestRoadmapFailedContentRetainsOriginalQuestionAndDiagnostic(t *testing.T) {
	for _, wrongTurn := range []bool{false, true} {
		t.Run(map[bool]string{false: "original", true: "wrong-turn"}[wrongTurn], func(t *testing.T) {
			s, store, runner, input := generationSetup(t)
			f := &failureHistoryRunner{roadmapRunner: runner}
			if wrongTurn {
				f.change = func(q map[string]any) { q["turn_id"] = "other" }
			}
			s.Runner = f
			r, err := s.Parse(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			f.run.Execution = runnercontract.Execution{AttemptID: "attempt", SessionID: "session", TurnID: "turn"}
			f.run.ContentResult = &runnercontract.ContentResult{Status: "failed", Diagnostic: "human_input_required", Source: runnercontract.Final{CommandID: "docs", AttemptID: "attempt", SessionID: "session", TurnID: "turn"}}
			f.run.Status, f.run.StopStatus = "failed", "stopped"
			done, err := s.Observe(context.Background(), r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if done.State != "failed" || !strings.Contains(done.Diagnostic, "human_input_required") || strings.Contains(done.Diagnostic, "N201-CONTENT") == wrongTurn || f.pages != 2 || f.submits != 1 {
				t.Fatal(done.Diagnostic, err, f.pages)
			}
			stored, _ := store.Roadmap(context.Background(), s.Project, r.ID)
			if stored.Diagnostic != done.Diagnostic {
				t.Fatal("failure detail lost during persistence")
			}
		})
	}
}

func TestRoadmapFailedContentRetainsNativeFailure(t *testing.T) {
	s, _, f, input := generationSetup(t)
	r, err := s.Parse(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	f.run.ContentResult = &runnercontract.ContentResult{Status: "failed", Diagnostic: "schema_validation_failed", Source: runnercontract.Final{Diagnostic: "required property tests missing"}}
	f.run.Status, f.run.StopStatus = "failed", "stopped"
	done, err := s.Observe(context.Background(), r.ID)
	if err != nil || !strings.Contains(done.Diagnostic, "schema_validation_failed") || !strings.Contains(done.Diagnostic, "required property tests missing") {
		t.Fatal(done.Diagnostic, err)
	}
}

func TestRoadmapTerminalWithoutContentRetainsRunAndFinalFailure(t *testing.T) {
	s, _, f, input := generationSetup(t)
	r, err := s.Parse(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	f.run.Status, f.run.StopStatus = "failed", "stopped"
	f.run.Diagnostic = "material_snapshot_failed"
	f.run.Final = &runnercontract.Final{Diagnostic: "read source.md: access denied"}
	done, err := s.Observe(context.Background(), r.ID)
	if err != nil || !strings.Contains(done.Diagnostic, "material_snapshot_failed") || !strings.Contains(done.Diagnostic, "source.md: access denied") {
		t.Fatal(done.Diagnostic, err)
	}
}
