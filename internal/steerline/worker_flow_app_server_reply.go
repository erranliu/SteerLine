package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
)

func (s *Service) replyAppServerAsk(ctx context.Context, run *Run, execution *ExecutionRecord, answer DecisionAnswer) error {
	if run == nil || run.Decision == nil || run.Decision.Request.NativeAsk == nil || execution == nil {
		return errors.New("saved native answer has no fixed request")
	}
	ask := run.Decision.Request.NativeAsk
	if execution.Process == nil || inspectProcess(execution.Process) != "matching process still exists" || execution.AppServerEndpoint == "" || execution.AppServerGeneration != ask.Generation || execution.SessionID != ask.ThreadID || execution.CurrentTurnID != ask.TurnID || execution.LastStoppedTurnID == ask.TurnID || run.Decision.ID != answer.DecisionID {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "native answer no longer belongs to the live original app-server request"}
	}
	key := "appserver-answer:" + execution.ExecutionID + ":" + answer.DecisionID
	for _, op := range s.State.Operations {
		if op.IdempotencyKey == key {
			return nil /* The original reply may already be applying; reconcile, never resend. */
		}
	}
	request := struct {
		ExecutionID string              `json:"execution_id"`
		Generation  string              `json:"generation"`
		ThreadID    string              `json:"thread_id"`
		TurnID      string              `json:"turn_id"`
		CallID      string              `json:"call_id"`
		RequestID   json.RawMessage     `json:"request_id"`
		Answers     map[string][]string `json:"answers"`
	}{execution.ExecutionID, ask.Generation, ask.ThreadID, ask.TurnID, ask.CallID, ask.RequestID, answer.Answers}
	opID, err := s.startOperation("reply_appserver_question", key, request)
	if err != nil {
		return err
	}
	if s.workerMessages != nil {
		askCopy := *ask
		answers := answer.Answers
		endpoint, workspace, runID := execution.AppServerEndpoint, execution.Workspace.Worktree, run.RunID
		s.forwarders.Add(1)
		go func() {
			defer s.forwarders.Done()
			err := sendAppServerAskReply(s.runContext, endpoint, workspace, askCopy, answers)
			message := &appServerReplyMessage{decisionID: answer.DecisionID, opID: opID, callID: askCopy.CallID, err: err}
			select {
			case s.workerMessages <- workerMessage{runID: runID, reply: message}:
			case <-s.runContext.Done():
			}
		}()
		return nil
	}
	err = sendAppServerAskReply(ctx, execution.AppServerEndpoint, execution.Workspace.Worktree, *ask, answer.Answers)
	if err != nil {
		_ = s.changeOperation(opID, "outcome_unknown", ask.CallID, nil, err.Error())
		execution.Error = ptr("Original Codex question reply is unverified: " + err.Error())
		s.refreshRun(run)
		return s.save()
	}
	if err := s.setOperationState(opID, "accepted", ask.CallID, mustJSON(map[string]any{"request_id": ask.RequestID, "call_id": ask.CallID, "sent_at": now()}), ""); err != nil {
		return err
	}
	s.note(run, "steer", "decision", "reply_sent", "answer sent to the original Codex tool request", ask.CallID, nil)
	return s.save()
}

func sendAppServerAskReply(ctx context.Context, endpoint, workspace string, ask codexAskCall, answers map[string][]string) error {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client, err := codexapp.Open(probeCtx, endpoint)
	if err != nil {
		return err
	}
	defer client.Close()
	var resumed struct {
		Thread struct {
			ID  string `json:"id"`
			Cwd string `json:"cwd"`
		} `json:"thread"`
	}
	err = client.Call(probeCtx, "thread/resume", map[string]any{"threadId": ask.ThreadID, "cwd": workspace, "excludeTurns": true}, &resumed)
	if err != nil {
		return err
	}
	if resumed.Thread.ID != ask.ThreadID || canonical(resumed.Thread.Cwd) != canonical(workspace) {
		return errors.New("original Codex question thread/workspace changed before reply")
	}
	return client.ReplyRequest(ask.RequestID, codexAskToolResponse(answers))
}

func (s *Service) handleAppServerReplyMessage(runID string, message *appServerReplyMessage) error {
	var run *Run
	for _, candidate := range s.State.Runs {
		if candidate != nil && candidate.RunID == runID {
			run = candidate
			break
		}
	}
	if run == nil || run.TaskStatus == "closed" || run.TaskStatus == "discarded" || run.Decision == nil || run.Decision.Status != "answer_saved" || run.Decision.ID != message.decisionID || run.Decision.Request.NativeAsk == nil || run.Decision.Request.NativeAsk.CallID != message.callID {
		return nil // A terminal or replaced Decision cannot be changed by a late reply result.
	}
	for _, op := range s.State.Operations {
		if op.OperationID != message.opID {
			continue
		}
		if op.Status == "confirmed" {
			return nil // The matching native tool item arrived before the send result.
		}
		if message.err != nil {
			if err := s.changeOperation(op.OperationID, "outcome_unknown", message.callID, nil, message.err.Error()); err != nil {
				return err
			}
			s.executionRecord(run).Error = ptr("Original Codex question reply is unverified: " + message.err.Error())
			s.refreshRun(run)
			return s.save()
		}
		if err := s.setOperationState(message.opID, "accepted", message.callID, mustJSON(map[string]any{"call_id": message.callID, "sent_at": now()}), ""); err != nil {
			return err
		}
		s.note(run, "steer", "decision", "reply_sent", "answer sent to the original Codex tool request", message.callID, nil)
		return s.save()
	}
	return errors.New("original Codex question reply operation disappeared")
}
