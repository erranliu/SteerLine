package steerline

import (
	"errors"
	"fmt"
)

type AdapterErrorKind string

const (
	AdapterUnsupported      AdapterErrorKind = "capability_unsupported"
	AdapterRejected         AdapterErrorKind = "rejected"
	AdapterUnreachable      AdapterErrorKind = "unreachable"
	AdapterIdentityMismatch AdapterErrorKind = "identity_mismatch"
	AdapterProtocolInvalid  AdapterErrorKind = "protocol_invalid"
	AdapterOutcomeUnknown   AdapterErrorKind = "outcome_unknown"
)

type AdapterError struct {
	Kind       AdapterErrorKind
	Operation  string
	Diagnostic string
	Cause      error
}

func (e *AdapterError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s %s: %s: %v", e.Kind, e.Operation, e.Diagnostic, e.Cause)
	}
	return fmt.Sprintf("%s %s: %s", e.Kind, e.Operation, e.Diagnostic)
}
func (e *AdapterError) Unwrap() error { return e.Cause }
func UnsupportedAdapterOperation(operation, diagnostic string) error {
	return &AdapterError{Kind: AdapterUnsupported, Operation: operation, Diagnostic: diagnostic}
}
func IsAdapterError(err error, kind AdapterErrorKind) bool {
	var target *AdapterError
	return errors.As(err, &target) && target.Kind == kind
}

func ExecutionBackendKind(host string) (backend, protocol string, err error) {
	switch host {
	case "", "local":
		return string(BackendLocal), string(ProtocolLocalProcess), nil
	case "orca":
		return string(BackendOrca), string(ProtocolOrcaTerminalV2), nil
	case "orca_supervised":
		return string(BackendOrca), string(ProtocolOrcaSupervised), nil
	default:
		return "", "", fmt.Errorf("unsupported execution host %q; fallback is disabled", host)
	}
}
