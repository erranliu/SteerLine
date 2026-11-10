package sqlite

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func (s *SQLiteStore) SaveMessageObservation(ctx context.Context, m ports.MessageObservation) error {
	if s.readOnly || len(m.Text) > 4096 || !utf8.ValidString(m.Text) || len(m.Diagnostic) > 1024 {
		return errors.New("invalid message observation")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO message_observations(project_identity,candidate_id,run_id,attempt_id,session_id,turn_id,text,observed_at,diagnostic)
	SELECT project_identity,candidate_id,run_id,?,?, ?,?,?,? FROM runner_candidates WHERE project_identity=? AND candidate_id=? AND run_id=? AND json_extract(payload,'$.projection.execution.attempt_id')=? AND json_extract(payload,'$.projection.execution.session_id')=?
	ON CONFLICT(project_identity,candidate_id) DO UPDATE SET run_id=excluded.run_id,attempt_id=excluded.attempt_id,session_id=excluded.session_id,
	turn_id=CASE WHEN excluded.text='' AND message_observations.run_id=excluded.run_id AND message_observations.attempt_id=excluded.attempt_id AND message_observations.session_id=excluded.session_id THEN message_observations.turn_id ELSE excluded.turn_id END,
	text=CASE WHEN excluded.text='' AND message_observations.run_id=excluded.run_id AND message_observations.attempt_id=excluded.attempt_id AND message_observations.session_id=excluded.session_id THEN message_observations.text ELSE excluded.text END,
	observed_at=CASE WHEN excluded.text='' AND message_observations.run_id=excluded.run_id AND message_observations.attempt_id=excluded.attempt_id AND message_observations.session_id=excluded.session_id THEN message_observations.observed_at ELSE excluded.observed_at END,diagnostic=excluded.diagnostic
	WHERE message_observations.run_id<>excluded.run_id OR message_observations.attempt_id<>excluded.attempt_id OR message_observations.session_id<>excluded.session_id OR message_observations.turn_id<>excluded.turn_id OR message_observations.text<>excluded.text OR message_observations.observed_at<>excluded.observed_at OR message_observations.diagnostic<>excluded.diagnostic`, m.AttemptID, m.SessionID, m.TurnID, m.Text, m.ObservedAt, m.Diagnostic, m.Project, m.CandidateID, m.RunID, m.AttemptID, m.SessionID)
	return err
}

func (s *SQLiteStore) LoadMessageObservation(ctx context.Context, m ports.MessageObservation) (ports.MessageObservation, error) {
	db, err := s.asyncActivityPool()
	if err != nil {
		return m, err
	}
	err = db.QueryRowContext(ctx, `SELECT turn_id,text,observed_at,diagnostic FROM message_observations WHERE project_identity=? AND candidate_id=? AND run_id=? AND attempt_id=? AND session_id=?`, m.Project, m.CandidateID, m.RunID, m.AttemptID, m.SessionID).Scan(&m.TurnID, &m.Text, &m.ObservedAt, &m.Diagnostic)
	return m, err
}
