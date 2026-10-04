package orcaworker

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

const QuestionRouteImplementationKey = "orca.inbox"

type QuestionRouteData struct {
	RunID      string `json:"run_id"`
	TaskID     string `json:"task_id"`
	DispatchID string `json:"dispatch_id"`
	MessageID  string `json:"message_id"`
	Terminal   string `json:"terminal"`
}

func DecodeMessage(message Message, taskID, dispatchID, agentTerminal string, fixed executionbackend.SessionContext) (executionbackend.EventFact, bool, error) {
	fact := executionbackend.EventFact{SourceEventID: message.ID}
	if message.ID == "" || message.Type == "" {
		return fact, false, errors.New("inbox message lacks a stable identity or kind")
	}
	payload := MessagePayload(message)
	if !MessageMatches(message.FromHandle, message.Type, payload, taskID, dispatchID, agentTerminal) {
		return fact, false, nil
	}
	fact.Matched = true
	switch message.Type {
	case "question":
		question := ParseQuestion(payload)
		if strings.TrimSpace(question.Text) == "" {
			question.Text = strings.TrimSpace(message.Body)
			if question.Text == "" {
				question.Text = strings.TrimSpace(message.Subject)
			}
		}
		if strings.TrimSpace(question.Text) == "" {
			return fact, true, errors.New("inbox question has no display text")
		}
		questionID := message.ID
		fact.Kind = executionbackend.EventQuestion
		fact.Questions = []executionbackend.Question{{ExecutionID: fixed.ExecutionID, AttemptID: fixed.AttemptID,
			Generation: fixed.Generation, TurnID: fixed.CurrentTurnID, ID: questionID, Header: "Question",
			Question: question.Text, AllowFreeText: true}}
		fact.QuestionContext = message.Body
		for _, option := range question.Options {
			fact.Questions[0].Options = append(fact.Questions[0].Options, executionbackend.DecisionOption{ID: option.ID, Label: option.Label, Detail: option.Detail})
		}
		content := executionmodel.DecisionRequest{Question: fact.Questions[0].Question, Options: fact.Questions[0].Options, AllowFreeText: fact.Questions[0].AllowFreeText}
		if err := executionmodel.NormalizeDecisionContent(&content); err != nil {
			return fact, true, err
		}
		fact.Questions[0].Question, fact.Questions[0].Options = content.Question, content.Options
		data := QuestionRouteData{RunID: fixed.Target.OrchestrationRun, TaskID: taskID, DispatchID: dispatchID, MessageID: message.ID, Terminal: agentTerminal}
		encoded, err := json.Marshal(data)
		if err != nil {
			return fact, true, err
		}
		route, err := executionbackend.NewQuestionRoute(QuestionRouteImplementationKey, 1, executionbackend.QuestionRouteIdentity{
			ProjectID: fixed.ProjectID, WorkflowRunID: fixed.WorkflowRunID, ExecutionID: fixed.ExecutionID,
			AttemptID: fixed.AttemptID, Generation: fixed.Generation, TurnID: fixed.CurrentTurnID, QuestionID: message.ID,
		}, encoded)
		if err != nil {
			return fact, true, err
		}
		fact.QuestionRoute = &route
	case "worker_done":
		outcome, err := DoneOutcome(payload, taskID, dispatchID)
		if err != nil {
			return fact, true, err
		}
		fact.Kind, fact.Completed, fact.Outcome = executionbackend.EventCompleted, true, outcome
		fact.LastMessage = strings.TrimSpace(message.Body)
		fact.PRCandidates = executionbackend.PRClues(message.Body)
	case "status":
		fact.Kind = executionbackend.EventStatus
		fact.PRCandidates = executionbackend.PRClues(message.Body)
	case "escalation":
		fact.Kind = executionbackend.EventEscalation
		fact.QuestionText = "Execution requested human guidance: " + strings.TrimSpace(message.Subject)
		fact.QuestionContext = strings.TrimSpace(message.Body)
		fact.AllowFreeText = true
	}
	return fact, true, nil
}

func DecodeInbox(inbox Inbox, taskID, dispatchID, agentTerminal string, fixed executionbackend.SessionContext) (executionbackend.EventBatch, error) {
	batch := executionbackend.EventBatch{Source: "inbox", DeliveryID: inbox.DeliveryID, ExecutionID: fixed.ExecutionID, AttemptID: fixed.AttemptID, ExternalRunID: fixed.Target.OrchestrationRun}
	if len(inbox.Messages) > 0 && batch.DeliveryID == "" {
		return executionbackend.EventBatch{}, errors.New("inbox batch lacks its consumption token")
	}
	batch.Facts = make([]executionbackend.EventFact, 0, len(inbox.Messages))
	for _, message := range inbox.Messages {
		fact, _, err := DecodeMessage(message, taskID, dispatchID, agentTerminal, fixed)
		if err != nil {
			return executionbackend.EventBatch{}, err
		}
		batch.Facts = append(batch.Facts, fact)
	}
	return batch, nil
}

func DecodeQuestionRoute(route executionbackend.ImplementationDataEnvelope, fixed executionbackend.SessionContext, expectedTaskID, expectedDispatchID, expectedTerminal string) (QuestionRouteData, error) {
	want := executionbackend.QuestionRouteIdentity{ProjectID: fixed.ProjectID, WorkflowRunID: fixed.WorkflowRunID,
		ExecutionID: fixed.ExecutionID, AttemptID: fixed.AttemptID, Generation: fixed.Generation,
		TurnID: fixed.CurrentTurnID, QuestionID: route.QuestionID}
	if route.ImplementationKey != QuestionRouteImplementationKey || route.CodecVersion != 1 || route.Purpose != executionbackend.QuestionRouteAnswerPurpose {
		return QuestionRouteData{}, errors.New("unsupported inbox question route codec")
	}
	if err := route.ValidateIdentity(want); err != nil {
		return QuestionRouteData{}, err
	}
	var data QuestionRouteData
	if err := executionbackend.DecodeStrictJSON(route.Payload, &data, "inbox question route has trailing data"); err != nil {
		return QuestionRouteData{}, err
	}
	if data.RunID != fixed.Target.OrchestrationRun || data.TaskID != expectedTaskID || data.DispatchID != expectedDispatchID ||
		data.MessageID != route.QuestionID || data.Terminal != expectedTerminal || data.Terminal == "" {
		return QuestionRouteData{}, fmt.Errorf("inbox question route differs from its original Run, Task, Dispatch or recipient")
	}
	return data, nil
}
