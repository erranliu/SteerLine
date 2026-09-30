package steerline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

type sqliteTx struct {
	ctx context.Context
	tx  *sql.Tx
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func encodePayload(v any) ([]byte, error) { return json.Marshal(v) }

func requireIdentityWrite(result sql.Result, err error, message string) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New(message)
	}
	return nil
}
func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func sameOperationRequest(a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	var compactA, compactB bytes.Buffer
	if json.Compact(&compactA, a) != nil || json.Compact(&compactB, b) != nil {
		return false
	}
	return bytes.Equal(compactA.Bytes(), compactB.Bytes())
}
func isConstraintError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "constraint")
}
