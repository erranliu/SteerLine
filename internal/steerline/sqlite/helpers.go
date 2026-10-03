package sqlite

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/task"
)

func now() time.Time { return time.Now().UTC() }
func executionSessionFacts(v *ExecutionRecord) (string, string) {
	if v == nil {
		return "", ""
	}
	return v.SessionID, v.SessionGeneration
}
func sameRepositoryTarget(a, b string) bool { return a == b }
func mustJSON(v any) []byte                 { b, _ := json.Marshal(v); return b }
func decode(b []byte, v any) error          { return contracts.DecodeJSON(b, v) }
func digestValue(v any) string              { return recordFingerprint(v) }

func nonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func storageFailure(err error, code string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return errors.Join(persistence.ErrNotFound, err)
	}
	var invalid *task.Error
	if errors.As(err, &invalid) {
		return err
	}
	var existing *persistence.StorageError
	if errors.As(err, &existing) {
		return err
	}
	var coded interface{ Code() int }
	var pathError *os.PathError
	if errors.As(err, &coded) {
		if coded.Code()&255 == 5 || coded.Code()&255 == 6 {
			code = persistence.StorageLocked
		}
		return &persistence.StorageError{Code: code, Cause: err}
	}
	if errors.As(err, &pathError) {
		return &persistence.StorageError{Code: persistence.StorageIO, Cause: err}
	}
	return err
}

func schemaFailure(err error, message string) error {
	var coded interface{ Code() int }
	if errors.Is(err, sql.ErrNoRows) || errors.As(err, &coded) && (coded.Code()&255 == 1 || coded.Code()&255 == 11 || coded.Code()&255 == 26) {
		return errors.Join(nativeErr("unsupported_schema", message), err)
	}
	return storageFailure(err, persistence.StorageIO)
}

func requiredRecordError(err error, code, message string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return nativeErr(code, message)
	}
	return persistenceError(err)
}
