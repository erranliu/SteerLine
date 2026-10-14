package orcatransport

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

// Wrap the public CLI wire, retaining the same fixture host and command history.
func workspaceCalls(l *Lifecycle, intercept func([]string) ([]byte, bool)) *[][]string {
	original := l.client
	calls := new([][]string)
	l.client = native.New(native.Config{Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
		*calls = append(*calls, append([]string(nil), args...))
		if intercept != nil {
			if raw, override := intercept(args); override {
				return raw, nil
			}
		}
		return original.Raw(ctx, timeout, args...)
	}})
	return calls
}

func workspaceQueryError(message string) []byte {
	raw, _ := json.Marshal(map[string]any{"ok": false, "error": map[string]any{"code": "runtime_error", "message": message}, "_meta": map[string]any{"runtimeId": "runtime"}})
	return raw
}

func assertWorkspaceReadonly(t *testing.T, calls [][]string) {
	t.Helper()
	for _, call := range calls {
		if call[1] != "list" && call[1] != "show" {
			t.Fatal("workspace reconciliation mutated resources", call)
		}
	}
}

func TestWorkspaceCreationUsesConfiguredTimeout(t *testing.T) {
	for _, environment := range []string{"", "paired-worker"} {
		t.Run(environment, func(t *testing.T) {
			l, f, s := fixtureOn(t, environment)
			l.setup.AgentLaunchCommand, f.commandMode = "opaque-start", true
			l.setup.CommandTimeout = 2 * time.Minute
			original := l.client
			creations := 0
			l.client = native.New(native.Config{Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
				want := 40 * time.Second
				if args[0] == "worktree" && args[1] == "create" {
					want = 2 * time.Minute
					creations++
				}
				if timeout != want {
					t.Fatalf("%v timeout = %s, want %s", args[:2], timeout, want)
				}
				return original.Raw(ctx, timeout, args...)
			}})
			receipt, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if err != nil || receipt.Disposition != b.OperationConfirmed || creations != 1 {
				t.Fatal(receipt, err, creations)
			}
		})
	}
}

func TestWorkspaceTerminalSurfaceReadiness(t *testing.T) {
	for _, scenario := range []string{"list", "show", "empty-list", "shared-deadline", "other-error", "create-error"} {
		t.Run(scenario, func(t *testing.T) {
			l, f, s := fixture(t)
			l.setup.AgentLaunchCommand, f.commandMode = "opaque-start", true
			attempts := map[string]int{}
			calls := workspaceCalls(l, func(args []string) ([]byte, bool) {
				command := args[0] + " " + args[1]
				attempts[command]++
				if scenario == "create-error" && command == "worktree create" ||
					(scenario == "list" && command == "terminal list" || scenario == "show" && command == "terminal show" || scenario == "shared-deadline" && args[0] == "terminal") && attempts[command] == 1 {
					return workspaceQueryError("terminal_surface_ownership_unavailable"), true
				}
				if scenario == "other-error" && command == "terminal list" {
					return workspaceQueryError("permission_denied"), true
				}
				if scenario == "empty-list" && command == "terminal list" && attempts[command] == 1 {
					return []byte(`{"ok":true,"result":{"terminals":[]},"_meta":{"runtimeId":"runtime"}}`), true
				}
				return nil, false
			})
			if scenario == "shared-deadline" {
				l.profile.WaitTimeout = "300ms" // list consumes 200ms; show cannot receive a fresh budget.
			}
			started := time.Now()
			receipt, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if attempts["worktree create"] != 1 {
				t.Fatal("creation was repeated", attempts)
			}
			switch scenario {
			case "shared-deadline":
				if !errors.Is(err, context.DeadlineExceeded) || attempts["terminal show"] != 1 || time.Since(started) > time.Second || len(receipt.Operation.Receipt) == 0 {
					t.Fatal("query deadlines or creation checkpoint lost", receipt, err, attempts)
				}
			case "other-error", "create-error":
				wantCalls := 2
				if scenario == "create-error" {
					wantCalls = 1
				}
				if err == nil || len(*calls) != wantCalls {
					t.Fatal("unexpected retry", err, *calls)
				}
				if scenario == "create-error" && len(receipt.Operation.Receipt) != 0 {
					t.Fatal("unknown creation acquired a checkpoint")
				}
			default:
				if err != nil || receipt.Disposition != b.OperationConfirmed {
					t.Fatal(receipt, err)
				}
			}
			if f.launched || f.nativeSends != 0 {
				t.Fatal("workspace query sent input")
			}
		})
	}
}

