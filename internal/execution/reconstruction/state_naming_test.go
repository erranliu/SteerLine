package reconstruction

import "testing"

func TestStateNamingTerminalInputDoesNotOccupySlot(t *testing.T) {
	for _, status := range []string{"in_progress", "closed", "discarded"} {
		run := &RunView{TaskID: "task", TaskStatus: status, WorkflowStatus: "waiting_input", ExecutionStatus: "succeeded"}
		occupied, _ := Slot(Runtime{Runs: []*RunView{run}})
		if occupied != (status == "in_progress") {
			t.Fatalf("terminal workflow input occupied slot: %+v", run)
		}
		run.ExecutionStatus = "unknown"
		if occupied, _ := Slot(Runtime{Runs: []*RunView{run}}); !occupied {
			t.Fatal("terminal task suppressed unresolved execution facts")
		}
	}
}
