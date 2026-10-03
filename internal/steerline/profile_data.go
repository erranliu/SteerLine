package steerline

import (
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func implementationInputsFromData(data []executionapi.ImplementationDataEnvelope) []executionapi.ImplementationConfigInput {
	out := make([]executionapi.ImplementationConfigInput, 0, len(data))
	for _, envelope := range data {
		out = append(out, executionapi.ImplementationConfigInput{
			ImplementationKey: envelope.ImplementationKey,
			CodecVersion:      envelope.CodecVersion,
			Payload:           append([]byte(nil), envelope.Payload...),
		})
	}
	return out
}

func implementationProfilePayload(c Config, key string) ([]byte, error) {
	if c.ExecutionDirectory == nil {
		return nil, errors.New("execution directory is required")
	}
	if c.selectedProfileID == "" {
		return nil, errors.New("configuration has no selected implementation profile")
	}
	return c.ExecutionDirectory.DecodeProfile(c.ImplementationData, key, configurationProjectIdentity(c), c.selectedProfileID, c.acceptedConfigVersion())
}

// DecodeImplementationConfig asks the injected version directory to validate
// and copy one selected profile payload for an application adapter.
func (c Config) DecodeImplementationConfig(key string) ([]byte, error) {
	return implementationProfilePayload(c, key)
}

func validateImplementationProfileData(c Config) error {
	if c.ExecutionDirectory == nil {
		return errors.New("execution directory is required")
	}
	return c.ExecutionDirectory.ValidateProfileData(c)
}
