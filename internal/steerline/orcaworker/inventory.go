package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// Call provides only verified Orca result bytes; the caller owns host selection.
type Call func(context.Context, ...string) (json.RawMessage, error)

type Result struct {
	Payload   json.RawMessage
	RuntimeID string
}

type MetaCall func(context.Context, ...string) (Result, error)

var ErrDispatchIdentity = errors.New("recovered Orca Dispatch has no exact injected worker identity")

type DispatchIdentity struct {
	DispatchID  string
	TaskID      string
	RunID       string
	RuntimeID   string
	Terminal    string
	Incarnation string
	WorktreeID  string
}

func VerifyRecoveredDispatch(ctx context.Context, call MetaCall, expected DispatchIdentity) error {
	envelope, err := call(ctx, "orchestration", "worker-show", "--dispatch", expected.DispatchID, "--json")
	if err != nil {
		return err
	}
	var result struct {
		Dispatch struct {
			ID     string `json:"id"`
			TaskID string `json:"taskId"`
			RunID  string `json:"runId"`
			Status string `json:"status"`
		} `json:"dispatch"`
		Worker struct {
			DispatchID    string `json:"dispatchId"`
			Stage         string `json:"stage"`
			AgentTerminal string `json:"agentTerminalHandle"`
		} `json:"worker"`
		Terminal struct {
			Handle      string `json:"handle"`
			Incarnation string `json:"incarnationId"`
			WorktreeID  string `json:"worktreeId"`
		} `json:"terminal"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil {
		return err
	}
	if envelope.RuntimeID != expected.RuntimeID || result.Dispatch.ID != expected.DispatchID || result.Dispatch.TaskID != expected.TaskID || result.Dispatch.RunID != expected.RunID || result.Worker.DispatchID != expected.DispatchID || result.Worker.Stage != "injected" || result.Worker.AgentTerminal != expected.Terminal || result.Terminal.Handle != expected.Terminal || result.Terminal.Incarnation != expected.Incarnation || result.Terminal.WorktreeID != expected.WorktreeID {
		return ErrDispatchIdentity
	}
	return nil
}

func TerminalHasMarker(ctx context.Context, call Call, handle, marker string) (bool, error) {
	raw, err := call(ctx, "terminal", "read", "--terminal", handle, "--limit", "1000", "--json")
	if err != nil {
		return false, err
	}
	var result struct {
		Terminal struct {
			Handle string   `json:"handle"`
			Tail   []string `json:"tail"`
		} `json:"terminal"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || result.Terminal.Handle != handle {
		return false, errors.Join(err, errors.New("Orca terminal marker read changed handle"))
	}
	for _, line := range result.Terminal.Tail {
		if strings.Contains(line, marker) {
			return true, nil
		}
	}
	return false, nil
}

func FindRun(ctx context.Context, call Call, objective, coordinator string) (string, error) {
	cursor := ""
	found := ""
	for page := 0; page < 100; page++ {
		args := []string{"orchestration", "run-list", "--limit", "1000", "--json"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		raw, err := call(ctx, args...)
		if err != nil {
			return "", err
		}
		var listing struct {
			Runs []struct {
				ID          string `json:"id"`
				Objective   string `json:"objective"`
				Coordinator string `json:"coordinator_handle"`
			} `json:"runs"`
			NextCursor string `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &listing); err != nil {
			return "", err
		}
		for _, candidate := range listing.Runs {
			if candidate.Objective == objective && candidate.Coordinator == coordinator {
				if found != "" {
					return "", errors.New("more than one Orca Run has the fixed execution identity")
				}
				found = candidate.ID
			}
		}
		if listing.NextCursor == "" {
			return found, nil
		}
		if listing.NextCursor == cursor {
			return "", errors.New("Orca Run pagination did not advance")
		}
		cursor = listing.NextCursor
	}
	return "", errors.New("Orca Run inventory exceeded its recovery bound")
}

func FindTask(ctx context.Context, call Call, runID, coordinator, spec string) (string, error) {
	raw, err := call(ctx, "orchestration", "task-list", "--run", runID, "--json")
	if err != nil {
		return "", err
	}
	var listing struct {
		RunID string `json:"runId"`
		Tasks []struct {
			ID      string `json:"id"`
			RunID   string `json:"run_id"`
			Spec    string `json:"spec"`
			Creator string `json:"created_by_terminal_handle"`
		} `json:"tasks"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(raw, &listing); err != nil || listing.RunID != runID || listing.Count != len(listing.Tasks) {
		return "", errors.Join(err, errors.New("Orca Task inventory is incomplete or changed Run"))
	}
	found := ""
	for _, candidate := range listing.Tasks {
		if candidate.RunID == runID && candidate.Spec == spec && candidate.Creator == coordinator {
			if found != "" {
				return "", errors.New("more than one Orca Task matches the original turn spec")
			}
			found = candidate.ID
		}
	}
	return found, nil
}

func FindDispatch(ctx context.Context, call Call, taskID string) (string, string, string, string, error) {
	raw, err := call(ctx, "orchestration", "dispatch-show", "--task", taskID, "--json")
	if err != nil {
		return "", "", "", "", err
	}
	var result struct {
		Dispatch struct {
			ID          string `json:"id"`
			TaskID      string `json:"task_id"`
			RunID       string `json:"run_id"`
			Assignee    string `json:"assignee_handle"`
			Incarnation string `json:"process_incarnation"`
		} `json:"dispatch"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", "", "", "", err
	}
	if result.Dispatch.ID == "" || result.Dispatch.TaskID != taskID {
		return "", "", "", "", errors.New("Orca Dispatch is missing or changed Task")
	}
	return result.Dispatch.ID, result.Dispatch.RunID, result.Dispatch.Assignee, result.Dispatch.Incarnation, nil
}

func FindReply(ctx context.Context, call Call, runID, dispatchID, questionID, answer string) (string, error) {
	raw, err := call(ctx, "orchestration", "inbox", "--limit", "1000", "--full", "--json")
	if err != nil {
		return "", err
	}
	var result struct {
		Messages []struct {
			ID     string `json:"id"`
			RunID  string `json:"run_id"`
			From   string `json:"from_handle"`
			To     string `json:"to_handle"`
			Thread string `json:"thread_id"`
			Body   string `json:"body"`
			Type   string `json:"type"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	found := ""
	for _, message := range result.Messages {
		if message.RunID != runID || message.Thread != questionID || message.From != "run:"+runID || message.To != "dispatch:"+dispatchID || message.Type != "status" {
			continue
		}
		if message.Body != answer || message.ID == "" || found != "" {
			return "", errors.New("original Orca question has conflicting or duplicate reply history")
		}
		found = message.ID
	}
	return found, nil
}

func FindSend(ctx context.Context, call Call, runID, dispatchID, subject, body string) (string, error) {
	raw, err := call(ctx, "orchestration", "inbox", "--limit", "1000", "--full", "--json")
	if err != nil {
		return "", err
	}
	var result struct {
		Messages []struct {
			ID      string `json:"id"`
			RunID   string `json:"run_id"`
			From    string `json:"from_handle"`
			To      string `json:"to_handle"`
			Subject string `json:"subject"`
			Body    string `json:"body"`
			Type    string `json:"type"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	found := ""
	for _, message := range result.Messages {
		if message.RunID != runID || message.From != "run:"+runID || message.To != "dispatch:"+dispatchID || message.Subject != subject || message.Type != "status" {
			continue
		}
		if message.Body != body || message.ID == "" || found != "" {
			return "", errors.New("Orca feedback has conflicting or duplicate send history")
		}
		found = message.ID
	}
	return found, nil
}
