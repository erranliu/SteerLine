// Package developerconfig owns optional, home-wide development switches.
// Callers resolve values once at composition and pass the immutable policy on.
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

type Policy struct {
	skipEnvironmentReadiness bool
}

func Resolve(c Config) Policy {
	return Policy{skipEnvironmentReadiness: Available && c.SkipEnvironmentReadiness}
}

func (p Policy) SkipEnvironmentReadiness() bool {
	return Available && p.skipEnvironmentReadiness
}
