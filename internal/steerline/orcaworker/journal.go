package orcaworker

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

const maxJournalRecord = 8 << 20

type TurnCursor struct {
	Version int   `json:"version"`
	Cursor  int64 `json:"cursor"`
}

type TurnExit struct {
	Version  int    `json:"version"`
	TurnID   string `json:"turn_id"`
	Cursor   int64  `json:"cursor"`
	ExitCode int    `json:"exit_code"`
}

func LoadTurnCursor(events string) (int64, error) {
	dir := filepath.Dir(events)
	raw, err := os.ReadFile(filepath.Join(dir, "orca-cursor.json"))
	if err != nil {
		return 0, err
	}
	var start TurnCursor
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
	scanner.Buffer(make([]byte, 64<<10), maxJournalRecord)
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

func ReadTurnExit(events, turnID string) (*TurnExit, error) {
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(events), "orca-exit.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result TurnExit
	if err := json.Unmarshal(raw, &result); err != nil || result.Version != 1 || result.TurnID != turnID || result.Cursor < 0 || result.ExitCode < 0 {
		return nil, errors.Join(err, errors.New("saved Orca turn exit marker is invalid"))
	}
	return &result, nil
}

func AppendTurnEvent(events string, rowCursor int64, encoded string) error {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) == 0 || len(decoded) > maxJournalRecord {
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
