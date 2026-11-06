package app

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func (c Config) describeProfile(ctx context.Context, profile RunnerProfile) (runnercontract.ExecutionConfigurationResult, error) {
	return c.runtimeNative.Resolve(ctx, runnercontract.ExecutionConfigurationRequest{Operation: "describe", Execution: &profile.Execution, Workflow: runnercontract.Workflow{ID: profile.Workflow.ID, Version: profile.Workflow.Version}})
}
func (c Config) profileEnvironmentKey(ctx context.Context, profile RunnerProfile) (string, error) {
	result, err := c.runtimeNative.Lookup(runnercontract.ExecutionConfigurationRequest{Operation: "describe", Execution: &profile.Execution, Workflow: runnercontract.Workflow{ID: profile.Workflow.ID, Version: profile.Workflow.Version}})
	if err != nil {
		return "", err
	}
	if result.Identity == nil {
		return "", errors.New("Runner未返回原执行配置身份")
	}
	return runnercontract.ExecutionContextKey(profile.Execution, *result.Identity)
}
func (c Config) deriveProfile(ctx context.Context, profile RunnerProfile, workflow, permission string) (runnercontract.ExecutionConfigurationResult, error) {
	return c.runtimeNative.Resolve(ctx, runnercontract.ExecutionConfigurationRequest{Operation: "derive", Execution: &profile.Execution, Workflow: runnercontract.Workflow{ID: workflow, Version: 1}, AccessMode: permission})
}
func bindNativeConfiguration(c Config, cli runnerclient.Client) Config {
	if c.runtimeNative == nil {
		c.runtimeNative = &runnerclient.ConfigurationSnapshot{Client: cli, Catalog: c.runtimeCatalog}
	}
	return c
}
