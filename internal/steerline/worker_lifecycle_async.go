package steerline

func workerProcessEnvironment(c Config, run *Run, base []string) []string {
	blocked := append(c.publicationSettings().AuthenticationRefs, "BEADS_*", "BD_*", "GH_TOKEN", "GITHUB_TOKEN", "GITEA_TOKEN", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_*", "GIT_CONFIG_VALUE_*")
	return unsetEnv(overrideEnv(base, map[string]string{"STEERLINE_WORKFLOW_RUN_ID": run.RunID, "STEERLINE_EXECUTION_ID": run.ExecutionID}), blocked)
}

// WorkerProcessEnvironment applies the center's blocked-secret policy to a
// child process environment without exposing service or store state.
func WorkerProcessEnvironment(c Config, run *Run, inherited []string) []string {
	return workerProcessEnvironment(c, run, inherited)
}
