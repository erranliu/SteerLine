// Package publication validates caller publication configuration without effects.
package publication

import executionmodel "github.com/erranliu/SteerLine/internal/execution/model"

type Config executionmodel.PRProviderConfig

func (c Config) ValidateRemote(remote string) error {
	return executionmodel.PRProviderConfig(c).ValidateRemote(remote)
}
