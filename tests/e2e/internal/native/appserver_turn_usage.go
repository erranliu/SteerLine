package native

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
)

// AppServerTurnUsage handles the native thread/resume usage snapshot that precedes
// the next turn/started. Only that prefix may refer to an older turn of the same
// session. Foreign sessions, late foreign turns and missing start proof fail.
func AppServerTurnUsage(raw []byte, session, turn string) (Usage, []byte, error) {
	var selected, startEvidence bytes.Buffer
	started := false
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 16<<20)
	for scanner.Scan() {
		var event struct {
			Method string `json:"method"`
			Params struct {
				Thread string `json:"threadId"`
				TurnID string `json:"turnId"`
				Turn   struct {
					ID string `json:"id"`
				} `json:"turn"`
			} `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return Usage{}, nil, err
		}
		if event.Method == "turn/started" && event.Params.Thread == session && event.Params.Turn.ID == turn {
			started = true
			startEvidence.Write(scanner.Bytes())
			startEvidence.WriteByte('\n')
		}
		if event.Method != "thread/tokenUsage/updated" {
			continue
		}
		if event.Params.Thread != session {
			return Usage{}, nil, errors.New("usage crossed the original native session")
		}
		if event.Params.TurnID != turn {
			if started {
				return Usage{}, nil, errors.New("late usage crossed the current native turn")
			}
			continue
		}
		selected.Write(scanner.Bytes())
		selected.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return Usage{}, nil, err
	}
	if !started {
		return Usage{}, nil, errors.New("native usage has no exact current turn/started proof")
	}
	u, evidence, err := AppServerUsage(selected.Bytes(), session, turn)
	return u, append(startEvidence.Bytes(), evidence...), err
}
