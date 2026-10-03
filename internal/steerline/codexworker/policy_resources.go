package codexworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

const PolicyResourcesCodecKey = "codex.policy-resources"

type PolicyResourceIdentity struct {
	ProjectID    string
	ExecutionID  string
	AttemptID    string
	Generation   string
	ConfigDigest string
}

type ProfileConfigIdentity struct {
	ProjectID             string
	ProfileID             string
	AcceptedConfigVersion string
}

type PolicyResourceBuildRequest struct {
	ProfileData      []executionapi.ImplementationDataEnvelope
	ProfileIdentity  ProfileConfigIdentity
	ResourceIdentity PolicyResourceIdentity
	Policy           executionapi.ExecutionPolicy
	Host             string
	Agent            string
	GOOS             string
	Workspace        string
	LinkedWriteArgs  []string
}

type PolicyVersionProbe func(context.Context, string) (string, error)

type BoundPolicyResources struct {
	Envelope   executionapi.ImplementationDataEnvelope
	WriteRoots []string
}

// PolicyResourcePayload contains native version, permission contract and launch
// settings. The center stores it as opaque, identity-bound codec data.
type PolicyResourcePayload struct {
	NativeVersion string                     `json:"native_version"`
	Contract      string                     `json:"contract"`
	Settings      executionapi.LaunchCommand `json:"settings"`
}

func NewPolicyResourceEnvelope(identity PolicyResourceIdentity, payload PolicyResourcePayload) (executionapi.ImplementationDataEnvelope, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return executionapi.ImplementationDataEnvelope{}, err
	}
	return executionapi.NewImplementationDataEnvelope(executionapi.ImplementationDataEnvelope{
		CodecVersion: 1, Purpose: executionapi.ImplementationResourcePurpose,
		ImplementationKey: PolicyResourcesCodecKey, ProjectID: identity.ProjectID,
		ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID,
		Generation: identity.Generation, ConfigDigest: identity.ConfigDigest,
	}, encoded)
}

func DecodeProfileConfigEnvelope(data []executionapi.ImplementationDataEnvelope, identity ProfileConfigIdentity) (ProfileConfig, error) {
	return profileConfigFromEnvelopes(data, identity)
}

func BuildPolicyResourceEnvelope(ctx context.Context, request PolicyResourceBuildRequest, probe PolicyVersionProbe) (BoundPolicyResources, error) {
	if probe == nil || request.Workspace == "" || request.ResourceIdentity.Generation == "" || request.ResourceIdentity.ConfigDigest == "" {
		return BoundPolicyResources{}, errors.New("permission resource build identity or version probe is incomplete")
	}
	settings, err := profileConfigFromEnvelopes(request.ProfileData, request.ProfileIdentity)
	if err != nil {
		return BoundPolicyResources{}, err
	}
	permissions, err := CompilePolicy(request.Policy, request.GOOS, request.Host, request.Agent, settings.WindowsSandbox, settings.Profile, settings.Args)
	if err != nil {
		return BoundPolicyResources{}, err
	}
	roots, err := WritableRoots(request.LinkedWriteArgs)
	if err != nil {
		return BoundPolicyResources{}, err
	}
	native, err := NativeExecutable(settings.Executable)
	if err != nil {
		return BoundPolicyResources{}, err
	}
	version, err := probe(ctx, native)
	if err != nil {
		return BoundPolicyResources{}, err
	}
	version = strings.TrimSpace(version)
	if err := VerifyPolicyVersion(version); err != nil {
		return BoundPolicyResources{}, err
	}
	if permissions.Sandbox == "workspace-write" {
		roots = append([]string{request.Workspace}, roots...)
	}
	var command executionapi.LaunchCommand
	switch request.Host {
	case "local":
		command = executionapi.LaunchCommand{Executable: native, Args: append(append([]string(nil), settings.Args...), permissions.Args()...), WorkingDirectory: request.Workspace}
	case "orca":
		command, err = ManagedCommand(ManagedCommandOptions{
			Policy: request.Policy, Host: request.Host, Agent: request.Agent, Executable: native,
			Args: settings.Args, WindowsSandbox: permissions.WindowsSandbox, Sandbox: permissions.Sandbox,
			Model: settings.Model, ReasoningEffort: settings.ReasoningEffort,
			WorkingDirectory: request.Workspace, WriteRoots: roots,
		})
	default:
		return BoundPolicyResources{}, errors.New("permission resource codec does not support the selected host")
	}
	if err != nil {
		return BoundPolicyResources{}, err
	}
	payload := PolicyResourcePayload{NativeVersion: version, Contract: permissions.Contract, Settings: command}
	envelope, err := NewPolicyResourceEnvelope(request.ResourceIdentity, payload)
	if err != nil {
		return BoundPolicyResources{}, err
	}
	return BoundPolicyResources{Envelope: envelope, WriteRoots: roots}, nil
}

func profileConfigFromEnvelopes(data []executionapi.ImplementationDataEnvelope, identity ProfileConfigIdentity) (ProfileConfig, error) {
	var found *executionapi.ImplementationDataEnvelope
	for i := range data {
		if data[i].ImplementationKey != ProfileCodecKey {
			continue
		}
		if found != nil {
			return ProfileConfig{}, errors.New("duplicate Codex profile configuration envelope")
		}
		found = &data[i]
	}
	if found == nil {
		return ProfileConfig{}, errors.New("Codex profile configuration envelope is missing")
	}
	if found.CodecVersion != 1 {
		return ProfileConfig{}, errors.New("unsupported Codex profile configuration version")
	}
	if err := found.ValidateProfileIdentity(identity.ProjectID, identity.ProfileID, identity.AcceptedConfigVersion); err != nil {
		return ProfileConfig{}, err
	}
	return DecodeProfileConfig(found.Payload)
}

func DecodePolicyResourceEnvelope(envelope executionapi.ImplementationDataEnvelope, identity PolicyResourceIdentity) (PolicyResourcePayload, error) {
	if envelope.ImplementationKey != PolicyResourcesCodecKey || envelope.CodecVersion != 1 {
		return PolicyResourcePayload{}, errors.New("unsupported permission resource codec")
	}
	if err := envelope.ValidateResourceIdentity(identity.ProjectID, identity.ExecutionID, identity.AttemptID, identity.Generation, identity.ConfigDigest); err != nil {
		return PolicyResourcePayload{}, err
	}
	var payload PolicyResourcePayload
	if err := executionapi.DecodeStrictJSON(envelope.Payload, &payload, "permission resource payload has trailing JSON"); err != nil {
		return PolicyResourcePayload{}, err
	}
	if (payload.Contract != PolicyContract && payload.Contract != FullAccessPolicyContract) || VerifyPolicyVersion(payload.NativeVersion) != nil || payload.Settings.Executable == "" {
		return PolicyResourcePayload{}, errors.New("permission resource evidence is incomplete or unsupported")
	}
	return payload, nil
}
