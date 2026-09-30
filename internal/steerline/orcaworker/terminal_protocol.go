package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
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

func CreateTerminal(ctx context.Context, call MetaCall, worktreePath, runID string) (Terminal, error) {
	envelope, err := call(ctx, "terminal", "create", "--worktree", "path:"+filepath.ToSlash(worktreePath), "--title", "SteerLine-"+runID, "--shell", "cmd.exe", "--command", "echo SLREADY:"+runID, "--json")
	if err != nil {
		return Terminal{}, &PlatformError{Code: "outcome_unknown", Diagnostic: "Orca may have created the fixed worker terminal", Cause: err}
	}
	var result struct {
		Terminal Terminal `json:"terminal"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil || result.Terminal.Handle == "" {
		return Terminal{}, &PlatformError{Code: "outcome_unknown", Diagnostic: "Orca terminal creation omitted its handle", Cause: err}
	}
	return ShowTerminal(ctx, call, result.Terminal.Handle)
}

type TerminalIdentity struct {
	Kind        string
	Handle      string
	RuntimeID   string
	Incarnation string
}

type Output struct {
	Rows       []string
	NextCursor int64
	Oldest     int64
}

func ReadOutput(ctx context.Context, call MetaCall, identity TerminalIdentity, cursor int64) (Output, error) {
	if cursor < 0 || identity.Kind != "orca" || identity.Handle == "" || identity.RuntimeID == "" || identity.Incarnation == "" {
		return Output{}, errors.New("Orca output has no fixed terminal or cursor identity")
	}
	term, err := ShowTerminal(ctx, call, identity.Handle)
	if err != nil {
		return Output{}, err
	}
	if term.RuntimeID != identity.RuntimeID || term.IncarnationID != identity.Incarnation {
		return Output{}, &PlatformError{Code: "identity_mismatch", Diagnostic: "Orca output terminal was replaced"}
	}
	envelope, err := call(ctx, "terminal", "read", "--terminal", identity.Handle, "--cursor", strconv.FormatInt(cursor, 10), "--limit", "100", "--json")
	if err != nil {
		return Output{}, err
	}
	if envelope.RuntimeID != identity.RuntimeID {
		return Output{}, &PlatformError{Code: "identity_mismatch", Diagnostic: "Orca output came from another runtime"}
	}
	var result struct {
		Terminal struct {
			Handle       string   `json:"handle"`
			Tail         []string `json:"tail"`
			OldestCursor string   `json:"oldestCursor"`
			NextCursor   string   `json:"nextCursor"`
			Source       string   `json:"source"`
		} `json:"terminal"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil {
		return Output{}, err
	}
	oldest, oldErr := strconv.ParseInt(result.Terminal.OldestCursor, 10, 64)
	next, nextErr := strconv.ParseInt(result.Terminal.NextCursor, 10, 64)
	if oldErr != nil || nextErr != nil || result.Terminal.Handle != identity.Handle || result.Terminal.Source != "stream" || oldest > cursor || next < cursor || next-cursor != int64(len(result.Terminal.Tail)) {
		return Output{}, &PlatformError{Code: "protocol_invalid", Diagnostic: "Orca worker output cursor has a gap or malformed response"}
	}
	return Output{Rows: result.Terminal.Tail, NextCursor: next, Oldest: oldest}, nil
}

func SendCommand(ctx context.Context, call MetaCall, identity TerminalIdentity, command string) (string, error) {
	if identity.Kind != "orca" || identity.Handle == "" || command == "" {
		return "", errors.New("Orca send lacks fixed terminal or command")
	}
	term, err := ShowTerminal(ctx, call, identity.Handle)
	if err != nil {
		return "", err
	}
	if term.RuntimeID != identity.RuntimeID || term.IncarnationID != identity.Incarnation {
		return "", &PlatformError{Code: "identity_mismatch", Diagnostic: "worker send targets a replaced Orca terminal"}
	}
	envelope, err := call(ctx, "terminal", "send", "--terminal", identity.Handle, "--text", command, "--enter", "--json")
	if err != nil {
		return "", &PlatformError{Code: "outcome_unknown", Diagnostic: "Orca worker command send has an uncertain receipt", Cause: err}
	}
	if envelope.RuntimeID != identity.RuntimeID {
		return "", &PlatformError{Code: "identity_mismatch", Diagnostic: "worker send receipt came from another runtime"}
	}
	var result struct {
		Send struct {
			Handle   string `json:"handle"`
			Accepted bool   `json:"accepted"`
			Prompt   struct {
				RequestID          string `json:"requestId"`
				ProcessIncarnation string `json:"processIncarnation"`
			} `json:"prompt"`
		} `json:"send"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil || result.Send.Handle != identity.Handle || !result.Send.Accepted || result.Send.Prompt.ProcessIncarnation != identity.Incarnation {
		return "", &PlatformError{Code: "outcome_unknown", Diagnostic: "Orca did not confirm input acceptance on the original terminal", Cause: err}
	}
	return result.Send.Prompt.RequestID, nil
}

func CurrentCursor(ctx context.Context, call MetaCall, handle, runtimeID string) (int64, error) {
	envelope, err := call(ctx, "terminal", "read", "--terminal", handle, "--json")
	if err != nil {
		return 0, err
	}
	if envelope.RuntimeID != runtimeID {
		return 0, &PlatformError{Code: "identity_mismatch", Diagnostic: "worker output cursor came from another Orca runtime"}
	}
	var result struct {
		Terminal struct {
			Handle     string `json:"handle"`
			NextCursor string `json:"nextCursor"`
			Source     string `json:"source"`
		} `json:"terminal"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil {
		return 0, err
	}
	cursor, err := strconv.ParseInt(result.Terminal.NextCursor, 10, 64)
	if err != nil || cursor < 0 || result.Terminal.Handle != handle || result.Terminal.Source != "stream" {
		return 0, &PlatformError{Code: "protocol_invalid", Diagnostic: "worker terminal did not return a usable output cursor"}
	}
	return cursor, nil
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
