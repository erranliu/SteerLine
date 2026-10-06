package integration

import (
	"context"
	"testing"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	steerline "github.com/erranliu/SteerLine/internal/app/host"
)

func integrationHostConfig(t *testing.T, config steerlineapp.Config) steerline.Config {
	t.Helper()
	hostConfig, err := steerlineapp.BuildHostConfig(context.Background(), config)
	require(t, err)
	return hostConfig
}
