package orca_remote_worker

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (b Client) Check(ctx context.Context, ref Ref) (Delivery, error) {
	identity := ref.External
	sc := &b.config.Settings
	if identity.Kind != "orca_supervised" || identity.OrchestrationRun == "" || identity.CoordinatorRuntimeID == "" {
		return Delivery{}, &Error{Code: executionapi.BackendIdentityMismatch, Diagnostic: "supervised inbox read lacks the fixed coordinator and Orca Run identity"}
	}
	waitMillis := duration(sc.PollInterval).Milliseconds()
	if waitMillis <= 0 {
		waitMillis = 2000
	}
	coordinator, err := b.VerifyCoordinator(ctx, ref)
	if err != nil {
		return Delivery{}, err
	}
	out, callErr := b.config.Call(ctx, positiveDuration(b.config.CommandTimeout, time.Duration(waitMillis+5000)*time.Millisecond), "orchestration", "check", "--terminal", coordinator, "--run", identity.OrchestrationRun, "--wait", "--types", "worker_done,status,question,escalation", "--timeout-ms", strconv.FormatInt(waitMillis, 10), "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return Delivery{}, &Error{Code: executionapi.BackendUnreachable, Diagnostic: "supervised coordinator inbox read failed", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != identity.CoordinatorRuntimeID {
		return Delivery{}, &Error{Code: executionapi.BackendIdentityMismatch, Diagnostic: "supervised coordinator runtime changed; preserve the original Dispatch binding"}
	}
	var result struct {
		RunID          string    `json:"runId"`
		DeliveryID     string    `json:"deliveryId"`
		Messages       []Message `json:"messages"`
		TimedOut       bool      `json:"timedOut"`
		ConnectionLost bool      `json:"connectionLost"`
		Cancelled      bool      `json:"cancelled"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return Delivery{}, &Error{Code: executionapi.BackendProtocolInvalid, Diagnostic: "supervised inbox delivery is malformed", Cause: err}
	}
	if result.RunID != identity.OrchestrationRun {
		return Delivery{}, &Error{Code: executionapi.BackendIdentityMismatch, Diagnostic: "supervised inbox returned a different Orca Run"}
	}
	if result.ConnectionLost {
		return Delivery{}, &Error{Code: executionapi.BackendUnreachable, Diagnostic: "supervised inbox connection was lost"}
	}
	if result.Cancelled && ctx.Err() != nil {
		return Delivery{}, ctx.Err()
	}
	if len(result.Messages) > 0 && result.DeliveryID == "" {
		return Delivery{}, &Error{Code: executionapi.BackendProtocolInvalid, Diagnostic: "supervised inbox returned messages without a durable Delivery ID"}
	}
	return Delivery{ID: result.DeliveryID, Messages: result.Messages, TimedOut: result.TimedOut}, nil
}

func (b Client) Ack(ctx context.Context, ref Ref, deliveryID string) (string, error) {
	identity := ref.External
	if deliveryID == "" || identity.CoordinatorRuntimeID == "" || identity.OrchestrationRun == "" {
		return "", &Error{Code: executionapi.BackendIdentityMismatch, Diagnostic: "supervised Delivery acknowledgement lacks exact identity"}
	}
	coordinator, err := b.VerifyCoordinator(ctx, ref)
	if err != nil {
		return "", err
	}
	out, callErr := b.config.Call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "check", "--terminal", coordinator, "--run", identity.OrchestrationRun, "--ack", deliveryID, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", &Error{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "supervised Delivery acknowledgement is uncertain; reconcile the exact Delivery ID", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != identity.CoordinatorRuntimeID {
		return "", &Error{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Delivery acknowledgement came from a different coordinator runtime"}
	}
	var result struct {
		Acknowledged string `json:"acknowledged"`
		Mutation     struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.Acknowledged != deliveryID {
		return "", &Error{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "Orca did not confirm acknowledgement of the exact Delivery", Cause: err}
	}
	return result.Mutation.RequestID, nil
}

func (b Client) Reply(ctx context.Context, ref Ref, questionID, answer string) (string, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return "", err
	}
	if questionID == "" || strings.TrimSpace(answer) == "" {
		return "", &Error{Code: executionapi.BackendRejected, Diagnostic: "supervised reply requires a bounded answer and original question ID"}
	}
	coordinator := ref.Target.CoordinatorTerminal
	if coordinator == "" {
		coordinator = b.config.Settings.CoordinatorTerminal
	}
	serializedReply, err := json.Marshal(struct {
		QuestionID string `json:"question_id"`
		Body       string `json:"body"`
		Run        string `json:"run"`
		From       string `json:"from"`
		Dispatch   string `json:"dispatch"`
	}{questionID, answer, ref.External.OrchestrationRun, coordinator, ref.External.Dispatch})
	if err != nil || len(serializedReply) > maxSupervisedMessageBytes-512 {
		return "", &Error{Code: executionapi.BackendRejected, Diagnostic: "supervised reply serialized message exceeds the conservative 8 KiB pre-send bound", Cause: err}
	}
	coordinator, err = b.VerifyCoordinator(ctx, ref)
	if err != nil {
		return "", err
	}
	observation, err := b.Show(ctx, StartReceipt{RunID: ref.External.OrchestrationRun, TaskID: ref.External.Task, DispatchID: ref.External.Dispatch, WorktreeID: ref.External.WorktreeID, CenterRuntimeID: ref.External.CoordinatorRuntimeID})
	if err != nil {
		return "", err
	}
	if observation.Liveness != "live" {
		return "", &Error{Code: executionapi.BackendIdentityMismatch, Diagnostic: "supervised reply requires the original live Dispatch"}
	}
	out, callErr := b.config.Call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "reply", "--id", questionID, "--body", answer, "--run", ref.External.OrchestrationRun, "--from", coordinator, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", &Error{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "supervised reply outcome is uncertain; reconcile the exact question before retry", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != ref.External.CoordinatorRuntimeID {
		return "", &Error{Code: executionapi.BackendIdentityMismatch, Diagnostic: "supervised reply receipt came from a different coordinator runtime"}
	}
	var result struct {
		Message struct {
			FromHandle string `json:"from_handle"`
			ToHandle   string `json:"to_handle"`
		} `json:"message"`
		Question struct {
			MessageID  string `json:"message_id"`
			RunID      string `json:"run_id"`
			DispatchID string `json:"dispatch_id"`
			Status     string `json:"status"`
			AnswerBody string `json:"answer_body"`
		} `json:"question"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.Question.MessageID != questionID || result.Question.RunID != ref.External.OrchestrationRun || result.Question.DispatchID != ref.External.Dispatch || result.Question.AnswerBody != answer || result.Question.Status != "answered" || result.Message.FromHandle != "run:"+ref.External.OrchestrationRun || result.Message.ToHandle != "dispatch:"+ref.External.Dispatch || result.Mutation.RequestID == "" {
		return "", &Error{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "Orca did not confirm the exact question answer and request ID", Cause: err}
	}
	return result.Mutation.RequestID, nil
}

func (b Client) FindAnswer(ctx context.Context, ref Ref, questionID, answer string) (string, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return "", err
	}
	if questionID == "" || answer == "" {
		return "", &Error{Code: executionapi.BackendRejected, Diagnostic: "question reconciliation requires the original ID and answer"}
	}
	out, callErr := b.config.Call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "inbox", "--limit", "1000", "--full", "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", &Error{Code: executionapi.BackendUnreachable, Diagnostic: "read-only question reconciliation is unavailable", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != ref.External.CoordinatorRuntimeID {
		return "", &Error{Code: executionapi.BackendIdentityMismatch, Diagnostic: "question history came from a different coordinator runtime"}
	}
	if len(env.Result) > 2<<20 {
		return "", &Error{Code: executionapi.BackendProtocolInvalid, Diagnostic: "question history exceeded the bounded response size"}
	}
	var result struct {
		Messages []struct {
			ID         string `json:"id"`
			FromHandle string `json:"from_handle"`
			ToHandle   string `json:"to_handle"`
			ThreadID   string `json:"thread_id"`
			Body       string `json:"body"`
			Type       string `json:"type"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return "", &Error{Code: executionapi.BackendProtocolInvalid, Diagnostic: "question history is malformed", Cause: err}
	}
	found := ""
	for _, message := range result.Messages {
		if message.ThreadID != questionID || message.FromHandle != "run:"+ref.External.OrchestrationRun || message.ToHandle != "dispatch:"+ref.External.Dispatch || message.Type != "status" {
			continue
		}
		if message.Body != answer || message.ID == "" || found != "" {
			return "", &Error{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "original question has conflicting or duplicate answer history"}
		}
		found = message.ID
	}
	if found == "" {
		return "", &Error{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "original question answer is not yet uniquely visible in read-only inbox history"}
	}
	return found, nil
}
