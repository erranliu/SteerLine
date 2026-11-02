// Package developerconfig owns optional, home-wide development switches.
// Application composition maps these values to business-owned policies.
package developerconfig

import "errors"

type Config struct {
	SkipEnvironmentReadiness bool `json:"skip_environment_readiness"`
}

// Validate rejects development overrides in a release build, including files
// edited outside the UI. The zero configuration is always valid.
func (c Config) Validate() error {
	if !Available && c != (Config{}) {
		return errors.New("developer configuration is disabled in release builds; remove developer overrides")
	}
	return nil
}
