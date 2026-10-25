package runnercontract_test

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	protocol "github.com/erranliu/SteerLine/internal/testkit/protocol"
	"testing"
	"time"
)

func TestExecutionWaitContract(t *testing.T) {
	base := protocol.RunnerExecutionWait(time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC))
	for _, mode := range []string{"queue", "calling", "backoff", "exhausted", "missing", "null-count", "unknown-field", "wrong-reason", "long-window", "late-retry", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			w := base.Clone()
			switch mode {
			case "calling":
				w.Stage, w.ReasonCode, w.CallsStarted = mode, "orca_query_running", 1
			case "backoff", "late-retry":
				w.Stage, w.ReasonCode, w.CallsStarted = "backoff", "orca_query_transient", 1
				at := w.StartedAt.Add(time.Second)
				if mode == "late-retry" {
					at = w.DeadlineAt
				}
				w.NextAttemptAt = &at
			case "exhausted":
				w.Stage, w.ReasonCode, w.CallsStarted = mode, "query_attempts_exhausted", 5
			case "wrong-reason":
				w.ReasonCode = "operation_outcome_unknown"
			case "long-window":
				w.DeadlineAt = w.StartedAt.Add(91 * time.Second)
			}
			raw, _ := json.Marshal(w)
			var fields map[string]any
			_ = json.Unmarshal(raw, &fields)
			switch mode {
			case "missing":
				delete(fields, "next_attempt_at")
			case "null-count":
				fields["calls_started"] = nil
			case "unknown-field":
				fields["retryable"] = true
			}
			raw, _ = json.Marshal(fields)
			if mode == "duplicate" {
				raw = append([]byte(`{"calls_started":1,`), raw[1:]...)
			}
			var got runnercontract.ExecutionWait
			err := json.Unmarshal(raw, &got)
			valid := mode == "queue" || mode == "calling" || mode == "backoff" || mode == "exhausted"
			if (err == nil) != valid {
				t.Fatalf("%s: %v", mode, err)
			}
		})
	}
	for _, raw := range []json.RawMessage{nil, []byte("null")} {
		e := runnercontract.Event{Type: "execution_wait_changed", ExecutionWait: raw}
		_, err := e.Wait()
		if (err == nil) != (raw != nil) {
			t.Fatal("absent and cleared event confused", err)
		}
	}
}
