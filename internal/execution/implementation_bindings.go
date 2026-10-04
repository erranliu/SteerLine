package execution

import (
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func putImplementationEnvelope(record *executionmodel.ExecutionRecord, envelope executionbackend.ImplementationDataEnvelope) error {
	if record == nil {
		return errors.New("execution record is required for implementation binding")
	}
	if err := envelope.Validate(); err != nil {
		return err
	}
	next := make([]executionbackend.ImplementationDataEnvelope, 0, len(record.ImplementationData)+1)
	for _, existing := range record.ImplementationData {
		if existing.ImplementationKey != envelope.ImplementationKey {
			next = append(next, existing.Clone())
		}
	}
	next = append(next, envelope.Clone())
	record.ImplementationData = next
	return nil
}

func removeImplementationEnvelope(record *executionmodel.ExecutionRecord, key string) {
	if record == nil {
		return
	}
	next := make([]executionbackend.ImplementationDataEnvelope, 0, len(record.ImplementationData))
	for _, envelope := range record.ImplementationData {
		if envelope.ImplementationKey != key {
			next = append(next, envelope.Clone())
		}
	}
	record.ImplementationData = next
}

func validateExecutionImplementationData(record *executionmodel.ExecutionRecord) error {
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
func awaitingRecoveryResources(record *executionmodel.ExecutionRecord) bool {
	if record == nil || record.Process != nil || record.Status != "pending" || record.RecoveryState != "resuming" ||
		record.RecoveryRequired || record.Generation == "" || record.CurrentTurnID != "" || record.PendingTurnID != "" {
		return false
	}
	return record.SessionID != "" && record.SessionGeneration == ""
}

func executionSessionFacts(record *executionmodel.ExecutionRecord) (string, string) {
	if record == nil {
		return "", ""
	}
	return record.SessionID, record.SessionGeneration
}
