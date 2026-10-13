package app

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"

	"github.com/erranliu/SteerLine/internal/execution"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

var compositionProbeStopped = errors.New("composition probe stopped before process start")

type compositionHost struct {
	b.ManagedProcessHost
	starts, inspections int
	command             []string
	liveness            string
}

func (h *compositionHost) StartManaged(cmd *exec.Cmd) (b.StopHandle, error) {
	h.starts++
	h.command = append([]string(nil), cmd.Args...)
	return nil, compositionProbeStopped
}

func (h *compositionHost) InspectProcess(*b.ProcessIdentity) string {
	h.inspections++
	return h.liveness
}

func TestBuiltinCompositionIndependentCatalogsAndHosts(t *testing.T) {
	first := &compositionHost{liveness: "matching process still exists"}
	second := &compositionHost{liveness: "process absent"}
	d1, err := NewBuiltinDirectory(first)
	require(t, err)
	d2, err := NewBuiltinDirectory(second)
	require(t, err)
	if d1 == d2 || d1.Version() != executionDirectoryVersion || d2.Version() != d1.Version() || first.starts+second.starts+first.inspections+second.inspections != 0 {
		t.Fatal("construction probed a backend or reused its catalog")
	}
	if _, err := NewBuiltinDirectory(nil); err == nil {
		t.Fatal("missing process host accepted")
	}
	for i, d := range []*execution.ExecutionDirectory{d1, d2} {
		entry, err := d.Resolve(execution.Config{Host: "local", Agent: "codex"})
		require(t, err)
		if entry.Key != (execution.ExecutionRegistrationKey{Backend: b.BackendLocal, Agent: "codex", Protocol: b.ProtocolLocalCodexAppServer}) || entry.ProfileKey != "local/local_codex_app_server_v1/codex" || !entry.LocalWorkspace || entry.HostManagedPermissions || entry.ServiceStopBehavior != codexworker.ServiceStopBehavior || !reflect.DeepEqual(entry.ProfileCodecKeys, []string{codexworker.ProfileCodecKey}) {
			t.Fatal("local registration changed", entry.Key)
		}
		backend := entry.Backend(execution.Config{Host: "local", Agent: "codex"})
		local := backend.(localExecutionBackend)
		if local.directory != d {
			t.Fatal("factory captured another catalog")
		}
		ref := b.ExecutionRef{Backend: b.BackendLocal, Protocol: b.ProtocolLocalCodexAppServer, Target: entry.Target(execution.Config{}), External: b.ProcessIdentity{Kind: "os", PID: 123, Created: "original", Executable: "codex"}}
		observed, err := backend.Observe(context.Background(), ref)
		require(t, err)
		want := []string{"live", "exited"}[i]
		if observed.Liveness != want {
			t.Fatal("factory consulted another process host", observed)
		}
	}
	if first.inspections != 1 || second.inspections != 1 || first.starts+second.starts != 0 {
		t.Fatal("observation ownership crossed instances")
	}
}
