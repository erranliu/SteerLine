package app

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func (c Config) resolveSteerAgentExecution(ctx context.Context, workflow runnercontract.Workflow, accessMode string) (runnercontract.ExecutionInput, error) {
	settings, err := executionconfig.SteerAgentSettings(c.RunnerExecution.SteerAgent)
	if err != nil {
		return runnercontract.ExecutionInput{}, err
	}
	resolved, err := c.runtimeNative.Resolve(ctx, runnercontract.ExecutionConfigurationRequest{
		Operation:     "compose",
		BaseExecution: executionconfig.SteerAgentBase(c.executionConfig(ctx).Profiles),
		Workflow:      workflow,
		Selection: &runnercontract.ExecutionSelection{
			Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Agent: "codex",
			Model: &settings.Model, ReasoningEffort: &settings.ReasoningEffort, AccessMode: accessMode,
		},
	})
	if err != nil {
		return runnercontract.ExecutionInput{}, err
	}
	if resolved.Execution == nil || resolved.View.Model == nil || *resolved.View.Model != settings.Model ||
		resolved.View.ReasoningEffort == nil || *resolved.View.ReasoningEffort != settings.ReasoningEffort {
		return runnercontract.ExecutionInput{}, errors.New("Runner 未返回所选的 SteerAgent 模型和思考强度")
	}
	return *resolved.Execution, nil
}

func (c Config) prepareSteerAgentGeneration(ctx context.Context) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
	execution, err := c.resolveSteerAgentExecution(ctx, runnercontract.Workflow{ID: "generate", Version: 1}, "read_only")
	if err != nil {
		return runnercontract.ExecutionInput{}, nil, err
	}
	return execution, c.runtimeNative.Catalog, nil
}
