package steerline

import (
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
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

func orcaStateIdentity(record *ExecutionRecord) orcaworker.ExecutionStateIdentity {
	process := executionapi.ProcessIdentity{}
	if record.Process != nil {
		process = *record.Process
	}
	return orcaworker.ExecutionStateIdentity{
		ProjectID: record.ProjectIdentity, ExecutionID: record.ExecutionID,
		AttemptID: record.CurrentAttemptID, Generation: record.Generation,
		ConfigDigest: record.ConfigurationDigest,
		Protocol:     executionapi.ExecutionProtocol(record.Protocol), Process: process, CoordinatorTerminal: record.Target.CoordinatorTerminal,
	}
}

func decodeOrcaExecutionState(record *ExecutionRecord) (*implementationExecutionView, error) {
	envelope, ok, err := implementationEnvelope(record, orcaworker.ExecutionStateCodecKey)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	state, err := orcaworker.DecodeExecutionStateEnvelope(envelope, orcaStateIdentity(record))
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func storeOrcaExecutionState(record *ExecutionRecord, state *implementationExecutionView) error {
	if state == nil {
		return errors.New("native execution state is required")
	}
	if record.Generation == "" {
		record.Generation = newID()
	}
	envelope, err := orcaworker.NewExecutionStateEnvelope(orcaStateIdentity(record), *state)
	if err != nil {
		return err
	}
	return putImplementationEnvelope(record, envelope)
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
