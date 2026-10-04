package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (c CommandService) AnswerDecision(ctx context.Context, decisionID string, expectedVersion uint64, idempotencyKey, requestedBy string, answer []byte) (storageports.CommandRecord, bool, error) {
	return c.AnswerDecisionWithID(ctx, newID(), decisionID, expectedVersion, idempotencyKey, requestedBy, answer)
}

func (c CommandService) AnswerDecisionWithID(ctx context.Context, commandID, decisionID string, expectedVersion uint64, idempotencyKey, requestedBy string, answer []byte) (storageports.CommandRecord, bool, error) {
	if decisionID == "" || expectedVersion == 0 || idempotencyKey == "" || requestedBy == "" {
		return storageports.CommandRecord{}, false, errors.New("decision answer requires decision ID, expected version, idempotency key and requester")
	}
	if commandID == "" {
		return storageports.CommandRecord{}, false, errors.New("decision answer requires a stable command ID")
	}
	var value executionmodel.DecisionAnswer
	if err := decode(answer, &value); err != nil {
		return storageports.CommandRecord{}, false, err
	}
	value.OptionID = strings.TrimSpace(value.OptionID)
	// The accepting service validates against the authoritative request. Preserve
	// host-blocked feedback verbatim here, before its receiver is known.
	value.AnsweredBy = strings.TrimSpace(value.AnsweredBy)
	if value.AnsweredAt.IsZero() {
		value.AnsweredAt = time.Now().UTC()
	}
	if value.ExpectedDecisionVersion == 0 {
		value.ExpectedDecisionVersion = expectedVersion
	}
	if value.ExpectedDecisionVersion != expectedVersion {
		return storageports.CommandRecord{}, false, errors.New("answer payload decision version differs from its command revision")
	}
	if value.ExpectedRunRevision == 0 || value.ServiceInstanceID == "" {
		return storageports.CommandRecord{}, false, errors.New("decision answer requires the displayed run revision and service instance")
	}
	if value.DecisionID != decisionID || value.RunID == "" || value.AnsweredBy != requestedBy {
		return storageports.CommandRecord{}, false, errors.New("answer identity must match the command target and requester")
	}
	answer, err := json.Marshal(value)
	if err != nil {
		return storageports.CommandRecord{}, false, err
	}
	return c.Submit(ctx, storageports.CommandRecord{CommandID: commandID, Kind: "answer_decision", TargetID: decisionID, ExpectedVersion: expectedVersion, IdempotencyKey: idempotencyKey, RequestedBy: requestedBy, Payload: append([]byte(nil), answer...), Status: "pending", CreatedAt: time.Now().UTC()})
}

func (c CommandService) AcceptAnswer(ctx context.Context, command storageports.CommandRecord, answer executionmodel.DecisionAnswer) (executionmodel.DecisionRecord, bool, error) {
	if c.Store == nil || c.Project == "" || command.CommandID == "" || command.Kind != "answer_decision" || command.TargetID != answer.DecisionID {
		return executionmodel.DecisionRecord{}, false, errors.New("accepted decision command identity is incomplete")
	}
	command.ProjectIdentity = c.Project
	return c.Store.AcceptDecisionAnswer(ctx, command, answer)
}

func DecisionAnswerKey(answer executionmodel.DecisionAnswer) string {
	return "answer:" + answer.DecisionID + ":" + digestValue(struct {
		OptionID, Note               string
		Answers                      map[string][]string
		SourceAttemptID              string
		RecoveryVersion              uint64
		DecisionVersion, RunRevision uint64
	}{answer.OptionID, strings.TrimSpace(answer.Note), answer.Answers, answer.SourceAttemptID, answer.ExpectedRecoveryVersion, answer.ExpectedDecisionVersion, answer.ExpectedRunRevision})
}
