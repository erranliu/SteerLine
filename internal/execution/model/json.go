package model

import (
	"bytes"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func decodeJSON(data []byte, value any) error {
	return executionbackend.DecodeStrictJSON(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}), value, "expected exactly one JSON value")
}
