package console

import api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"

func consoleFixtureProfile(id, name, host, agent string, display []string) api.ExecutionProfileSummary {
	return api.ExecutionProfileSummary{ID: id, Name: name, Host: host, Agent: agent, Digest: "digest-" + id, ImplementationDisplay: display}
}
func fixturePresentation() api.ProjectPresentation {
	return api.ProjectPresentation{NativeWorkflow: true, TaskAuthority: "native", WorkflowProtocol: fixtureWorkflowVersion, ConfigDigest: "fixture-digest", ShutdownGrace: "10s", DefaultProfileID: "project", Profiles: []api.ExecutionProfileSummary{consoleFixtureProfile("project", "Fixture", "local", "codex", []string{"Model：未在全局配置固定", "Reasoning effort：high", "Sandbox：workspace-write"})}}
}

const fixtureWorkflowVersion = "worker-flow-v1"
