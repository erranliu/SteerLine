package execution

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
	m "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestWorkerPromptUsesOnlyStepTemplateAndBusinessFacts(t *testing.T) {
	task := json.RawMessage(`{"task_id":"84","spec":{"path":"bound.spec.md"},"plan":{"path":"bound.plan.md"}}`)
	snapshot := &m.StageSnapshot{Workspace: b.WorkspaceInspection{Head: strings.Repeat("a", 40), ContentDigest: "fixed"}}
	w := &m.WorkflowRunRecord{StageInputs: []m.StageInput{{CommandID: "execute", StepID: m.StepImplement, TaskFacts: task, Prompt: "old adapter instructions must not be replayed"}}, StageResults: []m.StageResult{{CommandID: "execute", TurnID: "original-turn", StepID: m.StepImplement, Succeeded: true, Snapshot: snapshot, Final: "checks completed"}}}
	for _, step := range []string{m.StepImplement, m.StepReport, m.StepPublish, m.StepPublishReport} {
		input := m.StageInput{StepID: step, TaskFacts: task, SourceCommandID: "execute", SourceTurnID: "original-turn", Snapshot: snapshot, CandidateSHA: snapshot.Workspace.Head, ReportToolPolicy: "codex_native_permissions"}
		if step == m.StepPublishReport {
			w.StageInputs = append(w.StageInputs, m.StageInput{CommandID: "publish", StepID: m.StepPublish, CandidateSHA: snapshot.Workspace.Head})
			w.StageResults = append(w.StageResults, m.StageResult{CommandID: "publish", StepID: m.StepPublish, TurnID: "publication-turn", Snapshot: snapshot, Final: "Published PR"})
			input.SourceCommandID, input.SourceTurnID = "publish", "publication-turn"
		}
		first, err := renderStageInput(w, input)
		require(t, err)
		input.CommandID, input.ReportToolPolicy = "different-command", "orca_managed_permissions"
		second, err := renderStageInput(w, input)
		require(t, err)
		if first != second || !strings.Contains(first, "bound.spec.md") || strings.Contains(first, "old adapter instructions") || strings.Contains(first, "original-turn") || strings.Contains(first, "native_permissions") || strings.Contains(first, "managed_permissions") {
			t.Fatalf("%s changed its business prompt with transport identities or policy", step)
		}
		if step == m.StepImplement && strings.Contains(first, stageResultContract) || step != m.StepImplement && strings.Count(first, stageResultContract) != 1 {
			t.Fatalf("%s carries the wrong result contract", step)
		}
	}
	marked := time.Now()
	recovery := resumeFacts(&m.ExecutionRecord{RecoveryMarkedAt: &marked, RecoveryState: "completed", LastTurnOutcome: "succeeded"})
	raw, err := json.Marshal(recovery)
	require(t, err)
	if !strings.Contains(string(raw), `"previous_outcome":"succeeded"`) || resumeFacts(&m.ExecutionRecord{}) != nil {
		t.Fatal("recovery context is not expressed as optional facts")
	}
}
