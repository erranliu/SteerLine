package codexworker

import (
	"encoding/json"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
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

func NewAppServerBindingEnvelope(identity AppServerBindingIdentity, binding AppServerBinding) (executionbackend.ImplementationDataEnvelope, error) {
	if identity.Generation == "" || binding.SessionID != "" && !ValidNativeID(binding.SessionID) ||
		binding.Endpoint == "" && (binding.SessionID == "" || binding.Generation != "") ||
		binding.Endpoint != "" && binding.Generation == "" {
		return executionbackend.ImplementationDataEnvelope{}, errors.New("app-server binding is incomplete")
	}
	if binding.Endpoint != "" {
		if err := codexapp.ValidateEndpoint(binding.Endpoint); err != nil {
			return executionbackend.ImplementationDataEnvelope{}, err
		}
	}
	payload, err := json.Marshal(binding)
	if err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	return executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{
		CodecVersion: 1, Purpose: executionbackend.ImplementationResourcePurpose,
		ImplementationKey: ExecutionBindingCodecKey, ProjectID: identity.ProjectID,
		ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID,
		Generation: identity.Generation, ConfigDigest: identity.ConfigDigest,
	}, payload)
}

func DecodeAppServerBindingEnvelope(envelope executionbackend.ImplementationDataEnvelope, identity AppServerBindingIdentity) (AppServerBinding, error) {
	if envelope.ImplementationKey != ExecutionBindingCodecKey || envelope.CodecVersion != 1 {
		return AppServerBinding{}, errors.New("unsupported app-server binding codec")
	}
	if err := envelope.ValidateResourceIdentity(identity.ProjectID, identity.ExecutionID, identity.AttemptID, identity.Generation, identity.ConfigDigest); err != nil {
		return AppServerBinding{}, err
	}
	var binding AppServerBinding
	if err := executionbackend.DecodeStrictJSON(envelope.Payload, &binding, "app-server binding payload has trailing JSON"); err != nil {
		return AppServerBinding{}, err
	}
	if binding.SessionID != "" && !ValidNativeID(binding.SessionID) ||
		binding.Endpoint == "" && (binding.SessionID == "" || binding.Generation != "") ||
		binding.Endpoint != "" && binding.Generation == "" {
		return AppServerBinding{}, errors.New("app-server binding differs from its fixed native identity")
	}
	if binding.Endpoint != "" {
		if err := codexapp.ValidateEndpoint(binding.Endpoint); err != nil {
			return AppServerBinding{}, err
		}
	}
	return binding, nil
}
