package query

import (
	"strings"
	"testing"
	"time"

	refactoring "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestRefactoringUnsettledRunnerBlockIsVisible(t *testing.T) {
	for _, phase := range []string{"first", "select", "execute", "audit", "report"} {
		t.Run(phase, func(t *testing.T) {
			run := &runnercontract.Run{Status: "blocked", Diagnostic: "Reconciliation deadline exhausted",
				Operations: []runnercontract.Operation{{Diagnostic: "app-server connection refused", OriginalDiagnostic: "app-server connection refused"}}}
			r := refactoring.Record{Phase: phase, Authorized: true, Slot: true,
				Operations: []refactoring.Operation{{ID: "original-operation", Purpose: phase, State: "submitted", RunID: "original-run", Run: run}}}
			for _, view := range []map[string]any{RefactoringSummary(r, time.Now()), Refactoring(r, time.Now())} {
				if view["phase"] != "blocked" || view["active"] != true || view["slot"] != true ||
					view["can_stop"] != true || view["can_finish"] != false || view["finish_action"] != "" {
					t.Fatalf("lost block or original settlement duties: %+v", view)
				}
				if view["diagnostic"] != "Reconciliation deadline exhausted\napp-server connection refused" {
					t.Fatalf("lost or duplicated original diagnostic: %v", view["diagnostic"])
				}
			}
			op := Refactoring(r, time.Now())["operations"].([]map[string]any)[0]
			if op["state"] != "submitted" || op["run_status"] != "blocked" || op["run_id"] != "original-run" ||
				!strings.Contains(op["run_diagnostic"].(string), "app-server connection refused") || r.Phase != phase {
				t.Fatalf("rewrote operation or workflow instead of projecting the block: %+v", op)
			}
			r.StopRequested, r.Phase = true, "stopping"
			if view := RefactoringSummary(r, time.Now()); view["phase"] != "stopping" || view["can_stop"] != false {
				t.Fatalf("block masked a saved stop: %+v", view)
			}
		})
	}
}

func TestRefactoringOnlyCurrentUnsettledRunOverridesPhase(t *testing.T) {
	r := refactoring.Record{Phase: "first", Authorized: true, Slot: true,
		Operations: []refactoring.Operation{{State: "submitted", RunID: "original-run", Run: &runnercontract.Run{Status: "blocked"}}}}
	r.Operations[0].Run.StopStatus = "stopped"
	if r.DisplayPhase() != "first" || !r.CanFinish() {
		t.Fatal("settled Runner fact overrode the workflow")
	}
	r.Operations[0].Run.StopStatus = ""
	r.Operations[0].State = "complete"
	if r.DisplayPhase() != "first" {
		t.Fatal("completed historical operation overrode the workflow")
	}
	r.Operations = append(r.Operations, refactoring.Operation{State: "submitted", RunID: "next-run", Run: &runnercontract.Run{Status: "running"}})
	if r.DisplayPhase() != "first" {
		t.Fatal("historical blocked run overrode the current run")
	}
	r.Operations[1].Run.Status = "blocked"
	r.Slot = false
	view := RefactoringSummary(r, time.Now())
	if view["phase"] != "blocked" || view["diagnostic"] != "Runner 已阻断，原操作尚未结算" {
		t.Fatalf("queue masked a current block: %+v", view)
	}
}
