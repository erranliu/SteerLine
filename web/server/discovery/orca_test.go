package discovery

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"github.com/coder/websocket"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	repository "github.com/erranliu/SteerLine/web/server/executionconfig"
	"golang.org/x/crypto/nacl/box"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOrcaCapabilityWire(t *testing.T) {
	for _, mode := range []string{"probe", "seed", "unsupported", "changed-runtime", "remote-pi"} {
		t.Run(mode, func(t *testing.T) {
			pub, secret, _ := box.GenerateKey(rand.Reader)
			var mu sync.Mutex
			methods := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				socket, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer socket.CloseNow()
				ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
				defer cancel()
				_, hello, _ := socket.Read(ctx)
				var h struct {
					PublicKey string `json:"publicKeyB64"`
				}
				json.Unmarshal(hello, &h)
				key, _ := base64.StdEncoding.DecodeString(h.PublicKey)
				if len(key) != 32 {
					return
				}
				var peer [32]byte
				copy(peer[:], key)
				c := &orcaConnection{socket: socket}
				box.Precompute(&c.shared, &peer, secret)
				socket.Write(ctx, websocket.MessageText, []byte(`{"type":"e2ee_ready"}`))
				auth, err := c.read(ctx)
				if err != nil {
					return
				}
				if !strings.Contains(string(auth), "agent-session.structured.registered-agents.v1") {
					return
				}
				c.write(ctx, map[string]string{"type": "e2ee_authenticated"})
				for {
					raw, err := c.read(ctx)
					if err != nil {
						return
					}
					var q struct {
						ID     string         `json:"id"`
						Method string         `json:"method"`
						Params map[string]any `json:"params"`
					}
					json.Unmarshal(raw, &q)
					mu.Lock()
					methods = append(methods, q.Method)
					mu.Unlock()
					var result any
					switch q.Method {
					case "status.get":
						caps := []string{"agent-session.structured.v1", "agent-session.structured.registered-agents.v1"}
						if mode == "unsupported" {
							caps = nil
						}
						result = map[string]any{"runtimeProtocolVersion": 3, "minCompatibleRuntimeClientVersion": 2, "pairedDeviceId": "device", "appVersion": "1.4.fixture", "capabilities": caps}
					case "agentSession.agents":
						if mode == "remote-pi" {
							result = map[string]any{"agents": []any{map[string]string{"agent": "pi"}, map[string]string{"agent": "custom-agent"}}}
							break
						}
						result = map[string]any{"agents": []any{map[string]string{"agent": "codex"}, map[string]string{"agent": "claude"}}}
					case "agentSession.modelCatalog":
						if mode == "remote-pi" {
							if q.Params["agent"] != "pi" {
								return
							}
							result = map[string]any{"origin": "probe", "models": []any{map[string]any{"id": "openrouter/vendor/family/model", "efforts": []Effort{{"high", "High"}}, "apiKey": "fixture-secret-must-not-cross"}}}
							break
						}
						if q.Params["waitForListing"] == true {
							result = map[string]any{"origin": mode, "fetchedAt": time.Now().UnixMilli(), "models": []any{map[string]any{"id": "model", "label": "Model", "defaultEffort": "high", "efforts": []Effort{{"high", "High"}}}}}
						} else {
							result = map[string]any{"origin": "unknown", "listingInProgress": true}
						}
					default:
						return
					}
					runtimeID := "runtime"
					if mode == "changed-runtime" && q.Method != "status.get" {
						runtimeID = "other"
					}
					if c.write(ctx, map[string]any{"id": q.ID, "ok": true, "result": result, "_meta": map[string]string{"runtimeId": runtimeID}}) != nil {
						return
					}
				}
			}))
			defer server.Close()
			e := nativeEnvironment{ID: "id", PairedDevice: "device", Preferred: "ep", Endpoints: []accessEndpoint{{ID: "ep", Endpoint: "ws" + strings.TrimPrefix(server.URL, "http"), Token: "test-token", Key: base64.StdEncoding.EncodeToString(pub[:])}}}
			var out Result
			var err error
			if mode == "remote-pi" {
				home, data := t.TempDir(), t.TempDir()
				config := cfg.File[cfg.Pin]{Version: 3, DefaultProfile: "local", Profiles: []cfg.Profile{{ID: "local", Name: "Local", Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{}`)}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}}}
				if err := localservice.WritePrivateFile(home, "execution.json", config); err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(map[string]any{"version": 1, "environments": []nativeEnvironment{e}})
				if err := os.WriteFile(filepath.Join(data, "orca-environments.json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
				m := New(repository.New(home))
				m.OrcaData = data
				out, err = m.Query(context.Background(), Request{Kind: "models", Adapter: "orca-terminal", Host: "orca_environment", Environment: e.ID, Agent: "pi"})
				m.Close()
				encoded, _ := json.Marshal(out)
				if err != nil || out.State != "ready" || len(out.Models) != 1 || out.Models[0].Provider != "openrouter" || out.Models[0].ID != "openrouter/vendor/family/model" || strings.Contains(string(encoded), "fixture-secret-must-not-cross") {
					t.Fatal("remote native directory lost identity or exposed credentials", out.State, err)
				}
				if len(out.Agents) != 2 || out.Channel != "paired-websocket" || out.RuntimeDigest == "" {
					t.Fatal("remote native facts incomplete")
				}
			} else {
				out, err = orcaCatalog(context.Background(), e, "models", "codex", Result{ContextUnverified: true})
			}
			switch mode {
			case "unsupported":
				if err != nil || out.State != "unsupported" {
					t.Fatal(out, err)
				}
			case "remote-pi":
			case "changed-runtime":
				if err == nil {
					t.Fatal("runtime switch accepted")
				}
			default:
				if err != nil || out.State != "ready" || out.Origin != mode || len(out.Models) != 1 || out.Models[0].Efforts[0].Value != "high" {
					t.Fatal(out, err)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			for _, method := range methods {
				if method != "status.get" && method != "agentSession.agents" && method != "agentSession.modelCatalog" {
					t.Fatal("business method invoked", methods)
				}
			}
		})
	}
}
