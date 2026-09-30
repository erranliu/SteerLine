package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type WorkerOutput struct {
	DispatchID      string   `json:"dispatchId"`
	Source          string   `json:"source"`
	Cursor          string   `json:"cursor"`
	NextCursor      string   `json:"nextCursor"`
	SourceChanged   bool     `json:"sourceChanged"`
	ContentComplete bool     `json:"contentComplete"`
	Clipping        []string `json:"clipping"`
	FallbackReason  string   `json:"fallbackReason"`
	Terminal        struct {
		Tail []string `json:"tail"`
	} `json:"terminal"`
	Transcript json.RawMessage `json:"transcript"`
}

// ReadWorkerOutput retries once without the old cursor only when Orca reports
// a changed source. resetCursor remains true even if the retry fails.
func ReadWorkerOutput(ctx context.Context, call MetaCall, dispatchID, cursor, runtimeID string) (output WorkerOutput, resetCursor bool, err error) {
	args := []string{"orchestration", "worker-read", "--dispatch", dispatchID, "--source", "auto", "--limit", "100", "--json"}
	if cursor != "" {
		args = append(args, "--cursor", cursor)
	}
	envelope, err := call(ctx, args...)
	if err != nil && strings.Contains(err.Error(), "source_changed") {
		resetCursor = true
		envelope, err = call(ctx, "orchestration", "worker-read", "--dispatch", dispatchID, "--source", "auto", "--limit", "100", "--json")
	}
	if err != nil {
		return WorkerOutput{}, resetCursor, err
	}
	if err := json.Unmarshal(envelope.Payload, &output); err != nil || output.DispatchID != dispatchID || output.Source == "" || envelope.RuntimeID != runtimeID {
		return WorkerOutput{}, resetCursor, errors.New("Orca worker-read changed source identity")
	}
	return output, resetCursor, nil
}

func VerifySettledDispatch(ctx context.Context, call MetaCall, expected DispatchIdentity) (string, error) {
	envelope, err := call(ctx, "orchestration", "worker-show", "--dispatch", expected.DispatchID, "--json")
	if err != nil {
		return "", &CallError{Cause: err, Result: envelope}
	}
	var result struct {
		Dispatch struct {
			ID     string `json:"id"`
			RunID  string `json:"runId"`
			TaskID string `json:"taskId"`
			Status string `json:"status"`
		} `json:"dispatch"`
		Worker struct {
			DispatchID          string `json:"dispatchId"`
			State               string `json:"state"`
			AgentTerminalHandle string `json:"agentTerminalHandle"`
		} `json:"worker"`
		Terminal struct {
			Handle        string `json:"handle"`
			IncarnationID string `json:"incarnationId"`
			WorktreeID    string `json:"worktreeId"`
		} `json:"terminal"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil || envelope.RuntimeID != expected.RuntimeID || result.Dispatch.ID != expected.DispatchID || result.Dispatch.TaskID != expected.TaskID || result.Dispatch.RunID != expected.RunID || result.Worker.DispatchID != expected.DispatchID || result.Worker.AgentTerminalHandle != expected.Terminal || result.Terminal.Handle != expected.Terminal || result.Terminal.IncarnationID != expected.Incarnation || result.Terminal.WorktreeID != expected.WorktreeID {
		return "", errors.New("Orca settled Dispatch identity differs from the fixed worker")
	}
	return result.Dispatch.Status, nil
}
