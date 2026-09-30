package steerline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func currentWorkerOrcaCursor(ctx context.Context, c Config, identity ProcessIdentity) (int64, error) {
	cursor, err := orcaworker.CurrentCursor(ctx, orcaWorkerMetaCaller(c), identity.Terminal, identity.RuntimeID)
	return cursor, workerOrcaBackendError(err)
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
	if err := AtomicWrite(filepath.Join(dir, "orca-cursor.json"), append(mustJSON(orcaworker.TurnCursor{Version: 1, Cursor: cursor}), '\n')); err != nil {
		return "", "", err
	}
	return events, identity, nil
}

func pullWorkerOrcaTurn(ctx context.Context, c Config, identity ProcessIdentity, turnID, events string, done chan<- error) {
	finish := func(err error) {
		select {
		case done <- err:
		case <-ctx.Done():
		}
	}
	if exited, err := orcaworker.ReadTurnExit(events, turnID); err != nil {
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
	cursor, err := orcaworker.LoadTurnCursor(events)
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
				if err := orcaworker.AppendTurnEvent(events, rowCursor, encoded); err != nil {
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
				marker := orcaworker.TurnExit{Version: 1, TurnID: turnID, Cursor: rowCursor, ExitCode: code}
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
