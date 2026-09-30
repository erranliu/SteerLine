package contracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

func TrimBOM(data []byte) []byte {
	return bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
}

// DecodeJSON accepts exactly one value with the current contract's fields.
func DecodeJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(TrimBOM(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("expected exactly one JSON value")
	}
	return nil
}
