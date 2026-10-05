package codexworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/execution/adapters/protocol/codexapp"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type Recovery struct {
	Binding   Binding
	TurnID    string
	Observer  *Observation
	Completed bool
	Dead      bool
}

// Reattach reads the original session and resumes only observation. It never
// sends input, starts a turn or replies to a saved request. Native thread/resume
// subscribes the new observation connection without running a model turn.
func (b Backend) Reattach(ctx context.Context, fixed Binding, commandID, turnID string, knownCalls []string) (result Recovery, err error) {
	defer func() { result.Binding = fixed; result.TurnID = turnID }()
	serverDir := b.Config.ServerDir
	var savedProcess ProcessReceipt
	processBytes, processErr := os.ReadFile(filepath.Join(serverDir, "process.json"))
	if processErr != nil || json.Unmarshal(processBytes, &savedProcess) != nil || savedProcess.Endpoint == "" || savedProcess.Generation == "" {
		return result, errors.New("original app-server process receipt is missing or unreadable")
	}
	if fixed.Process == nil {
		fixed.Process = &savedProcess.Identity
		fixed.Endpoint = savedProcess.Endpoint
		fixed.Generation = savedProcess.Generation
	}
	if *fixed.Process != savedProcess.Identity || fixed.Endpoint != savedProcess.Endpoint || fixed.Generation != savedProcess.Generation {
		return result, errors.New("app-server process receipt differs from authoritative execution")
	}
	if fixed.Process == nil || fixed.Endpoint == "" || fixed.Generation == "" || b.Process.Inspect(fixed.Process) != "matching process still exists" {
		return result, errors.New("original app-server process identity is unverified")
	}
	if fixed.SessionID == "" {
		var receipt struct {
			Thread struct {
				ID  string `json:"id"`
				Cwd string `json:"cwd"`
			} `json:"thread"`
		}
		if raw, err := os.ReadFile(filepath.Join(serverDir, "thread.json")); err == nil && json.Unmarshal(raw, &receipt) == nil && ValidNativeID(receipt.Thread.ID) && sameWorkspace(receipt.Thread.Cwd, b.Config.Workspace) {
			fixed.SessionID = receipt.Thread.ID
		}
	}
	if !ValidNativeID(fixed.SessionID) {
		return result, errors.New("original app-server thread identity is unverified")
	}
	if !executionbackend.StableExecutionID(commandID) {
		return result, errors.New("original app-server command identity is unverified")
	}
	dir := filepath.Join(b.Config.RunDir, "runs", b.Config.RunID, "turns", commandID)
	if !ValidNativeID(turnID) {
		var receipt struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if raw, err := os.ReadFile(filepath.Join(dir, "turn-start.json")); err == nil && json.Unmarshal(raw, &receipt) == nil && ValidNativeID(receipt.Turn.ID) {
			turnID = receipt.Turn.ID

		}
	}
	if !ValidNativeID(turnID) {
		return result, errors.New("original app-server native turn identity is unverified")
	}

	client, err := ConnectAppServer(ctx, fixed.Endpoint)
	if err != nil {
		return result, err
	}
	closeClient := true
	defer func() {
		if closeClient {
			client.Close()
		}
	}()
	nativeTurn, err := ReadTurnSnapshot(ctx, client, fixed.SessionID, b.Config.Workspace, turnID)
	if err != nil {
		return result, err
	}
	journalPath := filepath.Join(dir, "appserver.jsonl")
	journal, err := os.OpenFile(journalPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return result, err
	}
	if nativeTurn.CompletedAt != nil && nativeTurn.Status != "inProgress" {
		start := codexapp.Envelope{Method: "turn/started", Params: json.RawMessage(encode(map[string]any{"threadId": fixed.SessionID, "turn": map[string]any{"id": turnID}}))}
		completed := codexapp.Envelope{Method: "turn/completed", Params: json.RawMessage(encode(map[string]any{"threadId": fixed.SessionID, "turn": nativeTurn}))}
		if _, err = journal.Write(encode(start)); err == nil {
			_, err = journal.Write(encode(completed))
		}
		if err == nil {
			err = journal.Sync()
		}
		_ = journal.Close()
		result.Completed = err == nil
		return result, err
	}
	if nativeTurn.Status != "inProgress" {
		_ = journal.Close()
		return result, fmt.Errorf("native turn has unsupported status %q", nativeTurn.Status)
	}
	process, err := attachProcess(*fixed.Process)
	if err != nil {
		_ = journal.Close()
		return result, err
	}
	replay, err := PendingAskEvents(journalPath, fixed.SessionID, turnID, knownCalls)
	if err != nil {
		_ = journal.Close()
		return result, err
	}
	result.Observer = newObservation(process, client, journal, fixed.SessionID, turnID, replay)
	closeClient = false
	return result, nil
}

func PendingAskEvents(path, threadID, turnID string, knownCalls []string) ([]codexapp.Envelope, error) {
	journal, err := ReadAskJournal(path, threadID, turnID, func(e codexapp.Envelope) error { _, err := ParseAskCall(e, threadID, turnID); return err })
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, id := range knownCalls {
		known[id] = true
	}
	var replay []codexapp.Envelope
	for _, e := range journal.Calls {
		call, _ := ParseAskCall(e, threadID, turnID)
		if !journal.Completed[call.Route.CallID] && !known[call.Route.CallID] {
			replay = append(replay, e)
		}
	}
	return replay, nil
}

func (b Backend) Recover(ctx context.Context, fixed Binding, commandID, turnID string, knownCalls []string) (Recovery, error) {
	result, err := b.Reattach(ctx, fixed, commandID, turnID, knownCalls)
	if err != nil && b.ManagedProcessAbsent(result.Binding) {
		result.Dead = true
		if executionbackend.StableExecutionID(commandID) && ValidNativeID(result.Binding.SessionID) && ValidNativeID(result.TurnID) {
			path := filepath.Join(b.Config.RunDir, "runs", b.Config.RunID, "turns", commandID, "appserver.jsonl")
			if outcome, readErr := ReadAppServerOutcome(path, result.Binding.SessionID, result.TurnID); readErr == nil && outcome.Completed {
				result.Completed = true
				err = nil
			}
		}
	}
	return result, err
}
