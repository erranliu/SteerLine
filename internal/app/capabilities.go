package app

import (
	"github.com/erranliu/SteerLine/internal/developerconfig"
	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
)

// resolveCapabilities maps configuration and build availability to immutable
// business policies once, before services, writers and query ports are composed.
func (c *Config) resolveCapabilities() {
	c.environmentPolicy = env.NewAdmissionPolicy(!(developerconfig.Available && c.RunnerExecution.Developer.SkipEnvironmentReadiness))
}
