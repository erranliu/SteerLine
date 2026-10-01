package codexworker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

const ExecutionBindingCodecKey = "codex.app-server-binding"

type AppServerBinding struct {
	Endpoint   string `json:"endpoint"`
	SessionID  string `json:"session_id,omitempty"`
	Generation string `json:"generation"`
}

type AppServerBindingIdentity struct {
	ProjectID    string
	ExecutionID  string
	AttemptID    string
	Generation   string
	ConfigDigest string
}

func NewAppServerBindingEnvelope(identity AppServerBindingIdentity, binding AppServerBinding) (executionapi.ImplementationDataEnvelope, error) {
	if identity.Generation == "" || binding.SessionID != "" && !ValidNativeID(binding.SessionID) ||
		binding.Endpoint == "" && (binding.SessionID == "" || binding.Generation != "") ||
		binding.Endpoint != "" && binding.Generation == "" {
		return executionapi.ImplementationDataEnvelope{}, errors.New("app-server binding is incomplete")
	}
	payload, err := json.Marshal(binding)
	if err != nil {
		return executionapi.ImplementationDataEnvelope{}, err
	}
	return executionapi.NewImplementationDataEnvelope(executionapi.ImplementationDataEnvelope{
		CodecVersion: 1, Purpose: executionapi.ImplementationResourcePurpose,
		ImplementationKey: ExecutionBindingCodecKey, ProjectID: identity.ProjectID,
		ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID,
		Generation: identity.Generation, ConfigDigest: identity.ConfigDigest,
	}, payload)
}

func DecodeAppServerBindingEnvelope(envelope executionapi.ImplementationDataEnvelope, identity AppServerBindingIdentity) (AppServerBinding, error) {
	if envelope.ImplementationKey != ExecutionBindingCodecKey || envelope.CodecVersion != 1 {
		return AppServerBinding{}, errors.New("unsupported app-server binding codec")
	}
	if err := envelope.ValidateResourceIdentity(identity.ProjectID, identity.ExecutionID, identity.AttemptID, identity.Generation, identity.ConfigDigest); err != nil {
		return AppServerBinding{}, err
	}
	var binding AppServerBinding
	decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&binding); err != nil {
		return AppServerBinding{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return AppServerBinding{}, errors.New("app-server binding payload has trailing JSON")
	}
	if binding.SessionID != "" && !ValidNativeID(binding.SessionID) ||
		binding.Endpoint == "" && (binding.SessionID == "" || binding.Generation != "") ||
		binding.Endpoint != "" && binding.Generation == "" {
		return AppServerBinding{}, errors.New("app-server binding differs from its fixed native identity")
	}
	return binding, nil
}
