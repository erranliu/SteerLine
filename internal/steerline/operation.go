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

// startOperation persists an idempotent remote intent before its side effect.
func (s *Service) startOperation(kind, key string, payload any) (string, error) {
	if kind == "" || key == "" {
		return "", errors.New("operation kind and idempotency key are required")
	}
	request, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	for _, existing := range s.State.Operations {
		if existing.IdempotencyKey != key {
			continue
		}
		if existing.Kind != kind || existing.PayloadDigest != operationDigest(request) || string(existing.Request) != string(request) {
			return "", errors.New("operation idempotency key conflicts with the original request")
		}
		if existing.Status != "pending" {
			return "", fmt.Errorf("operation %s is already %s; reconcile the original operation before another action", existing.OperationID, existing.Status)
		}
		if err := s.changeOperation(existing.OperationID, "in_flight", "", nil, ""); err != nil {
			return "", err
		}
		return existing.OperationID, nil
	}
	op := OperationRecord{ProjectIdentity: s.Config.identity, OperationID: newID(), IdempotencyKey: key, Kind: kind, Status: "pending", PayloadDigest: operationDigest(request), Request: request, CreatedAt: now(), UpdatedAt: now()}
	s.State.Operations = append(s.State.Operations, op)
	if err := s.save(); err != nil {
		return "", err
	}
	if err := s.changeOperation(op.OperationID, "in_flight", "", nil, ""); err != nil {
		return "", err
	}
	return op.OperationID, nil
}

func (s *Service) changeOperation(id, next, requestID string, receipt []byte, reason string) error {
	if err := s.setOperationState(id, next, requestID, receipt, reason); err != nil {
		return err
	}
	return s.save()
}

func (s *Service) setOperationState(id, next, requestID string, receipt []byte, reason string) error {
	for i := range s.State.Operations {
		if s.State.Operations[i].OperationID == id {
			return TransitionOperation(&s.State.Operations[i], next, requestID, receipt, reason)
		}
	}
	return fmt.Errorf("operation %s not found", id)
}
