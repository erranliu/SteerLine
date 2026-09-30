package steerline

import "errors"

// workerFlowContext binds one persisted run to its fixed profile and records.
// It is resolved for each operation; no mutable "current worker" slot owns it.
type workerFlowContext struct {
	run       *Run
	workflow  *WorkflowRunRecord
	execution *ExecutionRecord
	attempt   *StepAttemptRecord
	config    Config
}

func (s *Service) workerFlowContext(run *Run) (workerFlowContext, error) {
	if run == nil || run.RunID == "" || run.ExecutionID == "" || run.AttemptID == "" {
		return workerFlowContext{}, errors.New("worker flow has no fixed run identity")
	}
	workflow, execution := s.workflowRecords(run)
	attempt := s.workflowAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil || workflow.WorkflowRunID != run.RunID ||
		execution.ExecutionID != run.ExecutionID || execution.WorkflowRunID != run.RunID || attempt.ExecutionID != execution.ExecutionID {
		return workerFlowContext{}, errors.New("worker flow has no matching persisted execution and attempt")
	}
	config, err := s.executionConfigForRun(run)
	if err != nil {
		return workerFlowContext{}, err
	}
	return workerFlowContext{run: run, workflow: workflow, execution: execution, attempt: attempt, config: copyWorkerFlowConfig(config)}, nil
}

func copyWorkerFlowConfig(c Config) Config {
	c.ExecutableTypes = append([]string(nil), c.ExecutableTypes...)
	c.Git.Args = append([]string(nil), c.Git.Args...)
	c.Codex.Args = append([]string(nil), c.Codex.Args...)
	c.Orca.Args = append([]string(nil), c.Orca.Args...)
	profiles := make([]ExecutionProfile, len(c.ExecutionProfiles))
	copy(profiles, c.ExecutionProfiles)
	for i := range profiles {
		profiles[i].Codex.Args = append([]string(nil), profiles[i].Codex.Args...)
		profiles[i].Orca.Args = append([]string(nil), profiles[i].Orca.Args...)
		if profiles[i].Orca.Supervised != nil {
			supervised := *profiles[i].Orca.Supervised
			profiles[i].Orca.Supervised = &supervised
		}
	}
	c.ExecutionProfiles = profiles
	c.PR.Query.Args = append([]string(nil), c.PR.Query.Args...)
	c.PR.Close.Args = append([]string(nil), c.PR.Close.Args...)
	if c.Orca.Supervised != nil {
		supervised := *c.Orca.Supervised
		c.Orca.Supervised = &supervised
	}
	if c.workerEnv != nil {
		env := make(map[string]string, len(c.workerEnv))
		for key, value := range c.workerEnv {
			env[key] = value
		}
		c.workerEnv = env
	}
	return c
}
