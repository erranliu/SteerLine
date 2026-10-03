package contracts

import (
	"bytes"

	"github.com/erranliu/SteerLine/internal/steerline/execution"
)

func TrimBOM(data []byte) []byte {
	return bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
}

// DecodeJSON accepts exactly one value with the current contract's fields.
func DecodeJSON(data []byte, value any) error {
	return execution.DecodeStrictJSON(TrimBOM(data), value, "expected exactly one JSON value")
}
