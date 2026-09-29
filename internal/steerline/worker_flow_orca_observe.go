package steerline

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type workerOrcaTurnCursor struct {
	Version int   `json:"version"`
	Cursor  int64 `json:"cursor"`
}

type workerOrcaTurnExit struct {
	Version  int    `json:"version"`
	TurnID   string `json:"turn_id"`
	Cursor   int64  `json:"cursor"`
	ExitCode int    `json:"exit_code"`
}

func currentWorkerOrcaCursor(ctx context.Context, c Config, identity ProcessIdentity) (int64, error) {
	env, err := workerOrcaCall(ctx, c, "terminal", "read", "--terminal", identity.Terminal, "--json")
	if err != nil {
		return 0, err
	}
	if env.Meta.RuntimeID != identity.RuntimeID {
		return 0, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker output cursor came from another Orca runtime"}
	}
	var result struct {
		Terminal struct {
			Handle     string `json:"handle"`
			NextCursor string `json:"nextCursor"`
			Source     string `json:"source"`
		} `json:"terminal"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return 0, err
	}
	cursor, err := strconv.ParseInt(result.Terminal.NextCursor, 10, 64)
	if err != nil || cursor < 0 || result.Terminal.Handle != identity.Terminal || result.Terminal.Source != "stream" {
		return 0, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "worker terminal did not return a usable output cursor"}
	}
	return cursor, nil
}

func orcaTurnPath(c Config, runID, turnID string) string {
	return filepath.Join(c.RunDir, "runs", runID, "turns", turnID)
}

func prepareOrcaTurnJournal(c Config, run *Run, turnID string, cursor int64) (string, string, error) {
	if run == nil || !stableExecutionID(turnID) || !stableExecutionID(run.ExecutionID) || !stableExecutionID(run.AttemptID) {
		return "", "", errors.New("Orca worker journal lacks fixed identities")
	}
	dir := orcaTurnPath(c, run.RunID, turnID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", err
	}
	events := filepath.Join(dir, "events.jsonl")
	file, err := os.OpenFile(events, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", "", err
	}
	if err := file.Close(); err != nil {
		return "", "", err
	}
	f, identity, err := openJournal(events)
	if err != nil {
		return "", "", err
	}
	_ = f.Close()
	segment := "orca-jsonl-v1:" + identity
	marker := ConversationSourceMarker{Version: 1, ProjectID: c.identity, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, Backend: BackendOrca, Protocol: ExecutionProtocol(run.Protocol), Agent: "codex", TargetID: run.Target.ID, Source: "codex-jsonl", SourceSegment: segment, SourceIdentity: identity, RecordName: "events.jsonl", CreatedAt: now(), ReaderCommand: codexReaderCommand(c)}
	if err := marker.validate(); err != nil {
		return "", "", err
	}
	if err := AtomicWrite(filepath.Join(dir, ConversationSourceMarkerName), append(mustJSON(marker), '\n')); err != nil {
		return "", "", err
	}
	if err := AtomicWrite(filepath.Join(dir, "orca-cursor.json"), append(mustJSON(workerOrcaTurnCursor{Version: 1, Cursor: cursor}), '\n')); err != nil {
		return "", "", err
	}
	return events, identity, nil
}

func loadOrcaTurnCursor(events string) (int64, error) {
	dir := filepath.Dir(events)
	raw, err := os.ReadFile(filepath.Join(dir, "orca-cursor.json"))
	if err != nil {
		return 0, err
	}
	var start workerOrcaTurnCursor
	if err := json.Unmarshal(raw, &start); err != nil || start.Version != 1 || start.Cursor < 0 {
		return 0, errors.Join(err, errors.New("saved Orca turn cursor is invalid"))
	}
	file, err := os.Open(events)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	cursor := start.Cursor
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), JournalMaxRecord)
	for scanner.Scan() {
		var event struct {
			OrcaCursor *int64 `json:"orca_cursor"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil || event.OrcaCursor == nil || *event.OrcaCursor < cursor {
			return 0, errors.Join(err, errors.New("Orca worker journal has an invalid or repeated source cursor"))
		}
		cursor = *event.OrcaCursor + 1
	}
	return cursor, scanner.Err()
}

