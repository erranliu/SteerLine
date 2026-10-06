package host

import (
	"bytes"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func decode(data []byte, v any) error {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	return executionbackend.DecodeStrictJSON(data, v, "expected exactly one JSON value")
}
