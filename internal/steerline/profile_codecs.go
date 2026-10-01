package steerline

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

type implementationProfileCodec struct {
	key       string
	version   int
	normalize func([]byte, executionapi.ExecutionPolicy, string, string) ([]byte, []string, error)
}

var builtinProfileCodecs = map[string]implementationProfileCodec{
	codexworker.ProfileCodecKey: {
		key: codexworker.ProfileCodecKey, version: 1,
		normalize: func(payload []byte, policy executionapi.ExecutionPolicy, host, agent string) ([]byte, []string, error) {
			settings, err := codexworker.DecodeProfileConfig(payload)
			if err != nil {
				return nil, nil, err
			}
			if settings.Executable == "" {
				return nil, nil, errors.New("Codex implementation requires an executable")
			}
			if host == "orca_supervised" {
				if err := codexworker.ValidateManagedArgs(settings.Args); err != nil {
					return nil, nil, err
				}
				if settings.Profile != "" || settings.WindowsSandbox != "" {
					return nil, nil, errors.New("remote Codex profile does not accept local profile or Windows sandbox overrides")
				}
				if settings.ReasoningEffort != "" && !codexworker.ValidReasoningEffort(settings.ReasoningEffort) {
					return nil, nil, errors.New("invalid reasoning_effort")
				}
				encoded, err := codexworker.EncodeProfileConfig(settings)
				display := []string{}
				if settings.Model != "" {
					display = append(display, "Model："+settings.Model)
				}
				if settings.ReasoningEffort != "" {
					display = append(display, "Reasoning effort："+settings.ReasoningEffort)
				}
				return encoded, display, err
			}
			settings, err = codexworker.NormalizeProfileConfig(settings, policy, runtime.GOOS, host, agent)
			if err != nil {
				return nil, nil, err
			}
			encoded, err := codexworker.EncodeProfileConfig(settings)
			if err != nil {
				return nil, nil, err
			}
			permissions, err := codexworker.CompilePolicy(policy, runtime.GOOS, host, agent, settings.WindowsSandbox, settings.Profile, settings.Args)
			if err != nil {
				return nil, nil, err
			}
			display := []string{"权限：" + permissions.Sandbox}
			if permissions.WindowsSandbox != "" {
				display = append(display, "Windows 权限："+permissions.WindowsSandbox)
			}
			if settings.Model != "" {
				display = append(display, "Model："+settings.Model)
			}
			if settings.ReasoningEffort != "" {
				display = append(display, "Reasoning effort："+settings.ReasoningEffort)
			}
			return encoded, display, nil
		},
	},
	orcaworker.ProfileCodecKey: {
		key: orcaworker.ProfileCodecKey, version: 1,
		normalize: func(payload []byte, _ executionapi.ExecutionPolicy, host, _ string) ([]byte, []string, error) {
			settings, err := orcaworker.DecodeProfileConfig(payload)
			if err != nil {
				return nil, nil, err
			}
			if err = orcaworker.ValidateProfileConfig(settings); err != nil {
				return nil, nil, err
			}
			if host == "orca" {
				if err = orcaworker.ValidateLocalTargetArgs(settings.Args); err != nil {
					return nil, nil, err
				}
			}
			encoded, err := orcaworker.EncodeProfileConfig(settings)
			return encoded, nil, err
		},
	},
	remote.ProfileCodecKey: {
		key: remote.ProfileCodecKey, version: 1,
		normalize: func(payload []byte, _ executionapi.ExecutionPolicy, _, _ string) ([]byte, []string, error) {
			settings, err := remote.DecodeProfileConfig(payload)
			if err != nil {
				return nil, nil, err
			}
			encoded, err := remote.EncodeProfileConfig(settings)
			return encoded, []string{"目标环境：" + settings.EnvironmentID, "连接：" + settings.ConnectionID}, err
		},
	},
}

