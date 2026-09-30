package steerline

import (
	"errors"
	"testing"
)

func TestWorkerFeedbackRouteUsesFixedExecutionIdentity(t *testing.T) {
	for _, test := range []struct {
		host     string
		protocol ExecutionProtocol
		want     workerFeedbackRoute
	}{
		{"local", ProtocolLocalCodexAppServer, feedbackLocalAppServer},
		{"orca", ProtocolOrcaTerminalV2, feedbackNativeOrca},
		{"orca_supervised", ProtocolOrcaSupervised, feedbackSupervisedOrca},
	} {
		t.Run(test.host, func(t *testing.T) {
			config := Config{Host: test.host, Agent: "codex", Orca: OrcaConfig{Executable: "orca", Supervised: &SupervisedOrcaConfig{ConnectionID: "fixed-connection", EnvironmentID: "fixed-environment"}}}
			backend, _, err := ExecutionBackendKind(test.host)
			require(t, err)
			target := targetFor(config)
			run := &Run{Host: test.host, Agent: "codex", Backend: backend, Protocol: string(test.protocol), Target: target}
			execution := &ExecutionRecord{Agent: "codex", Backend: backend, Protocol: string(test.protocol), Target: target}
			route, err := workerFeedbackRouteFor(config, run, execution)
			require(t, err)
			if route != test.want {
				t.Fatalf("route = %q, want %q", route, test.want)
			}
			changed := config
			changed.Host = "local"
			if test.host == "local" {
				changed.Host = "orca"
			}
			if _, err := workerFeedbackRouteFor(changed, run, execution); !feedbackIdentityMismatch(err) {
				t.Fatalf("replacement host routed fixed feedback: %v", err)
			}
			execution.Protocol = string(ProtocolLocalProcess)
			if _, err := workerFeedbackRouteFor(config, run, execution); !feedbackIdentityMismatch(err) {
				t.Fatalf("replacement protocol routed fixed feedback: %v", err)
			}
		})
	}
}

func feedbackIdentityMismatch(err error) bool {
	var backendErr *BackendError
	return errors.As(err, &backendErr) && backendErr.Code == BackendIdentityMismatch
}
