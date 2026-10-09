package driver

import (
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
	"testing"
	"time"
)

func TestProjectEnvironmentRejectsInheritedRuntimeAndTestSwitches(t *testing.T) {
	got := strings.Join(Environment([]string{"Path=tools", "STEERLINE_HOME=production", "STEERLINE_REAL_CODEX=1", "STEERLINE_WORKER_FLOW_MOCK=1", "CODEX_HOME=real", "TEMP=old", "GOFLAGS=-tags=e2e"}, "isolated", "state"), "\n")
	for _, bad := range []string{"production", "REAL_CODEX", "WORKER_FLOW_MOCK", "CODEX_HOME=real", "TEMP=old", "-tags=e2e"} {
		if strings.Contains(got, bad) {
			t.Fatalf("inherited %s: %s", bad, got)
		}
	}
	for _, want := range []string{"Path=tools", "STEERLINE_HOME=isolated", "STEERLINE_E2E_CONTROLLED_STATE=state"} {
		if !strings.Contains(got, want) {
			t.Fatal(got)
		}
	}
}

func TestAcceptedStartupObservationNeverRetargetsRequest(t *testing.T) {
	request := productapi.ControlRequest{RequestID: "request", InstanceID: "original", Action: "start_task"}
	at := time.Now()
	receipt := productapi.ControlReceipt{RequestID: "request", InstanceID: "original", Action: "start_task", Status: "applying", AcceptedAt: &at, RunID: "run", AttemptID: "attempt"}
	if !mayObserveAcceptedStart(request, &receipt) {
		t.Fatal("durably accepted original startup cannot be observed")
	}
	for _, change := range []string{"request", "instance", "action", "unaccepted", "terminal", "run", "attempt"} {
		r := receipt
		switch change {
		case "request":
			r.RequestID = "other"
		case "instance":
			r.InstanceID = "other"
		case "action":
			r.Action = "resume_dispatch"
		case "unaccepted":
			r.AcceptedAt = nil
		case "terminal":
			r.Status = "failed"
		case "run":
			r.RunID = ""
		case "attempt":
			r.AttemptID = ""
		}
		if mayObserveAcceptedStart(request, &r) {
			t.Fatalf("extended observation accepted %s", change)
		}
	}
}
