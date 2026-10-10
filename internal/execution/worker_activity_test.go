package execution

import (
	"context"
	"encoding/json"
	"testing"

	m "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/product/query"
)

func TestWorkerStateActivityCommitAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[fail], func(t *testing.T) {
			s, run := waitingDecisionService(t)
			t.Cleanup(func() { _ = s.Close() })
			ctx := context.Background()
			read := func() int {
				events, err := testSQLite(s).Events(ctx, s.Config.identity, run.ExecutionID, run.AttemptID, 0, 1000, query.MaxEventPageBytes)
				require(t, err)
				count := 0
				for _, event := range events {
					if event.Type == "activity.worker_state" {
						count++
					}
				}
				return count
			}
			before := read()
			w := s.workflowRecord(run)
			old := w.Status
			w.StepID, w.Status = m.StepReport, "blocked"
			if fail {
				_, err := testSQLite(s).db.ExecContext(ctx, `CREATE TRIGGER fail_state_checkpoint BEFORE UPDATE ON projection BEGIN SELECT RAISE(ABORT,'state checkpoint unavailable'); END`)
				require(t, err)
			}
			err := s.commitWorker(run)
			if fail {
				if err == nil || read() != before || s.workflowRecord(run).Status != old {
					t.Fatal("failed state commit published a transition")
				}
				for _, event := range s.pendingEvents {
					if event.Type == "activity.worker_state" {
						t.Fatal("failed derived transition remained queued")
					}
				}
				return
			}
			require(t, err)
			if read() != before+1 {
				t.Fatal("committed transition missing")
			}
			require(t, s.commitWorker(run))
			if read() != before+1 {
				t.Fatal("unchanged commit duplicated transition")
			}
			batch, err := (query.NewService(testSQLite(s), s.Config.identity)).ReadRecentActivity(ctx, run.RunID, run.ExecutionID, run.AttemptID)
			require(t, err)
			found := false
			for _, event := range batch.Activity {
				if event.Kind == "worker_state" && event.Status == "blocked" {
					raw, err := json.Marshal(event.Meta["to"])
					require(t, err)
					var fields map[string]json.RawMessage
					require(t, json.Unmarshal(raw, &fields))
					if string(fields["workflow_stage"]) != `"implement.report"` || string(fields["workflow_status"]) != `"blocked"` || fields["step_id"] != nil || fields["status"] != nil {
						t.Fatalf("durable query exposed obsolete workflow fields: %s", raw)
					}
					found = true
				}
			}
			if !found {
				t.Fatal("durable change was not exposed through public activity query")
			}
		})
	}
}
