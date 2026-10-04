package task

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/task/model"
)

type replayStore struct {
	CommandStore
	command Command
}

func (r replayStore) LookupCommand(context.Context, string, string) (Command, bool, error) {
	return r.command, true, nil
}

// Retains strict decoding and replay assertions from native_dag_commands_test.
// The embedded missing ports deliberately fail if replay starts document I/O or
// enters a write transaction after the original request was already accepted.
func TestTM03TaskBatch(t *testing.T) {
	t.Run("strict_decode", func(t *testing.T) {
		for _, raw := range []string{
			`{"schema_version":"task-batch/v1","project_id":"p","tasks":[],"unknown":true}`,
			`{"schema_version":"task-batch/v1","project_id":"p","tasks":[{"local_key":"a","title":"x","type":"task","contract":{}}]}`,
			`{"schema_version":"task-batch/v1","project_id":"p","tasks":[{"local_key":"a","task_id":1.5,"title":"x","type":"task","contract":{}}]}`,
			`{"schema_version":"task-batch/v1","project_id":"p","tasks":[],"tasks":[]}`,
		} {
			if _, _, err := model.DecodeBatch([]byte(raw)); err == nil {
				t.Fatalf("invalid batch accepted: %s", raw)
			}
		}
	})
	t.Run("original_receipt_without_documents_or_writes", func(t *testing.T) {
		payload := []byte(`{"schema_version":"task-batch/v1","project_id":"p","tasks":[{"local_key":"k20","task_id":20,"title":"Task 20","type":"task","contract":{}}]}`)
		_, canonical, err := model.DecodeBatch(payload)
		if err != nil {
			t.Fatal(err)
		}
		request := strings.Repeat("1", 32)
		receipt := model.TaskBatchReceipt{RequestID: request, BatchID: "original", GraphRevision: 1, Status: "accepted"}
		result, err := json.Marshal(receipt)
		if err != nil {
			t.Fatal(err)
		}
		commands := Commands{Input: Input{Project: "p", Authority: "native"}, Store: replayStore{command: Command{ProjectIdentity: "p", CommandID: request, Kind: "create_task_batch", TargetID: "p", RequestedBy: "planner", Payload: canonical, Status: "accepted", Result: result}}}
		got, err := commands.CreateTaskBatch(context.Background(), payload, request, 100, "planner")
		if err != nil || got.BatchID != receipt.BatchID || got.GraphRevision != receipt.GraphRevision {
			t.Fatalf("same request did not return original receipt: %+v / %v", got, err)
		}
		changed := []byte(strings.Replace(string(payload), "Task 20", "different title", 1))
		if _, err = commands.CreateTaskBatch(context.Background(), changed, request, 1, "planner"); err == nil || !strings.Contains(err.Error(), "idempotency_conflict") {
			t.Fatalf("different content reused request ID: %v", err)
		}
	})
}
