package codexworker_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/testprotocol"
)

func TestAppServerPendingFeedbackRecoveryUsesNewTurnReceipt(t *testing.T) {
	for _, exists := range []bool{true, false} {
		name := "receipt_lost"
		if exists {
			name = "receipt_saved"
		}
		t.Run(name, func(t *testing.T) {
			workspace, runDir, runID := t.TempDir(), t.TempDir(), newID()
			first, second := "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
			state := &testprotocol.AppServerState{Cwd: workspace, Turns: []map[string]any{{"id": first, "status": "completed", "completedAt": 1}, {"id": second, "status": "completed", "completedAt": 2}}}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require(t, err)
			defer listener.Close()
			go func() {
				for {
					c, err := listener.Accept()
					if err != nil {
						return
					}
					go testprotocol.MockAppServerConnection(c, state)
				}
			}()
			endpoint := "ws://" + listener.Addr().String()
			identity := executionapi.ProcessIdentity{Kind: "os", PID: 123, Created: "fixture-process", Executable: "fixture-process"}
			serverDir := filepath.Join(runDir, "runs", runID, "app-server")
			require(t, os.MkdirAll(serverDir, 0700))
			require(t, AtomicWrite(filepath.Join(serverDir, "process.json"), mustJSON(codexworker.ProcessReceipt{Endpoint: endpoint, Generation: "generation", Identity: identity})))
			command, previous := newID(), newID()
			turnDir := filepath.Join(runDir, "runs", runID, "turns", command)
			require(t, os.MkdirAll(turnDir, 0700))
			if exists {
				require(t, AtomicWrite(filepath.Join(turnDir, "turn-start.json"), mustJSON(map[string]any{"turn": map[string]string{"id": second}})))
			}
			backend := codexworker.Backend{Config: codexworker.Config{Workspace: workspace, RunDir: runDir, RunID: runID, ServerDir: serverDir}, Process: codexworker.ProcessPort{Inspect: func(*executionapi.ProcessIdentity) string { return "matching process still exists" }}}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			recovered, err := backend.Reattach(ctx, codexworker.Binding{Endpoint: endpoint, Generation: "generation", SessionID: testprotocol.AppServerSession, Process: &identity}, command, "", nil)
			if !exists {
				if err == nil || recovered.Completed || recovered.Observer != nil || recovered.TurnID == first {
					t.Fatalf("lost receipt reused preceding turn: %+v / %v", recovered, err)
				}
				return
			}
			if err != nil || !recovered.Completed || recovered.Observer != nil || recovered.TurnID != second {
				t.Fatalf("pending feedback did not recover new original receipt: %+v / %v", recovered, err)
			}
			outcome, err := codexworker.ReadAppServerOutcome(filepath.Join(turnDir, "appserver.jsonl"), testprotocol.AppServerSession, second)
			if err != nil || !outcome.Completed {
				t.Fatalf("wrong recovered journal: %+v / %v", outcome, err)
			}
			if _, err := os.Stat(filepath.Join(runDir, "runs", runID, "turns", previous, "appserver.jsonl")); !os.IsNotExist(err) {
				t.Fatalf("recovery rewrote preceding turn: %v", err)
			}
		})
	}
}
