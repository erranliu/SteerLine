package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
)

type PlatformError struct {
	Code       string
	Diagnostic string
	Cause      error
}

func (e *PlatformError) Error() string { return e.Diagnostic }
func (e *PlatformError) Unwrap() error { return e.Cause }

func ShowTerminal(ctx context.Context, call MetaCall, handle string) (Terminal, error) {
	if handle == "" {
		return Terminal{}, errors.New("worker terminal handle is empty")
	}
	envelope, err := call(ctx, "terminal", "show", "--terminal", handle, "--json")
	if err != nil {
		return Terminal{}, err
	}
	var result struct {
		Terminal Terminal `json:"terminal"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil {
		return Terminal{}, err
	}
	result.Terminal.RuntimeID = envelope.RuntimeID
	if result.Terminal.Handle != handle || result.Terminal.RuntimeID == "" || result.Terminal.IncarnationID == "" || result.Terminal.Orphaned || !result.Terminal.Connected || result.Terminal.Exited() {
		return Terminal{}, &PlatformError{Code: "identity_mismatch", Diagnostic: "original worker terminal is missing, disconnected or replaced"}
	}
	return result.Terminal, nil
}

type TerminalIdentity struct {
	Kind        string
	Handle      string
	RuntimeID   string
	Incarnation string
}

func CloseTerminal(ctx context.Context, call MetaCall, handle, runtimeID string) (json.RawMessage, error) {
	envelope, err := call(ctx, "terminal", "close", "--terminal", handle, "--json")
	if err != nil {
		return nil, &CallError{Cause: err, Result: envelope}
	}
	var result struct {
		Close struct {
			Handle    string `json:"handle"`
			PTYKilled bool   `json:"ptyKilled"`
		} `json:"close"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil || envelope.RuntimeID != runtimeID || result.Close.Handle != handle || !result.Close.PTYKilled {
		return nil, &PlatformError{Code: "outcome_unknown", Diagnostic: "Orca terminal close lacks proof for the original process"}
	}
	return envelope.Payload, nil
}
