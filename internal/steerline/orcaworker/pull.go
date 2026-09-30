package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// PullTurn copies the original terminal's bounded Codex output into the
// durable journal and reports its terminal exit only after the marker is saved.
func PullTurn(ctx context.Context, call MetaCall, identity TerminalIdentity, turnID, events string, done chan<- error, writeAtomic func(string, []byte) error, mapError func(error) error) {
	finish := func(err error) {
		if err != nil && mapError != nil {
			err = mapError(err)
		}
		select {
		case done <- err:
		case <-ctx.Done():
		}
	}
	if exited, err := ReadTurnExit(events, turnID); err != nil {
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
	cursor, err := LoadTurnCursor(events)
	if err != nil {
		finish(err)
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		batch, err := ReadOutput(ctx, call, identity, cursor)
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
				if err := AppendTurnEvent(events, rowCursor, encoded); err != nil {
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
				marker := TurnExit{Version: 1, TurnID: turnID, Cursor: rowCursor, ExitCode: code}
				encoded, err := json.Marshal(marker)
				if err == nil {
					err = writeAtomic(filepath.Join(filepath.Dir(events), "orca-exit.json"), append(encoded, '\n'))
				}
				if err != nil {
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
