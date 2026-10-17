package app

import (
	"bytes"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func decodeStrictJSON(data []byte, value any) error {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if _, err := runnercontract.CanonicalJSON(data); err != nil {
		return err
	}
	return executionbackend.DecodeStrictJSON(data, value, "expected exactly one JSON value")
}
