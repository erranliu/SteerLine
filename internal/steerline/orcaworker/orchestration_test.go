package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestCreateRunDistinguishesUncertainCallFromInvalidReceipt(t *testing.T) {
	transport := func(context.Context, ...string) (Result, error) {
		return Result{}, errors.New("receipt lost")
	}
	_, err := CreateRun(context.Background(), transport, "objective", "coordinator", "runtime")
	var callErr *CallError
	if !errors.As(err, &callErr) {
		t.Fatalf("unverified call lost its uncertainty: %v", err)
	}
	invalid := func(context.Context, ...string) (Result, error) {
		return Result{RuntimeID: "other", Payload: json.RawMessage(`{"run":{"id":"run"},"mutation":{"requestId":"request"}}`)}, nil
	}
	_, err = CreateRun(context.Background(), invalid, "objective", "coordinator", "runtime")
	if err == nil || errors.As(err, &callErr) {
		t.Fatalf("invalid receipt was treated as an uncertain call: %v", err)
	}
}

func TestDispatchReceiptRejectsAnotherTask(t *testing.T) {
	envelope := Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"dispatch":{"id":"dispatch","task_id":"other","run_id":"run"},"mutation":{"requestId":"request"}}`)}
	if _, err := DispatchReceipt(envelope, "task", "run", "runtime"); err == nil {
		t.Fatal("Dispatch receipt for another Task was accepted")
	}
}

func TestReplyQuestionRejectsWrongDispatch(t *testing.T) {
	call := func(context.Context, ...string) (Result, error) {
		return Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"question":{"message_id":"question","dispatch_id":"other","answer_body":"yes"},"mutation":{"requestId":"request"}}`)}, nil
	}
	if _, err := ReplyQuestion(context.Background(), call, "question", "yes", "run", "coordinator", "dispatch", "runtime"); err == nil {
		t.Fatal("reply receipt for another Dispatch was accepted")
	}
}

func TestSendFeedbackPreservesInactiveDispatchCode(t *testing.T) {
	call := func(context.Context, ...string) (Result, error) {
		return Result{ErrorCode: "dispatch_inactive"}, errors.New("dispatch is inactive")
	}
	_, err := SendFeedback(context.Background(), call, "subject", "body", "dispatch", "run", "coordinator", "runtime")
	var callErr *CallError
	if !errors.As(err, &callErr) || callErr.Result.ErrorCode != "dispatch_inactive" {
		t.Fatalf("inactive Dispatch evidence was lost: %v", err)
	}
}
