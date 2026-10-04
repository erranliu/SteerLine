package steerlineapp

import (
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestWorkerFlowModeUsesFixedExecutionIdentity(t *testing.T) {
	d, err := NewBuiltinDirectory(NewProcessHost(time.Second))
	require(t, err)
	for _, test := range []struct {
		host string
		want executionbackend.ExecutionProtocol
	}{{"local", executionbackend.ProtocolLocalCodexAppServer}, {"orca", executionbackend.ProtocolOrcaTerminalV2}, {"orca_supervised", executionbackend.ProtocolOrcaSupervised}} {
		t.Run(test.host, func(t *testing.T) {
			entry, err := d.Resolve(Config{Host: test.host, Agent: "codex"})
			require(t, err)
			if entry.Key.Protocol != test.want || entry.Lifecycle == nil {
				t.Fatalf("native route has wrong protocol or missing lifecycle: %+v", entry)
			}
			fixed, err := d.ResolveIdentity(entry.Key)
			require(t, err)
			if fixed.Key != entry.Key {
				t.Fatal("fixed native key selected a replacement")
			}
		})
	}
}
