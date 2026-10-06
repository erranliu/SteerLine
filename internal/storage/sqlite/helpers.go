package sqlite

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func now() time.Time { return time.Now().UTC() }

func executionSessionFacts(v *executionmodel.ExecutionRecord) (string, string) {
	if v == nil {
		return "", ""
	}
	return v.SessionID, v.SessionGeneration
}

func sameRepositoryTarget(a, b string) bool { return a == b }

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func decode(b []byte, v any) error {
	return executionbackend.DecodeStrictJSON(b, v, "expected exactly one JSON value")
}

func digestValue(v any) string { return recordFingerprint(v) }

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
		return errors.Join(storageports.ErrNotFound, err)
	}
	var invalid *taskmodel.Error
	if errors.As(err, &invalid) {
		return err
	}
	var existing *storageports.StorageError
	if errors.As(err, &existing) {
		return err
	}
	var coded interface{ Code() int }
	var pathError *os.PathError
	if errors.As(err, &coded) {
		if coded.Code()&255 == 5 || coded.Code()&255 == 6 {
			code = storageports.StorageLocked
		}
		return &storageports.StorageError{Code: code, Cause: err}
	}
	if errors.As(err, &pathError) {
		return &storageports.StorageError{Code: storageports.StorageIO, Cause: err}
	}
	return err
}

func schemaFailure(err error, message string) error {
	var coded interface{ Code() int }
	if errors.Is(err, sql.ErrNoRows) || errors.As(err, &coded) && (coded.Code()&255 == 1 || coded.Code()&255 == 11 || coded.Code()&255 == 26) {
		return errors.Join(nativeErr("unsupported_schema", message), err)
	}
	return storageFailure(err, storageports.StorageIO)
}

func requiredRecordError(err error, code, message string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return nativeErr(code, message)
	}
	return persistenceError(err)
}
