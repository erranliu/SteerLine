package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// RunnerEvents exposes caller-owned copies of the CLI event contract in stable
// local order. It does not read native logs or the Runner database.
func (s Reader) RunnerEvents(ctx context.Context, project, candidate string) ([]runnercontract.Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.payload,c.run_id FROM runner_events e JOIN runner_candidates c ON c.project_identity=e.project_identity AND c.candidate_id=e.candidate_id WHERE e.project_identity=? AND e.candidate_id=? ORDER BY seq`, project, candidate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []runnercontract.Event{}
	for rows.Next() {
		var raw []byte
		var run string
		if err = rows.Scan(&raw, &run); err != nil {
			return nil, err
		}
		var e runnercontract.Event
		if err = json.Unmarshal(raw, &e); err != nil {
			return nil, err
		}
		if e.RunID != run {
			return nil, errors.New("Runner event differs from fixed candidate Run")
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