func normalizeImplementationInputs(inputs []executionapi.ImplementationConfigInput, policy executionapi.ExecutionPolicy, host, agent, projectID, profileID, acceptedVersion string) ([]executionapi.ImplementationDataEnvelope, []string, error) {
	if projectID == "" || profileID == "" || acceptedVersion == "" {
		return nil, nil, errors.New("implementation configuration is missing project, profile or accepted version")
	}
	seen := map[string]bool{}
	data := make([]executionapi.ImplementationDataEnvelope, 0, len(inputs))
	display := []string{}
	for _, input := range inputs {
		codec, ok := builtinProfileCodecs[input.ImplementationKey]
		if !ok || input.CodecVersion != codec.version || seen[input.ImplementationKey] {
			return nil, nil, fmt.Errorf("unknown, duplicate or unsupported implementation codec %q/%d", input.ImplementationKey, input.CodecVersion)
		}
		seen[input.ImplementationKey] = true
		payload, projection, err := codec.normalize(input.Payload, policy, host, agent)
		if err != nil {
			return nil, nil, fmt.Errorf("implementation codec %s rejected profile: %w", codec.key, err)
		}
		envelope, err := executionapi.NewImplementationDataEnvelope(executionapi.ImplementationDataEnvelope{
			CodecVersion: codec.version, Purpose: executionapi.ImplementationConfigPurpose,
			ImplementationKey: codec.key, ProjectID: projectID, ProfileID: profileID,
			AcceptedConfigVersion: acceptedVersion,
		}, payload)
		if err != nil {
			return nil, nil, err
		}
		data = append(data, envelope)
		display = append(display, projection...)
	}
	if !seen[codexworker.ProfileCodecKey] {
		return nil, nil, errors.New("execution profile has no Codex implementation configuration")
	}
	if host == "local" && (len(data) != 1 || seen[orcaworker.ProfileCodecKey] || seen[remote.ProfileCodecKey]) ||
		host == "orca" && (!seen[orcaworker.ProfileCodecKey] || seen[remote.ProfileCodecKey]) ||
		host == "orca_supervised" && (!seen[orcaworker.ProfileCodecKey] || !seen[remote.ProfileCodecKey]) {
		return nil, nil, errors.New("execution profile has incomplete implementation configuration")
	}
	if host == "orca" {
		codex, err := codexworker.DecodeProfileConfig(payloadForImplementation(data, codexworker.ProfileCodecKey))
		orca, orcaErr := orcaworker.DecodeProfileConfig(payloadForImplementation(data, orcaworker.ProfileCodecKey))
		if err != nil || orcaErr != nil {
			return nil, nil, errors.New("Orca profile codecs could not be verified")
		}
		if policy.Filesystem.WriteScope != "workspace" || codex.Profile != "" {
			return nil, nil, errors.New("Orca worker-flow requires workspace-write and no Codex profile")
		}
		if agent != "codex" && (codex.Model != "" || codex.Profile != "" || codex.ReasoningEffort != "") {
			return nil, nil, errors.New("Orca model, profile and reasoning_effort overrides require agent=codex")
		}
		if codex.Model != "" && orca.Shell != "powershell.exe" && orca.Shell != "pwsh.exe" {
			return nil, nil, errors.New("fixed Orca Codex launch requires powershell.exe or pwsh.exe")
		}
		if codex.Model != "" && codex.Executable == "" {
			return nil, nil, errors.New("fixed Orca Codex launch requires codex.executable")
		}
	}
	if host == "orca_supervised" {
		codex, err := codexworker.DecodeProfileConfig(payloadForImplementation(data, codexworker.ProfileCodecKey))
		orca, orcaErr := orcaworker.DecodeProfileConfig(payloadForImplementation(data, orcaworker.ProfileCodecKey))
		remoteProfile, remoteErr := remote.DecodeProfileConfig(payloadForImplementation(data, remote.ProfileCodecKey))
		if err != nil {
			return nil, nil, err
		}
		if orcaErr != nil {
			return nil, nil, orcaErr
		}
		if remoteErr != nil {
			return nil, nil, remoteErr
		}
		selected := remote.Selection{Agent: agent, Model: codex.Model, ReasoningEffort: codex.ReasoningEffort, Executable: orca.Executable, Shell: orca.Shell, WaitTimeout: orca.WaitTimeout, Settings: &remote.SelectionSettings{Settings: remote.Settings{ConnectionID: remoteProfile.ConnectionID, EnvironmentID: remoteProfile.EnvironmentID, RepositorySelector: remoteProfile.RepositorySelector, CoordinatorTerminal: remoteProfile.CoordinatorTerminal, ExpectedCodexVersion: remoteProfile.ExpectedCodexVersion, PollInterval: remoteProfile.PollInterval}, RemoteCancelAvailable: remoteProfile.RemoteCancelAvailable, CapabilityReportVersion: remoteProfile.CapabilityReportVersion}}
		if err := selected.Validate(); err != nil {
			return nil, nil, err
		}
		if err := remote.CheckPolicy(policy, selected.Settings.Settings); err != nil {
			return nil, nil, err
		}
	}
	slices.SortFunc(data, func(a, b executionapi.ImplementationDataEnvelope) int {
		return strings.Compare(a.ImplementationKey, b.ImplementationKey)
	})
	return data, display, nil
}

