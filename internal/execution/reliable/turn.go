package reliable

import (
	"errors"
	"time"
)

type TurnState struct {
	SessionID, PendingTurnID, CurrentTurnID, LastStoppedTurnID, Status string
	Number                                                             uint64
	UpdatedAt                                                          time.Time
}
type TurnAuthority struct{ Allowed, OriginalOnly bool }

func BeginTurn(s TurnState, sessionID, turnID string, authority TurnAuthority, at time.Time) (TurnState, error) {
	if sessionID == "" || turnID == "" {
		return s, errors.New("worker turn start lacks native session or turn identity")
	}
	if s.PendingTurnID != "" && s.PendingTurnID != turnID {
		return s, errors.New("worker turn start differs from the persisted send intent")
	}
	if s.SessionID != "" && s.SessionID != sessionID {
		return s, errors.New("worker session replacement is not an ordinary continuation")
	}
	if s.CurrentTurnID == turnID {
		if s.LastStoppedTurnID == turnID {
			return s, ErrStale
		}
		return s, nil
	}
	if s.CurrentTurnID != "" && s.LastStoppedTurnID != s.CurrentTurnID {
		return s, errors.New("previous worker turn has not stopped")
	}
	if !authority.Allowed {
		return s, errors.New("worker turn start is not authorized in the current state")
	}
	if authority.OriginalOnly && s.PendingTurnID != turnID {
		return s, errors.New("reconciling start has no matching original accepted input")
	}
	s.SessionID, s.PendingTurnID, s.CurrentTurnID = sessionID, "", turnID
	s.Number++
	s.Status, s.UpdatedAt = "active", at
	return s, nil
}
func CurrentTurn(s TurnState, turnID string) error {
	if turnID == "" || s.CurrentTurnID != turnID || s.LastStoppedTurnID == turnID {
		return ErrStale
	}
	return nil
}
func StopTurn(s TurnState, turnID string, allowed bool, at time.Time) (TurnState, error) {
	if turnID == "" || s.CurrentTurnID != turnID {
		return s, ErrStale
	}
	if s.LastStoppedTurnID == turnID {
		return s, nil
	}
	if !allowed {
		return s, errors.New("worker turn stop is not authorized in the current state")
	}
	s.LastStoppedTurnID, s.Status, s.UpdatedAt = turnID, "idle", at
	return s, nil
}
