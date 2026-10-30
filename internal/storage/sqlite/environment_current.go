package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func validEnvironmentKey(key string) bool {
	b, err := hex.DecodeString(key)
	return err == nil && len(b) == 32 && key == strings.ToLower(key)
}
func readEnvironmentCurrent(ctx context.Context, db schemaQuery, project, key string) (env.Current, error) {
	var current env.Current
	var revision uint64
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT revision,payload FROM project_environment_current WHERE project_identity=? AND environment_key=?`, project, key).Scan(&revision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return current, env.ErrNotFound
	}
	if err != nil {
		return current, err
	}
	if json.Unmarshal(raw, &current) != nil || current.Project != project || current.Key != key || current.Revision != revision || revision == 0 || !validEnvironmentKey(key) || current.ActivityID != "" && !env.IDPattern.MatchString(current.ActivityID) {
		return env.Current{}, errors.New("environment current binding differs")
	}
	return current, nil
}
func (s *SQLiteStore) CurrentEnvironment(ctx context.Context, project, key string) (env.Current, error) {
	return readEnvironmentCurrent(ctx, s.db, project, key)
}
func writeEnvironmentCurrent(ctx context.Context, tx *sql.Tx, current env.Current, expected uint64) error {
	if current.Project == "" || !validEnvironmentKey(current.Key) || current.Revision != expected+1 {
		return errors.New("invalid environment current reference")
	}
	old, err := readEnvironmentCurrent(ctx, tx, current.Project, current.Key)
	if err != nil && !errors.Is(err, env.ErrNotFound) {
		return err
	}
	if errors.Is(err, env.ErrNotFound) && expected != 0 || err == nil && old.Revision != expected {
		return errors.New("stale_revision: environment selection changed")
	}
	raw, err := json.Marshal(current)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO project_environment_current(project_identity,environment_key,revision,payload) VALUES(?,?,?,?) ON CONFLICT(project_identity,environment_key) DO UPDATE SET revision=excluded.revision,payload=excluded.payload`, current.Project, current.Key, current.Revision, raw)
	return err
}

// StartEnvironment is the only operation which selects a new adaptation.
func (s *SQLiteStore) StartEnvironment(ctx context.Context, record env.Record, expected uint64) error {
	key, err := env.Key(record.Execution)
	if err != nil {
		return err
	}
	if record.Kind != "adaptation" || record.Revision != 1 {
		return errors.New("only a new adaptation can become current")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error {
		current, err := readEnvironmentCurrent(ctx, tx.tx, record.Project, key)
		if err != nil && !errors.Is(err, env.ErrNotFound) {
			return err
		}
		if current.ActivityID != "" {
			old, err := readEnvironment(ctx, tx.tx, record.Project, current.ActivityID)
			if err != nil {
				return err
			}
			if !old.Settled() {
				return errors.New("original environment activity remains unsettled")
			}
		}
		if err := saveEnvironmentTx(ctx, tx.tx, record, 0); err != nil {
			return err
		}
		return writeEnvironmentCurrent(ctx, tx.tx, env.Current{Project: record.Project, Key: key, Revision: expected + 1, ActivityID: record.ID, UpdatedAt: record.CreatedAt}, expected)
	})
}

// InvalidateEnvironment commits the revocation and the original command receipt
// together. Observation of a late ready activity cannot undo this revocation.
func (s *SQLiteStore) InvalidateEnvironment(ctx context.Context, project, key, reason string, expected uint64, command ports.CommandRecord) error {
	if strings.TrimSpace(reason) == "" || len(reason) > 4096 || !validEnvironmentKey(key) || command.ProjectIdentity != project || command.Status != "applied" || len(command.Result) == 0 {
		return errors.New("invalid environment invalidation")
	}
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		old, found, err := tx.LookupCommand(ctx, project, command.IdempotencyKey)
		if err != nil {
			return err
		}
		if !found || old.CommandID != command.CommandID || old.Kind != "project_environment" || !bytes.Equal(old.Payload, command.Payload) {
			return errors.New("original environment command differs")
		}
		if old.Status == "applied" {
			return errAtomicReplay
		}
		current, err := readEnvironmentCurrent(ctx, tx.tx, project, key)
		if err != nil && !errors.Is(err, env.ErrNotFound) {
			return err
		}
		result := env.Current{Project: project, Key: key, Revision: expected + 1, ActivityID: current.ActivityID, Invalidated: true, Reason: reason, UpdatedAt: time.Now().UTC()}
		if err := writeEnvironmentCurrent(ctx, tx.tx, result, expected); err != nil {
			return err
		}
		return tx.UpdateCommand(command)
	})
	if errors.Is(err, errAtomicReplay) {
		return nil
	}
	return err
}

func verifyCandidateEnvironment(ctx context.Context, tx *sql.Tx, project string, proof *env.Admission, execution runnercontract.ExecutionInput) error {
	if proof == nil {
		return nativeErr("environment_not_ready", "项目环境尚未准备")
	}
	key, keyErr := env.Key(execution)
	if keyErr != nil || key != proof.Key {
		return nativeErr("environment_not_ready", "候选执行环境与准备证明不符")
	}
	current, err := readEnvironmentCurrent(ctx, tx, project, proof.Key)
	if err != nil {
		return nativeErr("environment_not_ready", err.Error())
	}
	record, err := readEnvironment(ctx, tx, project, current.ActivityID)
	if err != nil {
		return nativeErr("environment_not_ready", err.Error())
	}
	actual, err := env.Qualify(current, record, proof.Basis)
	if err != nil {
		return nativeErr("environment_not_ready", err.Error())
	}
	if actual != *proof {
		return nativeErr("environment_not_ready", "环境准备引用在预留前已变化")
	}
	return nil
}
