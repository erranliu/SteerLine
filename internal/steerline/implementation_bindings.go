package steerline

import (
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func implementationEnvelope(record *ExecutionRecord, key string) (executionapi.ImplementationDataEnvelope, bool) {
	if record == nil {
		return executionapi.ImplementationDataEnvelope{}, false
	}
	var found executionapi.ImplementationDataEnvelope
	count := 0
	for _, envelope := range record.ImplementationData {
		if envelope.ImplementationKey == key {
			found = envelope.Clone()
			count++
		}
	}
	return found, count == 1
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
		switch envelope.ImplementationKey {
		case orcaworker.ExecutionStateCodecKey:
			if _, err := orcaworker.DecodeExecutionStateEnvelope(envelope, orcaStateIdentity(record)); err != nil {
				return err
			}
		case codexworker.ExecutionBindingCodecKey:
			if _, err := codexworker.DecodeAppServerBindingEnvelope(envelope, codexBindingIdentity(record)); err != nil {
				return err
			}
		default:
			return errors.New("execution contains an unknown implementation binding codec")
		}
	}
	if record.Protocol == string(executionapi.ProtocolOrcaTerminalV2) || record.Protocol == string(executionapi.ProtocolOrcaSupervised) {
		if record.Process != nil && !seen[orcaworker.ExecutionStateCodecKey] {
			return errors.New("original Orca execution binding is missing; preserve the external session")
		}
	}
	if record.Protocol == string(executionapi.ProtocolLocalCodexAppServer) && record.Process != nil && !seen[codexworker.ExecutionBindingCodecKey] {
		return errors.New("original app-server binding is missing; preserve the external session")
	}
	return nil
}

func orcaStateIdentity(record *ExecutionRecord) orcaworker.ExecutionStateIdentity {
	return orcaworker.ExecutionStateIdentity{
		ProjectID: record.ProjectIdentity, ExecutionID: record.ExecutionID,
		AttemptID: record.CurrentAttemptID, Generation: record.Generation,
		ConfigDigest: record.ConfigurationDigest,
	}
}

func decodeOrcaExecutionState(record *ExecutionRecord) (*implementationExecutionView, error) {
	envelope, ok := implementationEnvelope(record, orcaworker.ExecutionStateCodecKey)
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

func codexBindingIdentity(record *ExecutionRecord) codexworker.AppServerBindingIdentity {
	return codexworker.AppServerBindingIdentity{
		ProjectID: record.ProjectIdentity, ExecutionID: record.ExecutionID,
		AttemptID: record.CurrentAttemptID, Generation: record.Generation, ConfigDigest: record.ConfigurationDigest,
	}
}

func decodeCodexAppServerBinding(record *ExecutionRecord) (codexworker.AppServerBinding, error) {
	envelope, ok := implementationEnvelope(record, codexworker.ExecutionBindingCodecKey)
	if !ok {
		return codexworker.AppServerBinding{}, nil
	}
	return codexworker.DecodeAppServerBindingEnvelope(envelope, codexBindingIdentity(record))
}

func codexSessionID(record *ExecutionRecord) string {
	binding, err := decodeCodexAppServerBinding(record)
	if err != nil {
		return ""
	}
	return binding.SessionID
}

func executionSessionFacts(record *ExecutionRecord) (string, string) {
	if record == nil {
		return "", ""
	}
	switch executionapi.ExecutionProtocol(record.Protocol) {
	case executionapi.ProtocolLocalCodexAppServer:
		binding, err := decodeCodexAppServerBinding(record)
		if err == nil {
			return binding.SessionID, binding.Generation
		}
	case executionapi.ProtocolOrcaTerminalV2:
		if record.Process != nil {
			return orcaworker.SessionID(*record.Process), nativeOrcaGeneration(record)
		}
	case executionapi.ProtocolOrcaSupervised:
		if record.Process != nil {
			return remote.SessionID(executionapi.ProcessIdentity(*record.Process)), nativeOrcaGeneration(record)
		}
	}
	return "", ""
}

func storeCodexAppServerBinding(record *ExecutionRecord, binding codexworker.Binding) error {
	if binding.Endpoint == "" && binding.SessionID == "" && binding.Generation == "" {
		return nil
	}
	envelope, err := codexworker.NewAppServerBindingEnvelope(codexBindingIdentity(record), codexworker.AppServerBinding{
		Endpoint: binding.Endpoint, SessionID: binding.SessionID, Generation: binding.Generation,
	})
	if err != nil {
		return err
	}
	return putImplementationEnvelope(record, envelope)
}
