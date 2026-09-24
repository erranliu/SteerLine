package steerline

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func importActivityJournal(tx DomainTx, project string, run *Run) error {
	if run == nil || run.LogPath == nil {
		return nil
	}
	path := activityPath(*run.LogPath)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		return fmt.Errorf("activity journal %s has an incomplete final record", path)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	taskID := run.DomainTaskID
	if taskID == "" {
		taskID = legacyTaskID(project, run)
	}
	attemptID := run.AttemptID
	if attemptID == "" {
		attemptID = run.RunID
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		event, err := DecodeActivityLine([]byte(line))
		if err != nil {
			return fmt.Errorf("invalid activity journal %s line %d: %w", path, i+1, err)
		}
		received := event.TS
		if received.IsZero() {
			received = now()
		}
		executionID := run.ExecutionID
		if executionID == "" {
			executionID = legacyExecutionID(run.RunID)
		}
		if value, ok := event.Meta["attempt_id"].(string); ok && value != "" {
			attemptID = value
		}
		_, err = tx.AppendEvent(DomainEvent{ProjectIdentity: project, TaskID: taskID, WorkflowRunID: run.RunID, ExecutionID: executionID, AttemptID: attemptID, ExternalSource: "activity:" + run.RunID, ExternalID: strconv.FormatUint(event.Seq, 10), Type: "activity." + event.Kind, ReceivedAt: received.UTC(), Payload: []byte(line)})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) ReconcileActivity(ctx context.Context, _ string, runtime Runtime) error {
	return s.Transact(ctx, func(tx DomainTx) error {
		for _, run := range runtime.Runs {
			if err := importActivityJournal(tx, runtime.Repository.Identity, run); err != nil {
				return err
			}
		}
		return nil
	})
}
