package steerline

import "fmt"

type workerFeedbackRoute string

const (
	feedbackLocalAppServer workerFeedbackRoute = "local_app_server"
	feedbackNativeOrca     workerFeedbackRoute = "native_orca"
	feedbackSupervisedOrca workerFeedbackRoute = "supervised_orca"
)

// workerFeedbackRouteFor binds feedback to the execution recorded at claim
// time, including projects that do not use named execution profiles.
func workerFeedbackRouteFor(config Config, run *Run, execution *ExecutionRecord) (workerFeedbackRoute, error) {
	if run == nil || execution == nil {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker feedback has no fixed run or execution"}
	}
	backend, protocol, err := ExecutionBackendKind(config.hostName())
	if err != nil {
		return "", err
	}
	if backend == string(BackendLocal) {
		protocol = string(ProtocolLocalCodexAppServer)
	}
	if run.Host != config.hostName() || run.Agent != config.agentName() || execution.Agent != run.Agent ||
		run.Backend != backend || execution.Backend != backend || run.Protocol != protocol || execution.Protocol != protocol ||
		run.Target.ID == "" || run.Target.ID != execution.Target.ID || run.Target.ID != targetFor(config).ID {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "fixed worker feedback target differs from its persisted execution"}
	}
	switch ExecutionProtocol(protocol) {
	case ProtocolLocalCodexAppServer:
		return feedbackLocalAppServer, nil
	case ProtocolOrcaTerminalV2:
		return feedbackNativeOrca, nil
	case ProtocolOrcaSupervised:
		return feedbackSupervisedOrca, nil
	default:
		return "", fmt.Errorf("unsupported worker feedback protocol %q", protocol)
	}
}
