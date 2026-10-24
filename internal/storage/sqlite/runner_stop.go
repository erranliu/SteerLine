package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerstate"
)

// StopRunnerCandidate commits manual withdrawal without authorizing native stop.
func (s *SQLiteStore) StopRunnerCandidate(ctx context.Context, project, id string, expected uint64, requestID, requestedBy string) (runnerstate.Candidate, error) {
	var result runnerstate.Candidate
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		c, err := readRunnerCandidate(ctx, tx.tx, project, id)
		if err != nil {
			return err
		}
		if requestID == "" || requestedBy == "" {
			return nativeErr("invalid_request", "withdrawal requires its original request and requester")
		}
		if c.Withdrawal != nil && c.Withdrawal.RequestID == requestID {
			if c.Withdrawal.ExpectedRevision != expected || c.Withdrawal.RequestedBy != requestedBy {
				return nativeErr("idempotency_conflict", "withdrawal differs from its original request")
			}
			result = c
			return errAtomicReplay
		}
		if expected == 0 || expected > c.Revision {
			return nativeErr("stale_version", "withdrawal observation is ahead of the candidate")
		}
		if c.Participation != "active" || c.StopRequested {
			return nativeErr("candidate_ended", "candidate has already ended")
		}
		var active int
		if err := tx.tx.QueryRowContext(ctx, `SELECT count(*) FROM runner_candidates WHERE project_identity=? AND task_id=? AND json_extract(payload,'$.participation')='active'`, project, c.TaskID).Scan(&active); err != nil {
			return err
		}
		if active <= 1 {
			return nativeErr("last_candidate", "at least one participating candidate must remain")
		}
		c.Participation = "abandoned"
		c.Withdrawal = &runnerstate.Withdrawal{RequestID: requestID, ExpectedRevision: expected, RequestedBy: requestedBy, At: time.Now().UTC()}
		for i := range c.Actions {
			if c.Actions[i].State == "pending" {
				c.Actions[i].State = "cancelled"
			}
		}
		currentRevision := c.Revision
		c.Revision++
		body, err := json.Marshal(c)
		if err != nil {
			return err
		}
		written, err := tx.tx.ExecContext(ctx, `UPDATE runner_candidates SET revision=?,payload=? WHERE project_identity=? AND candidate_id=? AND revision=?`, c.Revision, body, project, id, currentRevision)
		if err := requireIdentityWrite(written, err, "stale_version: candidate changed"); err != nil {
			return err
		}
		result = c
		return nil
	})
	if errors.Is(err, errAtomicReplay) {
		err = nil
	}
	return result, err
}
