package orca

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/nacl/box"
)

func TestOrcaNativeStateStream(t *testing.T) {
	for _, scenario := range []string{"subscription-query", "wrong-runtime", "wrong-request", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			pub, private, err := box.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				ctx := r.Context()
				_, data, err := conn.Read(ctx)
				var hello struct{ Type, PublicKeyB64 string }
				if err != nil || json.Unmarshal(data, &hello) != nil || hello.Type != "e2ee_hello" {
					return
				}
				bytes, err := base64.StdEncoding.DecodeString(hello.PublicKeyB64)
				if err != nil || len(bytes) != 32 {
					return
				}
				var peer [32]byte
				copy(peer[:], bytes)
				socket := &stateSocket{conn: conn}
				box.Precompute(&socket.key, &peer, private)
				if conn.Write(ctx, websocket.MessageText, []byte(`{"type":"e2ee_ready"}`)) != nil {
					return
				}
				data, err = socket.read(ctx)
				var auth struct {
					Type, DeviceToken  string
					ClientCapabilities []string
				}
				if err != nil || json.Unmarshal(data, &auth) != nil || auth.Type != "e2ee_auth" || auth.DeviceToken != "test-only-token" || len(auth.ClientCapabilities) != 1 {
					return
				}
				if socket.write(ctx, map[string]string{"type": "e2ee_authenticated"}) != nil {
					return
				}
				for {
					data, err := socket.read(ctx)
					var request struct {
						ID, DeviceToken, Method string
						Params                  map[string]json.RawMessage
					}
					if err != nil || json.Unmarshal(data, &request) != nil || request.DeviceToken != "test-only-token" {
						return
					}
					id, runtime := request.ID, "runtime"
					if scenario == "wrong-runtime" {
						runtime = "replacement"
					}
					if scenario == "wrong-request" {
						id = "foreign"
					}
					result := any(map[string]string{"runtimeId": runtime})
					if request.Method == "worktree.ps" {
						if string(request.Params["limit"]) != "100" {
							return
						}
						result = map[string]any{"worktrees": []any{}}
					} else if request.Method != "status.get" {
						if string(request.Params["worktree"]) != `"id:worktree"` {
							return
						}
						if scenario == "cancelled" {
							<-ctx.Done()
							return
						}
						result = map[string]any{"type": "snapshot", "worktree": "worktree", "tabs": []any{}}
					}
					if socket.write(ctx, map[string]any{"id": id, "ok": true, "result": result, "_meta": map[string]string{"runtimeId": runtime}}) != nil {
						return
					}
				}
			}))
			defer server.Close()
			encoded, _ := json.Marshal(statePairing{Version: 2, Endpoint: "ws" + strings.TrimPrefix(server.URL, "http"), Token: "test-only-token", PublicKey: base64.StdEncoding.EncodeToString(pub[:]), Scope: "runtime"})
			t.Setenv("ORCA_PAIRING_CODE", "orca://pair?code="+base64.RawURLEncoding.EncodeToString(encoded))
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			sub, _, err := SubscribeState(ctx, "", "runtime", "id:worktree")
			if sub != nil {
				defer sub.Close()
			}
			if scenario != "subscription-query" {
				if err == nil {
					t.Fatal("unverified subscription was accepted")
				}
				if (scenario == "wrong-runtime" || scenario == "wrong-request") && !errors.Is(err, ErrStateIdentityChanged) {
					t.Fatal("native identity conflict was classified as retryable", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = QueryState(ctx, "", "runtime", "id:worktree"); err != nil {
				t.Fatal(err)
			}
			if _, err = QueryWorktreeAgents(ctx, "", "runtime"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
