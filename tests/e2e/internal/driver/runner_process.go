package driver

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"os"
	"time"
)

func ReadRunnerService(ctx context.Context, executionConfig, stateDir string) (runnerclient.ServiceInfo, error) {
	var config struct {
		Runner runnerclient.Selection `json:"runner"`
	}
	raw, err := os.ReadFile(executionConfig)
	if err != nil {
		return runnerclient.ServiceInfo{}, err
	}
	if err = json.Unmarshal(raw, &config); err != nil {
		return runnerclient.ServiceInfo{}, err
	}
	cli, _, err := config.Runner.Open(ctx, stateDir, 10*time.Second)
	if err != nil {
		return runnerclient.ServiceInfo{}, err
	}
	return cli.ServiceStatus(ctx)
}
