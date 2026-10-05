package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// ReadActivityView pins identity records and bounded events to one WAL snapshot.
func (s Reader) ReadActivityView(ctx context.Context, project, execution, attempt string, after int64, limit, payloadBudget int) (contracts.DomainSnapshot, []storageports.DomainEvent, error) {
	if s.db == nil {
		return contracts.DomainSnapshot{}, nil, errors.New("activity view reader is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contracts.DomainSnapshot{}, nil, err
	}
	defer tx.Rollback()
	snapshot, events, err := readActivityView(ctx, tx, project, execution, attempt, after, limit, payloadBudget)
	if err != nil {
		return contracts.DomainSnapshot{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return contracts.DomainSnapshot{}, nil, err
	}
	return snapshot, events, nil
}

func readActivityView(ctx context.Context, db QueryContext, project, execution, attempt string, after int64, limit, payloadBudget int) (contracts.DomainSnapshot, []storageports.DomainEvent, error) {
	snapshot, err := QuerySnapshot(ctx, db, project)
	if err != nil {
		return contracts.DomainSnapshot{}, nil, err
	}
	events, err := readEvents(ctx, db, project, execution, attempt, after, limit, payloadBudget)
	if err != nil {
		return contracts.DomainSnapshot{}, nil, err
	}
	return snapshot, events, nil
}