func readOrcaTurnExit(events, turnID string) (*workerOrcaTurnExit, error) {
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(events), "orca-exit.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result workerOrcaTurnExit
	if err := json.Unmarshal(raw, &result); err != nil || result.Version != 1 || result.TurnID != turnID || result.Cursor < 0 || result.ExitCode < 0 {
		return nil, errors.Join(err, errors.New("saved Orca turn exit marker is invalid"))
	}
	return &result, nil
}

func appendOrcaTurnEvent(events string, rowCursor int64, encoded string) error {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) == 0 || len(decoded) > JournalMaxRecord {
		return errors.Join(err, errors.New("Orca worker output has invalid encoded Codex event"))
	}
	var event map[string]json.RawMessage
	if err := json.Unmarshal(decoded, &event); err != nil || event == nil || event["type"] == nil {
		return errors.Join(err, errors.New("Orca worker output is not a Codex JSON event"))
	}
	event["orca_cursor"] = json.RawMessage(strconv.FormatInt(rowCursor, 10))
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(events, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(line, '\n')); err == nil {
		err = file.Sync()
	}
	return errors.Join(err, file.Close())
}

func pullWorkerOrcaTurn(ctx context.Context, c Config, identity ProcessIdentity, turnID, events string, done chan<- error) {
	finish := func(err error) {
		select {
		case done <- err:
		case <-ctx.Done():
		}
	}
	if exited, err := readOrcaTurnExit(events, turnID); err != nil {
		finish(err)
		return
	} else if exited != nil {
		if exited.ExitCode == 0 {
			finish(nil)
		} else {
			finish(fmt.Errorf("original Orca Codex turn exited with code %d", exited.ExitCode))
		}
		return
	}
	cursor, err := loadOrcaTurnCursor(events)
	if err != nil {
		finish(err)
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		batch, err := readWorkerOrcaOutput(ctx, c, identity, cursor)
		if err != nil {
			if ctx.Err() == nil {
				finish(err)
			}
			return
		}
		for i, raw := range batch.Rows {
			rowCursor := cursor + int64(i)
			line := strings.TrimSpace(raw)
			if encoded, ok := strings.CutPrefix(line, "SLJSON:"+turnID+":"); ok {
				if err := appendOrcaTurnEvent(events, rowCursor, encoded); err != nil {
					finish(err)
					return
				}
				continue
			}
			if exitText, ok := strings.CutPrefix(line, "SLEND:"+turnID+":"); ok {
				code, err := strconv.Atoi(exitText)
				if err != nil || code < 0 {
					finish(errors.Join(err, errors.New("Orca worker turn has invalid exit status")))
					return
				}
				marker := workerOrcaTurnExit{Version: 1, TurnID: turnID, Cursor: rowCursor, ExitCode: code}
				if err := AtomicWrite(filepath.Join(filepath.Dir(events), "orca-exit.json"), append(mustJSON(marker), '\n')); err != nil {
					finish(err)
					return
				}
				if code == 0 {
					finish(nil)
				} else {
					finish(fmt.Errorf("original Orca Codex turn exited with code %d", code))
				}
				return
			}
		}
		cursor = batch.NextCursor
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func observeWorkerOrcaTurn(c Config, run *Run, turnID string, identity ProcessIdentity, events, sourceIdentity string) *Worker {
	ctx, cancel := context.WithCancel(context.Background())
	worker := &Worker{proc: &workerOrcaProcess{config: c, identity: identity}, Events: make(chan WorkerEvent, 64), Done: make(chan error, 1), cancelObservation: cancel, observationDone: make(chan struct{})}
	waited := make(chan error, 1)
	go pullWorkerOrcaTurn(ctx, c, identity, turnID, events, waited)
	go observeDurableWorkerSegment(ctx, worker, events, sourceIdentity, "orca-jsonl-v1:"+sourceIdentity, run.RunID, run.AttemptID, waited)
	return worker
}
