package steerline

import (
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func implementationEnvelope(record *ExecutionRecord, key string) (executionapi.ImplementationDataEnvelope, bool, error) {
	if record == nil {
		return executionapi.ImplementationDataEnvelope{}, false, errors.New("execution record is required for implementation binding")
	}
	var found executionapi.ImplementationDataEnvelope
	count := 0
	for _, envelope := range record.ImplementationData {
		if envelope.ImplementationKey == key {
			found = envelope.Clone()
			count++
		}
	}
	if count > 1 {
		return executionapi.ImplementationDataEnvelope{}, false, errors.New("execution contains duplicate implementation binding envelopes")
	}
	return found, count == 1, nil
}

func putImplementationEnvelope(record *ExecutionRecord, envelope executionapi.ImplementationDataEnvelope) error {
	if record == nil {
		return errors.New("execution record is required for implementation binding")
	}
	if err := envelope.Validate(); err != nil {
		return err
	}
	next := make([]executionapi.ImplementationDataEnvelope, 0, len(record.ImplementationData)+1)
	for _, existing := range record.ImplementationData {
		if existing.ImplementationKey != envelope.ImplementationKey {
			next = append(next, existing.Clone())
		}
	}
	next = append(next, envelope.Clone())
	record.ImplementationData = next
	return nil
}

func removeImplementationEnvelope(record *ExecutionRecord, key string) {
	if record == nil {
		return
	}
	next := make([]executionapi.ImplementationDataEnvelope, 0, len(record.ImplementationData))
	for _, envelope := range record.ImplementationData {
		if envelope.ImplementationKey != key {
			next = append(next, envelope.Clone())
		}
	}
	record.ImplementationData = next
}

func validateExecutionImplementationData(record *ExecutionRecord) error {
	if record == nil {
		return errors.New("execution record is required for implementation data verification")
	}
	seen := map[string]bool{}
	for _, envelope := range record.ImplementationData {
		if seen[envelope.ImplementationKey] {
			return errors.New("execution contains duplicate implementation binding envelopes")
		}
		seen[envelope.ImplementationKey] = true
		if err := envelope.ValidateResourceIdentity(record.ProjectIdentity, record.ExecutionID, record.CurrentAttemptID, record.Generation, record.ConfigurationDigest); err != nil {
			return err
		}
	}
	return nil
}

// A manually confirmed new Attempt can carry the original session before its
// new process resources exist. This is a generic recovery stage, not a live binding.
func awaitingRecoveryResources(record *ExecutionRecord) bool {
	if record == nil || record.Process != nil || record.Status != "pending" || record.RecoveryState != "resuming" ||
		record.RecoveryRequired || record.Generation == "" || record.CurrentTurnID != "" || record.PendingTurnID != "" {
		return false
	}
	return record.SessionID != "" && record.SessionGeneration == ""
}

func executionSessionFacts(record *ExecutionRecord) (string, string) {
	if record == nil {
		return "", ""
	}
	return record.SessionID, record.SessionGeneration
}
