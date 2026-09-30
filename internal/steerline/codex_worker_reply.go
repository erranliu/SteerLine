package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (s *Service) replyAppServerAsk(ctx context.Context, run *Run, execution *ExecutionRecord, answer DecisionAnswer) error {
	if run == nil || run.Decision == nil || run.Decision.Request.NativeAsk == nil || execution == nil {
		return errors.New("saved native answer has no fixed request")
	}
	ask := run.Decision.Request.NativeAsk
	if execution.Process == nil || inspectProcess(execution.Process) != "matching process still exists" || execution.AppServerEndpoint == "" || execution.AppServerGeneration != ask.Generation || execution.SessionID != ask.ThreadID || execution.CurrentTurnID != ask.TurnID || execution.LastStoppedTurnID == ask.TurnID || run.Decision.ID != answer.DecisionID {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native answer no longer belongs to the live original app-server request"}
	}
	key := "appserver-answer:" + execution.ExecutionID + ":" + answer.DecisionID
	for _, op := range s.domain.Operations {
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
	opID, err := s.startWorkerOperation(run, "reply_appserver_question", key, request)
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
		_ = s.changeWorkerOperation(run, opID, "outcome_unknown", ask.CallID, nil, err.Error())
		execution.Error = ptr("Original Codex question reply is unverified: " + err.Error())
		s.refreshRun(run)
		return s.commitWorker(run)
	}
	if err := s.setOperationState(opID, "accepted", ask.CallID, mustJSON(map[string]any{"request_id": ask.RequestID, "call_id": ask.CallID, "sent_at": now()}), ""); err != nil {
		return err
	}
	s.note(run, "steer", "decision", "reply_sent", "answer sent to the original Codex tool request", ask.CallID, nil)
	return s.commitWorker(run, opID)
}

func sendAppServerAskReply(ctx context.Context, endpoint, workspace string, ask codexAskCall, answers map[string][]string) error {
	return codexworker.SendAskReply(ctx, codexworker.AskReply{
		Endpoint: endpoint, Workspace: workspace, ThreadID: ask.ThreadID,
		RequestID: ask.RequestID, Answer: executionapi.Answer{Answers: answers},
	})
}

func (s *Service) handleAppServerReplyMessage(runID string, message *appServerReplyMessage) error {
	var run *Run
	for _, candidate := range s.runs {
		if candidate != nil && candidate.RunID == runID {
			run = candidate
			break
		}
	}
	if run == nil || run.TaskStatus == "closed" || run.TaskStatus == "discarded" || run.Decision == nil || run.Decision.Status != "answer_saved" || run.Decision.ID != message.decisionID || run.Decision.Request.NativeAsk == nil || run.Decision.Request.NativeAsk.CallID != message.callID {
		return nil // A terminal or replaced Decision cannot be changed by a late reply result.
	}
	for _, op := range s.domain.Operations {
		if op.OperationID != message.opID {
			continue
		}
		if op.Status == "confirmed" {
			return nil // The matching native tool item arrived before the send result.
		}
		if message.err != nil {
			if err := s.changeWorkerOperation(run, op.OperationID, "outcome_unknown", message.callID, nil, message.err.Error()); err != nil {
				return err
			}
			s.executionRecord(run).Error = ptr("Original Codex question reply is unverified: " + message.err.Error())
			s.refreshRun(run)
			return s.commitWorker(run)
		}
		if err := s.setOperationState(message.opID, "accepted", message.callID, mustJSON(map[string]any{"call_id": message.callID, "sent_at": now()}), ""); err != nil {
			return err
		}
		s.note(run, "steer", "decision", "reply_sent", "answer sent to the original Codex tool request", message.callID, nil)
		return s.commitWorker(run, message.opID)
	}
	return errors.New("original Codex question reply operation disappeared")
}

const codexAskToolName = codexapp.AskToolName

type codexAskCall struct {
	RequestID  json.RawMessage         `json:"request_id"`
	ThreadID   string                  `json:"thread_id"`
	TurnID     string                  `json:"turn_id"`
	CallID     string                  `json:"call_id"`
	Generation string                  `json:"generation,omitempty"`
	Questions  []executionapi.Question `json:"questions"`
}

