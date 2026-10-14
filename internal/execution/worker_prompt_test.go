package execution

import (
	"encoding/json"
	workflow "github.com/erranliu/SteerLine/internal/execution/workflow"
	"strings"
	"testing"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
	m "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestWorkerPromptUsesOnlyStepTemplateAndBusinessFacts(t *testing.T) {
	task := json.RawMessage(`{"task_id":"84","spec":{"path":"bound.spec.md"},"plan":{"path":"bound.plan.md"}}`)
	w := &m.WorkflowRunRecord{StageInputs: []m.StageInput{{CommandID: "execute", StepID: m.StepImplement, TaskFacts: task, Prompt: "old adapter instructions must not be replayed"}}, StageResults: []m.StageResult{{CommandID: "execute", TurnID: "original-turn", StepID: m.StepImplement, Succeeded: true, FinalVerified: true, FinalSource: "fixture", FinalItemID: "item", Final: "checks completed"}}}
	for _, step := range []string{m.StepImplement, m.StepReport, m.StepPublish, m.StepPublishReport} {
		input := m.StageInput{StepID: step, TaskFacts: task, SourceCommandID: "execute", SourceTurnID: "original-turn", ReportToolPolicy: "codex_native_permissions"}
		if step == m.StepPublishReport {
			w.StageInputs = append(w.StageInputs, m.StageInput{CommandID: "publish", StepID: m.StepPublish})
			w.StageResults = append(w.StageResults, m.StageResult{CommandID: "publish", StepID: m.StepPublish, TurnID: "publication-turn", Succeeded: true, FinalVerified: true, FinalSource: "fixture", FinalItemID: "item", Final: "Published PR"})
			input.SourceCommandID, input.SourceTurnID = "publish", "publication-turn"
		}
		first, err := workflow.RenderInput(workflowSnapshot(w), workflowInput(input))
		require(t, err)
		input.CommandID, input.ReportToolPolicy = "different-command", "orca_managed_permissions"
		second, err := workflow.RenderInput(workflowSnapshot(w), workflowInput(input))
		require(t, err)
		for _, unwanted := range []string{"task_id", "Current step:", "snapshot", "source_execution", "checks completed", "Published PR"} {
			if strings.Contains(first, unwanted) {
				t.Fatalf("%s repeats %q: %s", step, unwanted, first)
			}
		}
		if first != second || (strings.Contains(first, "bound.spec.md") != (step == m.StepImplement)) || strings.Contains(first, "old adapter instructions") || strings.Contains(first, "original-turn") || strings.Contains(first, "native_permissions") || strings.Contains(first, "managed_permissions") {
			t.Fatalf("%s changed its business prompt with transport identities or policy", step)
		}
		if (step != m.StepImplement) != strings.Contains(first, workflow.ResultContract) {
			t.Fatalf("%s carries the wrong result contract", step)
		}
		if m.PublicationStep(step) != strings.Contains(first, `For done add outputs: {"pr_url":"actual PR URL"}.`) {
			t.Fatalf("%s carries the wrong publication output contract", step)
		}
		if step == m.StepPublish {
			for _, instruction := range []string{"git branch --show-current", "same name for the remote branch and PR head", "Do not reconstruct the branch name"} {
				if !strings.Contains(first, instruction) {
					t.Fatalf("publication prompt missing %q: %s", instruction, first)
				}
			}
		}
	}
}

func TestWorkerPromptReportAddsOnlyMissingEvidenceAndRetrySource(t *testing.T) {
	w := &m.WorkflowRunRecord{
		StageInputs:  []m.StageInput{{CommandID: "execute", StepID: m.StepImplement}},
		StageResults: []m.StageResult{{CommandID: "execute", TurnID: "source-turn", StepID: m.StepImplement, Failure: "compiler failed"}},
	}
	input := m.StageInput{StepID: m.StepReport, SourceCommandID: "execute", SourceTurnID: "source-turn"}
	prompt, err := workflow.RenderInput(workflowSnapshot(w), workflowInput(input))
	require(t, err)
	if !strings.Contains(prompt, "compiler failed") || !strings.Contains(prompt, "evidence_gap") || strings.Contains(prompt, "snapshot") {
		t.Fatalf("missing compact diagnostics: %s", prompt)
	}
	w.StageResults[0].Succeeded = true
	w.StageResults[0].Failure = ""
	w.StageResults[0].FinalVerified = true
	w.StageResults[0].FinalSource, w.StageResults[0].FinalItemID = "fixture", "item"
	w.StageResults[0].Final = "original implementation result"
	w.StageInputs = append(w.StageInputs, input)
	prompt, err = workflow.RenderInput(workflowSnapshot(w), workflowInput(input))
	require(t, err)
	if !strings.Contains(prompt, "original implementation result") || strings.Contains(prompt, "evidence_gap") {
		t.Fatalf("retry lost its original result: %s", prompt)
	}
}

func TestWorkerPromptPublicationTargetOnlyForPublication(t *testing.T) {
	c := Config{Remote: "delivery", Branch: "release", PublicationPolicy: b.PublicationPolicy{Publisher: "worker"}}
	for _, step := range []string{m.StepImplement, m.StepReport, m.StepPublish, m.StepPublishReport} {
		prompt := stagePublicationInstruction(step, c)
		if !strings.Contains(prompt, `"publication_policy":{"publisher":"worker"}`) {
			t.Fatal("fixed publisher missing from model step")
		}
		if (step == m.StepPublish) != strings.Contains(prompt, "PR target:") {
			t.Fatalf("unexpected target on %s: %s", step, prompt)
		}
		if step == m.StepPublish && (!strings.Contains(prompt, "delivery") || !strings.Contains(prompt, "release")) {
			t.Fatal("publication target missing")
		}
	}
}
