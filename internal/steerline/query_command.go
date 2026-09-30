package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/query"
)

type CommandService struct {
	Store   *SQLiteStore
	Project string
}

func (c CommandService) Submit(ctx context.Context, command CommandRecord) (CommandRecord, bool, error) {
	if c.Store == nil || c.Project == "" {
		return command, false, errors.New("command service store and project identity are required")
	}
	command.ProjectIdentity = c.Project
	var result CommandRecord
	var created bool
	err := c.Store.Transact(ctx, func(tx DomainTx) error { var err error; result, created, err = tx.EnqueueCommand(command); return err })
	return result, created, err
}

func (c CommandService) AnswerDecision(ctx context.Context, decisionID string, expectedVersion uint64, idempotencyKey, requestedBy string, answer []byte) (CommandRecord, bool, error) {
	return c.AnswerDecisionWithID(ctx, newID(), decisionID, expectedVersion, idempotencyKey, requestedBy, answer)
}

func (c CommandService) AnswerDecisionWithID(ctx context.Context, commandID, decisionID string, expectedVersion uint64, idempotencyKey, requestedBy string, answer []byte) (CommandRecord, bool, error) {
	if decisionID == "" || expectedVersion == 0 || idempotencyKey == "" || requestedBy == "" {
		return CommandRecord{}, false, errors.New("decision answer requires decision ID, expected version, idempotency key and requester")
	}
	if commandID == "" {
		return CommandRecord{}, false, errors.New("decision answer requires a stable command ID")
	}
	var value DecisionAnswer
	if err := decode(answer, &value); err != nil {
		return CommandRecord{}, false, err
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
		return CommandRecord{}, false, errors.New("answer payload decision version differs from its command revision")
	}
	if value.ExpectedRunRevision == 0 || value.ServiceInstanceID == "" {
		return CommandRecord{}, false, errors.New("decision answer requires the displayed run revision and service instance")
	}
	if value.DecisionID != decisionID || value.RunID == "" || value.AnsweredBy != requestedBy {
		return CommandRecord{}, false, errors.New("answer identity must match the command target and requester")
	}
	answer, err := json.Marshal(value)
	if err != nil {
		return CommandRecord{}, false, err
	}
	return c.Submit(ctx, CommandRecord{CommandID: commandID, Kind: "answer_decision", TargetID: decisionID, ExpectedVersion: expectedVersion, IdempotencyKey: idempotencyKey, RequestedBy: requestedBy, Payload: append([]byte(nil), answer...), Status: "pending", CreatedAt: time.Now().UTC()})
}

func (c CommandService) AcceptAnswer(ctx context.Context, command CommandRecord, answer DecisionAnswer) (DecisionRecord, bool, error) {
	if c.Store == nil || c.Project == "" || command.CommandID == "" || command.Kind != "answer_decision" || command.TargetID != answer.DecisionID {
		return DecisionRecord{}, false, errors.New("accepted decision command identity is incomplete")
	}
	command.ProjectIdentity = c.Project
	var record DecisionRecord
	var alreadyAccepted bool
	err := c.Store.Transact(ctx, func(tx DomainTx) error {
		var err error
		record, alreadyAccepted, err = tx.AcceptDecisionAnswer(command, answer)
		return err
	})
	return record, alreadyAccepted, err
}

func decisionAnswerKey(answer DecisionAnswer) string {
	return "answer:" + answer.DecisionID + ":" + digestValue(struct {
		OptionID, Note  string
		Answers         map[string][]string
		SourceAttemptID string
		RecoveryVersion uint64
	}{answer.OptionID, strings.TrimSpace(answer.Note), answer.Answers, answer.SourceAttemptID, answer.ExpectedRecoveryVersion})
}

func (c CommandService) Pending(ctx context.Context, limit int) ([]CommandRecord, error) {
	if c.Store == nil || c.Project == "" {
		return nil, errors.New("command service store and project identity are required")
	}
	return c.Store.Commands(ctx, c.Project, "pending", limit)
}

func (c CommandService) Complete(ctx context.Context, command CommandRecord, result []byte) error {
	if c.Store == nil || c.Project == "" {
		return errors.New("command service store and project identity are required")
	}
	command.ProjectIdentity = c.Project
	command.Result = append([]byte(nil), result...)
	command.Status = "completed"
	return c.Store.Transact(ctx, func(tx DomainTx) error { return tx.UpdateCommand(command) })
}

func (c CommandService) SetStatus(ctx context.Context, command CommandRecord, status string, result []byte) error {
	if c.Store == nil || c.Project == "" {
		return errors.New("command service store and project identity are required")
	}
	command.ProjectIdentity = c.Project
	command.Status = status
	command.Result = append([]byte(nil), result...)
	return c.Store.Transact(ctx, func(tx DomainTx) error { return tx.UpdateCommand(command) })
}

type QueryService = query.Service
type EventFilter = query.EventFilter
type EventPage = query.EventPage
type EventBodyPage = query.EventBodyPage
type CursorError = query.CursorError

const DefaultEventPageSize = query.DefaultEventPageSize
const MaxEventPageSize = query.MaxEventPageSize
const MaxEventPageBytes = query.MaxEventPageBytes
const eventReadBatchSize = query.EventReadBatchSize

func NewQueryService(store query.Store, project string) QueryService {
	return query.NewService(store, project)
}
