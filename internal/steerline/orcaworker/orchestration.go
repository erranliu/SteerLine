package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type MutationReceipt struct {
	ID        string
	RequestID string
	Payload   json.RawMessage
}

// CallError distinguishes an unverified platform call from a malformed
// response received after the call was accepted.
type CallError struct {
	Cause  error
	Result Result
}

func (e *CallError) Error() string { return e.Cause.Error() }
func (e *CallError) Unwrap() error { return e.Cause }

func CreateRun(ctx context.Context, call MetaCall, objective, coordinator, runtimeID string) (MutationReceipt, error) {
	envelope, err := call(ctx, "orchestration", "run-create", "--objective", objective, "--from", coordinator, "--json")
	if err != nil {
		return MutationReceipt{}, &CallError{Cause: err, Result: envelope}
	}
	var result struct {
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil {
		return MutationReceipt{}, fmt.Errorf("Orca Run receipt is invalid JSON: %w", err)
	}
	if result.Run.ID == "" || result.Mutation.RequestID == "" {
		return MutationReceipt{}, errors.New("Orca Run receipt lacks its Run or mutation request identity")
	}
	if envelope.RuntimeID != runtimeID {
		return MutationReceipt{}, fmt.Errorf("Orca Run receipt belongs to runtime %q, expected %q", envelope.RuntimeID, runtimeID)
	}
	return MutationReceipt{ID: result.Run.ID, RequestID: result.Mutation.RequestID, Payload: envelope.Payload}, nil
}

func CreateTask(ctx context.Context, call MetaCall, spec, title, runID, coordinator, runtimeID string) (MutationReceipt, error) {
	envelope, err := call(ctx, "orchestration", "task-create", "--spec", spec, "--task-title", title, "--run", runID, "--from", coordinator, "--json")
	if err != nil {
		return MutationReceipt{}, &CallError{Cause: err, Result: envelope}
	}
	var result struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil || result.Task.ID == "" || result.Mutation.RequestID == "" || envelope.RuntimeID != runtimeID {
		return MutationReceipt{}, errors.New("Orca Task receipt lacks exact Task/request/runtime identity")
	}
	return MutationReceipt{ID: result.Task.ID, RequestID: result.Mutation.RequestID, Payload: envelope.Payload}, nil
}

func DispatchReceipt(envelope Result, taskID, runID, runtimeID string) (MutationReceipt, error) {
	var result struct {
		Dispatch struct {
			ID     string `json:"id"`
			TaskID string `json:"task_id"`
			RunID  string `json:"run_id"`
		} `json:"dispatch"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil || result.Dispatch.ID == "" || result.Mutation.RequestID == "" || envelope.RuntimeID != runtimeID || result.Dispatch.TaskID != "" && result.Dispatch.TaskID != taskID || result.Dispatch.RunID != "" && result.Dispatch.RunID != runID {
		return MutationReceipt{}, errors.New("Orca Dispatch receipt lacks exact identity")
	}
	return MutationReceipt{ID: result.Dispatch.ID, RequestID: result.Mutation.RequestID, Payload: envelope.Payload}, nil
}

func ReplyQuestion(ctx context.Context, call MetaCall, questionID, body, runID, coordinator, dispatchID, runtimeID string) (MutationReceipt, error) {
	envelope, err := call(ctx, "orchestration", "reply", "--id", questionID, "--body", body, "--run", runID, "--from", coordinator, "--json")
	if err != nil {
		return MutationReceipt{}, &CallError{Cause: err, Result: envelope}
	}
	var result struct {
		Question struct {
			MessageID  string `json:"message_id"`
			DispatchID string `json:"dispatch_id"`
			AnswerBody string `json:"answer_body"`
		} `json:"question"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil || result.Mutation.RequestID == "" || envelope.RuntimeID != runtimeID || result.Question.MessageID != questionID || result.Question.DispatchID != dispatchID || result.Question.AnswerBody != body {
		return MutationReceipt{}, errors.New("Orca reply receipt lacks its original request identity")
	}
	return MutationReceipt{RequestID: result.Mutation.RequestID, Payload: envelope.Payload}, nil
}

func SendFeedback(ctx context.Context, call MetaCall, subject, body, dispatchID, runID, coordinator, runtimeID string) (MutationReceipt, error) {
	envelope, err := call(ctx, "orchestration", "send", "--subject", subject, "--body", body, "--type", "status", "--to", "dispatch:"+dispatchID, "--run", runID, "--from", coordinator, "--json")
	if err != nil {
		return MutationReceipt{}, &CallError{Cause: err, Result: envelope}
	}
	var result struct {
		Message struct {
			ID       string `json:"id"`
			ToHandle string `json:"to_handle"`
		} `json:"message"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil || result.Mutation.RequestID == "" || result.Message.ID == "" || result.Message.ToHandle != "dispatch:"+dispatchID || envelope.RuntimeID != runtimeID {
		return MutationReceipt{}, errors.New("Orca accepted feedback without an exact Dispatch receipt")
	}
	return MutationReceipt{ID: result.Message.ID, RequestID: result.Mutation.RequestID, Payload: envelope.Payload}, nil
}

type Inbox struct {
	RunID          string    `json:"runId"`
	DeliveryID     string    `json:"deliveryId"`
	Messages       []Message `json:"messages"`
	ConnectionLost bool      `json:"connectionLost"`
	RuntimeID      string    `json:"-"`
}

func CheckInbox(ctx context.Context, call MetaCall, coordinator, runID string) (Inbox, error) {
	envelope, err := call(ctx, "orchestration", "check", "--terminal", coordinator, "--run", runID, "--json")
	if err != nil {
		return Inbox{}, err
	}
	var inbox Inbox
	if err := json.Unmarshal(envelope.Payload, &inbox); err != nil {
		return Inbox{}, err
	}
	inbox.RuntimeID = envelope.RuntimeID
	return inbox, nil
}

func AckInbox(ctx context.Context, call MetaCall, coordinator, runID, deliveryID, runtimeID string) (MutationReceipt, error) {
	envelope, err := call(ctx, "orchestration", "check", "--terminal", coordinator, "--run", runID, "--ack", deliveryID, "--json")
	if err != nil {
		return MutationReceipt{}, &CallError{Cause: err, Result: envelope}
	}
	var result struct {
		Acknowledged string `json:"acknowledged"`
		Mutation     struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil || envelope.RuntimeID != runtimeID || result.Acknowledged != deliveryID || result.Mutation.RequestID == "" {
		return MutationReceipt{}, errors.New("Orca did not acknowledge the saved Delivery")
	}
	return MutationReceipt{RequestID: result.Mutation.RequestID, Payload: envelope.Payload}, nil
}
