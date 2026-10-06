package execution

import (
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func implementationInputsFromData(data []executionbackend.ImplementationDataEnvelope) []executionbackend.ImplementationConfigInput {
	out := make([]executionbackend.ImplementationConfigInput, 0, len(data))
	for _, envelope := range data {
		out = append(out, executionbackend.ImplementationConfigInput{
			ImplementationKey: envelope.ImplementationKey,
			CodecVersion:      envelope.CodecVersion,
			Payload:           append([]byte(nil), envelope.Payload...),
		})
	}
	return out
}

func implementationProfilePayload(c Config, key string, directory *ExecutionDirectory) ([]byte, error) {
	if directory == nil {
		return nil, errors.New("execution directory is required")
	}
	if c.selectedProfileID == "" {
		return nil, errors.New("configuration has no selected implementation profile")
	}
	return directory.DecodeProfile(c.ImplementationData, key, ConfigurationProjectIdentity(c), c.selectedProfileID, c.acceptedConfigVersion)
}

// DecodeImplementationConfig asks the injected version directory to validate
// and copy one selected profile payload for an application adapter.
func (c Config) DecodeImplementationConfig(key string, directories ...*ExecutionDirectory) ([]byte, error) {
	if len(directories) > 0 && directories[0] != nil {
		return implementationProfilePayload(c, key, directories[0])
	}
	if c.selectedProfileID == "" {
		return nil, errors.New("configuration has no selected implementation profile")
	}
	for _, envelope := range c.ImplementationData {
		if envelope.ImplementationKey != key {
			continue
		}
		if err := envelope.ValidateProfileIdentity(ConfigurationProjectIdentity(c), c.selectedProfileID, c.acceptedConfigVersion); err != nil {
			return nil, err
		}
		return append([]byte(nil), envelope.Payload...), nil
	}
	return nil, errors.New("selected implementation config is missing")
}

func validateImplementationProfileData(c Config, directory *ExecutionDirectory) error {
	if directory == nil {
		return errors.New("execution directory is required")
	}
	return directory.ValidateProfileData(c)
}
