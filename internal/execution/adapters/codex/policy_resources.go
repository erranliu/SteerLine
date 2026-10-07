package codexworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
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
	ProfileData      []executionbackend.ImplementationDataEnvelope
	ProfileIdentity  ProfileConfigIdentity
	ResourceIdentity PolicyResourceIdentity

	Host            string
	Agent           string
	GOOS            string
	Workspace       string
	LinkedWriteArgs []string
}

type PolicyVersionProbe func(context.Context, string) (string, error)

type BoundPolicyResources struct {
	Envelope   executionbackend.ImplementationDataEnvelope
	WriteRoots []string
}

// PolicyResourcePayload contains native version, permission contract and launch
// settings. The center stores it as opaque, identity-bound codec data.
type PolicyResourcePayload struct {
	NativeVersion string                         `json:"native_version"`
	Contract      string                         `json:"contract"`
	Settings      executionbackend.LaunchCommand `json:"settings"`
}

func NewPolicyResourceEnvelope(identity PolicyResourceIdentity, payload PolicyResourcePayload) (executionbackend.ImplementationDataEnvelope, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	return executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{
		CodecVersion: 2, Purpose: executionbackend.ImplementationResourcePurpose,
		ImplementationKey: PolicyResourcesCodecKey, ProjectID: identity.ProjectID,
		ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID,
		Generation: identity.Generation, ConfigDigest: identity.ConfigDigest,
	}, encoded)
}

func DecodeProfileConfigEnvelope(data []executionbackend.ImplementationDataEnvelope, identity ProfileConfigIdentity) (ProfileConfig, error) {
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
	permissions, err := NativePermissions(request.GOOS, request.Host, request.Agent)
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
	var command executionbackend.LaunchCommand
	switch request.Host {
	case "local":
		command = executionbackend.LaunchCommand{Executable: native, Args: append([]string(nil), permissions.Args()...), WorkingDirectory: request.Workspace}
	case "orca":
		command, err = ManagedCommand(ManagedCommandOptions{
			Host: request.Host, Agent: request.Agent, Executable: native,
			Args: nil, WindowsSandbox: permissions.WindowsSandbox, Sandbox: permissions.Sandbox,
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

func profileConfigFromEnvelopes(data []executionbackend.ImplementationDataEnvelope, identity ProfileConfigIdentity) (ProfileConfig, error) {
	var found *executionbackend.ImplementationDataEnvelope
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
	if found.CodecVersion != ProfileCodecVersion {
		return ProfileConfig{}, errors.New("unsupported Codex profile configuration version")
	}
	if err := found.ValidateProfileIdentity(identity.ProjectID, identity.ProfileID, identity.AcceptedConfigVersion); err != nil {
		return ProfileConfig{}, err
	}
	return DecodeProfileConfig(found.Payload)
}

func DecodePolicyResourceEnvelope(envelope executionbackend.ImplementationDataEnvelope, identity PolicyResourceIdentity) (PolicyResourcePayload, error) {
	if envelope.ImplementationKey != PolicyResourcesCodecKey || envelope.CodecVersion != 2 {
		return PolicyResourcePayload{}, errors.New("unsupported permission resource codec")
	}
	if err := envelope.ValidateResourceIdentity(identity.ProjectID, identity.ExecutionID, identity.AttemptID, identity.Generation, identity.ConfigDigest); err != nil {
		return PolicyResourcePayload{}, err
	}
	var payload PolicyResourcePayload
	if err := executionbackend.DecodeStrictJSON(envelope.Payload, &payload, "permission resource payload has trailing JSON"); err != nil {
		return PolicyResourcePayload{}, err
	}
	if payload.Contract != PolicyContract || VerifyPolicyVersion(payload.NativeVersion) != nil || payload.Settings.Executable == "" {
		return PolicyResourcePayload{}, errors.New("permission resource evidence is incomplete or unsupported")
	}
	if err := validateNativeLaunchArgs(payload.Settings.Args); err != nil {
		return PolicyResourcePayload{}, err
	}
	return payload, nil
}

func validateNativeLaunchArgs(args []string) error {
	permissions, _ := NativePermissions("windows", "local", "codex")
	expected := map[string]string{}
	for i := 0; i < len(permissions.Args()); i += 2 {
		key, value, _ := strings.Cut(permissions.Args()[i+1], "=")
		expected[key] = value
	}
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--no-daemon", "--strict-config":
		case "--model", "--add-dir", "--cd":
			i++
			if i >= len(args) || args[i] == "" {
				return errors.New("native launch has an incomplete argument")
			}
		case "--config":
			i++
			if i >= len(args) {
				return errors.New("native launch has an incomplete setting")
			}
			key, value, _ := strings.Cut(args[i], "=")
			if key == "model_reasoning_effort" && ValidReasoningEffort(value) {
				continue
			}
			want, exists := expected[key]
			if !exists || value != want || seen[key] {
				return errors.New("native launch setting differs from the fixed contract: " + key)
			}
			seen[key] = true
		default:
			return errors.New("native launch has an unsupported argument")
		}
	}
	if len(seen) != len(expected) {
		return errors.New("native launch is missing fixed sandbox/reviewer settings")
	}
	return nil
}