func parseCodexAskCall(envelope codexapp.Envelope, expectedThread, expectedTurn string) (codexAskCall, error) {
	wire, err := codexworker.ParseAskEnvelope(envelope, expectedThread, expectedTurn)
	if err != nil {
		return codexAskCall{}, err
	}
	return codexAskCallFromEnvelope(wire)
}

func codexAskCallFromEnvelope(wire codexworker.AskEnvelope) (codexAskCall, error) {
	questions, err := codexworker.DecodeQuestions(wire.Arguments)
	if err != nil {
		return codexAskCall{}, fmt.Errorf("Codex ask arguments: %w", err)
	}
	var args struct{ Questions []executionapi.Question }
	for _, q := range questions {
		question := executionapi.Question{ID: q.ID, Header: q.Header, Question: q.Question, AllowFreeText: q.AllowFreeText}
		for _, o := range q.Options {
			question.Options = append(question.Options, executionapi.DecisionOption{ID: o.ID, Label: o.Label, Detail: o.Detail})
		}
		args.Questions = append(args.Questions, question)
	}
	if len(args.Questions) == 0 || len(args.Questions) > 3 {
		return codexAskCall{}, errors.New("Codex ask needs one to three questions")
	}
	seen := map[string]bool{}
	for i := range args.Questions {
		q := &args.Questions[i]
		q.ID = strings.TrimSpace(q.ID)
		q.Header = strings.TrimSpace(q.Header)
		q.Question = strings.TrimSpace(q.Question)
		if q.ID == "" || len(q.ID) > 128 || seen[q.ID] || q.Header == "" || len([]rune(q.Header)) > 40 || q.Question == "" || len([]rune(q.Question)) > 2000 {
			return codexAskCall{}, fmt.Errorf("Codex ask question %d has invalid identity or text", i+1)
		}
		seen[q.ID] = true
		req := DecisionRequest{Question: q.Question, Options: q.Options, AllowFreeText: q.AllowFreeText}
		if err := validateDecisionRequest(&req); err != nil {
			return codexAskCall{}, fmt.Errorf("Codex ask question %q: %w", q.ID, err)
		}
		q.Options = req.Options
	}
	call := codexAskCall{RequestID: wire.RequestID, ThreadID: wire.ThreadID, TurnID: wire.TurnID, CallID: wire.CallID, Questions: args.Questions}
	request := codexAskDecisionRequest(call)
	request.NativeAsk = nil // The process generation is attached by the service writer.
	if err := validateDecisionRequest(&request); err != nil {
		return codexAskCall{}, fmt.Errorf("Codex ask cannot be displayed completely: %w", err)
	}
	return call, nil
}

func codexAskDecisionRequest(call codexAskCall) DecisionRequest {
	request := DecisionRequest{Context: "Codex 正在等待回答；提交后会继续当前轮次。", AllowFreeText: true, NativeAsk: &call}
	if len(call.Questions) == 1 {
		q := call.Questions[0]
		request.Question = q.Question
		request.Options = append([]executionapi.DecisionOption(nil), q.Options...)
		request.AllowFreeText = q.AllowFreeText
		return request
	}
	request.Question = fmt.Sprintf("Codex 有 %d 个问题需要回答", len(call.Questions))
	var body strings.Builder
	body.WriteString(request.Context)
	body.WriteString(" 多题请在备注中提交 JSON，例如 {\"question_id\":[\"answer\"]}，每个问题 ID 都须填写。")
	for _, q := range call.Questions {
		fmt.Fprintf(&body, "\n\n[%s] %s：%s", q.ID, q.Header, q.Question)
		for _, option := range q.Options {
			fmt.Fprintf(&body, "\n- %s：%s", option.ID, option.Label)
			if option.Detail != "" {
				fmt.Fprintf(&body, " — %s", option.Detail)
			}
		}
		if q.AllowFreeText {
			body.WriteString("\n允许自由文字回答。")
		} else {
			body.WriteString("\n请选择以上选项，可附加备注。")
		}
	}
	request.Context = body.String()
	return request
}
