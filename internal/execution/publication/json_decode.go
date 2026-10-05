package publication

import (
	"bytes"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func decodeStrictJSON(data []byte, value any) error {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	return executionbackend.DecodeStrictJSON(data, value, "expected exactly one JSON value")
}
