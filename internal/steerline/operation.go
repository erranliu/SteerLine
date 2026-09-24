package steerline

import (
	"encoding/json"
	"errors"
	"fmt"
)

var allowedOperationTransitions = map[string]map[string]bool{
	"pending":         {"in_flight": true, "rejected": true},
	"in_flight":       {"accepted": true, "confirmed": true, "rejected": true, "outcome_unknown": true},
	"accepted":        {"confirmed": true, "rejected": true, "outcome_unknown": true},
	"outcome_unknown": {"accepted": true, "confirmed": true, "rejected": true},
}

func TransitionOperation(op *OperationRecord, next string, requestID string, receipt []byte, reason string) error {
	if op == nil {
		return errors.New("operation is required")
	}
	if op.Status == next {
		return nil
	}
	if !allowedOperationTransitions[op.Status][next] {
		return fmt.Errorf("operation transition %s -> %s is not allowed", op.Status, next)
	}
	op.Status = next
	op.PlatformRequestID = requestID
	op.Receipt = append([]byte(nil), receipt...)
	op.UnknownReason = reason
	op.UpdatedAt = now()
	return nil
}

func (s *Service) startOperation(kind, key string, payload any) (string, error) {
	if kind == "" || key == "" {
		return "", errors.New("operation kind and idempotency key are required")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	operation := OperationRecord{ProjectIdentity: s.Config.identity, OperationID: newID(), IdempotencyKey: key, Kind: kind, Status: "pending", PayloadDigest: operationDigest(b), Request: b, CreatedAt: now(), UpdatedAt: now()}
	s.State.Operations = append(s.State.Operations, operation)
	if err = s.save(); err != nil {
		return "", err
	}
	if err = s.changeOperation(operation.OperationID, "in_flight", "", nil, ""); err != nil {
		return "", err
	}
	return operation.OperationID, nil
}

func (s *Service) changeOperation(id, next, requestID string, receipt []byte, reason string) error {
	for i := range s.State.Operations {
		if s.State.Operations[i].OperationID != id {
			continue
		}
		if err := TransitionOperation(&s.State.Operations[i], next, requestID, receipt, reason); err != nil {
			return err
		}
		return s.save()
	}
	return fmt.Errorf("operation %s not found", id)
}

func (s *Service) setOperationState(id, next, requestID string, receipt []byte, reason string) error {
	for i := range s.State.Operations {
		if s.State.Operations[i].OperationID == id {
			return TransitionOperation(&s.State.Operations[i], next, requestID, receipt, reason)
		}
	}
	return fmt.Errorf("operation %s not found", id)
}

func (s *Service) reconcileOperationsAfterRestart() {
	for i := range s.State.Operations {
		op := &s.State.Operations[i]
		if op.Status == "in_flight" || op.Status == "accepted" {
			op.Status = "outcome_unknown"
			op.UnknownReason = "center restarted before terminal operation evidence was committed"
			op.UpdatedAt = now()
		}
	}
}
