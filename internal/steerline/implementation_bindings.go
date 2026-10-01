package steerline

import (
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
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
		switch envelope.ImplementationKey {
		case orcaworker.ExecutionStateCodecKey:
			if record.Protocol != string(executionapi.ProtocolOrcaTerminalV2) && record.Protocol != string(executionapi.ProtocolOrcaSupervised) {
				return errors.New("Orca binding codec differs from the fixed execution protocol")
			}
			if _, err := orcaworker.DecodeExecutionStateEnvelope(envelope, orcaStateIdentity(record)); err != nil {
				return err
			}
		case codexworker.ExecutionBindingCodecKey:
			if record.Protocol != string(executionapi.ProtocolLocalCodexAppServer) {
				return errors.New("app-server binding codec differs from the fixed execution protocol")
			}
			binding, err := codexworker.DecodeAppServerBindingEnvelope(envelope, codexBindingIdentity(record))
			if err != nil {
				return err
			}
			if record.Process != nil && (binding.Endpoint == "" || binding.Generation == "") {
				return errors.New("bound app-server process lacks its original endpoint and native generation")
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

func codexBindingIdentity(record *ExecutionRecord) codexworker.AppServerBindingIdentity {
	return codexworker.AppServerBindingIdentity{
		ProjectID: record.ProjectIdentity, ExecutionID: record.ExecutionID,
		AttemptID: record.CurrentAttemptID, Generation: record.Generation, ConfigDigest: record.ConfigurationDigest,
	}
}

func decodeCodexAppServerBinding(record *ExecutionRecord) (codexworker.AppServerBinding, error) {
	envelope, ok, err := implementationEnvelope(record, codexworker.ExecutionBindingCodecKey)
	if err != nil {
		return codexworker.AppServerBinding{}, err
	}
	if !ok {
		return codexworker.AppServerBinding{}, nil
	}
	return codexworker.DecodeAppServerBindingEnvelope(envelope, codexBindingIdentity(record))
}

// A manually confirmed new Attempt can carry the original thread before its
// new process resources exist. A live or previously bound process cannot.
func awaitingLocalRecoveryResources(record *ExecutionRecord) bool {
	if record == nil || record.Protocol != string(executionapi.ProtocolLocalCodexAppServer) ||
		record.Process != nil || record.Status != "pending" || record.RecoveryState != "resuming" ||
		record.RecoveryRequired || record.Generation == "" || record.CurrentTurnID != "" || record.PendingTurnID != "" {
		return false
	}
	binding, err := decodeCodexAppServerBinding(record)
	return err == nil && binding.SessionID != "" && binding.Endpoint == "" && binding.Generation == ""
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
