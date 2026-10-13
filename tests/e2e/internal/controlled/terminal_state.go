package controlled

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
	"unicode/utf16"

	ws "github.com/coder/websocket"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

// OpenTerminalState serves the public encrypted Orca state protocol against the
// isolated native fixture. It cannot read or mutate SteerLine business state.
func OpenTerminalState(path string) (pairing map[string]any, close func(), err error) {
	pub, private, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_, raw, err := conn.Read(ctx)
		var hello struct{ Type, PublicKeyB64 string }
		if err != nil || json.Unmarshal(raw, &hello) != nil || hello.Type != "e2ee_hello" {
			return
		}
		peerBytes, err := base64.StdEncoding.DecodeString(hello.PublicKeyB64)
		if err != nil || len(peerBytes) != 32 {
			return
		}
		var peer, key [32]byte
		copy(peer[:], peerBytes)
		box.Precompute(&key, &peer, private)
		if conn.Write(ctx, ws.MessageText, []byte(`{"type":"e2ee_ready"}`)) != nil {
			return
		}
		read := func() ([]byte, error) {
			_, wire, err := conn.Read(ctx)
			if err != nil {
				return nil, err
			}
			wire, err = base64.StdEncoding.DecodeString(string(wire))
			if err != nil || len(wire) < 40 {
				return nil, errors.New("invalid fixture state frame")
			}
			var nonce [24]byte
			copy(nonce[:], wire[:24])
			plain, ok := secretbox.Open(nil, wire[24:], &nonce, &key)
			if !ok {
				return nil, errors.New("fixture state authentication failed")
			}
			return plain, nil
		}
		write := func(value any) error {
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			var nonce [24]byte
			if _, err := rand.Read(nonce[:]); err != nil {
				return err
			}
			wire := secretbox.Seal(nonce[:], raw, &nonce, &key)
			return conn.Write(ctx, ws.MessageText, []byte(base64.StdEncoding.EncodeToString(wire)))
		}
		raw, err = read()
		var auth struct{ Type, DeviceToken string }
		if err != nil || json.Unmarshal(raw, &auth) != nil || auth.Type != "e2ee_auth" || auth.DeviceToken != "isolated-fixture-token" {
			return
		}
		if write(map[string]string{"type": "e2ee_authenticated"}) != nil {
			return
		}
		for {
			raw, err := read()
			var req struct {
				ID, Method, DeviceToken string
				Params                  struct{ Worktree string }
			}
			if err != nil || json.Unmarshal(raw, &req) != nil || req.DeviceToken != auth.DeviceToken {
				return
			}
			s, err := Read(path)
			if err != nil || s.Recovery == nil || s.Recovery.Orca == nil {
				return
			}
			o := s.Recovery.Orca
			reply := func(result any) error {
				return write(map[string]any{"id": req.ID, "ok": true, "result": result, "_meta": map[string]string{"runtimeId": o.Runtime}})
			}
			if req.Method == "status.get" {
				if reply(map[string]string{"runtimeId": o.Runtime}) != nil {
					return
				}
				continue
			}
			if req.Method == "worktree.ps" {
				if reply(map[string]any{"worktrees": []any{map[string]any{"worktreeId": "fixture::" + o.Worktree, "agents": []any{terminalAgentState(o, o.Busy)}}}}) != nil {
					return
				}
				continue
			}
			if (req.Method != "session.tabs.subscribe" && req.Method != "session.tabs.list") || req.Params.Worktree != "id:fixture::"+o.Worktree {
				return
			}
			if reply(terminalStateSnapshot(o, o.Busy)) != nil {
				return
			}
			if req.Method == "session.tabs.list" {
				continue
			}
			seen, busy := o.Sends, o.Busy
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				s, err := Read(path)
				if err != nil || s.Recovery == nil || s.Recovery.Orca == nil {
					continue
				}
				o = s.Recovery.Orca
				if o.Gap {
					_ = write(map[string]any{"id": req.ID, "ok": false, "error": map[string]string{"code": "state_gap", "message": "original native state gap"}, "_meta": map[string]string{"runtimeId": o.Runtime}})
					return
				}
				if o.Sends > seen {
					if reply(terminalStateSnapshot(o, true)) != nil {
						return
					}
					seen, busy = o.Sends, true
				}
				if busy && !o.Busy {
					if reply(terminalStateSnapshot(o, false)) != nil {
						return
					}
					busy = false
				}
			}
		}
	}))
	return map[string]any{"v": 2, "endpoint": "ws" + strings.TrimPrefix(server.URL, "http"), "deviceToken": "isolated-fixture-token", "publicKeyB64": base64.StdEncoding.EncodeToString(pub[:]), "scope": "runtime"}, func() { cancel(); server.Close() }, nil
}

func terminalAgentState(o *TerminalFixture, busy bool) map[string]any {
	stamp, revision, state := int64(o.Sends*2000+1000), o.Sends*10+2, "done"
	if busy {
		stamp, revision, state = stamp-900, revision-1, "working"
	}
	units := utf16.Encode([]rune(strings.Join(strings.Fields(o.Prompt), " ")))
	if len(units) > 200 {
		units = units[:200]
		if units[199] >= 0xd800 && units[199] <= 0xdbff {
			units = units[:199]
		}
	}
	return map[string]any{"paneKey": "tab:pane", "agentType": "codex", "state": state, "prompt": string(utf16.Decode(units)), "updatedAt": stamp, "stateStartedAt": stamp,
		"mainAgent": map[string]any{"state": state, "stateStartedAt": stamp, "outcome": "success"}, "providerSession": map[string]string{"id": "fixture-session"},
		"observation": map[string]any{"origin": "hook", "kind": "transition", "authorityId": "fixture-hooks", "revision": revision, "observedAt": stamp}, "lastAssistantMessage": o.NativeReply}
}

func terminalStateSnapshot(o *TerminalFixture, busy bool) map[string]any {
	return map[string]any{"type": "snapshot", "worktree": "fixture::" + o.Worktree, "tabs": []any{map[string]any{"terminal": o.Handle, "incarnationId": o.Incarnation, "parentTabId": "tab", "leafId": "pane", "agentStatus": terminalAgentState(o, busy)}}}
}