func TestWorkspaceCreationReceiptRecovery(t *testing.T) {
	for _, environment := range []string{"", "paired-worker"} {
		t.Run(environment, func(t *testing.T) {
			l, f, s := fixtureOn(t, environment)
			l.setup.AgentLaunchCommand, f.commandMode = "opaque-start", true
			l.profile.WaitTimeout = "20ms"
			workspaceCalls(l, func(args []string) ([]byte, bool) {
				if args[0] == "terminal" && args[1] == "show" {
					return workspaceQueryError("terminal_surface_ownership_unavailable"), true
				}
				return nil, false
			})
			req := request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan})
			receipt, _, err := l.Execute(context.Background(), req)
			if !errors.Is(err, context.DeadlineExceeded) || receipt.Process != nil {
				t.Fatal(receipt, err)
			}
			var creation workspaceCreation
			if json.Unmarshal(receipt.Operation.Receipt, &creation) != nil || creation.Terminal.IncarnationID != f.incarnation {
				t.Fatal("original inventory identity not retained")
			}
			// Serialize the exact outer operation receipt, as the service does, and rebuild the adapter.
			req.Operation.Receipt, _ = json.Marshal(receipt)
			port, err := NewLifecycleWithStateFactory(l.setup, f.stateFactory)
			if err != nil {
				t.Fatal(err)
			}
			restarted := port.(*Lifecycle)
			unavailable := true
			calls := workspaceCalls(restarted, func(args []string) ([]byte, bool) {
				if unavailable && args[0] == "worktree" {
					return workspaceQueryError("runtime_unavailable"), true
				}
				return nil, false
			})
			retried, _, err := restarted.Reconcile(context.Background(), req)
			if err == nil || string(retried.Operation.Receipt) != string(receipt.Operation.Receipt) {
				t.Fatal("failed recovery lost or nested the checkpoint", err)
			}
			req.Operation.Receipt, _ = json.Marshal(retried)
			unavailable = false
			recovered, _, err := restarted.Reconcile(context.Background(), req)
			if err != nil || recovered.Disposition != b.OperationConfirmed || recovered.Process == nil || recovered.Process.Incarnation != f.incarnation {
				t.Fatal(recovered, err)
			}
			assertWorkspaceReadonly(t, *calls)
			bind(&s, recovered)
			candidate, err := restarted.Next(context.Background(), s)
			if err != nil || candidate.Action != b.LifecycleCreateResource {
				t.Fatal("recovery skipped the existing agent-launch step", candidate, err)
			}
		})
	}
}

func TestWorkspaceCreationRecoveryRejectsChangedIdentity(t *testing.T) {
	for _, fault := range []string{"runtime", "instanceId", "path", "branch", "head", "baseRef", "handle", "incarnation", "orphan", "multiple", "missing-receipt", "foreign-operation"} {
		t.Run(fault, func(t *testing.T) {
			l, f, s := fixture(t)
			l.setup.AgentLaunchCommand, f.commandMode = "opaque-start", true
			l.profile.WaitTimeout = "20ms"
			workspaceCalls(l, func(args []string) ([]byte, bool) {
				if args[0] == "terminal" && args[1] == "show" {
					return workspaceQueryError("terminal_surface_ownership_unavailable"), true
				}
				return nil, false
			})
			req := request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan})
			receipt, _, err := l.Execute(context.Background(), req)
			if err == nil || len(receipt.Operation.Receipt) == 0 {
				t.Fatal(receipt, err)
			}
			if fault == "foreign-operation" {
				receipt.Operation.ID = "another-operation"
			}
			req.Operation.Receipt, _ = json.Marshal(receipt)
			if fault == "missing-receipt" {
				req.Operation.Receipt = nil
			}
			port, err := NewLifecycleWithStateFactory(l.setup, f.stateFactory)
			if err != nil {
				t.Fatal(err)
			}
			original := port.(*Lifecycle).client
			calls := workspaceCalls(port.(*Lifecycle), func(args []string) ([]byte, bool) {
				raw, callErr := original.Raw(context.Background(), time.Second, args...)
				if callErr != nil {
					t.Fatal(callErr)
				}
				var wire map[string]any
				if json.Unmarshal(raw, &wire) != nil {
					t.Fatal("invalid fixture wire")
				}
				result := wire["result"].(map[string]any)
				if fault == "runtime" {
					wire["_meta"].(map[string]any)["runtimeId"] = "replacement"
				}
				if args[0] == "worktree" && strings.Contains(" instanceId path branch head baseRef ", " "+fault+" ") {
					result["worktree"].(map[string]any)[fault] = "replacement"
				}
				if args[0] == "terminal" && args[1] == "list" {
					terminals := result["terminals"].([]any)
					terminal := terminals[0].(map[string]any)
					switch fault {
					case "handle":
						terminal["handle"] = "replacement"
					case "incarnation":
						terminal["incarnationId"] = "replacement"
					case "orphan":
						terminal["orphaned"] = true
					case "multiple":
						result["terminals"] = append(terminals, terminal)
					}
				}
				modified, _ := json.Marshal(wire)
				return modified, true
			})
			recovered, _, err := port.Reconcile(context.Background(), req)
			if err == nil || recovered.Process != nil || recovered.Disposition != b.OperationUnknown {
				t.Fatal("changed resource accepted", recovered, err)
			}
			assertWorkspaceReadonly(t, *calls)
			if (fault == "missing-receipt" || fault == "foreign-operation") && len(*calls) != 0 {
				t.Fatal("unbound recovery queried resources")
			}
		})
	}
}
