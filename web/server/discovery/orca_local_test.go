//go:build windows

package discovery

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	repository "github.com/erranliu/SteerLine/web/server/executionconfig"
	"github.com/google/uuid"
)

func TestLocalOrcaCatalog(t *testing.T) {
	for _, mode := range []string{"probe", "seed", "missing", "empty", "in-progress", "unsupported", "changed-runtime", "wrong-id", "wrong-protocol", "metadata-runtime", "invalid", "oversize", "disconnect", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			data := t.TempDir()
			endpoint := `\\.\pipe\sl-web-catalog-` + uuid.NewString()
			listener, err := winio.ListenPipe(endpoint, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			var mu sync.Mutex
			methods := []string{}
			var connections sync.WaitGroup
			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					connections.Add(1)
					go func(conn net.Conn) {
						defer connections.Done()
						defer conn.Close()
						conn.SetDeadline(time.Now().Add(5 * time.Second))
						sc := bufio.NewScanner(conn)
						enc := json.NewEncoder(conn)
						for sc.Scan() {
							var q struct {
								ID, Method, AuthToken string
								Params                map[string]any
								ClientCapabilities    []string
							}
							if json.Unmarshal(sc.Bytes(), &q) != nil {
								return
							}
							if q.AuthToken != "fixture-token" || len(q.ClientCapabilities) != 2 {
								return
							}
							mu.Lock()
							methods = append(methods, q.Method)
							mu.Unlock()
							if mode == "disconnect" {
								return
							}
							if mode == "cancel" {
								for sc.Scan() {
								}
								return
							}
							if mode == "invalid" {
								conn.Write([]byte("invalid\n"))
								return
							}
							if mode == "oversize" {
								conn.Write([]byte(strings.Repeat("x", (1<<20)+2) + "\n"))
								return
							}
							var result any
							switch q.Method {
							case "status.get":
								caps := []string{"agent-session.structured.v1", "agent-session.structured.registered-agents.v1"}
								if mode == "unsupported" {
									caps = nil
								}
								protocol := 3
								if mode == "wrong-protocol" {
									protocol = 2
								}
								result = map[string]any{"runtimeProtocolVersion": protocol, "minCompatibleRuntimeClientVersion": 2, "appVersion": "fixture", "capabilities": caps}
							case "agentSession.agents":
								agents := []any{map[string]string{"agent": "codex"}, map[string]string{"agent": "pi"}, map[string]string{"agent": "dsh"}, map[string]string{"agent": "claude"}}
								if mode == "missing" {
									agents = []any{}
								}
								result = map[string]any{"agents": agents}
							case "agentSession.modelCatalog":
								if q.Params["waitForListing"] != true || mode == "in-progress" {
									result = map[string]any{"listingInProgress": true}
								} else {
									modelID := q.Params["agent"]
									if modelID == "pi" {
										modelID = "openrouter/vendor/family/model"
									}
									models := []any{map[string]any{"id": modelID, "efforts": []any{map[string]string{"value": "high", "label": "High"}}}}
									if mode == "empty" {
										models = []any{}
									}
									result = map[string]any{"origin": mode, "models": models}
								}
							default:
								t.Error("business method requested", q.Method)
								return
							}
							runtimeID := "fixture-runtime"
							if mode == "metadata-runtime" || mode == "changed-runtime" && q.Method != "status.get" {
								runtimeID = "other"
							}
							id := q.ID
							if mode == "wrong-id" {
								id = "other"
							}
							if enc.Encode(map[string]any{"id": id, "ok": true, "result": result, "_meta": map[string]string{"runtimeId": runtimeID}}) != nil {
								return
							}
						}
					}(conn)
				}
			}()
			writeMeta := func(token string) {
				t.Helper()
				raw, _ := json.Marshal(map[string]any{"runtimeId": "fixture-runtime", "authToken": token, "transports": []any{map[string]string{"kind": "named-pipe", "endpoint": endpoint}}})
				if err := os.WriteFile(filepath.Join(data, "orca-runtime.json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			writeMeta("fixture-token")
			c := cfg.File[cfg.Pin]{Version: 3, DefaultProfile: "local", Profiles: []cfg.Profile{{ID: "local", Name: "Local", Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{}`)}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}}}
			if err := localservice.WritePrivateFile(home, "execution.json", c); err != nil {
				t.Fatal(err)
			}
			m := New(repository.New(home))
			m.OrcaData = data
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if mode == "cancel" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			}
			defer cancel()
			q := Request{Kind: "models", Adapter: "orca-terminal", Host: "local", Agent: "pi"}
			out, err := m.Query(ctx, q)
			switch mode {
			case "cancel":
				if err == nil && out.State != "unavailable" {
					t.Fatal("cancellation ignored")
				}
			case "wrong-id", "changed-runtime", "wrong-protocol", "metadata-runtime", "invalid", "oversize", "disconnect":
				if err != nil || out.State != "unavailable" {
					t.Fatal(out.State, err)
				}
			case "missing", "unsupported":
				if err != nil || out.State != "unsupported" {
					t.Fatal(out.State, err)
				}
			case "in-progress":
				if err != nil || out.State != "in_progress" {
					t.Fatal(out.State, err)
				}
			default:
				if err != nil || out.State != "ready" {
					t.Fatal(out.State, err)
				}
				if mode != "empty" && (len(out.Models) != 1 || out.Models[0].ID != "openrouter/vendor/family/model" || out.Models[0].Provider != "openrouter") {
					t.Fatal("wrong Agent catalog")
				}
				cached, e := m.Query(ctx, q)
				if e != nil || cached.Freshness != "cached" {
					t.Fatal("cache missing")
				}
				q.Agent = "dsh"
				other, e := m.Query(ctx, q)
				if e != nil || other.ContextKey == out.ContextKey {
					t.Fatal("cross-Agent cache")
				}
				writeMeta("rotated-token")
				rotated, e := m.Query(ctx, q)
				if e != nil || rotated.Freshness == "cached" {
					t.Fatal("old native identity cache reused")
				}
			}
			if _, e := m.Query(context.Background(), Request{Kind: "models", Host: "local"}); e == nil {
				t.Fatal("implicit adapter accepted")
			}
			if _, e := m.Query(context.Background(), Request{Kind: "models", Host: "local", Adapter: "orca-terminal", Environment: "remote"}); e == nil {
				t.Fatal("local environment fallback accepted")
			}
			m.Close()
			listener.Close()
			<-done
			connections.Wait()
			mu.Lock()
			defer mu.Unlock()
			for _, method := range methods {
				if method != "status.get" && method != "agentSession.agents" && method != "agentSession.modelCatalog" {
					t.Fatal(method)
				}
			}
			if mode == "missing" {
				for _, a := range out.Agents {
					if a.ID == "pi" {
						t.Fatal("missing directory marked registered")
					}
				}
			}
		})
	}
	t.Run("metadata-boundaries", func(t *testing.T) {
		m := New(nil)
		defer m.Close()
		m.OrcaData = t.TempDir()
		for _, raw := range []string{`{}`, `{"runtimeId":"r","authToken":"t","transports":[{"kind":"named-pipe","endpoint":"\\\\remote\\pipe\\x"}]}`, `{"runtimeId":"r","authToken":"t","transports":[{"kind":"websocket","endpoint":"ws://localhost"}]}`, strings.Repeat("x", (1<<20)+1)} {
			os.WriteFile(filepath.Join(m.OrcaData, "orca-runtime.json"), []byte(raw), 0600)
			if _, err := m.localRuntime(); err == nil {
				t.Fatal("unsafe metadata accepted")
			}
		}
	})
}
