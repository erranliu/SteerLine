package orca_remote_worker

import (
	"encoding/json"
	"errors"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const PolicyResourcesCodecKey = "orca.remote-native-resources"

type PolicyResourceIdentity struct{ ProjectID, ExecutionID, AttemptID, Generation, ConfigDigest string }
type NativeResourcePayload struct {
	Target             string `json:"target,omitempty"`
	EnvironmentID      string `json:"environment_id"`
	ConnectionID       string `json:"connection_id"`
	RepositorySelector string `json:"repository_selector"`
	Workspace          string `json:"workspace"`
	Agent              string `json:"agent"`
	Model              string `json:"model"`
	Effort             string `json:"effort"`
	LaunchSource       string `json:"launch_source"`
}

// BuildNativeResource uses the existing opaque resource envelope to freeze the
// remote launch authority. It does not compile center-local sandbox arguments,
// probe an agent executable or interpret any provider permission mode.
func BuildNativeResource(identity PolicyResourceIdentity, profileID, accepted, workspace, agent string, data []executionbackend.ImplementationDataEnvelope) (executionbackend.ImplementationDataEnvelope, error) {
	var remotePayload []byte
	for _, e := range data {
		if e.ImplementationKey != ProfileCodecKey {
			return executionbackend.ImplementationDataEnvelope{}, errors.New("remote resource cannot consume a foreign implementation codec")
		}
		if err := e.ValidateProfileIdentity(identity.ProjectID, profileID, accepted); err != nil {
			return executionbackend.ImplementationDataEnvelope{}, err
		}
		if remotePayload != nil || e.CodecVersion != ProfileCodecVersion {
			return executionbackend.ImplementationDataEnvelope{}, errors.New("remote resource profile codec mismatch")
		}
		remotePayload = e.Payload
	}
	peer, err := DecodeProfileConfig(remotePayload)
	if err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	if err := ValidateProfileSelection(agent, remotePayload); err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	payload := NativeResourcePayload{Target: peer.Target, EnvironmentID: peer.EnvironmentID, ConnectionID: peer.ConnectionID, RepositorySelector: peer.RepositorySelector, Workspace: workspace, Agent: agent, Model: peer.Model, Effort: peer.ReasoningEffort, LaunchSource: "remote-orca-native"}
	if peer.Target == "local" {
		payload.LaunchSource = "local-orca-native"
	}
	if err := payload.Validate(); err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	return executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{CodecVersion: 1, Purpose: executionbackend.ImplementationResourcePurpose, ImplementationKey: PolicyResourcesCodecKey, ProjectID: identity.ProjectID, ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID, Generation: identity.Generation, ConfigDigest: identity.ConfigDigest}, raw)
}

func (p NativeResourcePayload) Validate() error {
	if err := (Settings{Target: p.Target, EnvironmentID: p.EnvironmentID, ConnectionID: p.ConnectionID, RepositorySelector: p.RepositorySelector}).validateTarget(); err != nil {
		return err
	}
	source := "remote-orca-native"
	if p.Target == "local" {
		source = "local-orca-native"
	}
	if p.Workspace == "" || strings.TrimSpace(p.Agent) == "" || (p.Effort != "" && p.Model == "") || p.LaunchSource != source {
		return errors.New("remote native resource authority is incomplete")
	}
	return nil
}

func DecodeNativeResource(e executionbackend.ImplementationDataEnvelope, identity PolicyResourceIdentity) (NativeResourcePayload, error) {
	var payload NativeResourcePayload
	if e.ImplementationKey != PolicyResourcesCodecKey || e.CodecVersion != 1 {
		return payload, errors.New("unsupported remote native resource codec")
	}
	if err := e.ValidateResourceIdentity(identity.ProjectID, identity.ExecutionID, identity.AttemptID, identity.Generation, identity.ConfigDigest); err != nil {
		return payload, err
	}
	if err := executionbackend.DecodeStrictJSON(e.Payload, &payload, "remote resource has trailing JSON"); err != nil {
		return payload, err
	}
	return payload, payload.Validate()
}