func payloadForImplementation(data []executionapi.ImplementationDataEnvelope, key string) []byte {
	for _, envelope := range data {
		if envelope.ImplementationKey == key {
			return append([]byte(nil), envelope.Payload...)
		}
	}
	return nil
}

func decodeImplementationProfile(data []executionapi.ImplementationDataEnvelope, key, projectID, profileID, acceptedVersion string) ([]byte, error) {
	var found *executionapi.ImplementationDataEnvelope
	for i := range data {
		if data[i].ImplementationKey != key {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("duplicate implementation profile envelope %q", key)
		}
		found = &data[i]
	}
	if found == nil {
		return nil, fmt.Errorf("missing implementation profile envelope %q", key)
	}
	if err := found.ValidateProfileIdentity(projectID, profileID, acceptedVersion); err != nil {
		return nil, err
	}
	codec, ok := builtinProfileCodecs[key]
	if !ok || found.CodecVersion != codec.version {
		return nil, fmt.Errorf("unsupported implementation profile codec %q/%d", key, found.CodecVersion)
	}
	return append([]byte(nil), found.Payload...), nil
}

func implementationInputsFromData(data []executionapi.ImplementationDataEnvelope) []executionapi.ImplementationConfigInput {
	out := make([]executionapi.ImplementationConfigInput, 0, len(data))
	for _, envelope := range data {
		out = append(out, executionapi.ImplementationConfigInput{ImplementationKey: envelope.ImplementationKey, CodecVersion: envelope.CodecVersion, Payload: append([]byte(nil), envelope.Payload...)})
	}
	return out
}

func implementationProfilePayload(c Config, key string) ([]byte, error) {
	if c.selectedProfileID == "" {
		return nil, errors.New("configuration has no selected implementation profile")
	}
	return decodeImplementationProfile(c.ImplementationData, key, configurationProjectIdentity(c), c.selectedProfileID, c.acceptedConfigVersion())
}

func codexProfileSettings(c Config) codexworker.ProfileConfig {
	payload, err := implementationProfilePayload(c, codexworker.ProfileCodecKey)
	if err != nil {
		return codexworker.ProfileConfig{}
	}
	settings, err := codexworker.DecodeProfileConfig(payload)
	if err != nil {
		return codexworker.ProfileConfig{}
	}
	return settings
}

func orcaProfileSettings(c Config) orcaworker.ProfileConfig {
	payload, err := implementationProfilePayload(c, orcaworker.ProfileCodecKey)
	if err != nil {
		return orcaworker.ProfileConfig{}
	}
	settings, err := orcaworker.DecodeProfileConfig(payload)
	if err != nil {
		return orcaworker.ProfileConfig{}
	}
	return settings
}

func remoteProfileSettings(c Config) remote.ProfileConfig {
	payload, err := implementationProfilePayload(c, remote.ProfileCodecKey)
	if err != nil {
		return remote.ProfileConfig{}
	}
	settings, err := remote.DecodeProfileConfig(payload)
	if err != nil {
		return remote.ProfileConfig{}
	}
	return settings
}

func validateImplementationProfileData(c Config) error {
	if c.selectedProfileID == "" {
		return errors.New("configuration has no selected implementation profile")
	}
	inputs := implementationInputsFromData(c.ImplementationData)
	data, _, err := normalizeImplementationInputs(inputs, c.ExecutionPolicy, c.hostName(), c.agentName(), configurationProjectIdentity(c), c.selectedProfileID, c.acceptedConfigVersion())
	if err != nil {
		return err
	}
	if !bytes.Equal(mustJSON(data), mustJSON(c.ImplementationData)) {
		return errors.New("implementation profile payload is not canonical or its identity changed")
	}
	return nil
}
