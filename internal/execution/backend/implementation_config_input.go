package backend

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// ImplementationConfigInput is an input-only envelope used by application
// configuration. Acceptance turns it into an identity-bound persisted envelope.
type ImplementationConfigInput struct {
	ImplementationKey string `json:"implementation_key"`
	CodecVersion      int    `json:"codec_version"`
	Payload           []byte `json:"-"`
}

type implementationConfigInputWire struct {
	ImplementationKey string          `json:"implementation_key"`
	CodecVersion      int             `json:"codec_version"`
	Payload           json.RawMessage `json:"payload"`
}

func (input *ImplementationConfigInput) UnmarshalJSON(data []byte) error {
	var wire implementationConfigInputWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("implementation configuration input has trailing JSON")
	}
	if wire.ImplementationKey == "" || wire.CodecVersion <= 0 || len(wire.Payload) == 0 || len(wire.Payload) > ImplementationDataMaxBytes || !json.Valid(wire.Payload) {
		return errors.New("implementation configuration input has an unsupported codec or invalid payload")
	}
	*input = ImplementationConfigInput{ImplementationKey: wire.ImplementationKey, CodecVersion: wire.CodecVersion, Payload: append([]byte(nil), wire.Payload...)}
	return nil
}

func (input ImplementationConfigInput) MarshalJSON() ([]byte, error) {
	if input.ImplementationKey == "" || input.CodecVersion <= 0 || len(input.Payload) == 0 || len(input.Payload) > ImplementationDataMaxBytes || !json.Valid(input.Payload) {
		return nil, errors.New("implementation configuration input has an unsupported codec or invalid payload")
	}
	return json.Marshal(implementationConfigInputWire{ImplementationKey: input.ImplementationKey, CodecVersion: input.CodecVersion, Payload: append(json.RawMessage(nil), input.Payload...)})
}
