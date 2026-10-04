package backend

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// DecodeStrictJSON decodes one value without stripping a BOM. Codec owners
// retain their payload validation and diagnostic for any trailing data.
func DecodeStrictJSON(data []byte, value any, trailingMessage string) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New(trailingMessage)
	}
	return nil
}
