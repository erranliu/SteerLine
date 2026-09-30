package steerline

import (
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

func (s *Service) setOperationState(id, next, requestID string, receipt []byte, reason string) error {
	for i := range s.domain.Operations {
		if s.domain.Operations[i].OperationID == id {
			return TransitionOperation(&s.domain.Operations[i], next, requestID, receipt, reason)
		}
	}
	return fmt.Errorf("operation %s not found", id)
}

func (s *Service) operationByKey(key string) *OperationRecord {
	for i := range s.domain.Operations {
		if s.domain.Operations[i].IdempotencyKey == key {
			return &s.domain.Operations[i]
		}
	}
	return nil
}
