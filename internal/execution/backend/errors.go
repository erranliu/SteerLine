package backend

import (
	"errors"
)

type BackendErrorCode string

const (
	BackendUnsupported      BackendErrorCode = "unsupported"
	BackendRejected         BackendErrorCode = "rejected"
	BackendUnreachable      BackendErrorCode = "unreachable"
	BackendIdentityMismatch BackendErrorCode = "identity_mismatch"
	BackendProtocolInvalid  BackendErrorCode = "protocol_invalid"
	BackendOutcomeUnknown   BackendErrorCode = "outcome_unknown"
)

type BackendError struct {
	Code       BackendErrorCode
	Diagnostic string
	Cause      error
}

func (e *BackendError) Error() string {
	if e.Cause != nil {
		return string(e.Code) + ": " + e.Diagnostic + ": " + e.Cause.Error()
	}
	return string(e.Code) + ": " + e.Diagnostic
}

func (e *BackendError) Unwrap() error { return e.Cause }

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
		return string(e.Kind) + " " + e.Operation + ": " + e.Diagnostic + ": " + e.Cause.Error()
	}
	return string(e.Kind) + " " + e.Operation + ": " + e.Diagnostic
}

func (e *AdapterError) Unwrap() error { return e.Cause }

func UnsupportedAdapterOperation(operation, diagnostic string) error {
	return &AdapterError{Kind: AdapterUnsupported, Operation: operation, Diagnostic: diagnostic}
}

func IsAdapterError(err error, kind AdapterErrorKind) bool {
	var target *AdapterError
	return errors.As(err, &target) && target.Kind == kind
}
