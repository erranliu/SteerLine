package sqlite

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/erranliu/SteerLine/internal/runnerstate"
)

// StopRunnerCandidate commits caller withdrawal. Native stop facts arrive
// separately through the bridge; no native call belongs in this transaction.
func (s *SQLiteStore) StopRunnerCandidate(ctx context.Context, project, id string, expected uint64) (runnerstate.Candidate, error) {
	var result runnerstate.Candidate
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		c, err := readRunnerCandidate(ctx, tx.tx, project, id)
		if err != nil {
			return err
		}
		if c.Revision != expected {
			return nativeErr("stale_version", "candidate changed before withdrawal")
		}
		if c.StopRequested {
			result = c
			return errAtomicReplay
		}
		c.StopRequested, c.InputsRevoked = true, false
		if c.Participation == "active" {
			c.Participation = "abandoned"
		}
		for i := range c.Actions {
			if c.Actions[i].State == "pending" {
				c.Actions[i].State = "cancelled"
			}
		}
		c.Revision++
		body, err := json.Marshal(c)
		if err != nil {
			return err
		}
		written, err := tx.tx.ExecContext(ctx, `UPDATE runner_candidates SET revision=?,payload=? WHERE project_identity=? AND candidate_id=? AND revision=?`, c.Revision, body, project, id, expected)
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
