package workflow

import (
	"github.com/erranliu/SteerLine/internal/execution/reliable"
	"strings"
	"testing"
)

func TestWorkflowExecutionFacts(t *testing.T) {
	w := Snapshot{}
	input := StageInput{StepID: StepImplement, CommandID: "implementation"}
	result := reliable.Result{Succeeded: true, TurnID: "source"}
	next := Route(w, input, result, "")
	if next.Next == nil || next.Next.StepID != StepReport || next.Next.SourceTurnID != "source" {
		t.Fatal(next)
	}
	result.Succeeded = false
	next = Route(w, input, result, "")
	if !next.RecoverOriginal || next.Next != nil {
		t.Fatal("failed execution automatically created business input")
	}
	input.StepID = StepPublish
	result.Succeeded = true
	result.ContentVerified = false
	result.Content = `{"outcome":"done","summary":"published","outputs":{"pr_url":"https://example.test/1"}}`
	next = Route(w, input, result, "")
	if next.Status != "reconciling" || next.AcceptResult || next.Next != nil {
		t.Fatal("missing content evidence advanced workflow")
	}
}
func TestWorkflowSourceAndFormatRules(t *testing.T) {
	w := Snapshot{StageResults: []StageResult{{CommandID: "execute", TurnID: "source", StepID: StepImplement, Succeeded: true}}}
	input := StageInput{StepID: StepReport, CommandID: "report", SourceCommandID: "execute", SourceTurnID: "source"}
	result := reliable.Result{Succeeded: true, ContentVerified: true, Content: "bad json"}
	next := Route(w, input, result, "")
	if next.Next == nil || next.ReportRetry != 1 || next.Next.SourceCommandID != "execute" {
		t.Fatal("format correction lost original source")
	}
	w.ReportRetry = 1
	next = Route(w, input, result, "")
	if next.Status != "blocked" || next.Next != nil {
		t.Fatal("format correction was unbounded")
	}
	result.Content = `{"outcome":"done","summary":"done"}`
	next = Route(w, input, result, "")
	if next.Next == nil || next.Next.StepID != StepPublish {
		t.Fatal(next)
	}
	w.StageResults[0].Succeeded = false
	next = Route(w, input, result, "")
	if next.Status != "blocked" || next.Next != nil {
		t.Fatal("report upgraded failed source")
	}
	w.PRURL = "https://example.test/pr/1"
	prompt, err := RenderInput(w, StageInput{StepID: StepPublish})
	if err != nil || !strings.Contains(prompt, "update this same PR") {
		t.Fatal("PR reuse lost")
	}
}
